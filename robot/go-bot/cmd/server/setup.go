package main

import (
	"context"
	"log/slog"
	"time"

	"trading/robot/go-bot/internal/background"
	"trading/robot/go-bot/internal/components/execution"
	"trading/robot/go-bot/internal/components/health"
	"trading/robot/go-bot/internal/components/portfolio"
	reconcil "trading/robot/go-bot/internal/components/reconciliation"
	"trading/robot/go-bot/internal/config"
	"trading/robot/go-bot/internal/database"
	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/jobs"
)

// setupHealthMonitor configures a periodic health check for the specified exchanges.
func setupHealthMonitor(
	cfg *config.Config, execService execution.Service, bgManager *background.Manager,
) {
	var exchangeNames []string
	for _, ex := range cfg.Exchanges {
		if ex.HealthCheck {
			exchangeNames = append(exchangeNames, ex.Name)
		}
	}
	if len(exchangeNames) == 0 {
		slog.Warn("No exchanges configured for health monitoring. Health monitor will not be started.")
		return
	}
	slog.Info("Setting up health monitor for configured exchanges", "exchanges", exchangeNames)

	healthMonitor := health.NewMonitor(slog.Default(), exchangeNames)

	checkMethod := func(ctx context.Context, exchange string) error {
		slog.Info("Health Check: Checking exchange connectivity", "exchange", exchange)
		job := func(ctx context.Context) error {
			_, err := execService.GetBalance(ctx, exchange, cfg.Health.Asset)
			return err
		}
		return background.WithRetry(job, cfg.Health.RetryAttempts, cfg.Health.RetryDelay)(ctx)
	}

	healthTask := background.NewPeriodicTask(
		"health-check", cfg.Health.Interval, true,
		func(ctx context.Context) error {
			return healthMonitor.CheckHealth(ctx, checkMethod)
		},
	)
	bgManager.Add(healthTask)
}

// setupOrderSync (Order Pipe) validates new and open orders with the exchanges.
// Very high priority (15s) to update orders from exchange without configured webhooks.
func setupOrderSync(cfg *config.Config, recon reconcil.Reconciler, bgManager *background.Manager) {
	task := background.NewPeriodicTask(
		"order-sync", 15*time.Second, false,
		func(ctx context.Context) error {
			for _, ex := range cfg.Exchanges {
				slog.Info("Order Sync: Starting sync for exchange", "exchange", ex.Name)
				if err := recon.SyncOrders(ctx, ex.Name, ""); err != nil {
					slog.Error(
						"Order Sync: Failed to sync orders",
						"exchange", ex.Name, "error", err,
					)
				}
			}
			return nil
		},
	)
	bgManager.Add(task)
}

// setupStopOrderSync validates stop orders with the exchanges and resets inactive stop losses.
// Medium priority (5m) to detect expired or cancelled stop orders and allow orchestrator re-arming.
func setupStopOrderSync(
	cfg *config.Config,
	exec execution.Service,
	pf portfolio.Portfolio,
	recon reconcil.Reconciler,
	bgManager *background.Manager,
) {
	task := background.NewPeriodicTask(
		"stop-order-sync", 5*time.Minute, false,
		func(ctx context.Context) error {
			for _, ex := range cfg.Exchanges {
				slog.Info("Stop Order Sync: Starting sync for exchange", "exchange", ex.Name)
				if _, err := exec.GetBalance(ctx, ex.Name, ""); err != nil {
					slog.Error(
						"Stop Order Sync: Failed to fetch balance",
						"exchange", ex.Name, "error", err,
					)
				}

				if err := recon.SyncStopOrders(ctx, ex.Name, ""); err != nil {
					slog.Error("Stop Order Sync: Alignment failed", "exchange", ex.Name, "error", err)
				}
			}

			// Final step: Load the aligned database state into the portfolio memory maps.
			return pf.LoadState(ctx)
		},
	)
	bgManager.Add(task)
}

// setupPositionSync (Position Pipe) aligns DB positions with Exchange balances.
// High priority (1m) to detect external Stop Losses, liquidations, ghost balances, manual and untracked trades.
func setupPositionSync(
	cfg *config.Config,
	exec execution.Service,
	pf portfolio.Portfolio,
	recon reconcil.Reconciler,
	bgManager *background.Manager,
) {
	task := background.NewPeriodicTask(
		"position-sync", 1*time.Minute, false,
		func(ctx context.Context) error {
			for _, ex := range cfg.Exchanges {
				slog.Info("Position Sync: Starting sync for exchange", "exchange", ex.Name)
				if _, err := exec.GetBalance(ctx, ex.Name, ""); err != nil {
					slog.Error(
						"Position Sync: Failed to fetch balance",
						"exchange", ex.Name, "error", err,
					)
				}

				if err := recon.SyncPositions(ctx, ex.Name, ""); err != nil {
					slog.Error("Position Sync: Alignment failed", "exchange", ex.Name, "error", err)
				}
			}

			// Final step: Load the aligned database state into the portfolio memory maps.
			return pf.LoadState(ctx)
		},
	)
	bgManager.Add(task)
}

// setupCronJobs initializes and registers scheduled cron jobs.
func setupCronJobs(
	cfg *config.Config,
	db *database.DB,
	repo *repository.Container,
	bgManager *background.Manager,
) {
	// Register the Market Data Cleanup Job if enabled in the configuration.
	if cfg.Cron.MarketDataCleanup.Enabled {
		mdCleanupJob := jobs.NewMarketDataCleanupJob(slog.Default(), db, repo, cfg.Cron.MarketDataCleanup)
		bgManager.Add(mdCleanupJob.AsTask())
		slog.Info(
			"Registered market data cleanup cron job",
			"schedule", cfg.Cron.MarketDataCleanup.Schedule,
			"retention_days", cfg.Cron.MarketDataCleanup.RetentionDays,
		)
	}
}
