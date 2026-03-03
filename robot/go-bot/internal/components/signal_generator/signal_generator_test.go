package signal_generator

import (
	"context"
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
	// Setup
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockClient := new(MockMarketDataProvider)
	symbol := "BTC/USD"
	exchange := "binance"
	cfg := config.StrategyConfig{
		Type: "dummy",
	}

	// Create the component
	sg := NewSignalGenerator(logger, mockClient, symbol, exchange, cfg)
	defer sg.strategy.Close()

	// Define expected behavior
	expectedPrice := 50000.0
	mockClient.On("GetTicker", mock.Anything, symbol, exchange).Return(&pb.TickerResponse{
		Symbol: symbol,
		Price:  expectedPrice,
	}, nil)

	// Execute the private process method directly for testing purposes
	// Note: In a real scenario, we might export a Process() method or use the Run() loop with a timeout.
	// For this unit test, we are verifying the logic inside process().
	err := sg.process(context.Background())

	// Assertions
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)

	// Verify the strategy state
	// The dummy strategy returns 1.0 (BUY) on the first update (inside process).
	// Since GetSignal() is stateful, the subsequent call here returns 0.0 (HOLD) as we are now in position.
	assert.Equal(t, 0.0, sg.strategy.GetSignal())
}
