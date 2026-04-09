package portfolio

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/strategy"
)

func TestPortfolio_UpdatePrice(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockRepo := &MockPositionsRepo{}
	container := &repository.Container{
		Positions: mockRepo,
	}

	t.Run("Price increase updates unrealized pnl and persists new peak", func(t *testing.T) {
		p := NewPortfolio(logger, nil, container, 1000.0)
		p.mu.Lock()
		p.positions["binance|BTC/USDT"] = &Position{
			Exchange: "binance", Symbol: "BTC/USDT", Quantity: 1.0, EntryPrice: 100.0, HighestPrice: 100.0,
		}
		p.mu.Unlock()

		persisted := false
		mockRepo.UpsertPositionFn = func(ctx context.Context, db repository.DBExecutor, pos repository.PositionData) error {
			persisted = true
			assert.Equal(t, 110.0, pos.HighestPrice)
			return nil
		}

		p.UpdatePrice(context.Background(), "binance", "BTC/USDT", 110.0)
		pos, _ := p.GetPosition("binance", "BTC/USDT")
		assert.Equal(t, 10.0, pos.UnrealizedPnL)
		assert.Equal(t, 110.0, pos.HighestPrice)
		assert.True(t, persisted)
	})

	t.Run("Price decrease updates pnl but does not update peak or persist", func(t *testing.T) {
		p := NewPortfolio(logger, nil, container, 1000.0)
		p.mu.Lock()
		p.positions["binance|BTC/USDT"] = &Position{
			Exchange: "binance", Symbol: "BTC/USDT", Quantity: 1.0, EntryPrice: 100.0, HighestPrice: 120.0,
		}
		p.mu.Unlock()

		mockRepo.UpsertPositionFn = func(ctx context.Context, db repository.DBExecutor, pos repository.PositionData) error {
			t.Fatal("UpsertPosition should not be called on price drop")
			return nil
		}

		p.UpdatePrice(context.Background(), "binance", "BTC/USDT", 110.0)
		pos, _ := p.GetPosition("binance", "BTC/USDT")
		assert.Equal(t, 10.0, pos.UnrealizedPnL)
		assert.Equal(t, 120.0, pos.HighestPrice)
	})
}

func TestPortfolio_SyncMetadata(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockRepo := &MockPositionsRepo{}
	container := &repository.Container{Positions: mockRepo}
	p := NewPortfolio(logger, nil, container, 1000.0)
	p.mu.Lock()
	p.positions["binance|BTC/USDT"] = &Position{Exchange: "binance", Symbol: "BTC/USDT"}
	p.mu.Unlock()

	t.Run("Updates memory and triggers persistence", func(t *testing.T) {
		called := false
		mockRepo.UpsertPositionFn = func(ctx context.Context, db repository.DBExecutor, pos repository.PositionData) error {
			called = true
			assert.Equal(t, "pending_sell", pos.StrategyState)
			return nil
		}
		p.SyncMetadata(context.Background(), "binance", "BTC/USDT", strategy.StatePendingSell)
		assert.True(t, called)
	})
}
