package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPgRisksRepo_GetRiskPair(t *testing.T) {
	repo := NewRisksRepo()
	mockDB, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockDB.Close()

	t.Run("Success", func(t *testing.T) {
		rows := pgxmock.NewRows([]string{
			"exchange_name", "instrument_symbol", "risk_per_trade", "max_position_size",
		}).AddRow("binance", "BTC/USDT", 100.0, 1.5)

		mockDB.ExpectQuery("SELECT").
			WithArgs("binance", "BTC/USDT").
			WillReturnRows(rows)

		data, err := repo.GetRiskPair(context.Background(), mockDB, "binance", "BTC/USDT")
		require.NoError(t, err)
		assert.Equal(t, 100.0, data.RiskPerTrade)
		assert.True(t, data.MaxPositionSize.Valid)
		assert.Equal(t, 1.5, data.MaxPositionSize.Float64)
		assert.NoError(t, mockDB.ExpectationsWereMet())
	})

	t.Run("Not Found", func(t *testing.T) {
		mockDB.ExpectQuery("SELECT").
			WithArgs("binance", "UNKNOWN/USDT").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.GetRiskPair(context.Background(), mockDB, "binance", "UNKNOWN/USDT")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "risk configuration not found")
	})
}
