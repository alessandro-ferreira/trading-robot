package portfolio

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"trading/robot/go-bot/internal/database/repository"
)

// MockPositionsRepo implements repository.PositionsRepo for testing
type MockPositionsRepo struct{}

func (m *MockPositionsRepo) GetPosition(ctx context.Context, db repository.DBExecutor, exchangeName, symbol string) (repository.PositionData, error) {
	return repository.PositionData{}, nil
}

func (m *MockPositionsRepo) GetOpenPositions(ctx context.Context, db repository.DBExecutor) ([]repository.PositionData, error) {
	return []repository.PositionData{}, nil
}

func (m *MockPositionsRepo) UpsertPosition(ctx context.Context, db repository.DBExecutor, pos repository.PositionData) error {
	return nil
}
func (m *MockPositionsRepo) DeletePosition(ctx context.Context, db repository.DBExecutor, exchangeName, symbol string) error {
	return nil
}

type portfolioAction struct {
	exchange string
	symbol   string
	quantity float64
	price    float64
}

func TestPortfolio_UpdatePosition(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockRepo := &MockPositionsRepo{}
	container := &repository.Container{
		Positions: mockRepo,
	}

	testCases := []struct {
		name              string
		initialCash       float64
		actions           []portfolioAction
		expectErr         bool
		expectErrContains string
		expectedCash      float64
		expectedPosition  *Position
	}{
		{
			name:        "Buy new position",
			initialCash: 1000.0,
			actions: []portfolioAction{
				{exchange: "binance", symbol: "BTC/USD", quantity: 0.1, price: 5000.0},
			},
			expectErr:    false,
			expectedCash: 500.0,
			expectedPosition: &Position{
				Exchange:   "binance",
				Symbol:     "BTC/USD",
				Quantity:   0.1,
				EntryPrice: 5000.0,
			},
		},
		{
			name:        "Buy to average up position with sufficient funds",
			initialCash: 2000.0,
			actions: []portfolioAction{
				{exchange: "binance", symbol: "BTC/USD", quantity: 0.1, price: 5000.0},
				{exchange: "binance", symbol: "BTC/USD", quantity: 0.1, price: 7000.0},
			},
			expectErr:    false,
			expectedCash: 800.0,
			expectedPosition: &Position{
				Exchange:   "binance",
				Symbol:     "BTC/USD",
				Quantity:   0.2,
				EntryPrice: 6000.0,
			},
		},
		{
			name:        "Buy to average up position with insufficient funds",
			initialCash: 1000.0,
			actions: []portfolioAction{
				{exchange: "binance", symbol: "BTC/USD", quantity: 0.1, price: 5000.0},
				{exchange: "binance", symbol: "BTC/USD", quantity: 0.1, price: 6000.0},
			},
			expectErr:         true,
			expectErrContains: "insufficient funds",
		},
		{
			name:        "Sell partial position",
			initialCash: 1000.0,
			actions: []portfolioAction{
				{exchange: "binance", symbol: "BTC/USD", quantity: 0.2, price: 100.0},
				{exchange: "binance", symbol: "BTC/USD", quantity: -0.1, price: 120.0},
			},
			expectErr:    false,
			expectedCash: 992.0,
			expectedPosition: &Position{
				Exchange:   "binance",
				Symbol:     "BTC/USD",
				Quantity:   0.1,
				EntryPrice: 100.0,
			},
		},
		{
			name:        "Sell entire position",
			initialCash: 1000.0,
			actions: []portfolioAction{
				{exchange: "binance", symbol: "BTC/USD", quantity: 0.1, price: 100.0},
				{exchange: "binance", symbol: "BTC/USD", quantity: -0.1, price: 110.0},
			},
			expectErr:        false,
			expectedCash:     1001.0,
			expectedPosition: nil,
		},
		{
			name:        "Sell insufficient position",
			initialCash: 1000.0,
			actions: []portfolioAction{
				{exchange: "binance", symbol: "BTC/USD", quantity: -0.1, price: 100.0},
			},
			expectErr:         true,
			expectErrContains: "insufficient position",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewPortfolio(logger, nil, container, tc.initialCash)
			var err error
			ctx := context.Background()
			for _, action := range tc.actions {
				err = p.UpdatePosition(ctx, action.exchange, action.symbol, action.quantity, action.price)
				if err != nil {
					break
				}
			}

			if tc.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectErrContains)
			} else {
				assert.NoError(t, err)
				assert.InDelta(t, tc.expectedCash, p.GetCashBalance(), 0.0001)
				lastAction := tc.actions[len(tc.actions)-1]
				pos, exists := p.GetPosition(lastAction.exchange, lastAction.symbol)
				if tc.expectedPosition == nil {
					assert.False(t, exists, "Position should not exist")
				} else {
					assert.True(t, exists, "Position should exist")
					assert.InDelta(t, tc.expectedPosition.Quantity, pos.Quantity, 0.000001)
					assert.InDelta(t, tc.expectedPosition.EntryPrice, pos.EntryPrice, 0.0001)
				}
			}
		})
	}
}

func TestPortfolio_UpdatePrice(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockRepo := &MockPositionsRepo{}
	container := &repository.Container{
		Positions: mockRepo,
	}
	p := NewPortfolio(logger, nil, container, 1000.0)
	_ = p.UpdatePosition(context.Background(), "binance", "BTC/USD", 1.0, 100.0) // Entry @ 100

	// Price goes up
	p.UpdatePrice("binance", "BTC/USD", 110.0)
	pos, _ := p.GetPosition("binance", "BTC/USD")
	assert.Equal(t, 10.0, pos.UnrealizedPnL)

	// Price goes down
	p.UpdatePrice("binance", "BTC/USD", 90.0)
	pos, _ = p.GetPosition("binance", "BTC/USD")
	assert.Equal(t, -10.0, pos.UnrealizedPnL)
}

func TestPortfolio_Metrics(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockRepo := &MockPositionsRepo{}
	container := &repository.Container{
		Positions: mockRepo,
	}

	// Initial Cash: 1000
	p := NewPortfolio(logger, nil, container, 1000.0)

	// Buy 1 BTC @ 100. Cost = 100. Cash = 900.
	// Position Value = 1 * 100 = 100.
	// Total Value = 900 + 100 = 1000.
	_ = p.UpdatePosition(context.Background(), "binance", "BTC/USD", 1.0, 100.0)

	assert.Equal(t, 1000.0, p.GetTotalValue())
	assert.Equal(t, 1, p.GetOpenPositionsCount())
}
