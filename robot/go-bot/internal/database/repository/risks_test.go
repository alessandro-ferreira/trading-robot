package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPgRisksRepo_GetRiskPair(t *testing.T) {
	repo := NewRisksRepo()

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
			exchange: "binance",
			symbol:   "BTC/USDT",
			setupMock: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{
					"exchange_name", "instrument_symbol", "risk_per_trade", "max_position_size",
				}).AddRow("binance", "BTC/USDT", 100.0, 1.5)
				mock.ExpectQuery("SELECT").WithArgs("binance", "BTC/USDT").WillReturnRows(rows)
			},
			validate: func(t *testing.T, data RiskPair) {
				assert.Equal(t, 100.0, data.RiskPerTrade)
				assert.Equal(t, 1.5, data.MaxPositionSize.Float64)
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
	data := RiskPair{
		ExchangeName:     "binance",
		InstrumentSymbol: "BTC/USDT",
		RiskPerTrade:     100.0,
		MaxPositionSize:  sql.NullFloat64{Float64: 1.5, Valid: true},
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
					WithArgs(data.ExchangeName, data.InstrumentSymbol).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(int64(1), int64(10)))

				mock.ExpectQuery("UPDATE trading.risk_pairs").
					WithArgs(int64(1), int64(10), data.RiskPerTrade, data.MaxPositionSize, DefaultUser).
					WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(100)))
			},
		},
		{
			name: "Success - Insert",
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(data.ExchangeName, data.InstrumentSymbol).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(int64(1), int64(10)))

				mock.ExpectQuery("UPDATE trading.risk_pairs").
					WithArgs(int64(1), int64(10), data.RiskPerTrade, data.MaxPositionSize, DefaultUser).
					WillReturnError(sql.ErrNoRows)
				mock.ExpectExec("INSERT INTO trading.risk_pairs").
					WithArgs(int64(1), int64(10), data.RiskPerTrade, data.MaxPositionSize, DefaultUser).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
			},
		},
		{
			name: "Fail - Resolution",
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(data.ExchangeName, data.InstrumentSymbol).
					WillReturnError(errors.New("res error"))
			},
			expectedErrContains: "failed to resolve exchange and instrument IDs",
		},
		{
			name: "Fail - Update Error",
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(data.ExchangeName, data.InstrumentSymbol).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(int64(1), int64(10)))

				mock.ExpectQuery("UPDATE trading.risk_pairs").
					WithArgs(int64(1), int64(10), data.RiskPerTrade, data.MaxPositionSize, DefaultUser).
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
			err = repo.UpsertRiskPair(context.Background(), mock, data)

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
