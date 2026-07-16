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

// setupTradeAudit (Audit Pipe) fetches execution history for tax reporting and position promotion.
// Low priority (15m) as it handles historical data and untracked trades.
func setupTradeAudit(cfg *config.Config, recon reconcil.Reconciler, bgManager *background.Manager) {
	const initialAuditLookback = 24 * time.Hour
	const periodicAuditLookback = 1 * time.Hour

	// Initial Audit: Perform a deep sync on startup to capture trades that happened while the bot was offline.
	// This runs in background goroutines with retries to ensure completion without blocking startup.
	for _, ex := range cfg.Exchanges {
		exName := ex.Name
		go func() {
			job := func(ctx context.Context) error {
				return recon.SyncTradeHistory(ctx, exName, "", initialAuditLookback)
			}
			if err := background.WithRetry(job, 3, 10*time.Second)(context.Background()); err != nil {
				slog.Error(
					"Initial Trade Audit: Failed after retries",
					"exchange", exName, "error", err,
				)
			}
		}()
	}

	task := background.NewPeriodicTask(
		"trade-audit", 15*time.Minute, false,
		func(ctx context.Context) error {
			for _, ex := range cfg.Exchanges {
				slog.Info("Trade Audit: Starting periodic sync for exchange", "exchange", ex.Name)
				if err := recon.SyncTradeHistory(ctx, ex.Name, "", periodicAuditLookback); err != nil {
					slog.Error(
						"Trade Audit: Periodic sync failed",
						"exchange", ex.Name, "error", err,
					)
				}
			}
			return nil
		},
	)
	bgManager.Add(task)
}
