package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getSamplePosition() PositionData {
	return PositionData{
		ID:               1,
		ExchangeName:     "binance",
		InstrumentSymbol: "BTC/USDT",
		Side:             PositionSideLong,
		Quantity:         0.5,
		EntryPrice:       50000.0,
		HighestPrice:     52000.0,
		StrategyState:    "active",
		Active:           true,
		CreatedAt:        time.Now(),
		UpdatedAt:        sql.NullTime{Valid: false},
	}
}

func TestPgPositionsRepo_GetPosition(t *testing.T) {
	repo := NewPositionsRepo()
	pos := getSamplePosition()

	testCases := []struct {
		name                string
		setupMock           func(mockDB pgxmock.PgxPoolIface)
		expectedErrContains string
		assertResult        func(t *testing.T, result PositionData)
	}{
		{
			name: "Success",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{
					"id", "exchange_name", "instrument_symbol", "side", "quantity",
					"entry_price", "highest_price", "strategy_state", "active", "created_at", "updated_at",
				}).AddRow(
					pos.ID, pos.ExchangeName, pos.InstrumentSymbol, pos.Side, pos.Quantity,
					pos.EntryPrice, pos.HighestPrice, pos.StrategyState, pos.Active, pos.CreatedAt, pos.UpdatedAt,
				)
				mockDB.ExpectQuery("SELECT p.id, e.name AS exchange_name").
					WithArgs(pos.ExchangeName, pos.InstrumentSymbol).
					WillReturnRows(rows)
			},
			assertResult: func(t *testing.T, result PositionData) {
				assert.Equal(t, pos.ID, result.ID)
				assert.Equal(t, pos.Quantity, result.Quantity)
			},
		},
		{
			name: "Not Found",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT p.id, e.name AS exchange_name").
					WithArgs(pos.ExchangeName, pos.InstrumentSymbol).
					WillReturnError(pgx.ErrNoRows)
			},
			expectedErrContains: "failed to get position",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDB, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mockDB.Close()

			tc.setupMock(mockDB)

			result, err := repo.GetPosition(context.Background(), mockDB, pos.ExchangeName, pos.InstrumentSymbol)

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

func TestPgPositionsRepo_GetOpenPositions(t *testing.T) {
	repo := NewPositionsRepo()
	pos := getSamplePosition()

	testCases := []struct {
		name                string
		setupMock           func(mockDB pgxmock.PgxPoolIface)
		expectedErrContains string
		assertResult        func(t *testing.T, result []PositionData)
	}{
		{
			name: "Success",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{
					"id", "exchange_name", "instrument_symbol", "side", "quantity",
					"entry_price", "highest_price", "strategy_state", "active", "created_at", "updated_at",
				}).AddRow(
					pos.ID, pos.ExchangeName, pos.InstrumentSymbol, pos.Side, pos.Quantity,
					pos.EntryPrice, pos.HighestPrice, pos.StrategyState, pos.Active, pos.CreatedAt, pos.UpdatedAt,
				)
				mockDB.ExpectQuery("SELECT p.id, e.name AS exchange_name").
					WillReturnRows(rows)
			},
			assertResult: func(t *testing.T, result []PositionData) {
				require.Len(t, result, 1)
				assert.Equal(t, pos.ID, result[0].ID)
				assert.Equal(t, pos.Quantity, result[0].Quantity)
			},
		},
		{
			name: "Query Error",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT p.id, e.name AS exchange_name").
					WillReturnError(errors.New("db query error"))
			},
			expectedErrContains: "failed to get open positions",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDB, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mockDB.Close()

			tc.setupMock(mockDB)

			result, err := repo.GetOpenPositions(context.Background(), mockDB)

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

func TestPgPositionsRepo_UpsertPosition(t *testing.T) {
	repo := NewPositionsRepo()
	pos := getSamplePosition()

	testCases := []struct {
		name                string
		setupMock           func(mockDB pgxmock.PgxPoolIface)
		expectedErrContains string
	}{
		{
			name: "Success Update",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(pos.ExchangeName, pos.InstrumentSymbol).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(int64(1), int64(10)))

				rows := pgxmock.NewRows([]string{"id"}).AddRow(pos.ID)
				mockDB.ExpectQuery("UPDATE trading.positions").
					WithArgs(
						int64(1), int64(10), pos.Quantity,
						pos.EntryPrice, pos.HighestPrice,
						pos.StrategyState, DefaultUser,
					).WillReturnRows(rows)
			},
		},
		{
			name: "Success Insert (Update fails with NoRows)",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(pos.ExchangeName, pos.InstrumentSymbol).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(int64(1), int64(10)))

				mockDB.ExpectQuery("UPDATE trading.positions").
					WithArgs(
						int64(1), int64(10), pos.Quantity,
						pos.EntryPrice, pos.HighestPrice,
						pos.StrategyState, DefaultUser,
					).WillReturnError(pgx.ErrNoRows)

				mockDB.ExpectExec("INSERT INTO trading.positions").
					WithArgs(
						int64(1), int64(10), pos.Side, pos.Quantity,
						pos.EntryPrice, pos.HighestPrice,
						pos.StrategyState, DefaultUser,
					).WillReturnResult(pgxmock.NewResult("INSERT", 1))
			},
		},
		{
			name: "Select Error",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(pos.ExchangeName, pos.InstrumentSymbol).
					WillReturnError(errors.New("select id error"))
			},
			expectedErrContains: "failed to resolve exchange and instrument IDs",
		},
		{
			name: "Update Error",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(pos.ExchangeName, pos.InstrumentSymbol).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(int64(1), int64(10)))

				mockDB.ExpectQuery("UPDATE trading.positions").
					WithArgs(
						int64(1), int64(10), pos.Quantity,
						pos.EntryPrice, pos.HighestPrice,
						pos.StrategyState, DefaultUser,
					).WillReturnError(errors.New("db update error"))
			},
			expectedErrContains: "failed to update position",
		},
		{
			name: "Insert Error",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(pos.ExchangeName, pos.InstrumentSymbol).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(int64(1), int64(10)))

				mockDB.ExpectQuery("UPDATE trading.positions").
					WithArgs(
						int64(1), int64(10), pos.Quantity,
						pos.EntryPrice, pos.HighestPrice,
						pos.StrategyState, DefaultUser,
					).
					WillReturnError(pgx.ErrNoRows)

				mockDB.ExpectExec("INSERT INTO trading.positions").
					WithArgs(
						int64(1), int64(10), pos.Side, pos.Quantity,
						pos.EntryPrice, pos.HighestPrice,
						pos.StrategyState, DefaultUser,
					).
					WillReturnError(errors.New("db insert error"))
			},
			expectedErrContains: "failed to insert position",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDB, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mockDB.Close()

			tc.setupMock(mockDB)

			err = repo.UpsertPosition(context.Background(), mockDB, pos)

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

func TestPgPositionsRepo_DeletePosition(t *testing.T) {
	repo := NewPositionsRepo()
	pos := getSamplePosition()

	testCases := []struct {
		name                string
		setupMock           func(mockDB pgxmock.PgxPoolIface)
		expectedErrContains string
	}{
		{
			name: "Success",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(pos.ExchangeName, pos.InstrumentSymbol).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(int64(1), int64(10)))

				mockDB.ExpectExec("UPDATE trading.positions").
					WithArgs(int64(1), int64(10), DefaultUser).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))
			},
		},
		{
			name: "Select Error",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(pos.ExchangeName, pos.InstrumentSymbol).
					WillReturnError(errors.New("select id error"))
			},
			expectedErrContains: "failed to resolve exchange and instrument IDs",
		},
		{
			name: "Update Error",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(pos.ExchangeName, pos.InstrumentSymbol).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(int64(1), int64(10)))

				mockDB.ExpectExec("UPDATE trading.positions").
					WithArgs(int64(1), int64(10), DefaultUser).
					WillReturnError(errors.New("db update error"))
			},
			expectedErrContains: "failed to delete position",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDB, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mockDB.Close()

			tc.setupMock(mockDB)

			err = repo.DeletePosition(context.Background(), mockDB, pos.ExchangeName, pos.InstrumentSymbol)

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
