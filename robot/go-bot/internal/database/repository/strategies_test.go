package repository

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var strategyPairColumns = []string{
	"exchange_name", "instrument_symbol", "strategy_type", "window_seconds",
	"lookbacks", "thresholds", "require_all", "stop_loss_pct", "profit_target_pct",
	"activation_pct", "trailing_stop_pct",
}

func getSampleMomentum() StrategyMomentum {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return StrategyMomentum{
		WindowSeconds: int(r.Int31n(3600) + 300),
		Windows: []MomentumWindow{
			{LookbackSeconds: 60, Threshold: r.Float64() * 0.05},
			{LookbackSeconds: 120, Threshold: r.Float64() * 0.1},
		},
		RequireAll:      r.Intn(2) == 0,
		StopLossPct:     r.Float64() * 0.05,
		ProfitTargetPct: r.Float64() * 0.1,
		ActivationPct:   r.Float64() * 0.02,
		TrailingStopPct: r.Float64() * 0.01,
	}
}

func getSampleStrategyPair() StrategyPair {
	return StrategyPair{
		ExchangeName:     "binance",
		InstrumentSymbol: "BTC/USDT",
		Type:             StrategyMomentumTrailing,
		WarmupWindow:     300,
		Momentum:         getSampleMomentum(),
	}
}

func toStrategyPairRow(p StrategyPair) []any {
	lookbacks := make([]int32, len(p.Momentum.Windows))
	thresholds := make([]float64, len(p.Momentum.Windows))
	for i, w := range p.Momentum.Windows {
		lookbacks[i] = int32(w.LookbackSeconds)
		thresholds[i] = w.Threshold
	}

	var windowSec any = int32(p.Momentum.WindowSeconds)
	if p.Type == StrategyDummy {
		windowSec = nil
	}

	return []any{
		p.ExchangeName, p.InstrumentSymbol, p.Type, windowSec,
		lookbacks, thresholds, p.Momentum.RequireAll, p.Momentum.StopLossPct,
		p.Momentum.ProfitTargetPct, p.Momentum.ActivationPct, p.Momentum.TrailingStopPct,
	}
}

func TestPgStrategiesRepo_GetStrategyPairs(t *testing.T) {
	repo := NewStrategiesRepo()
	columns := strategyPairColumns
	p1 := getSampleStrategyPair()

	tests := []struct {
		name          string
		setupMock     func(mock pgxmock.PgxPoolIface)
		expectedCount int
		validate      func(t *testing.T, results []StrategyPair)
	}{
		{
			name: "0 active pairs",
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT").WithArgs(true).WillReturnRows(pgxmock.NewRows(columns))
			},
			expectedCount: 0,
		},
		{
			name: "1 dummy pair (no momentum config)",
			setupMock: func(mock pgxmock.PgxPoolIface) {
				d := p1
				d.Type = StrategyDummy
				d.Momentum = StrategyMomentum{}
				rows := pgxmock.NewRows(columns).AddRow(toStrategyPairRow(d)...)
				mock.ExpectQuery("SELECT").WithArgs(true).WillReturnRows(rows)
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
				m := p1
				m.Momentum.Windows = m.Momentum.Windows[:1]
				rows := pgxmock.NewRows(columns).AddRow(toStrategyPairRow(m)...)
				mock.ExpectQuery("SELECT").WithArgs(true).WillReturnRows(rows)
			},
			expectedCount: 1,
			validate: func(t *testing.T, results []StrategyPair) {
				assert.Equal(t, p1.Type, results[0].Type)
				require.Len(t, results[0].Momentum.Windows, 1)
			},
		},
		{
			name: "1 momentum pair (n > 1 windows)",
			setupMock: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(columns).AddRow(toStrategyPairRow(p1)...)
				mock.ExpectQuery("SELECT").WithArgs(true).WillReturnRows(rows)
			},
			expectedCount: 1,
			validate: func(t *testing.T, results []StrategyPair) {
				require.Len(t, results[0].Momentum.Windows, 2)
			},
		},
		{
			name: "Mixed pairs",
			setupMock: func(mock pgxmock.PgxPoolIface) {
				p2 := getSampleStrategyPair()
				p2.ExchangeName = "kraken"
				p2.Type = StrategyMomentumProfit

				rows := pgxmock.NewRows(columns).
					AddRows(toStrategyPairRow(p1), toStrategyPairRow(p2))
				mock.ExpectQuery("SELECT").WithArgs(true).WillReturnRows(rows)
			},
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mockDB.Close()

			tt.setupMock(mockDB)

			// Testing with onlyEnabled=true to match typical usage
			results, err := repo.GetStrategyPairs(context.Background(), mockDB, true)
			require.NoError(t, err)
			assert.Len(t, results, tt.expectedCount)

			if tt.validate != nil {
				tt.validate(t, results)
			}
			require.NoError(t, mockDB.ExpectationsWereMet())
		})
	}

	t.Run("Query Error", func(t *testing.T) {
		mockDB, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mockDB.Close()

		mockDB.ExpectQuery("SELECT").WithArgs(true).WillReturnError(fmt.Errorf("db error"))
		_, err = repo.GetStrategyPairs(context.Background(), mockDB, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to query strategy pairs")
	})
}

func TestPgStrategiesRepo_UpsertEnabledStrategy(t *testing.T) {
	repo := NewStrategiesRepo()
	exchange := "binance"
	symbol := "BTC/USDT"
	strategyType := StrategyMomentumProfit
	label := "default"

	momentum := getSampleMomentum()
	exchangeID := int64(1)
	instrumentID := int64(10)
	pairID := int64(100)

	// Argument mapping lambdas to ensure tests are dry and stay in sync with repo logic
	lookbacks := []int32{int32(momentum.Windows[0].LookbackSeconds), int32(momentum.Windows[1].LookbackSeconds)}
	thresholds := []float64{momentum.Windows[0].Threshold, momentum.Windows[1].Threshold}

	toPairUpdateArgs := func(st string) []any {
		return []any{exchangeID, instrumentID, st, DefaultUser}
	}
	toMomArgs := func(st string, isInsert bool) []any {
		profitValid := st == StrategyMomentumProfit
		trailValid := st == StrategyMomentumTrailing

		commonArgs := []any{
			momentum.WindowSeconds, lookbacks, thresholds,
			momentum.RequireAll, momentum.StopLossPct,
			sql.NullFloat64{Float64: momentum.ProfitTargetPct, Valid: profitValid},
			sql.NullFloat64{Float64: momentum.ActivationPct, Valid: trailValid},
			sql.NullFloat64{Float64: momentum.TrailingStopPct, Valid: trailValid},
			DefaultUser,
		}

		if isInsert {
			// Insert query starts with label and pairID
			return append([]any{label, pairID, st}, commonArgs...)
		}
		// Update query starts with IDs/Label and ends with DefaultUser
		return append([]any{pairID, st, label}, commonArgs...)
	}

	disablePairArgs := []any{exchangeID, instrumentID, DefaultUser}
	disableMomArgs := []any{pairID, strategyType, DefaultUser}

	testCases := []struct {
		name                string
		strategyType        string
		setupMock           func(mock pgxmock.PgxPoolIface)
		expectedErrContains string
	}{
		{
			name:         "Success - New pair and new momentum",
			strategyType: strategyType,
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(exchange, symbol).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(exchangeID, instrumentID))

				mock.ExpectBegin()
				mock.ExpectExec("UPDATE trading.strategy_pairs").
					WithArgs(disablePairArgs...).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))

				mock.ExpectQuery("UPDATE trading.strategy_pairs").
					WithArgs(toPairUpdateArgs(strategyType)...).
					WillReturnError(sql.ErrNoRows)

				mock.ExpectQuery("INSERT INTO trading.strategy_pairs").
					WithArgs(toPairUpdateArgs(strategyType)...).
					WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(pairID))

				mock.ExpectExec("UPDATE trading.strategy_momentum").
					WithArgs(disableMomArgs...).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))

				mock.ExpectQuery("UPDATE trading.strategy_momentum").
					WithArgs(toMomArgs(strategyType, false)...).
					WillReturnError(sql.ErrNoRows)

				mock.ExpectExec("INSERT INTO trading.strategy_momentum").
					WithArgs(toMomArgs(strategyType, true)...).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
				mock.ExpectCommit()
			},
		},
		{
			name:         "Success - Existing pair and momentum",
			strategyType: strategyType,
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(exchange, symbol).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(exchangeID, instrumentID))

				mock.ExpectBegin()
				mock.ExpectExec("UPDATE trading.strategy_pairs").
					WithArgs(disablePairArgs...).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				mock.ExpectQuery("UPDATE trading.strategy_pairs").
					WithArgs(toPairUpdateArgs(strategyType)...).
					WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(pairID))
				mock.ExpectExec("UPDATE trading.strategy_momentum").
					WithArgs(disableMomArgs...).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				mock.ExpectQuery("UPDATE trading.strategy_momentum").
					WithArgs(toMomArgs(strategyType, false)...).
					WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(500)))
				mock.ExpectCommit()
			},
		},
		{
			name:         "Success - Dummy strategy",
			strategyType: StrategyDummy,
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(exchange, symbol).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(exchangeID, instrumentID))

				mock.ExpectBegin()
				mock.ExpectExec("UPDATE trading.strategy_pairs").
					WithArgs(disablePairArgs...).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				mock.ExpectQuery("UPDATE trading.strategy_pairs").
					WithArgs(toPairUpdateArgs(StrategyDummy)...).
					WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(pairID))
				mock.ExpectCommit()
			},
		},
		{
			name:         "Fail - ID Resolution",
			strategyType: strategyType,
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(exchange, symbol).
					WillReturnError(fmt.Errorf("resolution error"))
			},
			expectedErrContains: "failed to resolve exchange and instrument IDs",
		},
		{
			name:         "Fail - Transaction Start",
			strategyType: strategyType,
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(exchange, symbol).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(exchangeID, instrumentID))
				mock.ExpectBegin().WillReturnError(fmt.Errorf("begin error"))
			},
			expectedErrContains: "failed to begin transaction",
		},
		{
			name:         "Fail - Update Pair Error",
			strategyType: strategyType,
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(exchange, symbol).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(exchangeID, instrumentID))
				mock.ExpectBegin()
				mock.ExpectExec("UPDATE trading.strategy_pairs").
					WithArgs(disablePairArgs...).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				mock.ExpectQuery("UPDATE trading.strategy_pairs").
					WithArgs(toPairUpdateArgs(strategyType)...).
					WillReturnError(fmt.Errorf("db error"))
				mock.ExpectRollback()
			},
			expectedErrContains: "failed to update strategy pair",
		},
		{
			name:         "Fail - Momentum Update Error",
			strategyType: strategyType,
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(exchange, symbol).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(exchangeID, instrumentID))
				mock.ExpectBegin()
				mock.ExpectExec("UPDATE trading.strategy_pairs").
					WithArgs(disablePairArgs...).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				mock.ExpectQuery("UPDATE trading.strategy_pairs").
					WithArgs(toPairUpdateArgs(strategyType)...).
					WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(pairID))
				mock.ExpectExec("UPDATE trading.strategy_momentum").
					WithArgs(disableMomArgs...).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				mock.ExpectQuery("UPDATE trading.strategy_momentum").
					WithArgs(toMomArgs(strategyType, false)...).
					WillReturnError(fmt.Errorf("db error"))
				mock.ExpectRollback()
			},
			expectedErrContains: "failed to update momentum configuration",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDB, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mockDB.Close()

			tc.setupMock(mockDB)

			err = repo.UpsertEnabledStrategy(context.Background(), mockDB, exchange, symbol, tc.strategyType, label, momentum)

			if tc.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrContains)
			} else {
				require.NoError(t, err)
			}

			if err := mockDB.ExpectationsWereMet(); err != nil {
				t.Errorf("there were unfulfilled expectations: %s", err)
			}
		})
	}
}
