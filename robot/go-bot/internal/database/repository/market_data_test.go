//go:build unit

package repository

import (
	"context"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var marketDataColumns = []string{"tick_unix_at", "price"}

func getSampleTick() MarketDataTick {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return MarketDataTick{
		ExchangeName: "binance",
		Symbol:       "BTC/USDT",
		TickUnixAt:   time.Now().Unix() - r.Int63n(10000),
		Price:        r.Float64() * 50000,
	}
}

func toTickRow(t MarketDataTick) []any {
	return []any{t.TickUnixAt, t.Price}
}

func TestPgMarketDataRepo_GetMarketDataTicks(t *testing.T) {
	repo := NewMarketDataRepo()
	tick1 := getSampleTick()
	tick2 := getSampleTick()
	tick2.TickUnixAt = tick1.TickUnixAt + 60
	columns := marketDataColumns

	testCases := []struct {
		name                string
		exchange            string
		symbol              string
		sinceEpoch          int64
		setupMock           func(mockDB pgxmock.PgxPoolIface)
		expectedErrContains string
		assertResult        func(t *testing.T, ticks []MarketDataTick)
	}{
		{
			name:       "Success",
			exchange:   tick1.ExchangeName,
			symbol:     tick1.Symbol,
			sinceEpoch: 1000,
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(columns).
					AddRow(toTickRow(tick1)...).
					AddRow(toTickRow(tick2)...)
				mockDB.ExpectQuery("SELECT t.tick_unix_at, t.price").
					WithArgs(tick1.ExchangeName, tick1.Symbol, int64(1000)).
					WillReturnRows(rows)
			},
			assertResult: func(t *testing.T, ticks []MarketDataTick) {
				assert.Len(t, ticks, 2)
				assert.Equal(t, tick1.Price, ticks[0].Price)
			},
		},
		{
			name:       "Query Error",
			exchange:   "binance",
			symbol:     "BTC/USDT",
			sinceEpoch: 1000,
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT t.tick_unix_at, t.price").
					WithArgs("binance", "BTC/USDT", int64(1000)).
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
			result, err := repo.GetMarketDataTicks(context.Background(), mockDB, tc.exchange, tc.symbol, tc.sinceEpoch)

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
	tick := getSampleTick()

	exchangeID := int64(1)
	instrumentID := int64(10)
	resolveArgs := []any{tick.ExchangeName, tick.Symbol, tick.TickUnixAt}
	insertArgs := []any{exchangeID, instrumentID, tick.TickUnixAt, tick.Price}

	testCases := []struct {
		name                string
		setupMock           func(mockDB pgxmock.PgxPoolIface)
		expectedErrContains string
	}{
		{
			name: "Success - New Tick",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT i.exchange_id, i.id, t.tick_unix_at").
					WithArgs(resolveArgs...).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id", "tick_unix_at"}).AddRow(exchangeID, instrumentID, nil))

				mockDB.ExpectExec("INSERT INTO trading.market_data_ticks").
					WithArgs(insertArgs...).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
			},
		},
		{
			name: "Success - Duplicate Skip",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT i.exchange_id, i.id, t.tick_unix_at").
					WithArgs(resolveArgs...).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id", "tick_unix_at"}).AddRow(exchangeID, instrumentID, tick.TickUnixAt))
			},
		},
		{
			name: "Resolution Error",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT i.exchange_id, i.id, t.tick_unix_at").
					WithArgs(resolveArgs...).
					WillReturnError(errors.New("db resolution error"))
			},
			expectedErrContains: "failed to resolve exchange and instrument IDs",
		},
		{
			name: "Insert Error",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT i.exchange_id, i.id, t.tick_unix_at").
					WithArgs(resolveArgs...).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id", "tick_unix_at"}).AddRow(exchangeID, instrumentID, nil))
				mockDB.ExpectExec("INSERT INTO trading.market_data_ticks").
					WithArgs(insertArgs...).
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
