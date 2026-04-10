package repository

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPgMarketDataRepo_GetMarketDataTicks(t *testing.T) {
	repo := NewMarketDataRepo()
	mockDB, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockDB.Close()

	t.Run("Success", func(t *testing.T) {
		now := time.Now().Unix()
		rows := pgxmock.NewRows([]string{"tick_unix_at", "price"}).
			AddRow(now-60, 50000.0).
			AddRow(now, 50001.0)

		mockDB.ExpectQuery("SELECT tick_unix_at, price").
			WithArgs("binance", "BTC/USDT", 2).
			WillReturnRows(rows)

		ticks, err := repo.GetMarketDataTicks(context.Background(), mockDB, "binance", "BTC/USDT", 2)
		require.NoError(t, err)
		assert.Len(t, ticks, 2)
		assert.Equal(t, 50000.0, ticks[0].Price)
		assert.NoError(t, mockDB.ExpectationsWereMet())
	})
}

func TestPgMarketDataRepo_InsertTick(t *testing.T) {
	repo := NewMarketDataRepo()
	mockDB, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockDB.Close()

	t.Run("Success", func(t *testing.T) {
		tick := MarketDataTick{
			ExchangeName: "binance",
			Symbol:       "BTC/USDT",
			TickUnixAt:   time.Now().Unix(),
			Price:        50000.0,
		}

		// Mock check for existence returning no rows
		mockDB.ExpectQuery("SELECT 1 FROM trading.market_data_ticks").
			WithArgs(
				tick.ExchangeName,
				tick.Symbol,
				tick.TickUnixAt,
			).WillReturnError(pgx.ErrNoRows)

		mockDB.ExpectExec("INSERT INTO trading.market_data_ticks").
			WithArgs(
				tick.ExchangeName,
				tick.Symbol,
				tick.TickUnixAt,
				tick.Price,
			).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		err := repo.InsertTick(context.Background(), mockDB, tick)
		assert.NoError(t, err)
		assert.NoError(t, mockDB.ExpectationsWereMet())
	})
}
