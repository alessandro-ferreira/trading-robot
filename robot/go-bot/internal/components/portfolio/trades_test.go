package portfolio

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "trading/robot/go-bot/gen/go/v1"
	"trading/robot/go-bot/internal/database/repository"
)

func TestPortfolio_UpdatePosition(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockRepo := &MockPositionsRepo{}
	container := &repository.Container{
		Positions: mockRepo,
		Balances:  &MockBalancesRepo{},
	}

	tests := []struct {
		name                string
		setup               func(*Portfolio)
		quantity            float64
		price               float64
		symbol              string
		upsertErr           error
		expectedErrContains string
		expectedCash        float64
		expectedQty         float64
		expectedExists      bool
	}{
		{
			name: "Buy increases quantity and decreases cash",
			setup: func(p *Portfolio) {
				p.mu.Lock()
				p.cashBalances["binance|USDT"] = &CashBalance{Exchange: "binance", Asset: "USDT", Free: 1000.0}
				p.mu.Unlock()
			},
			symbol:         "BTC/USDT",
			quantity:       0.1,
			price:          5000.0,
			expectedCash:   500.0,
			expectedQty:    0.1,
			expectedExists: true,
		},
		{
			name: "Insufficient funds for buy returns error",
			setup: func(p *Portfolio) {
				p.mu.Lock()
				p.cashBalances["binance|USDT"] = &CashBalance{Exchange: "binance", Asset: "USDT", Free: 10.0}
				p.mu.Unlock()
			},
			symbol:              "BTC/USDT",
			quantity:            1.0,
			price:               100.0,
			expectedErrContains: "insufficient USDT",
		},
		{
			name: "Sell decreases quantity and increases cash",
			setup: func(p *Portfolio) {
				p.mu.Lock()
				p.cashBalances["binance|USDT"] = &CashBalance{Exchange: "binance", Asset: "USDT", Free: 0.0}
				p.positions["binance|BTC/USDT"] = &Position{Exchange: "binance", Symbol: "BTC/USDT", Quantity: 1.0, EntryPrice: 100.0}
				p.mu.Unlock()
			},
			symbol:         "BTC/USDT",
			quantity:       -0.5,
			price:          120.0,
			expectedCash:   60.0,
			expectedQty:    0.5,
			expectedExists: true,
		},
		{
			name: "Full sell removes position from memory",
			setup: func(p *Portfolio) {
				p.mu.Lock()
				p.cashBalances["binance|USDT"] = &CashBalance{Exchange: "binance", Asset: "USDT", Free: 0.0}
				p.positions["binance|BTC/USDT"] = &Position{Exchange: "binance", Symbol: "BTC/USDT", Quantity: 1.0, EntryPrice: 100.0}
				p.mu.Unlock()
			},
			symbol:         "BTC/USDT",
			quantity:       -1.0,
			price:          110.0,
			expectedCash:   110.0,
			expectedExists: false,
		},
		{
			name: "Sell without existing position returns error",
			setup: func(p *Portfolio) {
				p.mu.Lock()
				p.cashBalances = make(map[string]*CashBalance)
				p.mu.Unlock()
			},
			symbol:              "BTC/USDT",
			quantity:            -1.0,
			price:               100.0,
			expectedErrContains: "insufficient position: holding 0",
		},
		{
			name: "Selling more than held returns error",
			setup: func(p *Portfolio) {
				p.mu.Lock()
				p.positions["binance|BTC/USDT"] = &Position{Quantity: 0.5}
				p.mu.Unlock()
			},
			symbol:              "BTC/USDT",
			quantity:            -1.0,
			price:               100.0,
			expectedErrContains: "insufficient position: holding 0.5",
		},
		{
			name: "Selling creates cash balance if missing",
			setup: func(p *Portfolio) {
				p.mu.Lock()
				p.cashBalances = make(map[string]*CashBalance)
				p.positions["binance|BTC/USDT"] = &Position{Exchange: "binance", Symbol: "BTC/USDT", Quantity: 1.0, EntryPrice: 100.0}
				p.mu.Unlock()
			},
			symbol:         "BTC/USDT",
			quantity:       -1.0,
			price:          100.0,
			expectedCash:   100.0,
			expectedExists: false,
		},
		{
			name:                "Invalid symbol format returns error",
			setup:               func(p *Portfolio) {},
			symbol:              "INVALID_SYMBOL",
			quantity:            1.0,
			price:               100.0,
			expectedErrContains: "invalid symbol format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPortfolio(logger, nil, container)
			tt.setup(p)

			mockRepo.UpsertPositionFn = func(ctx context.Context, db repository.DBExecutor, pos repository.PositionData) error {
				return tt.upsertErr
			}
			mockRepo.DeletePositionFn = func(ctx context.Context, db repository.DBExecutor, ex, sym string) error { return nil }

			err := p.UpdatePosition(context.Background(), "binance", tt.symbol, tt.quantity, tt.price)

			if tt.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrContains)
			} else {
				require.NoError(t, err)
				_, quote := splitSymbol(tt.symbol)
				assert.Equal(t, tt.expectedCash, p.GetCashBalance("binance", quote))
				pos, exists := p.GetPosition("binance", tt.symbol)
				assert.Equal(t, tt.expectedExists, exists)
				if tt.expectedExists {
					assert.Equal(t, tt.expectedQty, pos.Quantity)
				}
			}
		})
	}
}

func TestPortfolio_ApplyExecution(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockRepo := &MockPositionsRepo{}
	container := &repository.Container{
		Positions: mockRepo,
		Balances:  &MockBalancesRepo{},
	}

	tests := []struct {
		name         string
		order        *pb.OrderResponse
		initialCash  float64
		expectedCash float64
	}{
		{
			name:         "Skips non-closed orders",
			order:        &pb.OrderResponse{Status: "open", Side: "buy", Filled: 1.0},
			initialCash:  1000.0,
			expectedCash: 1000.0,
		},
		{
			name:         "Processes buy with standard price",
			order:        &pb.OrderResponse{Status: "closed", Side: "buy", Filled: 0.1, Price: 5000.0, Symbol: "BTC/USDT"},
			initialCash:  1000.0,
			expectedCash: 500.0,
		},
		{
			name:         "Uses average price over limit price if available",
			order:        &pb.OrderResponse{Status: "closed", Side: "buy", Symbol: "BTC/USDT", Filled: 1.0, Price: 100.0, Average: 105.0},
			initialCash:  1000.0,
			expectedCash: 895.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPortfolio(logger, nil, container)
			p.mu.Lock()
			p.cashBalances["binance|USDT"] = &CashBalance{Exchange: "binance", Asset: "USDT", Free: tt.initialCash}
			p.mu.Unlock()

			mockRepo.UpsertPositionFn = func(ctx context.Context, db repository.DBExecutor, pos repository.PositionData) error { return nil }

			err := p.ApplyExecution(context.Background(), "binance", tt.order)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedCash, p.GetCashBalance("binance", "USDT"))
		})
	}
}
