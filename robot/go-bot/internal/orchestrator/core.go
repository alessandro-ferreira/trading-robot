package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"trading/robot/go-bot/internal/components/signal_generator"
	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/strategy"
)

// Perform initial strategy warm-up and state hydration for all active strategy pairs.
func (o *Orchestrator) strategyWarmup(ctx context.Context) error {
	// Load enabled strategy pairs from database
	pairs, err := o.repo.Strategies.GetEnabledStrategyPairs(ctx, o.db)
	if err != nil {
		return fmt.Errorf("failed to load strategy pairs: %w", err)
	}

	// Initialize signal generators for each enabled strategy pair in parallel
	var initWg sync.WaitGroup
	for _, p := range pairs {
		initWg.Add(1)
		go func(pair repository.StrategyPair) {
			defer initWg.Done()
			if err := o.initSignalGenerator(ctx, pair); err != nil {
				o.logger.Error("Failed to initialize strategy pair",
					"exchange", pair.ExchangeName, "symbol", pair.InstrumentSymbol, "error", err)
			}

		}(p)
	}
	initWg.Wait()

	// Hydrate Portfolio state from DB (open positions, balances)
	if err := o.portfolio.LoadState(ctx); err != nil {
		return fmt.Errorf("failed to hydrate portfolio: %w", err)
	}

	for _, sig := range o.signals {
		if pos, exists := o.portfolio.GetPosition(sig.Exchange(), sig.Symbol()); exists {
			o.logger.Info("Hydrating strategy engine state",
				"symbol", sig.Symbol(),
				"state", pos.StrategyState,
				"entry_price", pos.EntryPrice,
				"highest_price", pos.HighestPrice)

			// Align engine state with existing Portfolio position for recovery
			if err := sig.SyncState(pos.StrategyState, pos.EntryPrice, pos.HighestPrice); err != nil {
				o.logger.Error("Failed to hydrate strategy state", "symbol", sig.Symbol(), "error", err)
			}
		}
	}

	return nil
}

// Initialize a SignalGenerator for a given strategy pair, performing warm-up with historical data and loading risk config.
func (o *Orchestrator) initSignalGenerator(ctx context.Context, p repository.StrategyPair) error {
	o.logger.Debug("Initializing strategy pair", "exchange", p.ExchangeName, "symbol", p.InstrumentSymbol)

	// Fetch historical ticks for warm-up and risk config for the pair
	ticks, err := o.repo.MarketData.GetMarketDataTicks(ctx, o.db, p.ExchangeName, p.InstrumentSymbol, p.WarmupWindow)
	if err != nil {
		return fmt.Errorf("warm-up data fetch failed: %w", err)
	}

	riskData, err := o.repo.Risks.GetRiskPair(ctx, o.db, p.ExchangeName, p.InstrumentSymbol)
	if err != nil {
		return fmt.Errorf("risk config fetch failed: %w", err)
	}

	// Create the SignalGenerator instance with the fetched data
	sigGen, err := signal_generator.NewSignalGenerator(o.logger, riskData, p)
	if err != nil {
		return fmt.Errorf("failed to create signal generator: %w", err)
	}

	// Perform warm-up using fetched ticks
	if len(ticks) > 0 {
		if err := sigGen.Warmup(ticks); err != nil {
			o.logger.Error("Warm-up failed", "symbol", p.InstrumentSymbol, "error", err)
		}
	}

	// Store the initialized SignalGenerator in the orchestrator's map with thread safety
	o.mu.Lock()
	o.signals[sigGen.Name()] = sigGen
	o.mu.Unlock()

	o.logger.Info("Initialized SignalGenerator for pair", "exchange", p.ExchangeName, "symbol", p.InstrumentSymbol)
	return nil
}

// Process signals for a given pair: fetch market data, update strategy, evaluate signals, and handle orders.
func (o *Orchestrator) processSignal(ctx context.Context, sig *signal_generator.SignalGenerator) {
	o.logger.Debug("Processing signal for pair", "exchange", sig.Exchange(), "symbol", sig.Symbol())
	exchange := sig.Exchange()
	symbol := sig.Symbol()

	// Fetch latest market data
	ticker, err := o.exec.GetTicker(ctx, exchange, symbol)
	if err != nil {
		o.logger.Error("Failed to fetch ticker", "symbol", symbol, "error", err)
		return
	}
	price := ticker.Price

	// Update Portfolio price for accurate valuation and risk sizing
	o.portfolio.UpdatePrice(ctx, exchange, symbol, price)

	// Update strategy state and evaluate signal
	signal, err := sig.UpdatePrice(price, time.Now().Unix())
	if err != nil {
		o.logger.Error("Strategy update failed", "symbol", symbol, "error", err)
		return
	}
	o.logger.Debug("Received signal", "exchange", exchange, "symbol", symbol, "signal", signal.String())

	// JIT Sync: verify balance and open orders with Exchange and update them in the DB before making any decisions.
	var openOrdersCount int
	if signal != strategy.SignalSearchingEntry && signal != strategy.SignalInvalid {
		syncBalance := (signal == strategy.SignalBuy || signal == strategy.SignalSell)
		if syncBalance {
			if _, err := o.exec.GetBalance(ctx, exchange, ""); err == nil {
				o.logger.Debug("Synced all balances for exchange", "exchange", exchange)
			}
		}

		if resp, err := o.exec.GetOpenOrders(ctx, exchange, symbol); err == nil {
			openOrdersCount = len(resp.Orders)
		}

		// Refresh Portfolio state from DB
		_ = o.portfolio.RefreshState(ctx, exchange, symbol, syncBalance, true)
	}

	// Fetch latest position state from memory for decision handling
	pos, exists := o.portfolio.GetPosition(exchange, symbol)

	// Decision and Risk Handling
	switch signal {
	case strategy.SignalSearchingEntry:
		// RECONCILIATION: If we have a position but the engine is idling, force it to ACTIVE.
		if exists {
			o.logger.Info("Reconciliation: Position found while searching, syncing engine to ACTIVE state", "symbol", symbol)
			if err := sig.SyncState(strategy.StateActive, pos.EntryPrice, pos.HighestPrice); err != nil {
				o.logger.Error("Reconciliation sync failed", "symbol", symbol, "error", err)
			}
		}

	case strategy.SignalBuy:
		// RECONCILIATION: If strategy core says BUY but Portfolio says we are already in,
		// confirm the signal so the strategy starts tracking the existing position.
		if exists {
			o.logger.Info("Reconciliation: BUY signal while position already exists, syncing to ACTIVE state", "symbol", symbol)
			if err := sig.SyncState(strategy.StateActive, pos.EntryPrice, pos.HighestPrice); err != nil {
				o.logger.Error("Reconciliation sync failed", "symbol", symbol, "error", err)
			}
			return
		}

		// Pending Order Protection: check for existing "open" orders to prevent double-entry
		if openOrdersCount > 0 {
			o.logger.Debug("Skipping BUY signal: open orders already exist, strategy will wait", "symbol", symbol)
			return
		}

		openCount := o.portfolio.GetOpenPositionsCount()
		// TODO: Daily loss tracking will be implemented in a future phase
		eval := o.risk.EvaluateEntry(openCount, 0, price, sig.Risk())
		if !eval.Allowed {
			o.logger.Warn("Risk check rejected entry", "symbol", symbol, "reason", eval.Reason)
			// RECOVERY: If risk check fails, revert to IDLE so the strategy can try again on the next tick.
			sig.Reset()
			return
		}

		o.logger.Info("Placing BUY order", "symbol", symbol, "qty", eval.ApprovedSize, "price", price)
		order, err := o.exec.CreateOrder(ctx, exchange, symbol, "buy", "limit", eval.ApprovedSize, price)
		if err != nil {
			o.logger.Error("Failed to place BUY order", "symbol", symbol, "error", err)
			// RECOVERY: If order placement failed, revert to IDLE so the strategy can try again on the next tick.
			sig.Cancel(strategy.SignalBuy)
			return
		}

		// If the order was filled immediately (e.g. Market order or filled Limit), sync state
		if order != nil && order.Status == "closed" {
			_ = o.portfolio.ApplyExecution(ctx, exchange, order)
			sig.Confirm(strategy.SignalBuy, order.Price)
		}

	case strategy.SignalWaitingBuyFill:
		if exists {
			// Standard Late Fill: Order filled while we were waiting.
			o.logger.Info("Reconciliation: Late fill detected, confirming BUY", "symbol", symbol)
			sig.Confirm(strategy.SignalBuy, pos.EntryPrice)
		} else {
			// Check if the order still exists on the exchange.
			if openOrdersCount == 0 {
				// RECOVERY: If the order is gone but we have no position, the buy likely failed or was canceled externally,
				// so we reset the strategy to IDLE.
				o.logger.Warn("Reconciliation: Pending BUY but no order found, unlocking strategy", "symbol", symbol)
				sig.Cancel(strategy.SignalBuy)
			}
		}

	case strategy.SignalSell:
		if !exists || pos.Quantity <= 0 {
			// RECONCILIATION: Strategy core thinks we are in a position, but Portfolio says no.
			// This happens if a Stop Loss triggered on the exchange. We must reset the engine state.
			o.logger.Info("Reconciliation: SELL signal but no position found, resetting engine to IDLE state", "symbol", symbol)
			sig.Reset()
			return
		}

		// Pending Order Protection: prevent sending another SELL if one is already open
		if openOrdersCount > 0 {
			o.logger.Debug("Skipping SELL signal: open orders already exist, strategy will wait", "symbol", symbol)
			return
		}

		o.logger.Info("Placing SELL order", "symbol", symbol, "qty", pos.Quantity, "price", price)
		order, err := o.exec.CreateOrder(ctx, exchange, symbol, "sell", "limit", pos.Quantity, price)
		if err != nil {
			o.logger.Error("Failed to place SELL order", "symbol", symbol, "error", err)
			// RECOVERY: If order placement failed, revert to ACTIVE so the exit rule can trigger again on the next tick.
			sig.Cancel(strategy.SignalSell)
			return
		}

		// If the order was filled immediately, update portfolio/positions
		if order != nil && order.Status == "closed" {
			_ = o.portfolio.ApplyExecution(ctx, exchange, order)
			sig.Confirm(strategy.SignalSell, order.Price)
		}

	case strategy.SignalWaitingSellFill:
		if !exists {
			// Standard Late Fill: Sell order cleared while we were waiting.
			o.logger.Info("Reconciliation: Late fill detected, confirming SELL", "symbol", symbol)
			sig.Confirm(strategy.SignalSell, price)
		} else {
			// Check if the sell order still exists.
			if openOrdersCount == 0 {
				// RECOVERY: If the position still exists but no sell order found, we should revert to ACTIVE.
				o.logger.Warn("Reconciliation: Pending SELL but no order found, reverting to tracking", "symbol", symbol)
				sig.Cancel(strategy.SignalSell)
			}
		}

	case strategy.SignalTrackingExit:
		// RECONCILIATION: If the position is gone (External Stop Loss/Manual Sale) while strategy is tracking, reset to IDLE.
		if !exists {
			o.logger.Info("Reconciliation: Position gone, syncing strategy to IDLE state", "symbol", symbol)
			sig.Reset()
		}

	case strategy.SignalInvalid:
		o.logger.Error("Received invalid signal from strategy core", "symbol", symbol)
	}

	// Sync latest engine state to portfolio for persistence ONLY if it has changed.
	currentState := sig.State()
	if exists && pos.StrategyState != currentState {
		o.portfolio.SyncMetadata(ctx, exchange, symbol, currentState)
	}
}
