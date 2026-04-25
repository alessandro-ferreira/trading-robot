package portfolio

import (
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"trading/robot/go-bot/internal/database/repository"
)

func TestPortfolio_Metrics(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// We use the basic MockPositionsRepo already defined in portfolio_test.go
	mockRepo := &MockPositionsRepo{}
	container := &repository.Container{
		Positions: mockRepo,
		Balances:  &MockBalancesRepo{},
	}

	tests := []struct {
		name              string
		setup             func(*Portfolio)
		checkCashEx       string
		checkCashAsset    string
		expectedCash      float64
		expectedOpenCount int
		expectedTotals    map[string]float64
	}{
		{
			name: "Initial state with one balance",
			setup: func(p *Portfolio) {
				p.mu.Lock()
				p.cashBalances["binance|USDT"] = &CashBalance{Exchange: "binance", Asset: "USDT", Free: 1000.0}
				p.mu.Unlock()
			},
			checkCashEx:       "binance",
			checkCashAsset:    "USDT",
			expectedCash:      1000.0,
			expectedOpenCount: 0,
			expectedTotals:    map[string]float64{"USDT": 1000.0},
		},
		{
			name: "GetCashBalance returns 0 for missing asset",
			setup: func(p *Portfolio) {
				p.mu.Lock()
				p.cashBalances["binance|USDT"] = &CashBalance{Exchange: "binance", Asset: "USDT", Free: 1000.0}
				p.mu.Unlock()
			},
			checkCashEx:       "binance",
			checkCashAsset:    "BTC",
			expectedCash:      0.0,
			expectedOpenCount: 0,
			expectedTotals:    map[string]float64{"USDT": 1000.0},
		},
		{
			name: "Calculates total value across multiple exchanges and positions",
			setup: func(p *Portfolio) {
				p.mu.Lock()
				p.cashBalances["binance|USDT"] = &CashBalance{Exchange: "binance", Asset: "USDT", Free: 500.0}
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
			},
			checkCashEx:       "binance",
			checkCashAsset:    "USDT",
			expectedCash:      500.0,
			expectedOpenCount: 2,
			expectedTotals:    map[string]float64{"USDT": 1400.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPortfolio(logger, nil, container)
			tt.setup(p)

			assert.Equal(t, tt.expectedCash, p.GetCashBalance(tt.checkCashEx, tt.checkCashAsset))
			assert.Equal(t, tt.expectedOpenCount, p.GetOpenPositionsCount())
			assert.Equal(t, tt.expectedTotals, p.GetTotalValue())
		})
	}

	t.Run("GetPosition Returns a Copy Not a Reference", func(t *testing.T) {
		p := NewPortfolio(logger, nil, container)
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
