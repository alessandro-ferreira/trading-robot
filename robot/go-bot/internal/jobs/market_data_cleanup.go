package jobs

import (
	"context"
	"fmt"
	"log/slog"

	"trading/robot/go-bot/internal/background"
	"trading/robot/go-bot/internal/config"
	"trading/robot/go-bot/internal/database"
	"trading/robot/go-bot/internal/database/repository"
)

// MarketDataCleanupJob manages the pruning of historical market data ticks.
type MarketDataCleanupJob struct {
	logger *slog.Logger
	db     *database.DB
	repo   *repository.Container
	cfg    config.MarketDataCleanupConfig
}

// NewMarketDataCleanupJob creates a new MarketDataCleanupJob instance.
func NewMarketDataCleanupJob(
	logger *slog.Logger,
	db *database.DB,
	repo *repository.Container,
	cfg config.MarketDataCleanupConfig,
) *MarketDataCleanupJob {
	return &MarketDataCleanupJob{
		logger: logger.With("job", "market_data_cleanup"),
		db:     db,
		repo:   repo,
		cfg:    cfg,
	}
}

// Execute performs the cleanup of market data ticks older than retention_days.
func (j *MarketDataCleanupJob) Execute(ctx context.Context) error {
	if !j.cfg.Enabled {
		return nil
	}

	j.logger.Info("Starting market data cleanup", "retention_days", j.cfg.RetentionDays)

	if err := j.repo.MarketData.DeleteMarketDataTicks(ctx, j.db, j.cfg.RetentionDays); err != nil {
		j.logger.Error("Market data cleanup failed", "error", err)
		return fmt.Errorf("market data cleanup: %w", err)
	}

	j.logger.Info("Market data cleanup completed successfully", "retention_days", j.cfg.RetentionDays)
	return nil
}

// AsTask wraps the job into a background.CronTask.
func (j *MarketDataCleanupJob) AsTask() background.Task {
	return background.NewCronTask(
		"market-data-cleanup",
		j.cfg.Schedule,
		j.cfg.RunOnStartup,
		j.Execute,
	)
}
