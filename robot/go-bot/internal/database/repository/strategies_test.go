package repository

import (
	"context"
	"fmt"
	"testing"

	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPgStrategiesRepo_GetActiveStrategyPairs(t *testing.T) {
	repo := NewStrategiesRepo()
	mockDB, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockDB.Close()

	columns := []string{
		"exchange_name", "instrument_symbol", "strategy_type", "window_seconds",
		"lookbacks", "thresholds", "require_all", "stop_loss_pct", "profit_target_pct",
		"activation_pct", "trailing_stop_pct",
	}

	tests := []struct {
		name          string
		setupMock     func(mock pgxmock.PgxPoolIface)
		expectedCount int
		validate      func(t *testing.T, results []StrategyPair)
	}{
		{
			name: "0 active pairs",
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT").WillReturnRows(pgxmock.NewRows(columns))
			},
			expectedCount: 0,
		},
		{
			name: "1 dummy pair (no momentum config)",
			setupMock: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(columns).
					AddRow("binance", "BTC/USDT", "dummy", nil, nil, nil, nil, nil, nil, nil, nil)
				mock.ExpectQuery("SELECT").WillReturnRows(rows)
			},
			expectedCount: 1,
			validate: func(t *testing.T, results []StrategyPair) {
				assert.Equal(t, "dummy", results[0].Type)
				assert.Equal(t, DefaultWarmupWindow, results[0].WarmupWindow)
				assert.Empty(t, results[0].Momentum.Windows)
			},
		},
		{
			name: "1 momentum pair (1 window)",
			setupMock: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(columns).
					AddRow("binance", "BTC/USDT", "momentum_trailing", 3600, []int32{60}, []float64{0.01}, true, 0.02, nil, 0.03, 0.01)
				mock.ExpectQuery("SELECT").WillReturnRows(rows)
			},
			expectedCount: 1,
			validate: func(t *testing.T, results []StrategyPair) {
				assert.Equal(t, "momentum_trailing", results[0].Type)
				assert.Equal(t, 3600, results[0].WarmupWindow)
				assert.Equal(t, 3600, results[0].Momentum.WindowSeconds)
				require.Len(t, results[0].Momentum.Windows, 1)
				assert.Equal(t, 60, results[0].Momentum.Windows[0].LookbackSeconds)
				assert.Equal(t, 0.01, results[0].Momentum.Windows[0].Threshold)
			},
		},
		{
			name: "1 momentum pair (n > 1 windows)",
			setupMock: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(columns).
					AddRow("binance", "BTC/USDT", "momentum_trailing", 3600, []int32{60, 120}, []float64{0.01, 0.02}, true, 0.02, nil, 0.03, 0.01)
				mock.ExpectQuery("SELECT").WillReturnRows(rows)
			},
			expectedCount: 1,
			validate: func(t *testing.T, results []StrategyPair) {
				require.Len(t, results[0].Momentum.Windows, 2)
				assert.Equal(t, 60, results[0].Momentum.Windows[0].LookbackSeconds)
				assert.Equal(t, 120, results[0].Momentum.Windows[1].LookbackSeconds)
			},
		},
		{
			name: "Mixed pairs",
			setupMock: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(columns).
					AddRow("binance", "BTC/USDT", "dummy", nil, nil, nil, nil, nil, nil, nil, nil).
					AddRow("kraken", "ETH/USDT", "momentum_profit", 300, []int32{60}, []float64{0.01}, false, 0.02, 0.05, nil, nil)
				mock.ExpectQuery("SELECT").WillReturnRows(rows)
			},
			expectedCount: 2,
			validate: func(t *testing.T, results []StrategyPair) {
				assert.Equal(t, "dummy", results[0].Type)
				assert.Equal(t, "momentum_profit", results[1].Type)
				assert.Equal(t, 300, results[1].WarmupWindow)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock(mockDB)

			results, err := repo.GetActiveStrategyPairs(context.Background(), mockDB)
			require.NoError(t, err)
			assert.Len(t, results, tt.expectedCount)

			if tt.validate != nil {
				tt.validate(t, results)
			}
			require.NoError(t, mockDB.ExpectationsWereMet())
		})
	}

	t.Run("Query Error", func(t *testing.T) {
		mockDB.ExpectQuery("SELECT").WillReturnError(fmt.Errorf("db error"))
		_, err := repo.GetActiveStrategyPairs(context.Background(), mockDB)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to query active strategy pairs")
	})
}
