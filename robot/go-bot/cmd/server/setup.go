package main

import (
	"context"
	"log/slog"
	"time"

	"trading/robot/go-bot/internal/background"
	"trading/robot/go-bot/internal/components/execution"
	"trading/robot/go-bot/internal/components/health"
	"trading/robot/go-bot/internal/components/portfolio"
	"trading/robot/go-bot/internal/components/reconciliation"
	"trading/robot/go-bot/internal/config"
)

// setupHealthMonitor configures a periodic health check for the specified exchanges.
func setupHealthMonitor(cfg *config.Config, execService execution.Service, bgManager *background.Manager) {
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
		job := func(ctx context.Context) error {
			_, err := execService.GetBalance(ctx, exchange, cfg.Health.Asset)
			return err
		}
		return background.WithRetry(job, cfg.Health.RetryAttempts, cfg.Health.RetryDelay)(ctx)
	}

	healthTask := background.NewPeriodicTask("health-check", cfg.Health.Interval, true, func(ctx context.Context) error {
		return healthMonitor.CheckHealth(ctx, checkMethod)
	})
	bgManager.Add(healthTask)
}

// setupOrderSync (Order Pipe) synchronizes open orders and resolves their final fates.
// Medium priority (1m) to recover from persistence failures and handle manual cancellations.
func setupOrderSync(cfg *config.Config, recon *reconciliation.Reconciler, bgManager *background.Manager) {
	task := background.NewPeriodicTask("order-sync", 1*time.Minute, false, func(ctx context.Context) error {
		for _, ex := range cfg.Exchanges {
			if err := recon.SyncOrders(ctx, ex.Name, ""); err != nil {
				slog.Error("Order Sync: Failed to sync orders", "exchange", ex.Name, "error", err)
			}
		}
		return nil
	})
	bgManager.Add(task)
}

// setupPositionSync (Position Pipe) aligns DB positions with Exchange balances and hydrates memory maps.
// High priority (30s) to detect external Stop Losses or liquidations.
func setupPositionSync(cfg *config.Config, exec execution.Service, pf *portfolio.Portfolio, recon *reconciliation.Reconciler, bgManager *background.Manager) {
	task := background.NewPeriodicTask("position-sync", 30*time.Second, false, func(ctx context.Context) error {
		for _, ex := range cfg.Exchanges {
			if _, err := exec.GetBalance(ctx, ex.Name, ""); err != nil {
				slog.Error("Position Sync: Failed to fetch balance", "exchange", ex.Name, "error", err)
			}

			if err := recon.SyncPositions(ctx, ex.Name, ""); err != nil {
				slog.Error("Position Sync: Alignment failed", "exchange", ex.Name, "error", err)
			}
		}

		// Final step: Load the aligned database state into the portfolio memory maps.
		return pf.LoadState(ctx)
	})
	bgManager.Add(task)
}

// setupTradeAudit (Audit Pipe) fetches execution history for tax reporting and position promotion.
// Low priority (15m) as it handles historical data and edge-case recoveries.
func setupTradeAudit(cfg *config.Config, recon *reconciliation.Reconciler, bgManager *background.Manager) {
	task := background.NewPeriodicTask("trade-audit", 15*time.Minute, false, func(ctx context.Context) error {
		for _, ex := range cfg.Exchanges {
			if err := recon.SyncTradeHistory(ctx, ex.Name, ""); err != nil {
				slog.Error("Trade Audit: Failed to sync history", "exchange", ex.Name, "error", err)
			}
		}
		return nil
	})
	bgManager.Add(task)
}
