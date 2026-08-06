package reconcil

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"trading/robot/go-bot/internal/components/execution"
	"trading/robot/go-bot/internal/components/portfolio"
	"trading/robot/go-bot/internal/database"
	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/utils"
)

const limitOpenOrders = 100

type Reconciler interface {
	SyncOrders(ctx context.Context, exchange, instrumentSymbol string) error
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

	// --- Resolve Vanished Buy Orders ---
	// Fetch buy orders our DB thinks are new or open.
	statuses := []string{repository.OrderStatusNew, repository.OrderStatusOpen}
	side := []string{repository.OrderSideBuy}
	dbOrders, err := r.repo.Orders.GetOrders(
		ctx, r.db, exchange, instrumentSymbol, statuses, []string{}, side, limitOpenOrders,
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

		if res.Status == repository.OrderStatusClosed {
			fillPrice := res.AveragePrice.Float64
			if fillPrice <= 0 {
				fillPrice = res.Price.Float64
			}
			if err := r.pf.CreatePosition(
				ctx, exchange, res.InstrumentSymbol, res.Filled, fillPrice, dbo.ID,
			); err != nil {
				log.Error("Failed to create position for filled order", "error", err)
			}
		}
	}

	// --- Resolve Vanished Sell Orders ---
	balances, err := r.repo.Balances.GetAllBalances(ctx, r.db, exchange)
	if err != nil {
		return fmt.Errorf("order sync: balance fetch failed: %w", err)
	}

	walletBalances := make(map[string]float64)
	for _, b := range balances {
		walletBalances[b.AssetSymbol] = b.Total
	}

	// Fetch sell orders our DB thinks are new or open.
	side = []string{repository.OrderSideSell}
	dbOrders, err = r.repo.Orders.GetOrders(
		ctx, r.db, exchange, instrumentSymbol, statuses, []string{}, side, limitOpenOrders,
	)
	if err != nil {
		return fmt.Errorf("order sync: get sell open orders failed: %w", err)
	}

	for _, dbo := range dbOrders {
		asset, _ := splitSymbol(dbo.InstrumentSymbol)
		// If no balance left, fetch the individual status from the exchange to update the order in our database.
		if utils.IsZeroEps(walletBalances[asset]) {
			_, err := r.exec.GetOrder(ctx, exchange, dbo.InstrumentSymbol, dbo.ExchangeOrderID.String)
			if err != nil {
				log.Error(
					"Order sync: failed to fetch individual sell order status",
					"id", dbo.ExchangeOrderID, "error", err,
				)
				continue
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

	// Fetch liquid truth
	balances, err := r.repo.Balances.GetAllBalances(ctx, r.db, exchange)
	if err != nil {
		return fmt.Errorf("position sync: balance fetch failed: %w", err)
	}

	walletBalances := make(map[string]float64)
	for _, b := range balances {
		walletBalances[b.AssetSymbol] = b.Total
	}

	// Fetch all actives positions for the exchange to detect base asset collisions.
	dbPositions, err := r.repo.Positions.GetActivePositions(ctx, r.db, exchange, "")
	if err != nil {
		return fmt.Errorf("position sync: db fetch failed: %w", err)
	}

	// Group existing positions by asset symbol (e.g., BTC/USDT and BTC/EURO both share a BTC balance).
	positionsByAsset := make(map[string][]repository.PositionData)
	for _, p := range dbPositions {
		asset, _ := splitSymbol(p.InstrumentSymbol)
		positionsByAsset[asset] = append(positionsByAsset[asset], p)
	}

	// --- Handle external events such as stop losses or quantity drift due to fees ---
	// Validate DB existing positions against exchange wallet balances.
	for asset, positions := range positionsByAsset {
		walletQty := walletBalances[asset]
		var positionQty float64
		for _, p := range positions {
			positionQty += p.Quantity
		}

		if utils.IsZeroEps(walletQty) {
			// If the wallet balance is zero, all associated trading positions must be closed.
			for _, p := range positions {
				log.Warn(
					"Reconciliation: Closing position (External liquidation detected)",
					"symbol", p.InstrumentSymbol,
				)
				if err := r.pf.DeletePosition(ctx, exchange, p.InstrumentSymbol); err != nil {
					log.Error(
						"Failed to close position in DB",
						"symbol", p.InstrumentSymbol, "error", err,
					)
				}
			}

		} else if !utils.IsEqualEps(positionQty, walletQty) {
			// If the total quantity drifted, we can only auto-correct if there is exactly one position for this base asset.
			// We snap to the exchange truth. This naturally reconciles deductions for trading fees or minor 'dust' remains.
			if len(positions) != 1 {
				log.Error(
					"Reconciliation: Ambiguous quantity drift detected for multi-pair asset",
					"asset", asset, "wallet_qty", walletQty, "position_qty", positionQty,
				)
				continue
			}

			posData := positions[0]
			log.Warn(
				"Reconciliation: Adjusting position quantity",
				"symbol", posData.InstrumentSymbol, "old", posData.Quantity, "new", walletQty,
			)
			posData.Quantity = walletQty
			err = r.pf.UpdatePosition(ctx, exchange, posData.InstrumentSymbol, posData)
			if err != nil {
				log.Error("Failed to update position for adjusting quantity", "error", err)
			}
		}
	}

	// --- Handle adoptions of ghost balances or manual and untracked trades. ----
	// Validate exchange wallet balances against DB existing positions.
	var instruments []string
	if instrumentSymbol != "" {
		instruments = append(instruments, instrumentSymbol)
	} else {
		statuses := []string{
			repository.StrategyEnabled,
			repository.StrategyPendingDisabled,
		}
		pairs, err := r.repo.Strategies.GetStrategyPairs(ctx, r.db, statuses)
		if err == nil {
			for _, p := range pairs {
				if p.ExchangeName == exchange {
					instruments = append(instruments, p.InstrumentSymbol)
				}
			}
		}
	}

	for _, iSymbol := range instruments {
		// If there are buy open orders in DB, no need to adopt, the position will be created when the order is filled.
		statuses := []string{repository.OrderStatusNew, repository.OrderStatusOpen}
		sides := []string{repository.OrderSideBuy}
		openOrders, err := r.repo.Orders.GetOrders(
			ctx, r.db, exchange, iSymbol, statuses, []string{}, sides, 1,
		)
		if err != nil {
			log.Error("Failed querying buy open orders", "symbol", iSymbol, "error", err)
			continue
		}
		if len(openOrders) > 0 {
			continue
		}

		asset, _ := splitSymbol(iSymbol)
		walletQty := walletBalances[asset]
		_, existsPosition := positionsByAsset[asset]

		// If we have a wallet balance but no open order or position in the DB, we adopt it as a unlinked position.
		// Since we removed automated promotion through trade sync, they should be corrected manually by the user.
		if !utils.IsZeroEps(walletQty) && !existsPosition {
			log.Warn(
				"Reconciliation: Adopting ghost balance as unlinked position",
				"symbol", iSymbol, "qty", walletQty,
			)
			err = r.pf.CreatePosition(ctx, exchange, iSymbol, walletQty, 0, 0)
			if err != nil {
				log.Error("Failed to create position for ghost balance", "error", err)
			}
		}
	}

	return nil
}

func splitSymbol(symbol string) (string, string) {
	parts := strings.Split(symbol, "/")
	if len(parts) != 2 {
		return symbol, ""
	}
	return parts[0], parts[1]
}
