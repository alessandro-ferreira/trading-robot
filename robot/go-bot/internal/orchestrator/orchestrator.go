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

	// Decision and Risk Handling
	switch signal {
	case strategy.SignalBuy:
		// Safety check: skip if we already have an open position for this pair
		if _, exists := o.portfolio.GetPosition(exchange, symbol); exists {
			return
		}

		openCount := o.portfolio.GetOpenPositionsCount()
		// TODO: Daily loss tracking will be implemented in a future phase
		eval := o.risk.EvaluateEntry(openCount, 0, price, sig.Risk())
		if !eval.Allowed {
			o.logger.Warn("Risk check rejected entry", "symbol", symbol, "reason", eval.Reason)
			return
		}

		o.logger.Info("Placing BUY order", "symbol", symbol, "qty", eval.ApprovedSize, "price", price)
		_, _ = o.exec.CreateOrder(ctx, symbol, "buy", "limit", eval.ApprovedSize, price, exchange)

	case strategy.SignalSell:
		// Safety check: only sell if we have an open position to avoid unnecessary orders
		pos, exists := o.portfolio.GetPosition(exchange, symbol)
		if !exists || pos.Quantity <= 0 {
			return
		}

		o.logger.Info("Placing SELL order", "symbol", symbol, "qty", pos.Quantity, "price", price)
		_, _ = o.exec.CreateOrder(ctx, symbol, "sell", "limit", pos.Quantity, price, exchange)

	default:
		// Handle SignalHold or any unexpected values by doing nothing
		return
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
