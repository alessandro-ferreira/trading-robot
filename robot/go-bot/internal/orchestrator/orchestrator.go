package orchestrator

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"trading/robot/go-bot/internal/components/execution"
	"trading/robot/go-bot/internal/components/portfolio"
	"trading/robot/go-bot/internal/components/risk"
	"trading/robot/go-bot/internal/components/signal_generator"
	"trading/robot/go-bot/internal/config"
	"trading/robot/go-bot/internal/database"
	"trading/robot/go-bot/internal/database/repository"
)

// Orchestrator coordinates the trading loop across multiple markets.
type Orchestrator struct {
	logger    *slog.Logger
	db        *database.DB
	repo      *repository.Container
	portfolio *portfolio.Portfolio
	risk      *risk.Manager
	exec      *execution.Service
	interval  time.Duration
	mu        sync.Mutex
	signals   map[string]*signal_generator.SignalGenerator
}

// New creates a new Orchestrator instance.
func New(
	logger *slog.Logger,
	db *database.DB,
	repo *repository.Container,
	cfg *config.Config,
	pf *portfolio.Portfolio,
	exec *execution.Service,
	interval time.Duration,
) (*Orchestrator, error) {
	// Initialize internal logic components
	riskMgr := risk.NewManager(logger, cfg.Risk)
	signals := make(map[string]*signal_generator.SignalGenerator)

	return &Orchestrator{
		logger:    logger,
		db:        db,
		repo:      repo,
		interval:  interval,
		portfolio: pf,
		risk:      riskMgr,
		exec:      exec,
		signals:   signals,
	}, nil
}

// Start runs the main orchestration loop until the context is cancelled.
func (o *Orchestrator) Start(ctx context.Context) error {
	// Perform initial strategy warm-up and state hydration
	if err := o.strategyWarmup(ctx); err != nil {
		return err
	}

	o.logger.Info("Starting Orchestrator loops", "interval", o.interval, "pairs", len(o.signals))

	// Start a separate loop for each trading pair/signal generator
	var wg sync.WaitGroup
	for _, sig := range o.signals {
		wg.Add(1)
		go func(s *signal_generator.SignalGenerator) {
			defer wg.Done()

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
		}(sig)
	}

	<-ctx.Done()
	o.logger.Info("Orchestrator shutting down, waiting for pair loops...")
	wg.Wait()
	return nil
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
