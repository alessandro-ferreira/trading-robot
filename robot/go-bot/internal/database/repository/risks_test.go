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

var riskColumns = []string{"exchange_name", "instrument_symbol", "risk_per_trade", "max_position_size", "created_at", "updated_at"}

func getSampleRisk() RiskPair {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	// Truncate to seconds to avoid precision issues with database timestamp comparisons
	now := time.Now().Truncate(time.Second)
	return RiskPair{
		ExchangeName:     "binance",
		InstrumentSymbol: "BTC/USDT",
		RiskPerTrade:     r.Float64() * 500,
		MaxPositionSize:  sql.NullFloat64{Float64: r.Float64() * 5, Valid: true},
		CreatedAt:        now,
		UpdatedAt:        sql.NullTime{Time: now, Valid: true},
	}
}

func toRiskRow(rp RiskPair) []any {
	return []any{rp.ExchangeName, rp.InstrumentSymbol, rp.RiskPerTrade, rp.MaxPositionSize, rp.CreatedAt, rp.UpdatedAt}
}

func TestPgRisksRepo_GetRiskPair(t *testing.T) {
	repo := NewRisksRepo()
	columns := riskColumns
	risk := getSampleRisk()

	testCases := []struct {
		name                string
		exchange            string
		symbol              string
		setupMock           func(mock pgxmock.PgxPoolIface)
		expectedErrContains string
		validate            func(t *testing.T, data RiskPair)
	}{
		{
			name:     "Success",
			exchange: risk.ExchangeName,
			symbol:   risk.InstrumentSymbol,
			setupMock: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(columns).AddRow(toRiskRow(risk)...)
				mock.ExpectQuery("SELECT").WithArgs(risk.ExchangeName, risk.InstrumentSymbol).WillReturnRows(rows)
			},
			validate: func(t *testing.T, data RiskPair) {
				assert.Equal(t, risk.RiskPerTrade, data.RiskPerTrade)
				assert.Equal(t, risk.MaxPositionSize, data.MaxPositionSize)
			},
		},
		{
			name:     "Not Found",
			exchange: "binance",
			symbol:   "UNKNOWN/USDT",
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT").WithArgs("binance", "UNKNOWN/USDT").WillReturnError(sql.ErrNoRows)
			},
			expectedErrContains: "risk configuration not found",
		},
		{
			name:     "Query Error",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT").
					WithArgs("binance", "BTC/USDT").
					WillReturnError(errors.New("db error"))
			},
			expectedErrContains: "failed to get risk pair",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mock, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mock.Close()

			tc.setupMock(mock)
			data, err := repo.GetRiskPair(context.Background(), mock, tc.exchange, tc.symbol)

			if tc.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrContains)
			} else {
				require.NoError(t, err)
				tc.validate(t, data)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestPgRisksRepo_UpsertRiskPair(t *testing.T) {
	repo := NewRisksRepo()
	risk := getSampleRisk()

	exchangeID := int64(1)
	instrumentID := int64(10)

	// Internal lambda for argument mapping to ensure encapsulation.
	toUpsertArgs := func(rp RiskPair) []any {
		return []any{exchangeID, instrumentID, rp.RiskPerTrade, rp.MaxPositionSize, DefaultUser}
	}

	testCases := []struct {
		name                string
		setupMock           func(mock pgxmock.PgxPoolIface)
		expectedErrContains string
	}{
		{
			name: "Success - Update",
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(risk.ExchangeName, risk.InstrumentSymbol).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(exchangeID, instrumentID))

				mock.ExpectQuery("UPDATE trading.risk_pairs").
					WithArgs(toUpsertArgs(risk)...).
					WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(100)))
			},
		},
		{
			name: "Success - Insert",
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(risk.ExchangeName, risk.InstrumentSymbol).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(exchangeID, instrumentID))

				mock.ExpectQuery("UPDATE trading.risk_pairs").
					WithArgs(toUpsertArgs(risk)...).
					WillReturnError(sql.ErrNoRows)
				mock.ExpectExec("INSERT INTO trading.risk_pairs").
					WithArgs(toUpsertArgs(risk)...).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
			},
		},
		{
			name: "Fail - Resolution",
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(risk.ExchangeName, risk.InstrumentSymbol).
					WillReturnError(errors.New("res error"))
			},
			expectedErrContains: "failed to resolve exchange and instrument IDs",
		},
		{
			name: "Fail - Update Error",
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(risk.ExchangeName, risk.InstrumentSymbol).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(exchangeID, instrumentID))

				mock.ExpectQuery("UPDATE trading.risk_pairs").
					WithArgs(toUpsertArgs(risk)...).
					WillReturnError(errors.New("db error"))
			},
			expectedErrContains: "failed to update risk pair",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mock, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mock.Close()

			tc.setupMock(mock)
			err = repo.UpsertRiskPair(context.Background(), mock, risk)

			if tc.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrContains)
			} else {
				require.NoError(t, err)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
