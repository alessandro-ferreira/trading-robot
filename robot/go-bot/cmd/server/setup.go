package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"time"

	"trading/robot/go-bot/internal/background"
	"trading/robot/go-bot/internal/components/execution"
	"trading/robot/go-bot/internal/components/health"
	"trading/robot/go-bot/internal/components/signal_generator"
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

func setupSignalGenerators(cfg *config.Config, gatewayClient *execution.GatewayClient, bgManager *background.Manager) []io.Closer {
	var closers []io.Closer
	if len(cfg.Exchanges) > 0 {
		slog.Info("Initializing Signal Generator", "symbol", "BTC/USD", "exchange", cfg.Exchanges[0].Name)
		sigGen, err := signal_generator.NewSignalGenerator(slog.Default(), gatewayClient, "BTC/USD", cfg.Exchanges[0].Name, cfg.Strategy)
		if err != nil {
			slog.Error("Failed to initialize signal generator", "error", err)
			os.Exit(1)
		}
		closers = append(closers, sigGen)
		bgManager.Add(background.NewPeriodicTask(sigGen.Name(), 5*time.Second, true, sigGen.Process))
	}
	return closers
}
