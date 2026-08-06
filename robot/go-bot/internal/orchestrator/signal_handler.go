package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"trading/robot/go-bot/internal/components/signal_generator"
	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/strategy"
	"trading/robot/go-bot/internal/utils"

	"github.com/jackc/pgx/v5"
)

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
	sinceEpoch := o.clock.Now().Unix() - int64(p.WarmupWindowSeconds)
	ticks, err := o.repo.MarketData.GetMarketDataTicks(
		ctx, o.db, p.ExchangeName, p.InstrumentSymbol, sinceEpoch,
	)
	if err != nil {
		return nil, fmt.Errorf("fetch warmup data failed %w", err)
	}

	var riskData repository.RiskPair
	if !o.cfg.Simulation.Enabled {
		riskData, err = o.repo.Risks.GetRiskPair(ctx, o.db, p.ExchangeName, p.InstrumentSymbol)
		if err != nil {
			return nil, fmt.Errorf("fetch risk config failed %w", err)
		}
	} else {
		// In simulation mode, we use a fraction of the initial USDT budget.
		riskData = repository.RiskPair{
			ExchangeName:     p.ExchangeName,
			InstrumentSymbol: p.InstrumentSymbol,
			AllocatedBudget:  o.cfg.Simulation.InitialUSDT / 10,
		}
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
	signal, err = sig.GetSignal(price, o.clock.Now().Unix())
	if err != nil {
		log.Error("strategy update price and get signal failed", "err", err)
		return
	}

	log.Info("Processing signal", "signal", signal.String())

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

// signalSearchingBuyEntry handles the logic for a searching buy entry signal
func (o *Orchestrator) signalSearchingBuyEntry(
	ctx context.Context,
	log *slog.Logger,
	sig *signal_generator.SignalGenerator,
) {
	ex := sig.Exchange()
	sym := sig.InstrumentSymbol()

	pos, err := o.checkPosition(ctx, log, ex, sym, sig)
	if err != nil {
		return
	}

	// If a position exists and is not of unknown origin, we sync the strategy state.
	if pos.Active && !pos.UnknownOrigin {
		log.Warn(
			"syncing strategy to active state due to existing position",
			"position", pos.Quantity,
		)
		sig.SetInPosition(true, pos.EntryPrice, pos.HighestPrice)
	}

}

// signalBuy handles the logic for a buy signal, including risk checks and order placement
func (o *Orchestrator) signalBuy(
	ctx context.Context,
	log *slog.Logger,
	sig *signal_generator.SignalGenerator,
	price float64,
) {
	ex := sig.Exchange()
	sym := sig.InstrumentSymbol()

	// Check if there's a pending order in the database to avoid duplication.
	err := o.checkPendingOrder(ctx, log, ex, sym)
	if err != nil {
		_ = sig.RetrySignal(strategy.SignalBuy)
		return
	}

	pos, err := o.checkPosition(ctx, log, ex, sym, sig)
	if err != nil {
		_ = sig.RetrySignal(strategy.SignalBuy)
		return
	}
	if pos.Active {
		if !pos.UnknownOrigin {
			log.Warn("position active already found, confirming signal")
			sig.SetInPosition(true, pos.EntryPrice, pos.HighestPrice)
		}
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
	eval := o.risk.EvaluateEntry(openCount, price, availableBudget, sig.Risk())
	if !eval.Allowed {
		log.Warn("buy rejected by risk manager (pre-check)", "reason", eval.Reason)
		_ = sig.RetrySignal(strategy.SignalBuy)
		return
	}

	// Check open orders locally and balance from exchange to avoid duplication.
	types := []string{repository.OrderTypeMarket}
	sides := []string{repository.OrderSideBuy}
	openOrders, err := o.getDbOpenOrders(ctx, log, ex, sym, types, sides)
	if err != nil {
		_ = sig.RetrySignal(strategy.SignalBuy)
		return
	}
	if len(openOrders) > 0 {
		log.Warn("buy skipped: open buy order already exists, proceeding to avoid duplication")
		return
	}

	balance, err := o.getBalance(ctx, log, ex, sym)
	if err != nil {
		_ = sig.RetrySignal(strategy.SignalBuy)
		return
	}
	if !utils.IsZeroEps(balance.Total) {
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
	eval = o.risk.EvaluateEntry(openCount, price, availableBudget, sig.Risk())

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

		log.Info("Buy order filled, position created", "qty", order.Filled, "price", fillPrice)
		sig.SetInPosition(true, fillPrice, fillPrice)
	}
}

// signalWaitingBuyFill handles the logic for a waiting buy fill signal, checking for order completion
func (o *Orchestrator) signalWaitingBuyFill(
	ctx context.Context,
	log *slog.Logger,
	sig *signal_generator.SignalGenerator,
	price float64,
) {
	ex := sig.Exchange()
	sym := sig.InstrumentSymbol()

	err := o.checkPendingOrder(ctx, log, ex, sym)
	if err != nil {
		return
	}

	// Check if we already have a position in the portfolio.
	pos, err := o.checkPosition(ctx, log, ex, sym, sig)
	if err != nil {
		return
	}
	if pos.Active {
		if !pos.UnknownOrigin {
			if price > pos.HighestPrice {
				log.Debug("updating highest price for trailing stop", "old", pos.HighestPrice, "new", price)
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
		return
	}

	// No active position found locally, check open orders locally and balance from exchange.
	types := []string{repository.OrderTypeMarket}
	sides := []string{repository.OrderSideBuy}
	openOrders, err := o.getDbOpenOrders(ctx, log, ex, sym, types, sides)
	if err != nil {
		return
	}
	if len(openOrders) > 0 {
		log.Info("buy order still not filled, waiting for next cycle")
		return
	}

	balance, err := o.getBalance(ctx, log, ex, sym)
	if err != nil {
		return
	}
	if !utils.IsZeroEps(balance.Total) {
		log.Warn("positive balance detected but no local position, waiting for reconciliation")
		return
	}

	log.Warn("no open buy orders or balance found, retrying buy signal")
	_ = sig.RetrySignal(strategy.SignalBuy)
}

// signalTrackingSellExit handles the logic for a tracking sell exit signal, managing stop loss placement
func (o *Orchestrator) signalTrackingSellExit(
	ctx context.Context,
	log *slog.Logger,
	sig *signal_generator.SignalGenerator,
	price float64,
) {
	ex := sig.Exchange()
	sym := sig.InstrumentSymbol()

	pos, err := o.checkPosition(ctx, log, ex, sym, sig)
	if err != nil {
		return
	}
	if !pos.Active || pos.UnknownOrigin {
		if !pos.Active {
			log.Warn("position removed externally or missing, resetting strategy state")
			sig.SetInPosition(false, 0, 0)
		}
		return
	}

	stopLossMissing := !pos.StopLossActive
	if stopLossMissing || price > pos.HighestPrice {
		// If stop loss is missing, verify if a stop loss is already placed on the exchange.
		if stopLossMissing {
			stopOrder, err := o.getStopLossOrder(ctx, log, ex, sym)
			if err != nil {
				return
			}

			if stopOrder.ID == 0 {
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
			log.Debug("updating highest price for trailing stop", "old", pos.HighestPrice, "new", price)
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

// signalSell handles the logic for a sell signal, including order placement and position closure
func (o *Orchestrator) signalSell(
	ctx context.Context,
	log *slog.Logger,
	sig *signal_generator.SignalGenerator,
	price float64,
) {
	ex := sig.Exchange()
	sym := sig.InstrumentSymbol()

	// Check if there's a pending order in the database to avoid duplication.
	err := o.checkPendingOrder(ctx, log, ex, sym)
	if err != nil {
		_ = sig.RetrySignal(strategy.SignalSell)
		return
	}

	pos, err := o.checkPosition(ctx, log, ex, sym, sig)
	if err != nil {
		_ = sig.RetrySignal(strategy.SignalSell)
		return
	}
	if !pos.Active || pos.UnknownOrigin {
		if !pos.Active {
			log.Warn("position already removed externally, resetting strategy state")
			sig.SetInPosition(false, 0, 0)
		}
		return
	}

	// Check if we have a market sell order already in flight to avoid duplication.
	types := []string{repository.OrderTypeMarket}
	sides := []string{repository.OrderSideSell}
	openOrders, err := o.getDbOpenOrders(ctx, log, ex, sym, types, sides)
	if err != nil {
		_ = sig.RetrySignal(strategy.SignalSell)
		return
	}
	if len(openOrders) > 0 {
		log.Warn("market sell order already exists, proceeding to avoid duplication")
		return
	}

	// Request balance from exchange, if gone, delete position and set strategy to IDLE.
	balance, err := o.getBalance(ctx, log, ex, sym)
	if err != nil {
		_ = sig.RetrySignal(strategy.SignalSell)
		return
	}
	if utils.IsZeroEps(balance.Total) {
		log.Info("exchange balance is zero, closing local position and setting strategy to idle")
		_ = o.portfolio.DeletePosition(ctx, ex, sym)
		sig.SetInPosition(false, 0, 0)
		return
	}

	stopOrder, err := o.getStopLossOrder(ctx, log, ex, sym)
	if err != nil {
		return
	}
	if stopOrder.ID != 0 {
		// If we have a stop order placed and the sell signal was triggered by a stop loss, we use the stop order.
		isStopLoss := price <= pos.EntryPrice
		if isStopLoss {
			log.Info("stop loss order already exists on exchange, waiting for fill")
			return
		}

		// Profit Take triggered, cancel existing SL order first to free balance.
		log.Info(
			"profit take triggered, canceling existing stop loss order",
			"order_id", stopOrder.ExchangeOrderID.String,
		)
		if err := o.exec.CancelOrder(ctx, ex, sym, stopOrder.ExchangeOrderID.String); err != nil {
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
		return // Let reconciler or next cycle handle recovery
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

// signalWaitingSellFill handles the logic for a waiting sell fill signal, checking for order completion
func (o *Orchestrator) signalWaitingSellFill(
	ctx context.Context,
	log *slog.Logger,
	sig *signal_generator.SignalGenerator,
) {
	ex := sig.Exchange()
	sym := sig.InstrumentSymbol()

	// Check if there's a pending order in the database to avoid duplication.
	err := o.checkPendingOrder(ctx, log, ex, sym)
	if err != nil {
		return
	}

	pos, err := o.checkPosition(ctx, log, ex, sym, sig)
	if err != nil {
		return
	}
	if !pos.Active || pos.UnknownOrigin {
		if !pos.Active {
			log.Warn("sell filled, position is gone, setting strategy to idle")
			sig.SetInPosition(false, 0, 0)
		}
		return
	}

	// Position still active, check market open orders locally (ignore stop loss orders).
	types := []string{repository.OrderTypeMarket}
	sides := []string{repository.OrderSideSell}
	openOrders, err := o.getDbOpenOrders(ctx, log, ex, sym, types, sides)
	if err != nil {
		return
	}
	if len(openOrders) > 0 {
		log.Info("sell order still not filled, waiting for next cycle")
		return
	}

	// Request balance from exchange, if gone, delete position and set strategy to IDLE
	balance, err := o.getBalance(ctx, log, ex, sym)
	if err != nil {
		return
	}
	if utils.IsZeroEps(balance.Total) {
		log.Info("exchange balance is zero, closing local position and setting strategy to idle")
		_ = o.portfolio.DeletePosition(ctx, ex, sym)
		sig.SetInPosition(false, 0, 0)
		return
	}

	// If there is still balance but no open market sell orders, we try to place a new sell order.
	log.Warn("no open market sell orders found but position still active, retrying sell signal")
	_ = sig.RetrySignal(strategy.SignalSell)

}

// signalInvalid handles the logic for an invalid signal, resyncing the strategy state based on portfolio position
func (o *Orchestrator) signalInvalid(
	ctx context.Context,
	log *slog.Logger,
	sig *signal_generator.SignalGenerator,
) {
	ex := sig.Exchange()
	sym := sig.InstrumentSymbol()

	log.Error("invalid signal received, resyncing the strategy state")

	pos, err := o.checkPosition(ctx, log, ex, sym, sig)
	if err != nil {
		return
	}
	if !pos.Active || pos.UnknownOrigin {
		sig.SetInPosition(false, 0, 0)
		return
	}

	log.Info("Set strategy state in position", "entry", pos.EntryPrice, "high", pos.HighestPrice)
	sig.SetInPosition(true, pos.EntryPrice, pos.HighestPrice)
}
