package reconciliation

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"trading/robot/go-bot/internal/components/execution"
	"trading/robot/go-bot/internal/database"
	"trading/robot/go-bot/internal/database/repository"
)

const (
	limitOpenOrders = 100
	limitTrades     = 100
)

// auditLookback defines how far back (24h) the Audit Pipe looks for trade history.
const auditLookback = 24 * time.Hour

// promotionMaxAge defines the maximum age of an execution record to be considered
// for automatic promotion of an 'unmanaged' position to 'active'.
const promotionMaxAge = 15 * time.Minute

// maxFeeMargin defines the maximum expected discrepancy (2%) between a gross trade
// quantity and the net wallet balance to account for exchange trading fees.
const maxFeeMargin = 0.02

const epsilon = 1e-9

// isZero checks if a value is effectively zero within an epsilon margin.
func isZero(val float64) bool {
	return math.Abs(val) <= epsilon
}

// isEqual checks if two values are effectively equal within an epsilon margin.
func isEqual(a, b float64) bool {
	return math.Abs(a-b) <= epsilon
}

// Reconciler checks the alignment between the Exchange truth and the Database state.
type Reconciler struct {
	logger *slog.Logger
	db     *database.DB
	repo   *repository.Container
	exec   execution.Service
}

// NewReconciler creates a new instance of the Reconciler.
func NewReconciler(logger *slog.Logger, db *database.DB, repo *repository.Container, exec execution.Service) *Reconciler {
	return &Reconciler{
		logger: logger,
		db:     db,
		repo:   repo,
		exec:   exec,
	}
}

// SyncOrders synchronizes open orders from the exchange with the local database.
// It handles non-persisted orders, status drift, partial fills, and external cancellations.
func (r *Reconciler) SyncOrders(ctx context.Context, exchange, symbol string) error {
	// Fetch truth from Exchange. Execution service handles DB synchronization.
	resp, err := r.exec.GetOpenOrders(ctx, exchange, symbol, limitOpenOrders)
	if err != nil {
		return fmt.Errorf("order sync: gateway call failed: %w", err)
	}

	// Map for quick lookup of exchange truth
	exchangeOrders := make(map[string]bool)
	for _, o := range resp.Orders {
		exchangeOrders[o.Id] = true
	}

	// --- Resolve Vanished Orders ---
	// Fetch orders our DB thinks are new or open.
	statuses := []string{repository.OrderStatusNew, repository.OrderStatusOpen}
	dbOrders, err := r.repo.Orders.GetOrders(ctx, r.db, exchange, symbol, statuses, 100)
	if err != nil {
		return fmt.Errorf("order sync: db fetch failed: %w", err)
	}

	for _, dbo := range dbOrders {
		if !exchangeOrders[dbo.ExchangeOrderID] {
			// If an order is active in the database but missing from the exchange open list,
			// fetch the individual status to determine if it was filled or canceled.
			r.logger.Info("Investigating fate of vanished order", "id", dbo.ExchangeOrderID, "symbol", dbo.InstrumentSymbol, "status", dbo.Status)
			if _, err := r.exec.GetOrder(ctx, exchange, dbo.InstrumentSymbol, dbo.ExchangeOrderID); err != nil {
				r.logger.Error("Failed to resolve fate for vanished order", "id", dbo.ExchangeOrderID, "error", err)
			}
		}
	}

	return nil
}

// SyncPositions aligns database positions with exchange balances.
// It handles external liquidations, manual trades, and quantity drift due to fees or dust.
func (r *Reconciler) SyncPositions(ctx context.Context, exchange, symbol string) error {
	// Fetch liquid truth
	balances, err := r.repo.Balances.GetAllBalances(ctx, r.db)
	if err != nil {
		return fmt.Errorf("position sync: balance fetch failed: %w", err)
	}

	walletBalances := make(map[string]float64)
	for _, b := range balances {
		if b.ExchangeName == exchange {
			walletBalances[b.AssetSymbol] = b.Total
		}
	}

	// Fetch all open positions for the exchange to detect base asset collisions.
	dbPositions, err := r.repo.Positions.GetOpenPositions(ctx, r.db, exchange, "")
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

		if isZero(walletQty) {
			// If the wallet balance is zero, all associated trading positions must be closed.
			for _, p := range positions {
				r.logger.Warn("Reconciliation: Closing position (External liquidation detected)", "symbol", p.InstrumentSymbol)
				if err := r.repo.Positions.DeletePosition(ctx, r.db, exchange, p.InstrumentSymbol); err != nil {
					r.logger.Error("Failed to close position in DB", "symbol", p.InstrumentSymbol, "error", err)
				}
			}
		} else if !isEqual(positionQty, walletQty) {
			// If the total quantity drifted, we can only auto-correct if there is exactly one position for this base asset.
			// We snap to the exchange truth. This naturally reconciles deductions for trading fees or minor 'dust' remains.
			if len(positions) == 1 {
				posData := positions[0]
				r.logger.Info("Reconciliation: Adjusting position quantity", "symbol", posData.InstrumentSymbol, "old", posData.Quantity, "new", walletQty)
				posData.Quantity = walletQty
				_ = r.repo.Positions.UpsertPosition(ctx, r.db, posData)
			} else {
				r.logger.Error("Reconciliation: Ambiguous quantity drift detected for multi-pair asset", "asset", asset, "wallet_qty", walletQty, "position_qty", positionQty)
			}
		}
	}

	// --- Handle adoptions of ghost balances or manual trades. ----
	// Validate exchange wallet balances against DB existing positions to detect 'ghost' balances or manual trades.
	var targets []string
	if symbol != "" {
		targets = append(targets, symbol)
	} else {
		pairs, err := r.repo.Strategies.GetStrategyPairs(ctx, r.db, false)
		if err == nil {
			for _, p := range pairs {
				if p.ExchangeName == exchange {
					targets = append(targets, p.InstrumentSymbol)
				}
			}
		}
	}

	// If no specific symbol is provided, we iterate through all enabled pairs for the exchange.
	for _, target := range targets {
		asset, _ := splitSymbol(target)
		walletQty := walletBalances[asset]

		positions, exists := positionsByAsset[asset]
		var positionQty float64
		if exists {
			for _, p := range positions {
				positionQty += p.Quantity
			}
		}

		if (!exists || isZero(positionQty)) && !isZero(walletQty) {
			// If we have a wallet balance but no corresponding position in the DB, we adopt it as a new 'unmanaged' position.
			// This prevents trading positions using inaccurate prices until SyncTradeHistory finds the true execution record.
			r.logger.Info("Reconciliation: Adopting ghost balance as unmanaged position", "symbol", target, "qty", walletQty)
			_ = r.repo.Positions.UpsertPosition(ctx, r.db, repository.PositionData{
				ExchangeName:     exchange,
				InstrumentSymbol: target,
				Side:             repository.PositionSideLong,
				Quantity:         walletQty,
				EntryPrice:       0.0,
				HighestPrice:     0.0,
				StrategyState:    "unmanaged",
				Active:           true,
			})
		}
	}

	return nil
}

// SyncTradeHistory fetches recent execution history to ensure all trades (including external ones)
// are recorded locally for reporting and position promotion.
func (r *Reconciler) SyncTradeHistory(ctx context.Context, exchange, symbol string) error {
	// Fetch recent trades from the exchange. Execution service handles DB synchronization.
	since := time.Now().Add(-auditLookback).UnixMilli()
	resp, err := r.exec.GetRecentTrades(ctx, exchange, symbol, since, limitTrades)
	if err != nil {
		return fmt.Errorf("trade history sync: %w", err)
	}
	orders := resp.Orders

	// If no orders are returned and no symbol is provided, we perform a per-symbol audit for all active pairs.
	if len(orders) == 0 && symbol == "" {
		pairs, err := r.repo.Strategies.GetStrategyPairs(ctx, r.db, false)
		if err != nil {
			return fmt.Errorf("audit: failed to load pairs: %w", err)
		}

		for _, p := range pairs {
			if p.ExchangeName == exchange {
				resp, err := r.exec.GetRecentTrades(ctx, exchange, p.InstrumentSymbol, since, limitTrades)
				if err != nil {
					r.logger.Error("Failed to fetch trade history for symbol during audit", "symbol", p.InstrumentSymbol, "error", err)
					continue
				}
				orders = append(orders, resp.Orders...)
			}
		}
	}

	// --- Promotion Logic ---
	for _, o := range orders {
		pos, err := r.repo.Positions.GetPosition(ctx, r.db, exchange, o.Symbol)
		if err != nil {
			return fmt.Errorf("trade history sync: failed to lookup position for %s: %w", o.Symbol, err)
		}

		// If a trade exists for a symbol currently in 'unmanaged' state, we promote it to 'active'.
		if pos.StrategyState == "unmanaged" {
			diff := o.Filled - pos.Quantity
			allowedMargin := o.Filled * maxFeeMargin
			// Validate that the trade quantity matches the unmanaged position quantity set by the Position Sync.
			// This ensures we don't promote a position that is unrelated to the discovered trade.
			// We allow for a small margin to account for exchange fees.
			if diff < 0 || diff > allowedMargin {
				r.logger.Debug("Trade found but quantity mismatch beyond fee margin, skipping promotion",
					"symbol", o.Symbol, "trade_qty", o.Filled, "pos_qty", pos.Quantity)
				continue
			}

			// Do not promote if the execution is too old, as the entry and highest prices
			// would likely be stale compared to current market reality.
			if o.Timestamp == 0 || time.Since(time.UnixMilli(o.Timestamp)) > promotionMaxAge {
				r.logger.Warn("Execution found but too old for automatic promotion",
					"symbol", o.Symbol, "id", o.Id)
				continue
			}

			// Use Average Price if available (common for filled orders), otherwise Limit Price.
			executionPrice := o.Average
			if executionPrice <= 0 {
				executionPrice = o.Price
			}

			if executionPrice > 0 {
				r.logger.Info("Reconciliation: Promoting unmanaged position to active from trade history",
					"symbol", pos.InstrumentSymbol, "price", executionPrice)

				pos.EntryPrice = executionPrice
				pos.HighestPrice = executionPrice
				pos.StrategyState = "active"
				if err := r.repo.Positions.UpsertPosition(ctx, r.db, pos); err != nil {
					r.logger.Error("Failed to promote position to active", "symbol", pos.InstrumentSymbol, "error", err)
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
