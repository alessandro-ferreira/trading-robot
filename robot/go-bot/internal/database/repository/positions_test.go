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

var positionColumns = []string{
	"id", "exchange_name", "instrument_symbol", "order_id", "side", "quantity",
	"entry_price", "highest_price", "stop_loss_block", "unknown_origin", "active", "created_at", "updated_at",
}

func getSamplePosition() PositionData {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	now := time.Now().Truncate(time.Second)
	return PositionData{
		ID:               r.Int63n(1000) + 1,
		ExchangeName:     "binance",
		InstrumentSymbol: "BTC/USDT",
		OrderID:          sql.NullInt64{Int64: r.Int63n(10000), Valid: true},
		Side:             PositionSideLong,
		Quantity:         r.Float64() * 2,
		EntryPrice:       r.Float64() * 50000,
		HighestPrice:     r.Float64() * 52000,
		StopLossBlock:    false,
		UnknownOrigin:    false,
		Active:           true,
		CreatedAt:        now,
		UpdatedAt:        sql.NullTime{Valid: false},
	}
}

func toPositionRow(p PositionData) []any {
	return []any{
		p.ID, p.ExchangeName, p.InstrumentSymbol, p.OrderID, p.Side, p.Quantity,
		p.EntryPrice, p.HighestPrice, p.StopLossBlock, p.UnknownOrigin, p.Active, p.CreatedAt, p.UpdatedAt,
	}
}

func TestPgPositionsRepo_GetPosition(t *testing.T) {
	repo := NewPositionsRepo()
	pos := getSamplePosition()
	columns := positionColumns

	testCases := []struct {
		name                string
		setupMock           func(mockDB pgxmock.PgxPoolIface)
		expectedErrContains string
		assertResult        func(t *testing.T, result PositionData)
	}{
		{
			name: "Success",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(columns).AddRow(toPositionRow(pos)...)

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
		{
			name: "Query Error",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT p.id, e.name AS exchange_name").
					WithArgs(pos.ExchangeName, pos.InstrumentSymbol).
					WillReturnError(errors.New("db query error"))
			},
			expectedErrContains: "db query error",
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

func TestPgPositionsRepo_GetActivePositions(t *testing.T) {
	repo := NewPositionsRepo()
	pos := getSamplePosition()
	columns := positionColumns

	testCases := []struct {
		name                string
		exchangeFilter      string
		symbolFilter        string
		setupMock           func(mockDB pgxmock.PgxPoolIface)
		expectedErrContains string
		assertResult        func(t *testing.T, result []PositionData)
	}{
		{
			name:           "Success",
			exchangeFilter: "",
			symbolFilter:   "",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(columns).AddRow(toPositionRow(pos)...)

				mockDB.ExpectQuery("SELECT p.id, e.name AS exchange_name").
					WithArgs("", "").
					WillReturnRows(rows)
			},
			assertResult: func(t *testing.T, result []PositionData) {
				require.Len(t, result, 1)
				assert.Equal(t, pos.ID, result[0].ID)
				assert.Equal(t, pos.Quantity, result[0].Quantity)
			},
		},
		{
			name:           "Success - Multiple rows",
			exchangeFilter: "binance",
			symbolFilter:   "",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				pos2 := getSamplePosition()
				pos2.InstrumentSymbol = "ETH/USDT"

				rows := pgxmock.NewRows(columns).AddRows(toPositionRow(pos), toPositionRow(pos2))
				mockDB.ExpectQuery("SELECT p.id, e.name AS exchange_name").
					WithArgs("binance", "").
					WillReturnRows(rows)
			},
			assertResult: func(t *testing.T, result []PositionData) {
				require.Len(t, result, 2)
			},
		},
		{
			name:           "Query Error",
			exchangeFilter: "",
			symbolFilter:   "",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT p.id, e.name AS exchange_name").
					WithArgs("", "").
					WillReturnError(errors.New("db query error"))
			},
			expectedErrContains: "failed to get active positions",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDB, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mockDB.Close()

			tc.setupMock(mockDB)

			result, err := repo.GetActivePositions(context.Background(), mockDB, tc.exchangeFilter, tc.symbolFilter)

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

	exchangeID := int64(1)
	instrumentID := int64(10)

	toUpsertArgs := func(p PositionData) []any {
		return []any{
			exchangeID, instrumentID, p.OrderID, p.Quantity, p.EntryPrice,
			p.HighestPrice, p.StopLossBlock, p.UnknownOrigin, DefaultUser,
		}
	}
	toInsertArgs := func(p PositionData) []any {
		return []any{
			exchangeID, instrumentID, p.OrderID, p.Side, p.Quantity, p.EntryPrice,
			p.HighestPrice, p.StopLossBlock, p.UnknownOrigin, DefaultUser,
		}
	}

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
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(exchangeID, instrumentID))

				rows := pgxmock.NewRows([]string{"id"}).AddRow(pos.ID)
				mockDB.ExpectQuery("UPDATE trading.positions").WithArgs(toUpsertArgs(pos)...).WillReturnRows(rows)
			},
		},
		{
			name: "Success Insert (Update fails with NoRows)",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(pos.ExchangeName, pos.InstrumentSymbol).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(exchangeID, instrumentID))

				mockDB.ExpectQuery("UPDATE trading.positions").WithArgs(toUpsertArgs(pos)...).WillReturnError(pgx.ErrNoRows)

				mockDB.ExpectExec("INSERT INTO trading.positions").WithArgs(toInsertArgs(pos)...).WillReturnResult(pgxmock.NewResult("INSERT", 1))
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
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(exchangeID, instrumentID))

				mockDB.ExpectQuery("UPDATE trading.positions").WithArgs(toUpsertArgs(pos)...).WillReturnError(errors.New("db update error"))
			},
			expectedErrContains: "failed to update position",
		},
		{
			name: "Insert Error",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(pos.ExchangeName, pos.InstrumentSymbol).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(exchangeID, instrumentID))

				mockDB.ExpectQuery("UPDATE trading.positions").WithArgs(toUpsertArgs(pos)...).WillReturnError(pgx.ErrNoRows)

				mockDB.ExpectExec("INSERT INTO trading.positions").WithArgs(toInsertArgs(pos)...).WillReturnError(errors.New("db insert error"))
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

	exchangeID := int64(1)
	instrumentID := int64(10)

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
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(exchangeID, instrumentID))

				mockDB.ExpectExec("UPDATE trading.positions").
					WithArgs(exchangeID, instrumentID, DefaultUser).
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
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(exchangeID, instrumentID))

				mockDB.ExpectExec("UPDATE trading.positions").
					WithArgs(exchangeID, instrumentID, DefaultUser).
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
