package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"trading/robot/go-bot/internal/components/signal_generator"
	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/strategy"

	"github.com/jackc/pgx/v5"
)

const epsilon = 1e-9

// isZeroEps checks if a value is effectively zero within an epsilon margin.
func isZeroEps(val float64) bool {
	return math.Abs(val) <= epsilon
}

// isEqualEps checks if two values are effectively equal within an epsilon margin.
func isEqualEps(a, b float64) bool {
	return math.Abs(a-b) <= epsilon
}

// ----------------------------------------------------------------------------
// Init Methods
// ----------------------------------------------------------------------------

// initSignalHandler sets up a signal generator with warmup data
func (o *Orchestrator) initSignalHandler(
	ctx context.Context,
	p repository.StrategyPair,
	name string,
) (*signal_generator.SignalGenerator, error) {
	log := o.logger.With("exchange", p.ExchangeName, "symbol", p.InstrumentSymbol)
	log.Info("Init signal generator")

	// Load historical ticks and risk configuration
	sinceEpoch := time.Now().Unix() - int64(p.WarmupWindowSeconds)
	ticks, err := o.repo.MarketData.GetMarketDataTicks(
		ctx, o.db, p.ExchangeName, p.InstrumentSymbol, sinceEpoch,
	)
	if err != nil {
		return nil, fmt.Errorf("fetch warmup data failed %w", err)
	}

	riskData, err := o.repo.Risks.GetRiskPair(ctx, o.db, p.ExchangeName, p.InstrumentSymbol)
	if err != nil {
		return nil, fmt.Errorf("fetch risk config failed %w", err)
	}

	// Create signal generator instance with warmup data
	sigGen, err := signal_generator.NewSignalGenerator(o.logger, riskData, p, name)
	if err != nil {
		return nil, fmt.Errorf("create signal generator failed %w", err)
	}

	if len(ticks) > 0 {
		log.Info("Warming up signal generator with historical ticks", "count", len(ticks))

		err = sigGen.Warmup(ticks)
		if err != nil {
			return nil, fmt.Errorf("warmup failed %w", err)
		}
	}

	if p.Status == repository.StrategyPendingDisabled {
		sigGen.SetPendingTerminate(true)
	}

	// Align strategy engine with recovered position
	pos, err := o.portfolio.GetPosition(ctx, p.ExchangeName, p.InstrumentSymbol)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			log.Error("error getting position, strategy state not hydrated", "err", err)
		}
	} else {
		if !pos.UnknownOrigin {
			log.Info("Hydrate strategy state", "entry", pos.EntryPrice, "high", pos.HighestPrice)
			sigGen.SetInPosition(true, pos.EntryPrice, pos.HighestPrice)
		}
	}

	o.mu.Lock()
	if _, exists := o.signals[name]; exists {
		o.mu.Unlock()
		return nil, fmt.Errorf("signal handler for %s already exists", name)
	}
	o.signals[name] = sigGen
	o.mu.Unlock()

	log.Info("Signal generator ready")

	return sigGen, nil
}

// ----------------------------------------------------------------------------
// Process Signal Method
// ----------------------------------------------------------------------------

// processSignal handles the logic for processing a signal from a signal generator
func (o *Orchestrator) processSignal(ctx context.Context, sig *signal_generator.SignalGenerator) {
	exchange := sig.Exchange()
	instrumentSymbol := sig.InstrumentSymbol()

	log := o.logger.With("exchange", exchange, "symbol", instrumentSymbol)
	log.Info("Processing signal")

	var signal strategy.StrategySignal

	defer func() {
		// Termination logic: if scheduled to disable and no position is active, finalize.
		if !sig.IsPendingTerminate() || signal != strategy.SignalSearchingBuyEntry {
			return
		}

		_, err := o.portfolio.GetPosition(ctx, exchange, instrumentSymbol)
		if err != nil && errors.Is(err, pgx.ErrNoRows) {
			log.Info("Applying strategy disablement for pending_disabled pair")

			if err := o.repo.Strategies.ApplyStrategyDisable(
				ctx, o.db, exchange, instrumentSymbol,
			); err != nil {
				log.Error("Failed to apply strategy disablement", "err", err)
			} else {
				o.stopWorker(sig.Name())
			}
		} else if err != nil {
			log.Error("error checking position for pending disablement", "err", err)
		}
	}()

	// Fetch latest price for valuation and sizing
	ticker, err := o.exec.GetTicker(ctx, exchange, instrumentSymbol)
	if err != nil {
		log.Error("fetch ticker failed", "err", err)
		return
	}
	price := ticker.Price

	// Update strategy with latest price and get next signal
	signal, err = sig.GetSignal(price, time.Now().Unix())
	if err != nil {
		log.Error("strategy update price and get signal failed", "err", err)
		return
	}

	log.Info("Signal generated", "signal", signal.String())

	// Handle the signal with corresponding logic
	switch signal {
	case strategy.SignalSearchingBuyEntry:
		o.signalSearchingBuyEntry(ctx, log, sig)
	case strategy.SignalBuy:
		if sig.IsPendingTerminate() {
			_ = sig.RetrySignal(strategy.SignalBuy)
			signal = strategy.SignalSearchingBuyEntry
		} else {
			o.signalBuy(ctx, log, sig, price)
		}
	case strategy.SignalWaitingBuyFill:
		o.signalWaitingBuyFill(ctx, log, sig, price)
	case strategy.SignalTrackingSellExit:
		o.signalTrackingSellExit(ctx, log, sig, price)
	case strategy.SignalSell:
		o.signalSell(ctx, log, sig, price)
	case strategy.SignalWaitingSellFill:
		o.signalWaitingSellFill(ctx, log, sig)
	case strategy.SignalInvalid:
		o.signalInvalid(ctx, log, sig)
	default:
		log.Error("unknown signal received: ", "signal", signal.String())
	}
}

// ----------------------------------------------------------------------------
// Signal Handler Methods
// ----------------------------------------------------------------------------

func (o *Orchestrator) signalSearchingBuyEntry(
	ctx context.Context,
	log *slog.Logger,
	sig *signal_generator.SignalGenerator,
) {
	ex := sig.Exchange()
	sym := sig.InstrumentSymbol()

	posIsSync := false
	for {
		pos, err := o.portfolio.GetPosition(ctx, ex, sym)
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				log.Error("failed to query position during searching buy entry", "err", err)
			}
			return
		}

		// If the position is from unknown origin, it should be fixed by the reconciler or manually.
		if pos.UnknownOrigin {
			return
		}

		// Request balance from exchange.
		balance, err := o.getBalance(ctx, log, ex, sym)
		if err != nil {
			return
		}

		// If the database position quantity matches the exchange total balance, we sync the strategy metadata.
		if isEqualEps(balance.Total, pos.Quantity) {
			log.Warn(
				"syncing strategy to active state due to existing position",
				"position", pos.Quantity,
			)
			sig.SetInPosition(true, pos.EntryPrice, pos.HighestPrice)
			return
		}

		if !posIsSync {
			// If we have a mismatch between position and balance, we trigger a sync to fix potential inconsistencies.
			log.Warn(
				"position/balance mismatch, triggering reconciliation",
				"position", pos.Quantity, "balance", balance.Total,
			)
			err1 := o.recon.SyncPositions(ctx, sig.Exchange(), sig.InstrumentSymbol())
			err2 := o.recon.SyncTradeHistory(ctx, sig.Exchange(), sig.InstrumentSymbol(), 15*time.Minute)
			if err1 != nil || err2 != nil {
				log.Error(
					"reconciliation failed during searching buy entry",
					"err_sync_positions", err1, "err_sync_trades", err2,
				)
				return
			}
			posIsSync = true

		} else {
			// If we already triggered a sync and we still have a mismatch, we mark the position as 'unknown origin'.
			log.Warn(
				"position/balance still mismatch after reconciliation, flaging position as unknown origin",
				"position", pos.Quantity, "balance", balance.Total,
			)
			pos.UnknownOrigin = true
			_ = o.portfolio.UpdatePosition(ctx, sig.Exchange(), sig.InstrumentSymbol(), pos)
			return
		}
	}
}

func (o *Orchestrator) signalBuy(
	ctx context.Context,
	log *slog.Logger,
	sig *signal_generator.SignalGenerator,
	price float64,
) {
	ex := sig.Exchange()
	sym := sig.InstrumentSymbol()

	pos, err := o.portfolio.GetPosition(ctx, ex, sym)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Error("failed to query position during buy signal", "err", err)
		_ = sig.RetrySignal(strategy.SignalBuy)
		return
	} else if err == nil {
		// If we already have a position but from unknown origin, it should be fixed by the reconciler or manually.
		if pos.UnknownOrigin {
			log.Warn("position stuck in invalid state (no order link), can't proceed until fixed, resetting strategy state")
			sig.SetInPosition(false, 0, 0)
			return
		}

		log.Warn("buy skipped: position already exists in portfolio")
		return
	}

	parts := strings.Split(sym, "/")
	if len(parts) != 2 {
		log.Error("invalid symbol format during buy signal", "symbol", sym)
		_ = sig.RetrySignal(strategy.SignalBuy)
		return
	}
	budgetAsset := parts[1]

	// Check risk first using local data to avoid unnecessary exchange requests.
	openCount := o.portfolio.GetActivePositionsCount()

	availableBudget := 0.0
	if b, err := o.repo.Balances.GetBalance(ctx, o.db, ex, budgetAsset); err == nil {
		availableBudget = b.Total
	}
	eval := o.risk.EvaluateEntry(openCount, 0, price, availableBudget, sig.Risk())
	if !eval.Allowed {
		log.Warn("buy rejected by risk manager (pre-check)", "reason", eval.Reason)
		_ = sig.RetrySignal(strategy.SignalBuy)
		return
	}

	// Request open orders and balance from exchange, in this specific order to avoid inconsistency.
	openOrders, err := o.exec.GetOpenOrders(ctx, ex, sym, 10)
	if err != nil {
		log.Error("failed to verify open orders on exchange", "err", err)
		_ = sig.RetrySignal(strategy.SignalBuy)
		return
	}
	for _, ord := range openOrders {
		if ord.Side == repository.OrderSideBuy {
			log.Warn(
				"buy skipped: existent pending order, proceeding to avoid duplication",
			)
			return
		}
	}

	balance, err := o.getBalance(ctx, log, ex, sym)
	if err != nil {
		_ = sig.RetrySignal(strategy.SignalBuy)
		return
	}
	if !isZeroEps(balance.Total) {
		log.Warn(
			"buy skipped: existent balance, proceeding to avoid duplication", "balance", balance.Total,
		)
		return
	}

	// Double check risk after exchange latency (use updated budget balance).
	availableBudget = 0.0
	if b, err := o.repo.Balances.GetBalance(ctx, o.db, ex, budgetAsset); err == nil {
		availableBudget = b.Total
	}
	openCount = o.portfolio.GetActivePositionsCount()
	eval = o.risk.EvaluateEntry(openCount, 0, price, availableBudget, sig.Risk())

	if !eval.Allowed {
		log.Warn("buy rejected by risk manager (final-check)", "reason", eval.Reason)
		_ = sig.RetrySignal(strategy.SignalBuy)
		return
	}

	log.Info("placing market buy order", "qty", eval.ApprovedUnits)
	order, err := o.exec.CreateOrder(
		ctx, ex, sym, repository.OrderSideBuy, repository.OrderTypeMarket, eval.ApprovedUnits, 0,
	)
	if err != nil {
		log.Error("market buy order failed", "err", err)
		return // Let reconciler or next cycle handle recovery
	}

	// Create position if the order was immediately filled.
	if order.Status == repository.OrderStatusClosed {
		fillPrice := order.AveragePrice.Float64
		if fillPrice <= 0 {
			fillPrice = order.Price.Float64
		}
		err = o.portfolio.CreatePosition(
			ctx, ex, sym, order.Filled, fillPrice, order.ID,
		)
		if err != nil {
			log.Error("failed to create position for filled order", "err", err)
		}
	}
}

func (o *Orchestrator) signalWaitingBuyFill(
	ctx context.Context,
	log *slog.Logger,
	sig *signal_generator.SignalGenerator,
	price float64,
) {
	ex := sig.Exchange()
	sym := sig.InstrumentSymbol()

	pos, err := o.portfolio.GetPosition(ctx, ex, sym)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Error("failed to query position during waiting buy fill", "err", err)
		return
	}

	// No active position found locally, check exchange truth.
	if err != nil {
		openOrders, err := o.exec.GetOpenOrders(ctx, ex, sym, 10)
		if err != nil {
			log.Error("failed to verify open orders on exchange", "err", err)
			return
		}
		var buyOrderExists bool
		for _, ord := range openOrders {
			if ord.Side == repository.OrderSideBuy {
				buyOrderExists = true
				break
			}
		}

		if buyOrderExists {
			log.Info("buy order still processing on exchange, waiting...")

		} else {
			balance, err := o.getBalance(ctx, log, ex, sym)
			if err != nil {
				return
			}

			// If SyncOrders can't find the order, SyncPositions will create a position from the existent balance.
			if balance.Total > 0 {
				log.Info("filled balance detected but no local position, triggering syncs")
				_ = o.recon.SyncOrders(ctx, ex, sym)
				_ = o.recon.SyncPositions(ctx, ex, sym)
			} else {
				log.Warn("no balance and no open buy orders found, resetting strategy state")
				sig.SetInPosition(false, 0, 0)
			}
		}
		return
	}

	// Attempt to fix position from unknown origin searching missing order via trade history.
	if pos.UnknownOrigin {
		err = o.recon.SyncTradeHistory(ctx, ex, sym, 15*time.Minute)
		if err != nil {
			log.Error("trade history sync failed during unlinked position recovery", "err", err)
			return
		}

		pos, err = o.portfolio.GetPosition(ctx, ex, sym)
		if err != nil {
			log.Error("failed to query position during invalid state recovery", "err", err)
			return
		}
		if pos.UnknownOrigin {
			log.Warn(
				"position stuck in invalid state (no order link), can't proceed until fixed, resetting strategy state",
			)
			sig.SetInPosition(false, 0, 0)
			return
		}
	}

	if price > pos.HighestPrice {
		log.Info("updating highest price for trailing stop", "old", pos.HighestPrice, "new", price)
		pos.HighestPrice = price
		err = o.portfolio.UpdatePosition(ctx, ex, sym, pos)
		if err != nil {
			// log the error but proceed to confirm the signal
			log.Error("failed to update position with new highest price", "err", err)
		}
	}

	log.Info("position active found, confirming signal")
	sig.SetInPosition(true, pos.EntryPrice, pos.HighestPrice)
}

func (o *Orchestrator) signalTrackingSellExit(
	ctx context.Context,
	log *slog.Logger,
	sig *signal_generator.SignalGenerator,
	price float64,
) {
	ex := sig.Exchange()
	sym := sig.InstrumentSymbol()

	pos, err := o.portfolio.GetPosition(ctx, ex, sym)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Warn("position removed externally or missing, resetting strategy state")
			sig.SetInPosition(false, 0, 0)
		} else {
			log.Error("failed to query position during tracking sell exit", "err", err)
		}
		return
	}

	if pos.UnknownOrigin {
		log.Warn("position unlinked during tracking sell exit, resetting strategy state")
		sig.SetInPosition(false, 0, 0)
		return
	}

	stopLossMissing := !pos.StopLossActive
	if stopLossMissing || price > pos.HighestPrice {
		// If stop loss is missing, verify if a stop loss is already placed on the exchange.
		if stopLossMissing {
			openOrders, err := o.exec.GetOpenOrders(ctx, ex, sym, 10)
			if err != nil {
				log.Error("failed to fetch open orders for stop loss verification", "err", err)
				return
			}
			var sellOrderExists bool
			for _, ord := range openOrders {
				if ord.Side == repository.OrderSideSell {
					sellOrderExists = true
					break
				}
			}

			if !sellOrderExists {
				log.Info("active position found without stop loss, placing protection")

				stopLossPrice := pos.EntryPrice * (1.0 - sig.StrategyConfig().StopLossPct)
				_, err := o.exec.CreateStopOrder(
					// limitPrice 0 => Stop Market
					ctx, ex, sym, repository.OrderSideSell, pos.Quantity, stopLossPrice, 0,
				)
				if err != nil {
					log.Error("failed to place stop loss order", "err", err)
					return
				}
			}

			pos.StopLossActive = true
		}

		if price > pos.HighestPrice {
			log.Info("updating highest price for trailing stop", "old", pos.HighestPrice, "new", price)
			pos.HighestPrice = price
		}

		err = o.portfolio.UpdatePosition(ctx, ex, sym, pos)
		if err != nil {
			if stopLossMissing {
				log.Error("failed to update position to set stop loss active", "err", err)
			} else {
				log.Error("failed to update position with new highest price", "err", err)
			}
			return
		}
	}
}

func (o *Orchestrator) signalSell(
	ctx context.Context,
	log *slog.Logger,
	sig *signal_generator.SignalGenerator,
	price float64,
) {
	ex := sig.Exchange()
	sym := sig.InstrumentSymbol()

	pos, err := o.portfolio.GetPosition(ctx, ex, sym)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Warn("position already removed externally, resetting strategy state")
			sig.SetInPosition(false, 0, 0)
		} else {
			log.Error("failed to query position during sell signal", "err", err)
			_ = sig.RetrySignal(strategy.SignalSell)
		}
		return
	}

	if pos.UnknownOrigin {
		log.Warn("position unlinked during sell signal, resetting strategy state")
		sig.SetInPosition(false, 0, 0)
		return
	}

	// Request balance from exchange, if gone, delete position and set strategy to IDLE.
	balance, err := o.getBalance(ctx, log, ex, sym)
	if err != nil {
		_ = sig.RetrySignal(strategy.SignalSell)
		return
	}
	if isZeroEps(balance.Total) {
		log.Info("exchange balance is zero, closing local position and setting strategy to idle")
		_ = o.portfolio.DeletePosition(ctx, ex, sym)
		sig.SetInPosition(false, 0, 0)
		return
	}

	// Check if we have a market sell order or limit (stop loss).
	openOrders, err := o.exec.GetOpenOrders(ctx, ex, sym, 10)
	if err != nil {
		log.Error("failed to fetch open orders for sell check", "err", err)
		_ = sig.RetrySignal(strategy.SignalSell)
		return
	}

	var marketSellExists bool
	var stopLossOrder *repository.OrderData
	for _, ord := range openOrders {
		if ord.Side == repository.OrderSideSell {
			if ord.OrderType == repository.OrderTypeMarket {
				marketSellExists = true
				break
			}
			if ord.OrderType == repository.OrderTypeStopMarket {
				// This is the stop loss order placed previously.
				stopLossOrder = &ord
			}
		}
	}

	if marketSellExists {
		log.Info("market sell order already in flight, proceeding to avoid duplication")
		return
	}

	if stopLossOrder != nil {
		isProfitTake := price >= pos.EntryPrice
		if !isProfitTake {
			log.Info("stop loss order already exists on exchange, waiting for fill")
			return
		}

		// Profit Take triggered, cancel existing SL order first to free balance.
		log.Info(
			"profit take triggered, canceling existing stop loss order",
			"order_id", stopLossOrder.ExchangeOrderID,
		)
		if err := o.exec.CancelOrder(ctx, ex, sym, stopLossOrder.ExchangeOrderID); err != nil {
			log.Error("failed to cancel stop loss order for profit take", "err", err)
			_ = sig.RetrySignal(strategy.SignalSell)
			return
		}
	}

	log.Info("placing market sell order", "qty", pos.Quantity)
	order, err := o.exec.CreateOrder(
		ctx, ex, sym, repository.OrderSideSell,
		repository.OrderTypeMarket, pos.Quantity, 0,
	)
	if err != nil {
		log.Error("market sell order failed", "err", err)
		_ = sig.RetrySignal(strategy.SignalSell) // If the order was created, we'll find it in the next cycle
		return
	}

	// If filled immediately, delete position and set strategy to IDLE.
	if order.Status == repository.OrderStatusClosed {
		log.Info("Sell order filled, closing local position")
		err = o.portfolio.DeletePosition(ctx, ex, sym)
		if err != nil {
			log.Error("failed to delete position after sell fill", "err", err)
			return
		}
		sig.SetInPosition(false, 0, 0)
	}
}

func (o *Orchestrator) signalWaitingSellFill(
	ctx context.Context,
	log *slog.Logger,
	sig *signal_generator.SignalGenerator,
) {
	ex := sig.Exchange()
	sym := sig.InstrumentSymbol()

	_, err := o.portfolio.GetPosition(ctx, ex, sym)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Warn("sell filled, position is gone, setting strategy to idle")
			sig.SetInPosition(false, 0, 0)
		} else {
			log.Error("failed to query position during waiting sell fill", "err", err)
		}
		return
	}

	// Request balance from exchange, if gone, delete position and set strategy to IDLE
	balance, err := o.getBalance(ctx, log, ex, sym)
	if err != nil {
		return
	}
	if isZeroEps(balance.Total) {
		log.Info("exchange balance is zero, closing local position and setting strategy to idle")
		_ = o.portfolio.DeletePosition(ctx, ex, sym)
		sig.SetInPosition(false, 0, 0)
		return
	}

	// Request sell open orders from exchange.
	openOrders, err := o.exec.GetOpenOrders(ctx, ex, sym, 10)
	if err != nil {
		log.Error("failed to fetch open orders for sell check", "err", err)
		return
	}
	var sellOrderExists bool
	for _, ord := range openOrders {
		if ord.Side == repository.OrderSideSell {
			sellOrderExists = true
			break
		}
	}

	// If there is still balance but no sell orders, we force to place a new sell order.
	if !sellOrderExists {
		log.Warn("sell order not found on exchange, canceling signal to trigger recovery")
		_ = sig.RetrySignal(strategy.SignalSell)
	} else {
		log.Info("sell order still processing on exchange, waiting...")
	}
}

func (o *Orchestrator) signalInvalid(
	ctx context.Context,
	log *slog.Logger,
	sig *signal_generator.SignalGenerator,
) {
	log.Error("invalid signal received, resyncing the strategy state")

	pos, err := o.portfolio.GetPosition(ctx, sig.Exchange(), sig.InstrumentSymbol())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			sig.SetInPosition(false, 0, 0)
		} else {
			log.Error("failed to query position", "err", err)
		}
		return
	}

	if pos.UnknownOrigin {
		sig.SetInPosition(false, 0, 0)
		return
	}

	log.Info("Set strategy state in position", "entry", pos.EntryPrice, "high", pos.HighestPrice)
	sig.SetInPosition(true, pos.EntryPrice, pos.HighestPrice)
}

func (o *Orchestrator) getBalance(
	ctx context.Context,
	log *slog.Logger,
	exchange, instrumentSymbol string,
) (repository.BalanceData, error) {
	asset := strings.Split(instrumentSymbol, "/")[0]
	balances, err := o.exec.GetBalance(ctx, exchange, asset)
	if err != nil || len(balances) == 0 {
		if err != nil {
			log.Error("failed to verify balance on exchange", "err", err)
		} else {
			log.Warn("balance no found on exchange", "asset", asset)
		}
		return repository.BalanceData{}, fmt.Errorf("failed to get balance")
	}

	return balances[0], nil
}
