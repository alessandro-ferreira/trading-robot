package main

import (
	"context"
	"log/slog"
	"os"

	"trading/robot/go-bot/internal/components/execution"
	"trading/robot/go-bot/internal/components/portfolio"
	reconcil "trading/robot/go-bot/internal/components/reconciliation"
	"trading/robot/go-bot/internal/config"
	"trading/robot/go-bot/internal/database"
	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/orchestrator"
	"trading/robot/go-bot/internal/simulation"
)

// runBacktest initializes and runs the backtesting simulation.
// It creates a simulated client that replays CSV prices sequentially,
// uses a simulated clock for time progression, and executes the trading strategy.
func runBacktest(ctx context.Context, cfg *config.Config) {
	logger := slog.Default()

	logger.Info("Starting backtest mode",
		slog.String("symbol", cfg.Simulation.Symbol),
		slog.String("begin", cfg.Simulation.Begin),
		slog.String("end", cfg.Simulation.End),
		slog.String("input", cfg.Simulation.Input),
		slog.String("output", cfg.Simulation.Output),
		slog.Float64("initial_usdt", cfg.Simulation.InitialUSDT),
	)

	// --- Infrastructure Initialization ---
	db, err := database.NewDBPool(ctx, cfg.Database)
	if err != nil {
		logger.Error("Failed to initialize database connection", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	repoContainer := repository.New()

	// The simulated client loads CSV price data and progresses sequentially through timestamps.
	simulatedClient, err := simulation.NewSimulatedClient(
		cfg.Simulation.Symbol,
		cfg.Simulation.Begin,
		cfg.Simulation.End,
		cfg.Simulation.Input,
		cfg.Simulation.Output,
		cfg.Simulation.InitialUSDT,
	)
	if err != nil {
		logger.Error("Failed to initialize simulated client", "error", err)
		os.Exit(1)
	}

	logger.Info("Simulated client initialized", "symbol", cfg.Simulation.Symbol)

	// The clock progresses through the CSV timestamps as the simulated client steps forward.
	clock := simulation.NewSimulatedClock(simulatedClient)

	execService := execution.NewService(logger, db, simulatedClient, repoContainer, clock)

	logger.Info("Execution service initialized for backtest")

	// Update the balance in database before starting the orchestrator.
	_, err = execService.GetBalance(ctx, simulation.ExchangeName, simulation.BudgetAsset)
	if err != nil {
		logger.Error("Failed to get balance from simulated client", "error", err)
		os.Exit(1)
	}

	pf := portfolio.NewPortfolio(logger, db, repoContainer)
	recon := reconcil.NewReconciler(logger, db, repoContainer, execService, pf)

	// --- Orchestration ---
	orch, err := orchestrator.New(
		logger, db, repoContainer, cfg, pf, recon, execService, clock,
	)
	if err != nil {
		logger.Error("Failed to initialize Orchestrator", "error", err)
		os.Exit(1)
	}
	defer orch.Close()

	logger.Info("Orchestrator initialized for backtest")

	// Unlike production, we don't spawn background tasks (monitor, sync, audit).
	// The orchestrator simply runs until context is canceled or prices are exhausted.
	go func() {
		if err := orch.Start(ctx); err != nil && err != context.Canceled {
			logger.Error("Orchestrator stopped with error", "error", err)
		}
	}()

	// --- Block until shutdown signal ---
	<-ctx.Done()

	logger.Info("Backtest shutdown signal received, exiting")
}
