package signal_generator

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	pb "trading/robot/go-bot/gen/go/v1"
	"trading/robot/go-bot/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockMarketDataProvider is a mock implementation of MarketDataProvider
type MockMarketDataProvider struct {
	mock.Mock
}

func (m *MockMarketDataProvider) GetTicker(ctx context.Context, symbol, exchange string) (*pb.TickerResponse, error) {
	args := m.Called(ctx, symbol, exchange)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pb.TickerResponse), args.Error(1)
}

func TestSignalGenerator_Process(t *testing.T) {
	// Setup logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Setup mock client
	mockClient := new(MockMarketDataProvider)
	symbol := "BTC/USD"
	exchange := "binance"

	// Setup config for momentum strategy
	appCfg := config.StrategyConfig{
		Type: config.StrategyMomentumTrailing, // Use momentum to trigger the specific config mapping path
		Momentum: config.MomentumConfig{
			WindowSeconds:   100,
			LookbackSeconds: 50,
			Threshold:       0.01,
			StopLossPct:     0.1,
			ActivationPct:   0.05,
			TrailingStopPct: 0.02,
		},
	}

	// Create generator
	sg, err := NewSignalGenerator(logger, mockClient, symbol, exchange, appCfg)
	assert.NoError(t, err)
	assert.NotNil(t, sg)

	t.Run("successful processing", func(t *testing.T) {
		// Mock successful ticker response
		expectedTicker := &pb.TickerResponse{
			Symbol: symbol,
			Price:  50000.0,
		}
		mockClient.On("GetTicker", mock.Anything, symbol, exchange).Return(expectedTicker, nil).Once()

		// Execute process
		err := sg.Process(context.Background())

		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("client error handling", func(t *testing.T) {
		// Mock client error
		mockClient.On("GetTicker", mock.Anything, symbol, exchange).Return((*pb.TickerResponse)(nil), errors.New("network error")).Once()

		// Execute process
		err := sg.Process(context.Background())

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "network error")
		mockClient.AssertExpectations(t)
	})
}
