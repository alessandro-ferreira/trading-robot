package portfolio

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "trading/robot/go-bot/gen/go/v1"
	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/strategy"
)

func TestPortfolio_UpdatePosition(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockRepo := &MockPositionsRepo{}
	container := &repository.Container{
		Positions: mockRepo,
	}

	t.Run("Buy increases quantity and decreases cash", func(t *testing.T) {
		p := NewPortfolio(logger, nil, container, 1000.0)
		persisted := false
		mockRepo.UpsertPositionFn = func(ctx context.Context, db repository.DBExecutor, pos repository.PositionData) error {
			persisted = true
			return nil
		}

		err := p.UpdatePosition(context.Background(), "binance", "BTC/USDT", 0.1, 5000.0)
		require.NoError(t, err)
		assert.Equal(t, 500.0, p.GetCashBalance())
		assert.True(t, persisted)

		pos, _ := p.GetPosition("binance", "BTC/USDT")
		assert.Equal(t, 0.1, pos.Quantity)
		assert.Equal(t, strategy.StateActive, pos.StrategyState)
	})

	t.Run("Sell decreases quantity and increases cash", func(t *testing.T) {
		p := NewPortfolio(logger, nil, container, 0.0)
		// Setup initial position
		p.mu.Lock()
		p.positions["binance|BTC/USDT"] = &Position{
			Exchange: "binance", Symbol: "BTC/USDT", Quantity: 1.0, EntryPrice: 100.0,
		}
		p.mu.Unlock()

		mockRepo.UpsertPositionFn = func(ctx context.Context, db repository.DBExecutor, pos repository.PositionData) error {
			return nil
		}

		err := p.UpdatePosition(context.Background(), "binance", "BTC/USDT", -0.5, 120.0)
		require.NoError(t, err)
		assert.Equal(t, 60.0, p.GetCashBalance())
		pos, _ := p.GetPosition("binance", "BTC/USDT")
		assert.Equal(t, 0.5, pos.Quantity)
	})

	t.Run("Full sell removes position from memory and deletes from DB", func(t *testing.T) {
		p := NewPortfolio(logger, nil, container, 0.0)
		p.mu.Lock()
		p.positions["binance|BTC/USDT"] = &Position{
			Exchange: "binance", Symbol: "BTC/USDT", Quantity: 1.0, EntryPrice: 100.0,
		}
		p.mu.Unlock()

		deleted := false
		mockRepo.DeletePositionFn = func(ctx context.Context, db repository.DBExecutor, exchange, symbol string) error {
			deleted = true
			return nil
		}

		err := p.UpdatePosition(context.Background(), "binance", "BTC/USDT", -1.0, 110.0)
		require.NoError(t, err)
		assert.True(t, deleted)
		assert.Equal(t, 110.0, p.GetCashBalance())
		_, exists := p.GetPosition("binance", "BTC/USDT")
		assert.False(t, exists)
	})

	t.Run("Insufficient funds returns error", func(t *testing.T) {
		p := NewPortfolio(logger, nil, container, 10.0)
		err := p.UpdatePosition(context.Background(), "binance", "BTC/USDT", 1.0, 100.0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "insufficient funds")
	})
}

func TestPortfolio_ApplyExecution(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockRepo := &MockPositionsRepo{}
	container := &repository.Container{
		Positions: mockRepo,
	}

	t.Run("Skips non-closed orders", func(t *testing.T) {
		p := NewPortfolio(logger, nil, container, 1000.0)
		order := &pb.OrderResponse{Status: "open", Side: "buy", Filled: 1.0}
		err := p.ApplyExecution(context.Background(), "binance", order)
		assert.NoError(t, err)
		assert.Equal(t, 1000.0, p.GetCashBalance())
	})

	t.Run("Corrects side for buy", func(t *testing.T) {
		p := NewPortfolio(logger, nil, container, 1000.0)
		order := &pb.OrderResponse{
			Status: "closed",
			Side:   "buy",
			Filled: 0.1,
			Price:  5000.0,
			Symbol: "BTC/USDT",
		}
		err := p.ApplyExecution(context.Background(), "binance", order)
		require.NoError(t, err)
		assert.Equal(t, 500.0, p.GetCashBalance())
	})

	t.Run("Uses average price if available", func(t *testing.T) {
		p := NewPortfolio(logger, nil, container, 1000.0)
		order := &pb.OrderResponse{
			Status: "closed", Side: "buy", Symbol: "BTC/USDT",
			Filled: 1.0, Price: 100.0, Average: 105.0,
		}
		err := p.ApplyExecution(context.Background(), "binance", order)
		require.NoError(t, err)
		assert.Equal(t, 895.0, p.GetCashBalance()) // 1000 - 105
	})
}
