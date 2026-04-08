package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"trading/robot/go-bot/internal/components/execution"
	"trading/robot/go-bot/internal/components/portfolio"
	"trading/robot/go-bot/internal/components/risk"
	"trading/robot/go-bot/internal/components/signal_generator"
	"trading/robot/go-bot/internal/config"
	"trading/robot/go-bot/internal/strategy"
)

// Orchestrator coordinates the trading loop across multiple markets.
type Orchestrator struct {
	logger    *slog.Logger
	interval  time.Duration
	portfolio *portfolio.Portfolio
	risk      *risk.Manager
	exec      *execution.Service
	signals   map[string]*signal_generator.SignalGenerator
}

// New creates a new Orchestrator instance.
func New(
	logger *slog.Logger,
	cfg *config.Config,
	pf *portfolio.Portfolio,
	exec *execution.Service,
	interval time.Duration,
) (*Orchestrator, error) {
	// Initialize internal logic components
	riskMgr := risk.NewManager(logger, cfg.Risk)

	signals := make(map[string]*signal_generator.SignalGenerator)

	// Setup strategy generators for all configured pairs
	for _, pair := range cfg.Pairs {
		sigGen, err := signal_generator.NewSignalGenerator(logger, pair.Symbol, pair.Exchange, pair.Risk, pair.Strategy)
		if err != nil {
			return nil, fmt.Errorf("failed to create signal generator for %s on %s: %w", pair.Symbol, pair.Exchange, err)
		}
		signals[sigGen.Name()] = sigGen
	}

	return &Orchestrator{
		logger:    logger,
		interval:  interval,
		portfolio: pf,
		risk:      riskMgr,
		exec:      exec,
		signals:   signals,
	}, nil
}

// Start runs the main orchestration loop until the context is cancelled.
func (o *Orchestrator) Start(ctx context.Context) error {
	o.logger.Info("Starting Orchestrator loops", "interval", o.interval, "pairs", len(o.signals))

	var wg sync.WaitGroup
	for _, sig := range o.signals {
		wg.Add(1)
		go func(s *signal_generator.SignalGenerator) {
			defer wg.Done()
			o.runPairLoop(ctx, s)
		}(sig)
	}

	<-ctx.Done()
	o.logger.Info("Orchestrator shutting down, waiting for pair loops...")
	wg.Wait()
	return nil
}

func (o *Orchestrator) runPairLoop(ctx context.Context, sig *signal_generator.SignalGenerator) {
	ticker := time.NewTicker(o.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			o.processSignal(ctx, sig)
		}
	}
}

func (o *Orchestrator) processSignal(ctx context.Context, sig *signal_generator.SignalGenerator) {
	o.logger.Debug("Processing signal for pair", "exchange", sig.Exchange(), "symbol", sig.Symbol())
	exchange := sig.Exchange()
	symbol := sig.Symbol()

	// Fetch latest market data
	ticker, err := o.exec.GetTicker(ctx, symbol, exchange)
	if err != nil {
		o.logger.Error("Failed to fetch ticker", "symbol", symbol, "error", err)
		return
	}
	price := ticker.Price

	// Update Portfolio price for accurate valuation and risk sizing
	o.portfolio.UpdatePrice(exchange, symbol, price)

	// Update strategy state and evaluate signal
	signal, err := sig.UpdateAndGetSignal(price, time.Now().Unix())
	if err != nil {
		o.logger.Error("Strategy update failed", "symbol", symbol, "error", err)
		return
	}
	o.logger.Debug("Received signal", "exchange", exchange, "symbol", symbol, "signal", signal)

	// Get current position from portfolio for reconciliation
	pos, exists := o.portfolio.GetPosition(exchange, symbol)

	// Decision and Risk Handling
	switch signal {
	case strategy.SignalSearchingEntry:
		// RECONCILIATION: If we have a position but the engine is idling, force it to ACTIVE.
		// This handles bot restarts/recovery.
		if exists {
			o.logger.Info("Reconciliation: Position found while searching, syncing engine to ACTIVE state", "symbol", symbol)
			sig.Confirm(strategy.SignalBuy, pos.EntryPrice)
		}

	case strategy.SignalBuy:
		// RECONCILIATION: If strategy core says BUY but Portfolio says we are already in,
		// confirm the signal so the strategy starts tracking the existing position.
		if exists {
			o.logger.Info("Reconciliation: BUY signal while position already exists, syncing to ACTIVE state", "symbol", symbol)
			sig.Confirm(strategy.SignalBuy, pos.EntryPrice)
			return
		}

		// Pending Order Protection: check for existing "open" orders to prevent double-entry
		openOrders, err := o.exec.GetOpenOrders(ctx, symbol, exchange)
		if err != nil {
			o.logger.Error("Failed to check open orders", "symbol", symbol, "error", err)
			return
		}
		if len(openOrders.Orders) > 0 {
			o.logger.Debug("Skipping BUY signal: open orders already exist, strategy will wait", "symbol", symbol)
			// We do NOT cancel here; let the strategy stay in SignalWaitingBuyFill
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
		order, err := o.exec.CreateOrder(ctx, symbol, "buy", "limit", eval.ApprovedSize, price, exchange)
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
			openOrders, _ := o.exec.GetOpenOrders(ctx, symbol, exchange)
			if len(openOrders.Orders) == 0 {
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
		openOrders, err := o.exec.GetOpenOrders(ctx, symbol, exchange)
		if err != nil {
			o.logger.Error("Failed to check open orders", "symbol", symbol, "error", err)
			return
		}
		if len(openOrders.Orders) > 0 {
			o.logger.Debug("Skipping SELL signal: open orders already exist, strategy will wait", "symbol", symbol)
			// We do NOT cancel here; let the strategy stay in SignalWaitingSellFill
			return
		}

		o.logger.Info("Placing SELL order", "symbol", symbol, "qty", pos.Quantity, "price", price)
		order, err := o.exec.CreateOrder(ctx, symbol, "sell", "limit", pos.Quantity, price, exchange)
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
			openOrders, _ := o.exec.GetOpenOrders(ctx, symbol, exchange)
			if len(openOrders.Orders) == 0 {
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
}

// Close cleans up all internal components managed by the Orchestrator.
func (o *Orchestrator) Close() error {
	o.logger.Info("Closing Orchestrator components...")
	for _, sig := range o.signals {
		if err := sig.Close(); err != nil {
			o.logger.Error("Failed to close signal generator", "name", sig.Name(), "error", err)
		}
	}
	return nil
}
