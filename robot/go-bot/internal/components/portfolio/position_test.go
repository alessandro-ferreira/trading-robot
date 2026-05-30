package portfolio

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"trading/robot/go-bot/internal/database/repository"
)

func TestPortfolio_GetPosition(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	exchange := "binance"
	symbol := "BTC/USDT"

	mockRepo := &MockPositionsRepo{
		GetPositionFn: func(ctx context.Context, db repository.DBExecutor, ex, sym string) (repository.PositionData, error) {
			return repository.PositionData{ExchangeName: ex, InstrumentSymbol: sym}, nil
		},
	}
	p := NewPortfolio(logger, nil, &repository.Container{Positions: mockRepo})

	res, err := p.GetPosition(context.Background(), exchange, symbol)
	assert.NoError(t, err)
	assert.Equal(t, exchange, res.ExchangeName)
	assert.Equal(t, symbol, res.InstrumentSymbol)
}

func TestPortfolio_CreatePosition(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	exchange := "binance"
	symbol := "BTC/USDT"

	tests := []struct {
		name                string
		quantity            float64
		price               float64
		orderID             int64
		getPositionFn       func(ctx context.Context, db repository.DBExecutor, ex, sym string) (repository.PositionData, error)
		upsertPositionFn    func(ctx context.Context, db repository.DBExecutor, pos repository.PositionData) error
		expectedErrContains string
		validateCount       int
	}{
		{
			name:     "Success New",
			quantity: 0.1,
			price:    50000.0,
			orderID:  123,
			getPositionFn: func(ctx context.Context, db repository.DBExecutor, ex, sym string) (repository.PositionData, error) {
				return repository.PositionData{}, pgx.ErrNoRows
			},
			upsertPositionFn: func(ctx context.Context, db repository.DBExecutor, pos repository.PositionData) error {
				assert.Equal(t, exchange, pos.ExchangeName)
				assert.Equal(t, symbol, pos.InstrumentSymbol)
				assert.Equal(t, 50000.0, pos.EntryPrice)
				assert.Equal(t, int64(123), pos.OrderID.Int64)
				assert.False(t, pos.UnknownOrigin)
				return nil
			},
			validateCount: 1,
		},
		{
			name:     "Promote Unknown Origin",
			quantity: 0.1,
			price:    51000.0,
			orderID:  999,
			getPositionFn: func(ctx context.Context, db repository.DBExecutor, ex, sym string) (repository.PositionData, error) {
				return repository.PositionData{
					ID: 1, ExchangeName: ex, InstrumentSymbol: sym,
					Quantity: 0.1, UnknownOrigin: true, Active: true,
				}, nil
			},
			upsertPositionFn: func(ctx context.Context, db repository.DBExecutor, pos repository.PositionData) error {
				assert.False(t, pos.UnknownOrigin)
				assert.Equal(t, int64(999), pos.OrderID.Int64)
				return nil
			},
			validateCount: 1,
		},
		{
			name:     "Promote Unknown Origin - Invalid Params",
			quantity: 0,
			price:    51000.0,
			orderID:  999,
			getPositionFn: func(ctx context.Context, db repository.DBExecutor, ex, sym string) (repository.PositionData, error) {
				return repository.PositionData{UnknownOrigin: true, Active: true}, nil
			},
			expectedErrContains: "parameter invalid",
		},
		{
			name:     "Fail on Existing Known Origin",
			quantity: 0.1,
			price:    50000.0,
			orderID:  123,
			getPositionFn: func(ctx context.Context, db repository.DBExecutor, ex, sym string) (repository.PositionData, error) {
				return repository.PositionData{ID: 1, UnknownOrigin: false, Active: true}, nil
			},
			expectedErrContains: "position already exists with known origin",
		},
		{
			name:     "Fail on check existing position error",
			quantity: 0.1,
			price:    50000.0,
			orderID:  123,
			getPositionFn: func(ctx context.Context, db repository.DBExecutor, ex, sym string) (repository.PositionData, error) {
				return repository.PositionData{}, errors.New("db check error")
			},
			expectedErrContains: "failed to check existing position",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &MockPositionsRepo{
				GetPositionFn:    tt.getPositionFn,
				UpsertPositionFn: tt.upsertPositionFn,
			}
			p := NewPortfolio(logger, nil, &repository.Container{Positions: mockRepo})

			err := p.CreatePosition(context.Background(), exchange, symbol, tt.quantity, tt.price, tt.orderID)
			if tt.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrContains)
			} else {
				assert.NoError(t, err)
				if tt.validateCount > 0 {
					assert.Equal(t, tt.validateCount, p.GetActivePositionsCount())
				}
			}
		})
	}
}

func TestPortfolio_UpdatePosition(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	exchange := "binance"
	symbol := "BTC/USDT"

	tests := []struct {
		name                string
		getPositionFn       func(ctx context.Context, db repository.DBExecutor, ex, sym string) (repository.PositionData, error)
		upsertPositionFn    func(ctx context.Context, db repository.DBExecutor, pos repository.PositionData) error
		updates             repository.PositionData
		expectedErrContains string
	}{
		{
			name: "Success Update",
			getPositionFn: func(ctx context.Context, db repository.DBExecutor, ex, sym string) (repository.PositionData, error) {
				return repository.PositionData{
					ID: 1, ExchangeName: ex, InstrumentSymbol: sym,
					Quantity: 0.1, EntryPrice: 50000.0, HighestPrice: 50000.0,
				}, nil
			},
			upsertPositionFn: func(ctx context.Context, db repository.DBExecutor, pos repository.PositionData) error {
				assert.Equal(t, 55000.0, pos.HighestPrice)
				assert.Equal(t, int64(888), pos.OrderID.Int64)
				return nil
			},
			updates: repository.PositionData{
				OrderID:      sql.NullInt64{Int64: 888, Valid: true},
				HighestPrice: 55000.0,
			},
		},
		{
			name: "Success Update - Conditional check (zeros don't overwrite)",
			getPositionFn: func(ctx context.Context, db repository.DBExecutor, ex, sym string) (repository.PositionData, error) {
				return repository.PositionData{
					ID: 1, ExchangeName: ex, InstrumentSymbol: sym,
					Quantity: 0.1, EntryPrice: 50000.0, HighestPrice: 50000.0,
				}, nil
			},
			upsertPositionFn: func(ctx context.Context, db repository.DBExecutor, pos repository.PositionData) error {
				// Updates provided in the call below are zero, so original values should be preserved
				assert.Equal(t, 0.1, pos.Quantity)
				assert.Equal(t, 50000.0, pos.EntryPrice)
				assert.Equal(t, 50000.0, pos.HighestPrice)
				return nil
			},
			updates: repository.PositionData{
				Quantity:   0,
				EntryPrice: -1.0,
			},
		},
		{
			name: "Fail on Missing Position",
			getPositionFn: func(ctx context.Context, db repository.DBExecutor, ex, sym string) (repository.PositionData, error) {
				return repository.PositionData{}, pgx.ErrNoRows
			},
			expectedErrContains: pgx.ErrNoRows.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &MockPositionsRepo{
				GetPositionFn:    tt.getPositionFn,
				UpsertPositionFn: tt.upsertPositionFn,
			}
			p := NewPortfolio(logger, nil, &repository.Container{Positions: mockRepo})

			err := p.UpdatePosition(context.Background(), exchange, symbol, tt.updates)
			if tt.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPortfolio_DeletePosition(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	exchange := "binance"
	symbol := "BTC/USDT"

	tests := []struct {
		name                string
		deletePositionFn    func(ctx context.Context, db repository.DBExecutor, ex, sym string) error
		expectedErrContains string
	}{
		{
			name: "Success Delete",
			deletePositionFn: func(ctx context.Context, db repository.DBExecutor, ex, sym string) error {
				assert.Equal(t, exchange, ex)
				assert.Equal(t, symbol, sym)
				return nil
			},
		},
		{
			name: "Fail on DB Error",
			deletePositionFn: func(ctx context.Context, db repository.DBExecutor, ex, sym string) error {
				return errors.New("db delete error")
			},
			expectedErrContains: "db delete error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &MockPositionsRepo{
				DeletePositionFn: tt.deletePositionFn,
			}
			p := NewPortfolio(logger, nil, &repository.Container{Positions: mockRepo})

			// Manual injection into memory map to verify it gets cleared
			p.(*portfolio).positions[fmt.Sprintf("%s|%s", exchange, symbol)] = struct{}{}

			err := p.DeletePosition(context.Background(), exchange, symbol)
			if tt.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrContains)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, 0, p.GetActivePositionsCount())
			}
		})
	}
}

func TestPortfolio_RefreshState(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	exchange := "binance"
	symbol := "BTC/USDT"

	tests := []struct {
		name          string
		getPositionFn func(ctx context.Context, db repository.DBExecutor, ex, sym string) (repository.PositionData, error)
		validateCount int
	}{
		{
			name: "Found Case",
			getPositionFn: func(ctx context.Context, db repository.DBExecutor, ex, sym string) (repository.PositionData, error) {
				return repository.PositionData{ExchangeName: ex, InstrumentSymbol: sym, Active: true}, nil
			},
			validateCount: 1,
		},
		{
			name: "Lost Case",
			getPositionFn: func(ctx context.Context, db repository.DBExecutor, ex, sym string) (repository.PositionData, error) {
				return repository.PositionData{}, pgx.ErrNoRows
			},
			validateCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &MockPositionsRepo{
				GetPositionFn: tt.getPositionFn,
			}
			p := NewPortfolio(logger, nil, &repository.Container{Positions: mockRepo})

			err := p.RefreshState(context.Background(), exchange, symbol)
			require.NoError(t, err)
			assert.Equal(t, tt.validateCount, p.GetActivePositionsCount())
		})
	}
}
