package portfolio

import (
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"trading/robot/go-bot/internal/database/repository"
)

func TestPortfolio_Metrics(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	// We use the basic MockPositionsRepo already defined in portfolio_test.go
	mockRepo := &MockPositionsRepo{}
	container := &repository.Container{
		Positions: mockRepo,
	}

	t.Run("Initial State", func(t *testing.T) {
		p := NewPortfolio(logger, nil, container, 1000.0)
		assert.Equal(t, 1000.0, p.GetCashBalance())
		assert.Equal(t, 1000.0, p.GetTotalValue())
		assert.Equal(t, 0, p.GetOpenPositionsCount())
		_, exists := p.GetPosition("binance", "BTC/USDT")
		assert.False(t, exists)
	})

	t.Run("Calculates Metrics Correctly with Active Positions", func(t *testing.T) {
		p := NewPortfolio(logger, nil, container, 500.0)

		// Manually populate internal map to test valuation logic in isolation
		p.mu.Lock()
		p.positions["binance|BTC/USDT"] = &Position{
			Exchange:     "binance",
			Symbol:       "BTC/USDT",
			Quantity:     0.1,
			CurrentPrice: 5000.0, // Value = 500.0
		}
		p.positions["kraken|ETH/USDT"] = &Position{
			Exchange:     "kraken",
			Symbol:       "ETH/USDT",
			Quantity:     2.0,
			CurrentPrice: 200.0, // Value = 400.0
		}
		p.mu.Unlock()

		// Expected Total: 500 (Cash) + 500 (BTC) + 400 (ETH) = 1400.0
		assert.Equal(t, 500.0, p.GetCashBalance())
		assert.Equal(t, 1400.0, p.GetTotalValue())
		assert.Equal(t, 2, p.GetOpenPositionsCount())

		pos, exists := p.GetPosition("binance", "BTC/USDT")
		assert.True(t, exists)
		assert.Equal(t, 0.1, pos.Quantity)
	})

	t.Run("GetPosition Returns a Copy Not a Reference", func(t *testing.T) {
		p := NewPortfolio(logger, nil, container, 1000.0)
		p.mu.Lock()
		p.positions["binance|BTC/USDT"] = &Position{
			Symbol:   "BTC/USDT",
			Quantity: 1.0,
		}
		p.mu.Unlock()

		pos, exists := p.GetPosition("binance", "BTC/USDT")
		require.True(t, exists)

		// Modifying the returned struct should not affect internal state
		pos.Quantity = 99.0

		assert.Equal(t, 1.0, p.positions["binance|BTC/USDT"].Quantity)
	})
}
