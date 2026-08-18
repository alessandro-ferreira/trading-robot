//go:build unit

package jobs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"trading/robot/go-bot/internal/config"
	"trading/robot/go-bot/internal/database/repository"
)

type mockMarketDataRepo struct {
	mock.Mock
}

func (m *mockMarketDataRepo) GetMarketDataTicks(ctx context.Context, db repository.DBExecutor, exchangeName, symbol string, since int64) ([]repository.MarketDataTick, error) {
	args := m.Called(ctx, db, exchangeName, symbol, since)
	return args.Get(0).([]repository.MarketDataTick), args.Error(1)
}

func (m *mockMarketDataRepo) InsertTick(ctx context.Context, db repository.DBExecutor, tick repository.MarketDataTick) error {
	return m.Called(ctx, db, tick).Error(0)
}

func (m *mockMarketDataRepo) DeleteMarketDataTicks(ctx context.Context, db repository.DBExecutor, retentionDays int) error {
	return m.Called(ctx, db, retentionDays).Error(0)
}

func TestMarketDataCleanupJob_Execute(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("Disabled job skips execution", func(t *testing.T) {
		mockRepo := new(mockMarketDataRepo)
		repoContainer := &repository.Container{MarketData: mockRepo}

		cfg := config.MarketDataCleanupConfig{
			Enabled:       false,
			RetentionDays: 7,
		}

		job := NewMarketDataCleanupJob(logger, nil, repoContainer, cfg)
		err := job.Execute(context.Background())

		require.NoError(t, err)
		mockRepo.AssertNotCalled(t, "DeleteMarketDataTicks", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("Enabled job executes successfully", func(t *testing.T) {
		mockRepo := new(mockMarketDataRepo)
		mockRepo.On("DeleteMarketDataTicks", mock.Anything, mock.Anything, 7).Return(nil).Once()
		repoContainer := &repository.Container{MarketData: mockRepo}

		cfg := config.MarketDataCleanupConfig{
			Enabled:       true,
			RetentionDays: 7,
		}

		job := NewMarketDataCleanupJob(logger, nil, repoContainer, cfg)
		err := job.Execute(context.Background())

		require.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("Enabled job returns error on repository failure", func(t *testing.T) {
		mockRepo := new(mockMarketDataRepo)
		mockRepo.On("DeleteMarketDataTicks", mock.Anything, mock.Anything, 14).Return(errors.New("db error")).Once()
		repoContainer := &repository.Container{MarketData: mockRepo}

		cfg := config.MarketDataCleanupConfig{
			Enabled:       true,
			RetentionDays: 14,
		}

		job := NewMarketDataCleanupJob(logger, nil, repoContainer, cfg)
		err := job.Execute(context.Background())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "market data cleanup: db error")
		mockRepo.AssertExpectations(t)
	})
}

func TestMarketDataCleanupJob_AsTask(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockRepo := new(mockMarketDataRepo)
	repoContainer := &repository.Container{MarketData: mockRepo}

	cfg := config.MarketDataCleanupConfig{
		Enabled:      true,
		Schedule:     "0 0 3 * * *",
		RunOnStartup: false,
	}

	job := NewMarketDataCleanupJob(logger, nil, repoContainer, cfg)
	task := job.AsTask()

	require.NotNil(t, task)
	assert.Equal(t, "market-data-cleanup", task.Name())
}
