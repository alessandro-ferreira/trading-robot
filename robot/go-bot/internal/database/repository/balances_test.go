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

func TestPgBalancesRepo_UpsertBalance(t *testing.T) {
	repo := NewBalancesRepo()
	balance := BalanceData{
		ExchangeName: "binance",
		AssetSymbol:  "BTC",
		Free:         1,
		Used:         2,
		Total:        3,
	}

	testCases := []struct {
		name                string
		setupMock           func(mockDB pgxmock.PgxPoolIface)
		expectedErrContains string
	}{
		{
			name: "Success on Update",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectExec("UPDATE trading.balances").
					WithArgs(balance.ExchangeName, balance.AssetSymbol, balance.Free, balance.Used, balance.Total, DefaultUser).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))
			},
		},
		{
			name: "Success on Insert",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectExec("UPDATE trading.balances").
					WithArgs(balance.ExchangeName, balance.AssetSymbol, balance.Free, balance.Used, balance.Total, DefaultUser).
					WillReturnResult(pgxmock.NewResult("UPDATE", 0))
				mockDB.ExpectExec("INSERT INTO trading.balances").
					WithArgs(balance.ExchangeName, balance.AssetSymbol, balance.Free, balance.Used, balance.Total, DefaultUser).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
			},
		},
		{
			name: "Error on Update",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectExec("UPDATE trading.balances").
					WithArgs(balance.ExchangeName, balance.AssetSymbol, balance.Free, balance.Used, balance.Total, DefaultUser).
					WillReturnError(errors.New("db update error"))
			},
			expectedErrContains: "failed to update balance",
		},
		{
			name: "Error on Insert",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectExec("UPDATE trading.balances").
					WithArgs(balance.ExchangeName, balance.AssetSymbol, balance.Free, balance.Used, balance.Total, DefaultUser).
					WillReturnResult(pgxmock.NewResult("UPDATE", 0))
				mockDB.ExpectExec("INSERT INTO trading.balances").
					WithArgs(balance.ExchangeName, balance.AssetSymbol, balance.Free, balance.Used, balance.Total, DefaultUser).
					WillReturnError(errors.New("db insert error"))
			},
			expectedErrContains: "failed to insert balance",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDB, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mockDB.Close()

			tc.setupMock(mockDB)

			err = repo.UpsertBalance(context.Background(), mockDB, balance)

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

func TestPgBalancesRepo_GetAllBalances(t *testing.T) {
	repo := NewBalancesRepo()
	now := time.Now()

	testCases := []struct {
		name          string
		setupMock     func(mockDB pgxmock.PgxPoolIface)
		assertResults func(t *testing.T, balances []BalanceData, err error)
	}{
		{
			name: "Success",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"id", "exchange_name", "asset_symbol", "free", "used", "total", "updated_at", "updated_by"}).
					AddRow(int64(1), "binance", "BTC", 1.0, 0.5, 1.5, now, DefaultUser)
				mockDB.ExpectQuery("SELECT b.id, e.name AS exchange_name").WillReturnRows(rows)
			},
			assertResults: func(t *testing.T, balances []BalanceData, err error) {
				require.NoError(t, err)
				require.Len(t, balances, 1)
				assert.Equal(t, "BTC", balances[0].AssetSymbol)
			},
		},
		{
			name: "Query Error",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT b.id, e.name AS exchange_name").WillReturnError(errors.New("db query error"))
			},
			assertResults: func(t *testing.T, balances []BalanceData, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "db query error")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDB, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mockDB.Close()

			tc.setupMock(mockDB)

			balances, err := repo.GetAllBalances(context.Background(), mockDB)

			tc.assertResults(t, balances, err)

			require.NoError(t, mockDB.ExpectationsWereMet())
		})
	}
}
