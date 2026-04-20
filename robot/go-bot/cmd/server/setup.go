package main

import (
	"context"
	"log/slog"
	"time"

	"trading/robot/go-bot/internal/background"
	"trading/robot/go-bot/internal/components/execution"
	"trading/robot/go-bot/internal/components/health"
	"trading/robot/go-bot/internal/components/portfolio"
	"trading/robot/go-bot/internal/config"
)

// setupHealthMonitor configures a periodic health check for the specified exchanges.
func setupHealthMonitor(cfg *config.Config, execService *execution.Service, bgManager *background.Manager) {
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

// setupReconciliation configures a background task to refresh the portfolio state from the database.
func setupReconciliation(cfg *config.Config, exec *execution.Service, pf *portfolio.Portfolio, bgManager *background.Manager) {
	reconTask := background.NewPeriodicTask("portfolio-reconciliation", 1*time.Minute, false, func(ctx context.Context) error {
		// Refresh balances and open orders for all configured exchanges.
		for _, ex := range cfg.Exchanges {
			if _, err := exec.GetBalance(ctx, ex.Name, ""); err != nil {
				slog.Warn("Reconciliation: Failed to sync balance from exchange", "exchange", ex.Name, "error", err)
			}
			if _, err := exec.GetOpenOrders(ctx, ex.Name, ""); err != nil {
				slog.Warn("Reconciliation: Failed to sync open orders from exchange", "exchange", ex.Name, "error", err)
			}
		}

		// Reload internal state from updated database
		return pf.LoadState(ctx)
	})
	bgManager.Add(reconTask)
}
