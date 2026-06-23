//go:build unit

package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var orderColumns = []string{
	"id", "exchange_name", "instrument_symbol", "exchange_order_id", "client_order_id",
	"side", "order_type", "price", "amount", "filled", "remaining", "average_price", "fee", "fee_asset_symbol",
	"cost", "order_status", "error_message", "exchange_timestamp", "created_at", "updated_at",
}

// getSampleOrder provides a consistent OrderData object for tests.
func getSampleOrder() OrderData {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	now := time.Now().Truncate(time.Second)
	return OrderData{
		ID:                r.Int63n(1000) + 1,
		ExchangeName:      "dummy",
		InstrumentSymbol:  "BTC/USDT",
		ExchangeOrderID:   fmt.Sprintf("order-%d", r.Intn(10000)),
		ClientOrderID:     sql.NullString{String: fmt.Sprintf("client-%d", r.Intn(10000)), Valid: true},
		Side:              OrderSideBuy,
		OrderType:         OrderTypeLimit,
		Price:             sql.NullFloat64{Float64: r.Float64() * 50000, Valid: true},
		Amount:            r.Float64() * 2,
		Filled:            0.0,
		Remaining:         r.Float64() * 2,
		AveragePrice:      sql.NullFloat64{Valid: false},
		Fee:               sql.NullFloat64{Float64: r.Float64() * 5, Valid: true},
		FeeAssetSymbol:    sql.NullString{String: "USDT", Valid: true},
		Cost:              0.0,
		Status:            OrderStatusOpen,
		ErrorMessage:      sql.NullString{Valid: false},
		ExchangeTimestamp: sql.NullTime{Time: now, Valid: true},
		CreatedAt:         now,
		UpdatedAt:         sql.NullTime{Valid: false},
	}
}

func toOrderRow(o OrderData) []any {
	return []any{o.ID, o.ExchangeName, o.InstrumentSymbol, o.ExchangeOrderID, o.ClientOrderID, o.Side,
		o.OrderType, o.Price, o.Amount, o.Filled, o.Remaining, o.AveragePrice, o.Fee, o.FeeAssetSymbol,
		o.Cost, o.Status, o.ErrorMessage, o.ExchangeTimestamp, o.CreatedAt, o.UpdatedAt}
}

func TestPgOrdersRepo_GetOrder(t *testing.T) {
	repo := NewOrdersRepo()
	order := getSampleOrder()

	testCases := []struct {
		name                string
		setupMock           func(mockDB pgxmock.PgxPoolIface)
		expectedErrContains string
		assertResult        func(t *testing.T, result OrderData, err error)
	}{
		{
			name: "Success",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(orderColumns).AddRow(toOrderRow(order)...)
				mockDB.ExpectQuery("SELECT o.id, e.name AS exchange_name").
					WithArgs(order.ExchangeName, order.ExchangeOrderID).
					WillReturnRows(rows)
			},
			assertResult: func(t *testing.T, result OrderData, err error) {
				require.NoError(t, err)
				assert.Equal(t, order.ID, result.ID)
				assert.Equal(t, order.ExchangeOrderID, result.ExchangeOrderID)
			},
		},
		{
			name: "Success_WithNulls",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				oNull := getSampleOrder()
				oNull.ClientOrderID = sql.NullString{Valid: false}
				oNull.Price = sql.NullFloat64{Valid: false}
				oNull.AveragePrice = sql.NullFloat64{Valid: false}
				oNull.ErrorMessage = sql.NullString{Valid: false}
				oNull.ExchangeTimestamp = sql.NullTime{Valid: false}

				rows := pgxmock.NewRows(orderColumns).AddRow(toOrderRow(oNull)...)
				mockDB.ExpectQuery("SELECT o.id, e.name AS exchange_name").
					WithArgs(order.ExchangeName, order.ExchangeOrderID).
					WillReturnRows(rows)
			},
			assertResult: func(t *testing.T, result OrderData, err error) {
				require.NoError(t, err)
				assert.False(t, result.ClientOrderID.Valid)
				assert.False(t, result.Price.Valid)
				assert.False(t, result.ExchangeTimestamp.Valid)
			},
		},
		{
			name: "Not Found",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT o.id, e.name AS exchange_name").
					WithArgs(order.ExchangeName, order.ExchangeOrderID).
					WillReturnError(pgx.ErrNoRows)
			},
			expectedErrContains: "failed to get order",
		},
		{
			name: "DB Error",
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT o.id, e.name AS exchange_name").
					WithArgs(order.ExchangeName, order.ExchangeOrderID).
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

			result, err := repo.GetOrder(context.Background(), mockDB, order.ExchangeName, order.ExchangeOrderID)

			if tc.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrContains)
			} else {
				tc.assertResult(t, result, err)
			}
			require.NoError(t, mockDB.ExpectationsWereMet())
		})
	}
}

func TestPgOrdersRepo_GetOrders(t *testing.T) {
	repo := NewOrdersRepo()
	order1 := getSampleOrder()
	order2 := getSampleOrder()
	columns := orderColumns

	testCases := []struct {
		name                string
		exchangeName        string
		symbolFilter        string
		statusFilter        []string
		limit               int
		setupMock           func(mockDB pgxmock.PgxPoolIface)
		expectedErrContains string
		assertResult        func(t *testing.T, result []OrderData)
	}{
		{
			name:         "Success with symbol filter",
			exchangeName: "dummy",
			symbolFilter: "BTC/USDT",
			statusFilter: []string{},
			limit:        10,
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(columns).AddRow(toOrderRow(order1)...)
				mockDB.ExpectQuery("SELECT o.id, e.name AS exchange_name").
					WithArgs("dummy", "BTC/USDT", []string{}, 10).
					WillReturnRows(rows)
			},
			assertResult: func(t *testing.T, result []OrderData) {
				require.Len(t, result, 1)
				assert.Equal(t, order1.ID, result[0].ID)
			},
		},
		{
			name:         "Success without symbol filter",
			exchangeName: "dummy",
			symbolFilter: "",
			statusFilter: []string{},
			limit:        10,
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(columns).AddRows(toOrderRow(order1), toOrderRow(order2))
				mockDB.ExpectQuery("SELECT o.id, e.name AS exchange_name").
					WithArgs("dummy", "", []string{}, 10).
					WillReturnRows(rows)
			},
			assertResult: func(t *testing.T, result []OrderData) {
				require.Len(t, result, 2)
			},
		},
		{
			name:         "Success with status filter",
			exchangeName: "dummy",
			symbolFilter: "BTC/USDT",
			statusFilter: []string{OrderStatusNew, OrderStatusOpen},
			limit:        10,
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(columns).AddRow(toOrderRow(order1)...)
				mockDB.ExpectQuery("SELECT o.id, e.name AS exchange_name").
					WithArgs("dummy", "BTC/USDT", []string{OrderStatusNew, OrderStatusOpen}, 10).
					WillReturnRows(rows)
			},
			assertResult: func(t *testing.T, result []OrderData) {
				require.Len(t, result, 1)
				assert.Equal(t, OrderStatusOpen, result[0].Status)
			},
		},
		{
			name:         "DB Error",
			exchangeName: "dummy",
			symbolFilter: "",
			statusFilter: []string{},
			limit:        10,
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT o.id, e.name AS exchange_name").
					WithArgs("dummy", "", []string{}, 10).
					WillReturnError(errors.New("db query error"))
			},
			expectedErrContains: "failed to get orders",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDB, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mockDB.Close()

			tc.setupMock(mockDB)

			result, err := repo.GetOrders(context.Background(), mockDB, tc.exchangeName, tc.symbolFilter, tc.statusFilter, tc.limit)

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

func TestPgOrdersRepo_CreateOrder(t *testing.T) {
	repo := NewOrdersRepo()

	exchangeID := int64(1)
	instrumentID := int64(10)

	toInsertArgs := func(o OrderData) []any {
		return []any{
			o.ExchangeOrderID, o.ClientOrderID, exchangeID, instrumentID,
			o.Side, o.OrderType, o.Price, o.Amount, o.Filled, o.Remaining,
			o.AveragePrice, o.Fee, o.FeeAssetSymbol, o.Cost, o.Status,
			o.ErrorMessage, o.ExchangeTimestamp, DefaultUser,
		}
	}

	testCases := []struct {
		name                string
		orderModifier       func(OrderData) OrderData
		setupMock           func(mockDB pgxmock.PgxPoolIface, order OrderData)
		expectedID          int64
		expectedErrContains string
	}{
		{
			name:          "Success",
			orderModifier: func(o OrderData) OrderData { return o },
			setupMock: func(mockDB pgxmock.PgxPoolIface, order OrderData) {
				mockDB.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(order.ExchangeName, order.InstrumentSymbol).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(exchangeID, instrumentID))

				insertArgs := toInsertArgs(order)
				rows := pgxmock.NewRows([]string{"id"}).AddRow(order.ID)
				mockDB.ExpectQuery("INSERT INTO trading.orders").WithArgs(insertArgs...).WillReturnRows(rows)
			},
		},
		{
			name: "Success_AllNulls",
			orderModifier: func(o OrderData) OrderData {
				o.ClientOrderID = sql.NullString{}
				o.Price = sql.NullFloat64{}
				o.AveragePrice = sql.NullFloat64{}
				o.ErrorMessage = sql.NullString{}
				o.ExchangeTimestamp = sql.NullTime{}
				return o
			},
			setupMock: func(mockDB pgxmock.PgxPoolIface, order OrderData) {
				mockDB.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(order.ExchangeName, order.InstrumentSymbol).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(exchangeID, instrumentID))

				insertArgs := toInsertArgs(order)
				rows := pgxmock.NewRows([]string{"id"}).AddRow(order.ID)
				mockDB.ExpectQuery("INSERT INTO trading.orders").WithArgs(insertArgs...).WillReturnRows(rows)
			},
		},
		{
			name: "Success_AllFilled",
			orderModifier: func(o OrderData) OrderData {
				o.AveragePrice = sql.NullFloat64{Float64: 50000.0, Valid: true}
				o.ErrorMessage = sql.NullString{String: "None", Valid: true}
				return o
			},
			setupMock: func(mockDB pgxmock.PgxPoolIface, order OrderData) {
				mockDB.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(order.ExchangeName, order.InstrumentSymbol).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(exchangeID, instrumentID))

				insertArgs := toInsertArgs(order)
				rows := pgxmock.NewRows([]string{"id"}).AddRow(order.ID)
				mockDB.ExpectQuery("INSERT INTO trading.orders").WithArgs(insertArgs...).WillReturnRows(rows)
			},
		},
		{
			name:          "DB Select Error",
			orderModifier: func(o OrderData) OrderData { return o },
			setupMock: func(mockDB pgxmock.PgxPoolIface, order OrderData) {
				mockDB.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(order.ExchangeName, order.InstrumentSymbol).
					WillReturnError(errors.New("db query error"))
			},
			expectedErrContains: "failed to resolve exchange and instrument IDs",
		},
		{
			name:          "DB Insert Error",
			orderModifier: func(o OrderData) OrderData { return o },
			setupMock: func(mockDB pgxmock.PgxPoolIface, order OrderData) {
				mockDB.ExpectQuery("SELECT i.exchange_id, i.id").
					WithArgs(order.ExchangeName, order.InstrumentSymbol).
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(exchangeID, instrumentID))

				insertArgs := toInsertArgs(order)
				mockDB.ExpectQuery("INSERT INTO trading.orders").WithArgs(insertArgs...).WillReturnError(errors.New("db insert error"))
			},
			expectedErrContains: "failed to create order",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDB, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mockDB.Close()

			order := getSampleOrder()
			if tc.orderModifier != nil {
				order = tc.orderModifier(order)
			}

			tc.setupMock(mockDB, order)

			id, err := repo.CreateOrder(context.Background(), mockDB, order)

			if tc.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, order.ID, id)
			}

			require.NoError(t, mockDB.ExpectationsWereMet())
		})
	}
}

func TestPgOrdersRepo_UpdateOrder(t *testing.T) {
	repo := NewOrdersRepo()

	b := getSampleOrder()
	updateArgs := []any{
		b.Filled, b.Remaining, b.AveragePrice, b.Fee, b.FeeAssetSymbol, b.Cost, b.Status,
		b.ErrorMessage, b.ExchangeTimestamp, DefaultUser, b.ExchangeOrderID, b.ExchangeName,
	}

	testCases := []struct {
		name                string
		setupMock           func(mockDB pgxmock.PgxPoolIface, order OrderData)
		expectedID          int64
		expectedErrContains string
	}{
		{
			name: "Success",
			setupMock: func(mockDB pgxmock.PgxPoolIface, order OrderData) {
				rows := pgxmock.NewRows([]string{"id"}).AddRow(order.ID)
				mockDB.ExpectQuery("UPDATE trading.orders").WithArgs(updateArgs...).WillReturnRows(rows)
			},
			expectedID: b.ID,
		},
		{
			name: "DB Error",
			setupMock: func(mockDB pgxmock.PgxPoolIface, order OrderData) {
				mockDB.ExpectQuery("UPDATE trading.orders").WithArgs(updateArgs...).WillReturnError(errors.New("db update error"))
			},
			expectedErrContains: "failed to update order",
		},
		{
			name: "Not Found",
			setupMock: func(mockDB pgxmock.PgxPoolIface, order OrderData) {
				mockDB.ExpectQuery("UPDATE trading.orders").WithArgs(updateArgs...).WillReturnError(pgx.ErrNoRows)
			},
			expectedErrContains: "failed to update order",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDB, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mockDB.Close()

			tc.setupMock(mockDB, b)

			id, err := repo.UpdateOrder(context.Background(), mockDB, b)

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
