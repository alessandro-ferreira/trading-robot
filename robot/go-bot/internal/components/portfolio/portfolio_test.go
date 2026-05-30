package portfolio

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"trading/robot/go-bot/internal/database/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockPositionsRepo implements repository.PositionsRepo for testing
type MockPositionsRepo struct {
	GetPositionFn        func(ctx context.Context, db repository.DBExecutor, exchangeName, instrumentSymbol string) (repository.PositionData, error)
	GetActivePositionsFn func(ctx context.Context, db repository.DBExecutor, exchangeName, instrumentSymbol string) ([]repository.PositionData, error)
	UpsertPositionFn     func(ctx context.Context, db repository.DBExecutor, pos repository.PositionData) error
	DeletePositionFn     func(ctx context.Context, db repository.DBExecutor, exchangeName, instrumentSymbol string) error
}

func (m *MockPositionsRepo) GetPosition(ctx context.Context, db repository.DBExecutor, exchangeName, instrumentSymbol string) (repository.PositionData, error) {
	if m.GetPositionFn != nil {
		return m.GetPositionFn(ctx, db, exchangeName, instrumentSymbol)
	}
	return repository.PositionData{}, nil
}

func (m *MockPositionsRepo) GetActivePositions(ctx context.Context, db repository.DBExecutor, exchangeName, instrumentSymbol string) ([]repository.PositionData, error) {
	if m.GetActivePositionsFn != nil {
		return m.GetActivePositionsFn(ctx, db, exchangeName, instrumentSymbol)
	}
	return []repository.PositionData{}, nil
}

func (m *MockPositionsRepo) UpsertPosition(ctx context.Context, db repository.DBExecutor, pos repository.PositionData) error {
	if m.UpsertPositionFn != nil {
		return m.UpsertPositionFn(ctx, db, pos)
	}
	return nil
}

func (m *MockPositionsRepo) DeletePosition(ctx context.Context, db repository.DBExecutor, exchangeName, instrumentSymbol string) error {
	if m.DeletePositionFn != nil {
		return m.DeletePositionFn(ctx, db, exchangeName, instrumentSymbol)
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
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockRepo := &MockPositionsRepo{}
	mockBalances := &MockBalancesRepo{}
	container := &repository.Container{
		Positions: mockRepo,
		Balances:  mockBalances,
	}

	tests := []struct {
		name                   string
		setupBalances          func()
		setupPositions         func()
		expectedErrContains    string
		expectedCashAsset      string
		expectedCashAmount     float64
		expectedPositionsCount int
	}{
		{
			name: "Success hydration",
			setupBalances: func() {
				mockBalances.GetAllBalancesFn = func(ctx context.Context, db repository.DBExecutor) ([]repository.BalanceData, error) {
					return []repository.BalanceData{
						{ExchangeName: "binance", AssetSymbol: "USDT", Free: 1000.0, Total: 1000.0},
					}, nil
				}
			},
			setupPositions: func() {
				mockRepo.GetActivePositionsFn = func(ctx context.Context, db repository.DBExecutor, exchangeName, instrumentSymbol string) ([]repository.PositionData, error) {
					return []repository.PositionData{
						{ExchangeName: "binance", InstrumentSymbol: "BTC/USDT", Quantity: 1.5, EntryPrice: 40000.0, HighestPrice: 45000.0, Active: true},
					}, nil
				}
			},
			expectedCashAsset:      "USDT",
			expectedCashAmount:     1000.0,
			expectedPositionsCount: 1,
		},
		{
			name: "Returns error when GetActivePositions fails",
			setupBalances: func() {
				mockBalances.GetAllBalancesFn = func(ctx context.Context, db repository.DBExecutor) ([]repository.BalanceData, error) {
					return []repository.BalanceData{}, nil
				}
			},
			setupPositions: func() {
				mockRepo.GetActivePositionsFn = func(ctx context.Context, db repository.DBExecutor, exchangeName, instrumentSymbol string) ([]repository.PositionData, error) {
					return nil, errors.New("db position error")
				}
			},
			expectedErrContains: "failed to fetch positions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupBalances()
			tt.setupPositions()
			p := NewPortfolio(logger, nil, container)

			err := p.LoadState(context.Background())

			if tt.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedPositionsCount, p.GetActivePositionsCount())
			}
		})
	}
}

func TestPortfolio_GetTotalValue(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name             string
		getAllBalancesFn func(ctx context.Context, db repository.DBExecutor) ([]repository.BalanceData, error)
		expectedTotals   map[string]float64
		expectedErr      bool
	}{
		{
			name: "Success aggregation",
			getAllBalancesFn: func(ctx context.Context, db repository.DBExecutor) ([]repository.BalanceData, error) {
				return []repository.BalanceData{
					{AssetSymbol: "USDT", Total: 1000.0},
					{AssetSymbol: "USDT", Total: 500.0},
					{AssetSymbol: "BTC", Total: 0.1},
				}, nil
			},
			expectedTotals: map[string]float64{"USDT": 1500.0, "BTC": 0.1},
		},
		{
			name: "Repository error",
			getAllBalancesFn: func(ctx context.Context, db repository.DBExecutor) ([]repository.BalanceData, error) {
				return nil, errors.New("db error")
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockBalances := &MockBalancesRepo{GetAllBalancesFn: tt.getAllBalancesFn}
			p := NewPortfolio(logger, nil, &repository.Container{Balances: mockBalances})

			totals, err := p.GetTotalValue(context.Background())
			if tt.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedTotals, totals)
			}
		})
	}
}
