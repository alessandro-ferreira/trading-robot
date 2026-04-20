package portfolio

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/strategy"
)

func TestPortfolio_RefreshState(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockPositions := &MockPositionsRepo{}
	mockBalances := &MockBalancesRepo{}
	container := &repository.Container{
		Positions: mockPositions,
		Balances:  mockBalances,
	}

	t.Run("Refresh balances and positions successfully", func(t *testing.T) {
		p := NewPortfolio(logger, nil, container)

		mockBalances.GetAllBalancesFn = func(ctx context.Context, db repository.DBExecutor) ([]repository.BalanceData, error) {
			return []repository.BalanceData{
				{ExchangeName: "binance", AssetSymbol: "USDT", Free: 100.0, Used: 10.0, Total: 110.0},
			}, nil
		}

		mockPositions.GetPositionFn = func(ctx context.Context, db repository.DBExecutor, exchange, symbol string) (repository.PositionData, error) {
			return repository.PositionData{
				ExchangeName:     "binance",
				InstrumentSymbol: "BTC/USDT",
				Quantity:         1.0,
				EntryPrice:       50000.0,
				HighestPrice:     55000.0,
				StrategyState:    "active",
			}, nil
		}

		err := p.RefreshState(context.Background(), "binance", "BTC/USDT", true, true)
		require.NoError(t, err)

		assert.Equal(t, 100.0, p.GetCashBalance("binance", "USDT"))
		pos, exists := p.GetPosition("binance", "BTC/USDT")
		assert.True(t, exists)
		assert.Equal(t, 1.0, pos.Quantity)
	})

	t.Run("Refresh position removal on error", func(t *testing.T) {
		p := NewPortfolio(logger, nil, container)
		p.mu.Lock()
		p.positions["binance|BTC/USDT"] = &Position{Symbol: "BTC/USDT"}
		p.mu.Unlock()

		mockPositions.GetPositionFn = func(ctx context.Context, db repository.DBExecutor, exchange, symbol string) (repository.PositionData, error) {
			return repository.PositionData{}, fmt.Errorf("not found")
		}

		err := p.RefreshState(context.Background(), "binance", "BTC/USDT", false, true)
		require.NoError(t, err)

		_, exists := p.GetPosition("binance", "BTC/USDT")
		assert.False(t, exists)
	})
}

func TestPortfolio_UpdatePrice(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockRepo := &MockPositionsRepo{}
	container := &repository.Container{
		Positions: mockRepo,
		Balances:  &MockBalancesRepo{},
	}

	t.Run("Price increase updates unrealized pnl and persists new peak", func(t *testing.T) {
		p := NewPortfolio(logger, nil, container)
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
		p := NewPortfolio(logger, nil, container)
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
	p := NewPortfolio(logger, nil, container)
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
