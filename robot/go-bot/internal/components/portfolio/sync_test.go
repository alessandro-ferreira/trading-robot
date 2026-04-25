package portfolio

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/strategy"
)

func TestPortfolio_RefreshState(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockPositions := &MockPositionsRepo{}
	mockBalances := &MockBalancesRepo{}
	container := &repository.Container{
		Positions: mockPositions,
		Balances:  mockBalances,
	}

	tests := []struct {
		name         string
		setup        func()
		initialState func(*Portfolio)
		syncBalance  bool
		syncPosition bool
		check        func(*testing.T, *Portfolio)
	}{
		{
			name: "Successful sync of both balance and position",
			setup: func() {
				mockBalances.GetAllBalancesFn = func(ctx context.Context, db repository.DBExecutor) ([]repository.BalanceData, error) {
					return []repository.BalanceData{{ExchangeName: "binance", AssetSymbol: "USDT", Free: 100.0}}, nil
				}
				mockPositions.GetPositionFn = func(ctx context.Context, db repository.DBExecutor, ex, sym string) (repository.PositionData, error) {
					return repository.PositionData{ExchangeName: ex, InstrumentSymbol: sym, Quantity: 1.0, StrategyState: "active"}, nil
				}
			},
			syncBalance:  true,
			syncPosition: true,
			check: func(t *testing.T, p *Portfolio) {
				assert.Equal(t, 100.0, p.GetCashBalance("binance", "USDT"))
				pos, exists := p.GetPosition("binance", "BTC/USDT")
				assert.True(t, exists)
				assert.Equal(t, 1.0, pos.Quantity)
			},
		},
		{
			name: "Removes position from memory if not found in DB",
			setup: func() {
				mockPositions.GetPositionFn = func(ctx context.Context, db repository.DBExecutor, ex, sym string) (repository.PositionData, error) {
					return repository.PositionData{}, errors.New("not found")
				}
			},
			initialState: func(p *Portfolio) {
				p.mu.Lock()
				p.positions["binance|BTC/USDT"] = &Position{Symbol: "BTC/USDT"}
				p.mu.Unlock()
			},
			syncPosition: true,
			check: func(t *testing.T, p *Portfolio) {
				_, exists := p.GetPosition("binance", "BTC/USDT")
				assert.False(t, exists)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			p := NewPortfolio(logger, nil, container)
			if tt.initialState != nil {
				tt.initialState(p)
			}
			err := p.RefreshState(context.Background(), "binance", "BTC/USDT", tt.syncBalance, tt.syncPosition)
			assert.NoError(t, err)
			tt.check(t, p)
		})
	}
}

func TestPortfolio_UpdatePrice(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockRepo := &MockPositionsRepo{}
	container := &repository.Container{
		Positions: mockRepo,
		Balances:  &MockBalancesRepo{},
	}

	tests := []struct {
		name         string
		initialPos   *Position
		newPrice     float64
		upsertErr    error
		expectUpsert bool
		check        func(*testing.T, *Position)
	}{
		{
			name: "New peak price updates HWM and persists",
			initialPos: &Position{
				Exchange: "binance", Symbol: "BTC/USDT", Quantity: 1.0, EntryPrice: 100.0, HighestPrice: 100.0,
			},
			newPrice:     110.0,
			expectUpsert: true,
			check: func(t *testing.T, p *Position) {
				assert.Equal(t, 110.0, p.HighestPrice)
				assert.Equal(t, 10.0, p.UnrealizedPnL)
			},
		},
		{
			name: "Price decrease updates unrealized pnl but not HWM",
			initialPos: &Position{
				Exchange: "binance", Symbol: "BTC/USDT", Quantity: 1.0, EntryPrice: 100.0, HighestPrice: 120.0,
			},
			newPrice:     110.0,
			expectUpsert: false,
			check: func(t *testing.T, p *Position) {
				assert.Equal(t, 120.0, p.HighestPrice)
				assert.Equal(t, 10.0, p.UnrealizedPnL)
			},
		},
		{
			name: "Gracefully handles persistence errors on peak update",
			initialPos: &Position{
				Exchange: "binance", Symbol: "BTC/USDT", Quantity: 1.0, EntryPrice: 100.0, HighestPrice: 100.0,
			},
			newPrice:     110.0,
			upsertErr:    errors.New("db error"),
			expectUpsert: true,
			check: func(t *testing.T, p *Position) {
				assert.Equal(t, 110.0, p.HighestPrice)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPortfolio(logger, nil, container)
			p.mu.Lock()
			key := makeKey(tt.initialPos.Exchange, tt.initialPos.Symbol)
			p.positions[key] = tt.initialPos
			p.mu.Unlock()

			upsertCalled := false
			mockRepo.UpsertPositionFn = func(ctx context.Context, db repository.DBExecutor, data repository.PositionData) error {
				upsertCalled = true
				return tt.upsertErr
			}

			p.UpdatePrice(context.Background(), tt.initialPos.Exchange, tt.initialPos.Symbol, tt.newPrice)

			if tt.expectUpsert {
				assert.True(t, upsertCalled, "UpsertPosition should have been called")
			} else {
				assert.False(t, upsertCalled, "UpsertPosition should not have been called")
			}

			pos, _ := p.GetPosition(tt.initialPos.Exchange, tt.initialPos.Symbol)
			tt.check(t, pos)
		})
	}
}

func TestPortfolio_SyncMetadata(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockRepo := &MockPositionsRepo{}
	container := &repository.Container{Positions: mockRepo}

	tests := []struct {
		name      string
		state     strategy.StrategyState
		upsertErr error
	}{
		{
			name:  "Metadata sync persists new state",
			state: strategy.StatePendingSell,
		},
		{
			name:      "Gracefully handles persistence errors",
			state:     strategy.StateActive,
			upsertErr: errors.New("db fail"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPortfolio(logger, nil, container)
			p.mu.Lock()
			p.positions["binance|BTC/USDT"] = &Position{Exchange: "binance", Symbol: "BTC/USDT"}
			p.mu.Unlock()

			upsertCalled := false
			mockRepo.UpsertPositionFn = func(ctx context.Context, db repository.DBExecutor, data repository.PositionData) error {
				upsertCalled = true
				assert.Equal(t, tt.state.String(), data.StrategyState)
				return tt.upsertErr
			}

			p.SyncMetadata(context.Background(), "binance", "BTC/USDT", tt.state)
			assert.True(t, upsertCalled)

			pos, _ := p.GetPosition("binance", "BTC/USDT")
			assert.Equal(t, tt.state, pos.StrategyState)
		})
	}
}
