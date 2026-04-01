package main

import (
	"context"
	"log/slog"

	"trading/robot/go-bot/internal/background"
	"trading/robot/go-bot/internal/components/execution"
	"trading/robot/go-bot/internal/components/health"
	"trading/robot/go-bot/internal/config"
)

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
