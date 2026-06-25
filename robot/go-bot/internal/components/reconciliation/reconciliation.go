package reconcil

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"trading/robot/go-bot/internal/components/execution"
	"trading/robot/go-bot/internal/components/portfolio"
	"trading/robot/go-bot/internal/database"
	"trading/robot/go-bot/internal/database/repository"

	"github.com/jackc/pgx/v5"
)

const (
	limitOpenOrders = 100
	limitTrades     = 100
)

// promotionMaxAge defines the maximum age of an execution record to be considered
// for automatic promotion of an 'unknown origin' position to 'active'.
const promotionMaxAge = 1 * time.Hour

// maxFeeMargin defines the maximum expected discrepancy (2%) between a gross trade
// quantity and the net wallet balance to account for exchange trading fees.
const maxFeeMargin = 0.05

const epsilon = 1e-9

// isZeroEps checks if a value is effectively zero within an epsilon margin.
func isZeroEps(val float64) bool {
	return math.Abs(val) <= epsilon
}

// isEqualEps checks if two values are effectively equal within an epsilon margin.
func isEqualEps(a, b float64) bool {
	return math.Abs(a-b) <= epsilon
}

type Reconciler interface {
	SyncOrders(ctx context.Context, exchange, instrumentSymbol string) error
	SyncPositions(ctx context.Context, exchange, instrumentSymbol string) error
	SyncTradeHistory(
		ctx context.Context, exchange, instrumentSymbol string, lookback time.Duration,
	) error
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
		// Fetch the individual status from the exchange to determine if it was already filled or canceled.
		// Execution service GetOrder handles DB synchronization of the order record.
		res, err := r.exec.GetOrder(ctx, exchange, dbo.InstrumentSymbol, dbo.ExchangeOrderID)
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
		if isZeroEps(walletBalances[asset]) {
			_, err := r.exec.GetOrder(ctx, exchange, dbo.InstrumentSymbol, dbo.ExchangeOrderID)
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

		if isZeroEps(walletQty) {
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

		} else if !isEqualEps(positionQty, walletQty) {
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
			log.Info(
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
		if !isZeroEps(walletQty) && !existsPosition {
			log.Info(
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

// SyncTradeHistory fetches recent execution history to ensure all trades (including external ones)
// are recorded locally for reporting and position promotion.
func (r *reconciler) SyncTradeHistory(
	ctx context.Context,
	exchange, instrumentSymbol string,
	lookback time.Duration,
) error {
	log := r.logger.With("exchange", exchange)

	// Fetch recent trades from the exchange. Execution service handles DB synchronization.
	since := time.Now().Add(-lookback).UnixMilli()
	orders, err := r.exec.GetRecentTrades(ctx, exchange, instrumentSymbol, since, limitTrades)
	if err != nil {
		return fmt.Errorf("trade history sync: gateway call failed: %w", err)
	}

	// If no orders are returned and no symbol is provided, we perform a per-symbol audit for all enabled pairs.
	if len(orders) == 0 && instrumentSymbol == "" {
		statuses := []string{
			repository.StrategyEnabled,
			repository.StrategyPendingDisabled,
		}
		pairs, err := r.repo.Strategies.GetStrategyPairs(ctx, r.db, statuses)
		if err != nil {
			return fmt.Errorf("trade history sync: failed to load pairs: %w", err)
		}

		for _, p := range pairs {
			if p.ExchangeName == exchange {
				ordersPerSymbol, err := r.exec.GetRecentTrades(
					ctx, exchange, p.InstrumentSymbol, since, limitTrades,
				)
				if err != nil {
					log.Error(
						"Failed to fetch trade history for symbol during audit",
						"symbol", p.InstrumentSymbol, "error", err,
					)
					continue
				}
				orders = append(orders, ordersPerSymbol...)
			}
		}
	}

	// --- Promotion Logic ---
	for _, o := range orders {
		if o.Side == repository.OrderSideSell {
			continue
		}

		pos, err := r.pf.GetPosition(ctx, exchange, o.InstrumentSymbol)
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				log.Error(
					"Trade history sync: failed to lookup position",
					"symbol", o.InstrumentSymbol, "error", err,
				)
			}
			continue
		}

		// If a trade exists for a position from unknown origin, we linked and promote it to active.
		if pos.UnknownOrigin || pos.EntryPrice == 0 {
			// Check if the execution is not too old to be promoted automatically.
			if !o.ExchangeTimestamp.Valid || time.Since(o.ExchangeTimestamp.Time) > promotionMaxAge {
				log.Warn(
					"Execution found but too old for automatic promotion", "symbol", o.InstrumentSymbol,
					"now", time.Now(), "exchange_timestamp", o.ExchangeTimestamp.Time,
				)
				continue
			}

			diff := o.Filled - pos.Quantity
			allowedMargin := o.Filled * maxFeeMargin
			// Validate that the trade quantity matches the unmanaged position quantity set by the Position Sync.
			// This ensures we don't promote a position that is completely unrelated to the discovered trade.
			// We allow a reasonable margin to cover exchange rates and other possible discrepancies.
			if diff < 0 || diff > allowedMargin {
				log.Debug(
					"Trade found but quantity mismatch beyond fee margin, skipping promotion",
					"symbol", o.InstrumentSymbol, "trade_qty", o.Filled, "pos_qty", pos.Quantity,
				)
				continue
			}

			// Use Average Price if available (common for filled orders), otherwise Limit Price.
			executionPrice := o.AveragePrice.Float64
			if executionPrice <= 0 {
				executionPrice = o.Price.Float64
			}

			if executionPrice > 0 {
				log.Info(
					"Reconciliation: Promoting unlinked position to active from trade history",
					"symbol", pos.InstrumentSymbol, "price", executionPrice, "order_id", o.ExchangeOrderID,
				)

				// Try to find the local order ID for a high-fidelity link
				dbOrder, err := r.repo.Orders.GetOrder(ctx, r.db, exchange, o.ExchangeOrderID)
				if err == nil {
					err = r.pf.UpdatePosition(
						ctx,
						exchange,
						pos.InstrumentSymbol,
						repository.PositionData{
							OrderID:       sql.NullInt64{Int64: dbOrder.ID, Valid: true},
							EntryPrice:    executionPrice,
							HighestPrice:  executionPrice,
							Quantity:      pos.Quantity,
							UnknownOrigin: false,
						},
					)
				} else {
					// For manual trades where no local order exists, use UpdatePosition for concurrency-safe state transition
					err = r.pf.UpdatePosition(ctx, exchange, pos.InstrumentSymbol, repository.PositionData{
						EntryPrice:    executionPrice,
						HighestPrice:  executionPrice,
						Quantity:      pos.Quantity,
						UnknownOrigin: true,
					})
				}

				if err != nil {
					log.Error("Failed to unlinked position from trade history", "error", err)
				}
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
