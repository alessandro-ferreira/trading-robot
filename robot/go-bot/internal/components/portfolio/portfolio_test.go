package portfolio

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/strategy"
)

// MockPositionsRepo implements repository.PositionsRepo for testing
type MockPositionsRepo struct {
	GetPositionFn      func(ctx context.Context, db repository.DBExecutor, exchangeName, symbol string) (repository.PositionData, error)
	GetOpenPositionsFn func(ctx context.Context, db repository.DBExecutor) ([]repository.PositionData, error)
	UpsertPositionFn   func(ctx context.Context, db repository.DBExecutor, pos repository.PositionData) error
	DeletePositionFn   func(ctx context.Context, db repository.DBExecutor, exchangeName, symbol string) error
}

func (m *MockPositionsRepo) GetPosition(ctx context.Context, db repository.DBExecutor, exchangeName, symbol string) (repository.PositionData, error) {
	if m.GetPositionFn != nil {
		return m.GetPositionFn(ctx, db, exchangeName, symbol)
	}
	return repository.PositionData{}, nil
}

func (m *MockPositionsRepo) GetOpenPositions(ctx context.Context, db repository.DBExecutor) ([]repository.PositionData, error) {
	if m.GetOpenPositionsFn != nil {
		return m.GetOpenPositionsFn(ctx, db)
	}
	return []repository.PositionData{}, nil
}

func (m *MockPositionsRepo) UpsertPosition(ctx context.Context, db repository.DBExecutor, pos repository.PositionData) error {
	if m.UpsertPositionFn != nil {
		return m.UpsertPositionFn(ctx, db, pos)
	}
	return nil
}
func (m *MockPositionsRepo) DeletePosition(ctx context.Context, db repository.DBExecutor, exchangeName, symbol string) error {
	if m.DeletePositionFn != nil {
		return m.DeletePositionFn(ctx, db, exchangeName, symbol)
	}
	return nil
}

// MockBalancesRepo implements repository.BalancesRepo for testing
type MockBalancesRepo struct {
	GetAllBalancesFn func(ctx context.Context, db repository.DBExecutor) ([]repository.BalanceData, error)
}

func (m *MockBalancesRepo) GetAllBalances(ctx context.Context, db repository.DBExecutor) ([]repository.BalanceData, error) {
	if m.GetAllBalancesFn != nil {
		return m.GetAllBalancesFn(ctx, db)
	}
	return []repository.BalanceData{}, nil
}

func (m *MockBalancesRepo) UpsertBalance(ctx context.Context, db repository.DBExecutor, balance repository.BalanceData) (int64, error) {
	return 0, nil
}

func TestPortfolio_LoadState(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockRepo := &MockPositionsRepo{}
	mockBalances := &MockBalancesRepo{}
	container := &repository.Container{
		Positions: mockRepo,
		Balances:  mockBalances,
	}

	t.Run("Hydrates internal map from DB positions", func(t *testing.T) {
		p := NewPortfolio(logger, nil, container)
		mockBalances.GetAllBalancesFn = func(ctx context.Context, db repository.DBExecutor) ([]repository.BalanceData, error) {
			return []repository.BalanceData{
				{ExchangeName: "binance", AssetSymbol: "USDT", Free: 1000.0, Used: 0.0, Total: 1000.0},
			}, nil
		}
		mockRepo.GetOpenPositionsFn = func(ctx context.Context, db repository.DBExecutor) ([]repository.PositionData, error) {
			return []repository.PositionData{
					{
						ExchangeName:     "binance",
						InstrumentSymbol: "BTC/USDT",
						Quantity:         1.5,
						EntryPrice:       40000.0,
						HighestPrice:     45000.0,
						StrategyState:    "active",
					},
				},
				nil
		}

		err := p.LoadState(context.Background())
		require.NoError(t, err)

		pos, exists := p.GetPosition("binance", "BTC/USDT")
		require.True(t, exists)
		assert.Equal(t, 1.5, pos.Quantity)
		assert.Equal(t, 40000.0, pos.EntryPrice)
		assert.Equal(t, 45000.0, pos.HighestPrice)
		assert.Equal(t, strategy.StateActive, pos.StrategyState)

		assert.Equal(t, 1000.0, p.GetCashBalance("binance", "USDT"))
	})
}
