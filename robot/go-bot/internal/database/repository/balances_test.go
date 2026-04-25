package repository

import (
	"context"
	"database/sql"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var balanceColumns = []string{"id", "exchange_name", "asset_symbol", "free", "used", "total", "created_at", "updated_at"}

func getSampleBalance() BalanceData {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	// Truncate to seconds to avoid precision issues with database timestamp comparisons
	now := time.Now().Truncate(time.Second)
	return BalanceData{
		ID:           r.Int63n(1000) + 1,
		ExchangeName: "binance",
		AssetSymbol:  "BTC",
		Free:         r.Float64() * 10,
		Used:         r.Float64() * 2,
		Total:        r.Float64() * 12,
		CreatedAt:    now,
		UpdatedAt:    sql.NullTime{Time: now, Valid: true},
	}
}

func toRow(b BalanceData) []any {
	return []any{b.ID, b.ExchangeName, b.AssetSymbol, b.Free, b.Used, b.Total, b.CreatedAt, b.UpdatedAt}
}

func TestPgBalancesRepo_GetAllBalances(t *testing.T) {
	repo := NewBalancesRepo()
	columns := balanceColumns
	b := getSampleBalance()
	baseRow := toRow(b)

	testCases := []struct {
		name          string
		setupMock     func(mockDB pgxmock.PgxPoolIface)
		assertResults func(t *testing.T, balances []BalanceData, err error)
	}{
		{
			name: "Success",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(columns).AddRow(baseRow...)
				mockDB.ExpectQuery("SELECT b.id, e.name AS exchange_name").WillReturnRows(rows)
			},
			assertResults: func(t *testing.T, balances []BalanceData, err error) {
				require.NoError(t, err)
				require.Len(t, balances, 1)
				assert.Equal(t, b.AssetSymbol, balances[0].AssetSymbol)
				assert.Equal(t, b.UpdatedAt, balances[0].UpdatedAt)
			},
		},
		{
			name: "Success many rows",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				b2 := getSampleBalance()
				b2.AssetSymbol = "ETH"
				row2 := toRow(b2)

				rows := pgxmock.NewRows(columns).
					AddRow(baseRow...).AddRow(row2...)
				mockDB.ExpectQuery("SELECT b.id, e.name AS exchange_name").WillReturnRows(rows)
			},
			assertResults: func(t *testing.T, balances []BalanceData, err error) {
				require.NoError(t, err)
				require.Len(t, balances, 2)
				assert.Equal(t, "ETH", balances[1].AssetSymbol)
				assert.Equal(t, b.UpdatedAt, balances[1].UpdatedAt)
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

func TestPgBalancesRepo_UpsertBalance(t *testing.T) {
	repo := NewBalancesRepo()
	b := getSampleBalance()
	upsertArgs := []any{b.ExchangeName, b.AssetSymbol, b.Free, b.Used, b.Total, DefaultUser}

	testCases := []struct {
		name                string
		setupMock           func(mockDB pgxmock.PgxPoolIface)
		expectedID          int64
		expectedErrContains string
	}{
		{
			name: "Success on Update",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"id"}).AddRow(int64(1))
				mockDB.ExpectQuery("UPDATE trading.balances").
					WithArgs(upsertArgs...).
					WillReturnRows(rows)
			},
			expectedID: 1,
		},
		{
			name: "Success on Insert",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("UPDATE trading.balances").
					WithArgs(upsertArgs...).
					WillReturnError(pgx.ErrNoRows)

				rows := pgxmock.NewRows([]string{"id"}).AddRow(int64(2))
				mockDB.ExpectQuery("INSERT INTO trading.balances").
					WithArgs(upsertArgs...).
					WillReturnRows(rows)
			},
			expectedID: 2,
		},
		{
			name: "Error on Update",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("UPDATE trading.balances").
					WithArgs(upsertArgs...).
					WillReturnError(errors.New("db update error"))
			},
			expectedErrContains: "failed to update balance",
		},
		{
			name: "Error on Insert",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("UPDATE trading.balances").
					WithArgs(upsertArgs...).
					WillReturnError(pgx.ErrNoRows)
				mockDB.ExpectQuery("INSERT INTO trading.balances").
					WithArgs(upsertArgs...).
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

			id, err := repo.UpsertBalance(context.Background(), mockDB, b)

			if tc.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedID, id)
			}

			require.NoError(t, mockDB.ExpectationsWereMet())
		})
	}
}
