package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPgMarketDataRepo_GetMarketDataTicks(t *testing.T) {
	repo := NewMarketDataRepo()

	now := time.Now().Unix()
	testCases := []struct {
		name                string
		exchange            string
		symbol              string
		limit               int
		setupMock           func(mockDB pgxmock.PgxPoolIface)
		expectedErrContains string
		assertResult        func(t *testing.T, ticks []MarketDataTick)
	}{
		{
			name:     "Success",
			exchange: "binance",
			symbol:   "BTC/USDT",
			limit:    2,
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"tick_unix_at", "price"}).
					AddRow(now-60, 50000.0).
					AddRow(now, 50001.0)
				mockDB.ExpectQuery("SELECT tick_unix_at, price").
					WithArgs("binance", "BTC/USDT", 2).
					WillReturnRows(rows)
			},
			assertResult: func(t *testing.T, ticks []MarketDataTick) {
				assert.Len(t, ticks, 2)
				assert.Equal(t, 50000.0, ticks[0].Price)
			},
		},
		{
			name:     "Query Error",
			exchange: "binance",
			symbol:   "BTC/USDT",
			limit:    2,
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT tick_unix_at, price").
					WithArgs("binance", "BTC/USDT", 2).
					WillReturnError(errors.New("db query error"))
			},
			expectedErrContains: "failed to get market data ticks",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDB, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mockDB.Close()

			tc.setupMock(mockDB)
			result, err := repo.GetMarketDataTicks(context.Background(), mockDB, tc.exchange, tc.symbol, tc.limit)

			if tc.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrContains)
			} else {
				require.NoError(t, err)
				tc.assertResult(t, result)
			}
			require.NoError(t, mockDB.ExpectationsWereMet())
		})
	}
}

func TestPgMarketDataRepo_InsertTick(t *testing.T) {
	repo := NewMarketDataRepo()
	tick := MarketDataTick{
		ExchangeName: "binance",
		Symbol:       "BTC/USDT",
		TickUnixAt:   time.Now().Unix(),
		Price:        50000.0,
	}

	testCases := []struct {
		name                string
		setupMock           func(mockDB pgxmock.PgxPoolIface)
		expectedErrContains string
	}{
		{
			name: "Success - New Tick",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT i.exchange_id, i.id, t.tick_unix_at").
					WithArgs(tick.ExchangeName, tick.Symbol, tick.TickUnixAt).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id", "tick_unix_at"}).AddRow(int64(1), int64(10), nil))

				mockDB.ExpectExec("INSERT INTO trading.market_data_ticks").
					WithArgs(int64(1), int64(10), tick.TickUnixAt, tick.Price).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
			},
		},
		{
			name: "Success - Duplicate Skip",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT i.exchange_id, i.id, t.tick_unix_at").
					WithArgs(tick.ExchangeName, tick.Symbol, tick.TickUnixAt).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id", "tick_unix_at"}).AddRow(int64(1), int64(10), int64(999)))
			},
		},
		{
			name: "Resolution Error",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT i.exchange_id, i.id, t.tick_unix_at").
					WithArgs(tick.ExchangeName, tick.Symbol, tick.TickUnixAt).
					WillReturnError(errors.New("db resolution error"))
			},
			expectedErrContains: "failed to resolve exchange and instrument IDs",
		},
		{
			name: "Insert Error",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT i.exchange_id, i.id, t.tick_unix_at").
					WithArgs(tick.ExchangeName, tick.Symbol, tick.TickUnixAt).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id", "tick_unix_at"}).AddRow(int64(1), int64(10), nil))
				mockDB.ExpectExec("INSERT INTO trading.market_data_ticks").
					WithArgs(int64(1), int64(10), tick.TickUnixAt, tick.Price).
					WillReturnError(errors.New("db insert error"))
			},
			expectedErrContains: "failed to insert market data tick",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDB, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mockDB.Close()

			tc.setupMock(mockDB)
			err = repo.InsertTick(context.Background(), mockDB, tick)

			if tc.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrContains)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mockDB.ExpectationsWereMet())
		})
	}
}
