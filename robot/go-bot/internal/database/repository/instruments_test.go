//go:build unit

package repository

import (
	"context"
	"database/sql"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var instrumentColumns = []string{
	"id", "exchange_name", "name", "base_asset_symbol", "quote_asset_symbol",
	"price_precision", "amount_precision", "min_amount", "created_at", "created_by",
	"updated_at", "updated_by", "active",
}

func TestPgInstrumentsRepo_GetInstrument(t *testing.T) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	// Truncate to seconds to avoid precision issues with database timestamp comparisons
	now := time.Now().Truncate(time.Second)
	instrument := InstrumentData{
		ID:               1,
		ExchangeName:     "binance",
		Name:             "BTC/USDT",
		BaseAssetSymbol:  "BTC",
		QuoteAssetSymbol: "USDT",
		PricePrecision:   r.Intn(8) + 1,
		AmountPrecision:  r.Intn(8) + 1,
		MinAmount:        r.Float64() * 0.01,
		CreatedAt:        now,
		CreatedBy:        sql.NullString{String: "migration_000021", Valid: true},
		UpdatedAt:        sql.NullTime{Time: now, Valid: true},
		UpdatedBy:        sql.NullString{String: "migration_000021", Valid: true},
		Active:           true,
	}

	toRow := func(data InstrumentData) []any {
		return []any{
			data.ID,
			data.ExchangeName,
			data.Name,
			data.BaseAssetSymbol,
			data.QuoteAssetSymbol,
			data.PricePrecision,
			data.AmountPrecision,
			data.MinAmount,
			data.CreatedAt,
			data.CreatedBy,
			data.UpdatedAt,
			data.UpdatedBy,
			data.Active,
		}
	}

	repo := NewInstrumentsRepo()

	testCases := []struct {
		name                string
		exchange            string
		symbol              string
		setupMock           func(mock pgxmock.PgxPoolIface)
		expectedErrContains string
		validate            func(t *testing.T, data InstrumentData)
	}{
		{
			name:     "Success",
			exchange: instrument.ExchangeName,
			symbol:   instrument.Name,
			setupMock: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(instrumentColumns).AddRow(toRow(instrument)...)
				mock.ExpectQuery("SELECT").
					WithArgs(instrument.ExchangeName, instrument.Name).
					WillReturnRows(rows)
			},
			validate: func(t *testing.T, data InstrumentData) {
				assert.Equal(t, instrument, data)
			},
		},
		{
			name:     "Not Found",
			exchange: "binance",
			symbol:   "UNKNOWN/USDT",
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT").
					WithArgs("binance", "UNKNOWN/USDT").
					WillReturnError(sql.ErrNoRows)
			},
			expectedErrContains: "instrument not found",
		},
		{
			name:     "Query Error",
			exchange: instrument.ExchangeName,
			symbol:   instrument.Name,
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT").
					WithArgs(instrument.ExchangeName, instrument.Name).
					WillReturnError(errors.New("db error"))
			},
			expectedErrContains: "failed to get instrument",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mock, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mock.Close()

			tc.setupMock(mock)
			data, err := repo.GetInstrument(context.Background(), mock, tc.exchange, tc.symbol)

			if tc.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrContains)
			} else {
				require.NoError(t, err)
				tc.validate(t, data)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
