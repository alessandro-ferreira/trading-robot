package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"trading/robot/go-bot/internal/components/execution"
	"trading/robot/go-bot/internal/components/portfolio"
	reconcil "trading/robot/go-bot/internal/components/reconciliation"
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
	cfg       *config.Config
	risk      *risk.Manager
	portfolio portfolio.Portfolio
	recon     reconcil.Reconciler
	exec      execution.Service
	mu        sync.Mutex
	signals   map[string]*signal_generator.SignalGenerator
}

// New creates a new Orchestrator instance.
func New(
	logger *slog.Logger,
	db *database.DB,
	repo *repository.Container,
	cfg *config.Config,
	pf portfolio.Portfolio,
	recon reconcil.Reconciler,
	exec execution.Service,
) (*Orchestrator, error) {
	// Initialize internal logic components
	riskMgr := risk.NewManager(logger, cfg.Risk)
	signals := make(map[string]*signal_generator.SignalGenerator)

	if cfg.Server.OrchestratorInterval <= 0 || cfg.Server.RefreshStratInterval <= 0 {
		return nil, fmt.Errorf(
			"invalid configuration: orchestrator_interval and refresh_strat_interval must be greater than 0",
		)
	}

	return &Orchestrator{
		logger:    logger,
		db:        db,
		repo:      repo,
		cfg:       cfg,
		portfolio: pf,
		risk:      riskMgr,
		recon:     recon,
		exec:      exec,
		signals:   signals,
	}, nil
}

// ----------------------------------------------------------------------------
// Lifecycle Methods (Start / Close)
// ----------------------------------------------------------------------------

// Start runs the main orchestration loop until the context is cancelled.
func (o *Orchestrator) Start(ctx context.Context) error {
	// Load portfolio state into memory
	if err := o.portfolio.LoadState(ctx); err != nil {
		return fmt.Errorf("load portfolio state failed %w", err)
	}

	var wg sync.WaitGroup
	// The strategy reloader runs on a deterministic interval (e.g., 1 minute)
	// to pick up new pairs enabled via the Management API/ML Engine.
	refreshTicker := time.NewTicker(o.cfg.Server.RefreshStratInterval)
	defer refreshTicker.Stop()

	// Perform initial load of strategies
	o.refreshStrategies(ctx, &wg)

	o.logger.Info("Orchestrator active", "loop_interval", o.cfg.Server.OrchestratorInterval)

	for {
		select {
		case <-ctx.Done():
			o.logger.Info("Orchestrator shutting down, waiting for pair loops...")
			wg.Wait()
			return nil
		case <-refreshTicker.C:
			o.refreshStrategies(ctx, &wg)
		}
	}
}

// Close cleans up all internal components managed by the Orchestrator.
func (o *Orchestrator) Close() error {
	o.logger.Info("Closing Orchestrator components...")
	o.mu.Lock()
	defer o.mu.Unlock()

	for _, sig := range o.signals {
		if err := sig.Close(); err != nil {
			o.logger.Error("Failed to close signal generator", "strategy_name", sig.Name(), "error", err)
		}
	}
	return nil
}

// ----------------------------------------------------------------------------
// Strategy Management
// ----------------------------------------------------------------------------

// refreshStrategies orchestrates the discovery, execution and reconciliation of strategy configurations.
func (o *Orchestrator) refreshStrategies(ctx context.Context, wg *sync.WaitGroup) {
	o.logger.Info("Refreshing strategy configurations from database")

	strategies, err := o.loadValidStrategies(ctx)
	if err != nil {
		o.logger.Error("Refresh: failed to load valid strategies", "error", err)
		return
	}

	o.orchestrateStrategies(ctx, strategies, wg)
}

// loadValidStrategies fetches the current intended state from the database.
func (o *Orchestrator) loadValidStrategies(ctx context.Context) ([]repository.StrategyPair, error) {
	statuses := []string{
		repository.StrategyEnabled,
		repository.StrategyPendingDisabled,
	}
	return o.repo.Strategies.GetStrategyPairs(ctx, o.db, statuses)
}

// orchestrateStrategies ensures required strategy workers are running and up to date.
func (o *Orchestrator) orchestrateStrategies(
	ctx context.Context,
	strategies []repository.StrategyPair,
	wg *sync.WaitGroup,
) {
	o.logger.Info("Orchestrator starting strategies", "active_pairs", len(strategies))

	for _, pair := range strategies {
		name := fmt.Sprintf("SignalGenerator-%s-%s", pair.ExchangeName, pair.InstrumentSymbol)

		o.mu.Lock()
		sig, exists := o.signals[name]
		o.mu.Unlock()

		if !exists {
			o.startWorker(ctx, pair, name, wg)
		} else {
			o.updateWorker(pair, sig, name)
		}
	}
}

// ----------------------------------------------------------------------------
// Worker Methods
// ----------------------------------------------------------------------------

// startWorker initializes a new signal generator and spawns its trading loop.
func (o *Orchestrator) startWorker(
	ctx context.Context,
	pair repository.StrategyPair,
	name string,
	wg *sync.WaitGroup,
) {
	wg.Add(1)
	go func(p repository.StrategyPair, n string) {
		defer wg.Done()

		// Ensure a panic in this specific pair's worker does not crash the entire orchestrator.
		defer func() {
			if r := recover(); r != nil {
				o.logger.Error("Worker panic recovered",
					"exchange", p.ExchangeName, "symbol", p.InstrumentSymbol, "error", r)

				// Remove from active map and release resources (C++ engine) so the reloader can restart it.
				o.stopWorker(n)
			}
		}()

		// Initialize the handler (warmup history, sync position metadata, add to signals map)
		sig, err := o.initSignalHandler(ctx, p, n)
		if err != nil {
			o.logger.Error(
				"Lifecycle: failed to initialize signal handler",
				"ex", p.ExchangeName, "sym", p.InstrumentSymbol, "error", err,
			)
			return
		}

		o.runWorker(ctx, sig)
	}(pair, name)
}

// updateWorker pushes configuration changes to an already running signal generator.
func (o *Orchestrator) updateWorker(
	pair repository.StrategyPair,
	sig *signal_generator.SignalGenerator,
	name string,
) {
	// Apply updates from the ML engine to running strategies.
	o.logger.Info("Refreshing configuration for active strategy", "pair", name)

	if err := sig.UpdateConfigFromPair(pair); err != nil {
		o.logger.Error("Lifecycle: failed to update strategy config", "pair", name, "error", err)
	}
}

// runWorker manages the infinite trading loop for a single signal generator.
func (o *Orchestrator) runWorker(ctx context.Context, sig *signal_generator.SignalGenerator) {
	o.logger.Info("Starting worker loop", "pair", sig.Name(), "interval", o.cfg.Server.OrchestratorInterval)

	ticker := time.NewTicker(o.cfg.Server.OrchestratorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Verify if the worker is still in the active set (it might have been stopped by termination logic).
			o.mu.Lock()
			_, exists := o.signals[sig.Name()]
			o.mu.Unlock()
			if !exists {
				return
			}

			o.processSignal(ctx, sig)
		}
	}
}

// stopWorker removes a signal generator from the map and releases its resources.
func (o *Orchestrator) stopWorker(name string) {
	o.mu.Lock()
	sig, exists := o.signals[name]
	if exists {
		delete(o.signals, name)
	}
	o.mu.Unlock()

	if exists {
		o.logger.Info("Stopping strategy worker loop", "pair", name)
		_ = sig.Close()
	}
}
