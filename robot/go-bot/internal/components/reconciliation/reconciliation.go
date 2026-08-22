package reconcil

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"trading/robot/go-bot/internal/components/execution"
	"trading/robot/go-bot/internal/components/portfolio"
	"trading/robot/go-bot/internal/database"
	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/utils"
)

const limitOpenOrders = 1000 // Set a high limit to ensure we fetch all open orders for reconciliation.

type Reconciler interface {
	SyncOrders(ctx context.Context, exchange, instrumentSymbol string) error
	SyncStopOrders(ctx context.Context, exchange, instrumentSymbol string) error
	SyncPositions(ctx context.Context, exchange, instrumentSymbol string) error
}

// Reconciler checks the alignment between the Exchange truth and the Database state.
type reconciler struct {
	logger *slog.Logger
	db     *database.DB
	repo   *repository.Container
	exec   execution.Service
	pf     portfolio.Portfolio
}

// NewReconciler creates a new instance of the Reconciler.
func NewReconciler(
	logger *slog.Logger,
	db *database.DB,
	repo *repository.Container,
	exec execution.Service,
	pf portfolio.Portfolio,
) Reconciler {
	return &reconciler{
		logger: logger,
		db:     db,
		repo:   repo,
		exec:   exec,
		pf:     pf,
	}
}

// SyncOrders synchronizes open orders from the exchange with the local database.
// It handles non-persisted orders, status drift, partial fills, and external cancellations.
func (r *reconciler) SyncOrders(
	ctx context.Context, exchange, instrumentSymbol string,
) error {
	log := r.logger.With("exchange", exchange)
	if instrumentSymbol != "" {
		log = log.With("symbol", instrumentSymbol)
	}

	// --- Resolve Vanished Orders ---
	// Fetch orders our DB thinks are new or open.
	statuses := []string{repository.OrderStatusNew, repository.OrderStatusOpen}
	types := []string{repository.OrderTypeLimit, repository.OrderTypeMarket}
	dbOrders, err := r.repo.Orders.GetOrders(
		ctx, r.db, exchange, instrumentSymbol, statuses, types, []string{}, limitOpenOrders,
	)
	if err != nil {
		return fmt.Errorf("order sync: get buy open orders failed: %w", err)
	}

	for _, dbo := range dbOrders {
		if !dbo.ExchangeOrderID.Valid {
			continue // Skip orders without an exchange order ID.
		}

		// Fetch the individual status from the exchange to determine if it was already filled or canceled.
		// Execution service GetOrder handles DB synchronization of the order record.
		res, err := r.exec.GetOrder(ctx, exchange, dbo.InstrumentSymbol, dbo.ExchangeOrderID.String)
		if err != nil {
			log.Error(
				"Order sync: failed to fetch individual buy order status",
				"id", dbo.ExchangeOrderID, "error", err,
			)
			continue
		}

		// --- Handle filled buy orders ---
		// If the order was filled, we need to create a position in the portfolio for it.
		if res.Side == repository.OrderSideBuy && res.Status == repository.OrderStatusClosed {
			fillPrice := res.AveragePrice.Float64
			if fillPrice <= 0 {
				fillPrice = res.Price.Float64
			}

			// Create the known-origin position before the balance lookup, which is
			// an external request and can leave a filled order without a position.
			if err := r.pf.CreatePosition(
				ctx, exchange, res.InstrumentSymbol, res.Filled, fillPrice, dbo.ID,
			); err != nil {
				log.Error("Order sync: Failed to create position for filled order", "error", err)
				continue
			}
			log.Info(
				"Order sync: created position for filled order",
				"symbol", res.InstrumentSymbol, "qty", res.Filled, "price", fillPrice,
			)

			asset, _ := splitSymbol(res.InstrumentSymbol)
			balances, err := r.exec.GetBalance(ctx, exchange, asset)
			if err != nil || len(balances) == 0 {
				// The position already reflects the exchange-reported fill. A later
				// position sync can correct it when the balance is available.
				if err != nil {
					log.Error("Order sync: Failed to fetch balance", "asset", asset, "error", err)
				} else {
					log.Error("Order sync: Asset balance not found", "asset", asset)
				}
			} else if balances[0].Total > 0 {
				if err := r.pf.UpdatePosition(ctx, exchange, res.InstrumentSymbol, repository.PositionData{
					Quantity: balances[0].Total,
				}); err != nil {
					log.Error("Order sync: Failed to update position with balance", "asset", asset, "error", err)
				}
			}
		}
	}

	return nil
}

// SyncStopOrders synchronizes stop orders from the exchange. It handles non-persisted stop orders, status drift,
// cancels stop orders without active positions, and validates stop loss protection for active positions.
func (r *reconciler) SyncStopOrders(
	ctx context.Context, exchange, instrumentSymbol string,
) error {
	log := r.logger.With("exchange", exchange)
	if instrumentSymbol != "" {
		log = log.With("symbol", instrumentSymbol)
	}

	// Fetch active positions to verify stop loss protection.
	dbPositions, err := r.repo.Positions.GetPositions(ctx, r.db, exchange, instrumentSymbol)
	if err != nil {
		return fmt.Errorf("stop order sync: db active positions fetch failed: %w", err)
	}
	positionsBySymbol := make(map[string]repository.PositionData)
	for _, pos := range dbPositions {
		positionsBySymbol[pos.InstrumentSymbol] = pos
	}

	balances, err := r.repo.Balances.GetAllBalances(ctx, r.db, exchange)
	if err != nil {
		return fmt.Errorf("stop order sync: balance fetch failed: %w", err)
	}
	walletBalances := make(map[string]float64)
	for _, b := range balances {
		walletBalances[b.AssetSymbol] = b.Total
	}

	// --- Resolve Vanished Stop Orders ---
	// Fetch stop orders our DB thinks are new or open.
	statuses := []string{repository.OrderStatusNew, repository.OrderStatusOpen}
	types := []string{repository.OrderTypeStopLimit, repository.OrderTypeStopMarket}
	side := []string{repository.OrderSideSell}
	dbOrders, err := r.repo.Orders.GetOrders(
		ctx, r.db, exchange, instrumentSymbol, statuses, types, side, limitOpenOrders,
	)
	if err != nil {
		return fmt.Errorf("stop order sync: get stop open orders failed: %w", err)
	}

	// Map open stop orders by instrument symbol and track active exchange status.
	protectedPositions := make(map[string]bool)
	for _, dbo := range dbOrders {
		if !dbo.ExchangeOrderID.Valid {
			continue // Skip orders without an exchange order ID.
		}

		// Fetch the individual status from the exchange to determine if it was executed or canceled.
		// Execution service GetOrder handles DB synchronization of the order record.
		res, err := r.exec.GetOrder(ctx, exchange, dbo.InstrumentSymbol, dbo.ExchangeOrderID.String)
		if err != nil {
			log.Error(
				"Stop order sync: failed to fetch individual stop order status",
				"id", dbo.ExchangeOrderID, "error", err,
			)
			continue
		}

		if res.Status != repository.OrderStatusNew && res.Status != repository.OrderStatusOpen {
			continue
		}
		protectedPositions[dbo.InstrumentSymbol] = true

		// --- Handle stop orders without active positions ---
		// Check if the stop order is linked to an active position. If not, cancel it.
		latestCreatedOrder := dbo.ID
		_, exists := positionsBySymbol[dbo.InstrumentSymbol]
		if exists {
			latestOrders, err := r.repo.Orders.GetOrders(
				ctx, r.db, exchange, dbo.InstrumentSymbol, []string{}, []string{}, []string{}, 1,
			)
			if err != nil {
				log.Error(
					"Stop order sync: failed to fetch latest order for instrument",
					"symbol", dbo.InstrumentSymbol, "error", err,
				)
				continue
			}
			// A newer buy order and position was created after this stop order, which can happen after a partial
			// fill leaves a very low remaining quantity (dust) and causes its position to be deleted locally.
			if latestOrders[0].ID != latestCreatedOrder {
				latestCreatedOrder = latestOrders[0].ID
			}
		}

		if !exists || latestCreatedOrder != dbo.ID {
			log.Warn(
				"Stop order sync: stop order is not linked to a valid active position, canceling",
				"symbol", dbo.InstrumentSymbol, "order_id", dbo.ExchangeOrderID.String,
			)
			if err := r.exec.CancelOrder(ctx, exchange, dbo.InstrumentSymbol, dbo.ExchangeOrderID.String); err != nil {
				log.Error(
					"Stop order sync: failed to cancel stop order",
					"symbol", dbo.InstrumentSymbol, "order_id", dbo.ExchangeOrderID.String, "error", err,
				)
			}
			protectedPositions[dbo.InstrumentSymbol] = false
		}
	}

	// --- Handle missing stop orders for active positions ---
	// For all active and known-origin positions, ensure their stop loss status aligns with open exchange stop orders.
	for _, pos := range dbPositions {
		if pos.UnknownOrigin || !pos.StopLossActive {
			continue
		}

		if !protectedPositions[pos.InstrumentSymbol] {
			asset, _ := splitSymbol(pos.InstrumentSymbol)
			walletQty := walletBalances[asset]
			// If the wallet balance is zero, the position will be closed in the next position sync.
			if utils.IsZeroEps(walletQty) {
				continue
			}

			log.Warn(
				"Stop order sync: active position without open stop order, resetting stop loss status",
				"symbol", pos.InstrumentSymbol, "old_qty", pos.Quantity, "new_qty", walletQty,
			)
			pos.Quantity = walletQty
			// Reset the stop loss active flag to allow re-arming in the orchestrator.
			pos.StopLossActive = false
			if err := r.pf.UpdatePosition(ctx, exchange, pos.InstrumentSymbol, pos); err != nil {
				log.Error(
					"Stop order sync: failed to update position for stop loss re-arm",
					"symbol", pos.InstrumentSymbol, "error", err,
				)
			}
		}
	}

	return nil
}

// SyncPositions aligns database positions with exchange balances.
// It handles external liquidations, manual trades, and quantity drift due to fees or dust.
func (r *reconciler) SyncPositions(
	ctx context.Context, exchange, instrumentSymbol string,
) error {
	log := r.logger.With("exchange", exchange)
	if instrumentSymbol != "" {
		log = log.With("symbol", instrumentSymbol)
	}

	// Fetch all actives positions for the exchange to detect base asset collisions.
	dbPositions, err := r.repo.Positions.GetPositions(ctx, r.db, exchange, instrumentSymbol)
	if err != nil {
		return fmt.Errorf("position sync: db fetch failed: %w", err)
	}
	positionsByAsset := make(map[string][]repository.PositionData)
	for _, p := range dbPositions {
		asset, _ := splitSymbol(p.InstrumentSymbol)
		positionsByAsset[asset] = append(positionsByAsset[asset], p)
	}
	positionPrices := r.fetchTickers(ctx, exchange, positionSymbols(dbPositions))

	balances, err := r.repo.Balances.GetAllBalances(ctx, r.db, exchange)
	if err != nil {
		return fmt.Errorf("position sync: balance fetch failed: %w", err)
	}
	walletBalances := make(map[string]float64)
	for _, b := range balances {
		walletBalances[b.AssetSymbol] = b.Total
	}

	// --- Handle external events such as stop losses execution or quantity drift due to fees ---
	// Validate DB existing positions against exchange wallet balances.
	for _, posData := range dbPositions {
		asset, _ := splitSymbol(posData.InstrumentSymbol)
		walletQty := walletBalances[asset]

		tick, ok := positionPrices[posData.InstrumentSymbol]
		// If the wallet balance is zero or below the retention threshold, we close the position in the DB.
		if utils.IsZeroEps(walletQty) || (ok && portfolio.BelowPositionThreshold(walletQty, tick.Price)) {

			if !utils.IsZeroEps(walletQty) && tick.Price < posData.EntryPrice*0.50 {
				log.Warn(
					"Position sync: Ignoring price for position removal check due to untrusted ticker",
					"symbol", posData.InstrumentSymbol, "ticker_price", tick.Price,
				)
				continue
			}

			log.Warn(
				"Position sync: Deleting position due to zero or below-retention balance",
				"symbol", posData.InstrumentSymbol, "qty", walletQty,
			)
			if err := r.pf.DeletePosition(ctx, exchange, posData.InstrumentSymbol); err != nil {
				log.Error(
					"Position sync: Failed to delete position in DB",
					"symbol", posData.InstrumentSymbol, "error", err,
				)
			}
			continue
		}

		if !utils.IsEqualEps(posData.Quantity, walletQty) {
			// If the total quantity drifted, we can only auto-correct if there is exactly one position for this base asset.
			// We snap to the exchange truth. This naturally reconciles deductions for trading fees or minor 'dust' remains.
			if len(positionsByAsset[asset]) != 1 {
				log.Error(
					"Position sync: Ambiguous quantity drift detected for multi-pair asset",
					"asset", asset, "wallet_qty", walletQty, "position_qty", posData.Quantity,
				)
				continue
			}

			log.Warn(
				"Position sync: Adjusting position quantity",
				"symbol", posData.InstrumentSymbol, "old", posData.Quantity, "new", walletQty,
			)
			posData.Quantity = walletQty
			posData.StopLossActive = false // Reset stop loss active flag to allow re-arming if needed.
			err = r.pf.UpdatePosition(ctx, exchange, posData.InstrumentSymbol, posData)
			if err != nil {
				log.Error("Position sync: Failed to update position for adjusting quantity", "error", err)
			}
		}
	}

	// --- Handle adoptions of ghost balances or manual and untracked trades. ----
	// Select active trading instruments with a non-zero wallet balance and no existing position.
	var instruments []string
	pairs, err := r.repo.Strategies.GetStrategyPairs(ctx, r.db, []string{
		repository.StrategyEnabled,
		repository.StrategyPendingDisabled,
	})
	if err != nil {
		return fmt.Errorf("position sync: strategy pairs fetch failed: %w", err)
	}

	for _, p := range pairs {
		if p.ExchangeName != exchange {
			continue
		}
		asset, _ := splitSymbol(p.InstrumentSymbol)
		if p.InstrumentSymbol != instrumentSymbol && instrumentSymbol != "" {
			continue
		}

		walletQty := walletBalances[asset]
		_, existsPosition := positionsByAsset[asset]
		if !utils.IsZeroEps(walletQty) && !existsPosition {
			instruments = append(instruments, p.InstrumentSymbol)
		}
	}

	instrumentPrices := r.fetchTickers(ctx, exchange, instruments)
	for _, symbol := range instruments {
		asset, _ := splitSymbol(symbol)
		walletQty := walletBalances[asset]
		tick, ok := instrumentPrices[symbol]
		// If the wallet balance is not above the activation threshold, we don't try to adopt it as a ghost position.
		if !ok || !portfolio.AbovePositionThreshold(walletQty, tick.Price) {
			continue
		}

		statuses := []string{repository.OrderStatusNew, repository.OrderStatusOpen}
		sides := []string{repository.OrderSideBuy}
		openOrders, err := r.repo.Orders.GetOrders(
			ctx, r.db, exchange, symbol, statuses, []string{}, sides, 1,
		)
		if err != nil {
			log.Error("Position sync: Failed querying buy open orders", "symbol", symbol, "error", err)
			continue
		}
		// If there are buy open orders in DB, no need to adopt, the position will be created when the order is filled.
		if len(openOrders) > 0 {
			continue
		}

		log.Warn(
			"Position sync: Adopting ghost balance as unlinked position", "symbol", symbol, "qty", walletQty,
		)
		err = r.pf.CreatePosition(ctx, exchange, symbol, walletQty, tick.Price, 0)
		if err != nil {
			log.Error("Failed to create position for ghost balance", "error", err)
		}
	}

	return nil
}

// fetchTickers fetches in parallel the latest market data for a list of symbols from the exchange.
func (r *reconciler) fetchTickers(
	ctx context.Context, exchange string, symbols []string,
) map[string]repository.MarketDataTick {
	tickerCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	var mu sync.Mutex
	var wg sync.WaitGroup
	prices := make(map[string]repository.MarketDataTick, len(symbols))
	done := make(chan struct{})
	for _, symbol := range symbols {
		wg.Add(1)
		go func(symbol string) {
			defer wg.Done()
			tick, err := r.exec.GetTicker(tickerCtx, exchange, symbol)
			if err != nil {
				return
			}
			mu.Lock()
			prices[symbol] = tick
			mu.Unlock()
		}(symbol)
	}

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-tickerCtx.Done():
	}
	return prices
}

func positionSymbols(positions []repository.PositionData) []string {
	symbols := make([]string, 0, len(positions))
	for _, pos := range positions {
		symbols = append(symbols, pos.InstrumentSymbol)
	}
	return symbols
}

func splitSymbol(symbol string) (string, string) {
	parts := strings.Split(symbol, "/")
	if len(parts) != 2 {
		return symbol, ""
	}
	return parts[0], parts[1]
}
