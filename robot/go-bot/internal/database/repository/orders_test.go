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

// getSampleOrder provides a consistent OrderData object for tests.
func getSampleOrder() OrderData {
	return OrderData{
		ExchangeName:      "dummy",
		InstrumentSymbol:  "BTC/USDT",
		ExchangeOrderID:   "dummy-order-123",
		ClientOrderID:     sql.NullString{String: "dummy-client-123", Valid: true},
		Side:              OrderSideBuy,
		OrderType:         OrderTypeLimit,
		Price:             sql.NullFloat64{Float64: 50000.0, Valid: true},
		Amount:            1.5,
		Filled:            0.0,
		Remaining:         1.5,
		AveragePrice:      sql.NullFloat64{Valid: false},
		Cost:              0.0,
		Status:            OrderStatusOpen,
		ErrorMessage:      sql.NullString{Valid: false},
		ExchangeTimestamp: sql.NullTime{Time: time.Now(), Valid: true},
	}
}

func TestPgOrdersRepo_CreateOrder(t *testing.T) {
	repo := NewOrdersRepo()

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
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(int64(1), int64(10)))

				rows := pgxmock.NewRows([]string{"id"}).AddRow(int64(1))
				mockDB.ExpectQuery("INSERT INTO trading.orders").
					WithArgs(
						order.ExchangeOrderID, order.ClientOrderID, int64(1), int64(10),
						order.Side, order.OrderType, order.Price, order.Amount, order.Filled, order.Remaining,
						order.AveragePrice, order.Cost, order.Status, order.ErrorMessage, order.ExchangeTimestamp,
						DefaultUser,
					).WillReturnRows(rows)
			},
			expectedID: 1,
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
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(int64(1), int64(10)))

				rows := pgxmock.NewRows([]string{"id"}).AddRow(int64(2))
				mockDB.ExpectQuery("INSERT INTO trading.orders").
					WithArgs(
						order.ExchangeOrderID, order.ClientOrderID, int64(1), int64(10),
						order.Side, order.OrderType, order.Price, order.Amount, order.Filled, order.Remaining,
						order.AveragePrice, order.Cost, order.Status, order.ErrorMessage, order.ExchangeTimestamp,
						DefaultUser,
					).WillReturnRows(rows)
			},
			expectedID: 2,
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
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(int64(1), int64(10)))

				rows := pgxmock.NewRows([]string{"id"}).AddRow(int64(3))
				mockDB.ExpectQuery("INSERT INTO trading.orders").
					WithArgs(
						order.ExchangeOrderID, order.ClientOrderID, int64(1), int64(10),
						order.Side, order.OrderType, order.Price, order.Amount, order.Filled, order.Remaining,
						order.AveragePrice, order.Cost, order.Status, order.ErrorMessage, order.ExchangeTimestamp,
						DefaultUser,
					).WillReturnRows(rows)
			},
			expectedID: 3,
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
					WillReturnRows(pgxmock.NewRows([]string{"exchange_id", "id"}).AddRow(int64(1), int64(10)))

				mockDB.ExpectQuery("INSERT INTO trading.orders").
					WithArgs(
						order.ExchangeOrderID, order.ClientOrderID, int64(1), int64(10),
						order.Side, order.OrderType, order.Price, order.Amount, order.Filled, order.Remaining,
						order.AveragePrice, order.Cost, order.Status, order.ErrorMessage, order.ExchangeTimestamp,
						DefaultUser,
					).WillReturnError(errors.New("db insert error"))
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
				assert.Equal(t, tc.expectedID, id)
			}

			require.NoError(t, mockDB.ExpectationsWereMet())
		})
	}
}

func TestPgOrdersRepo_UpdateOrder(t *testing.T) {
	repo := NewOrdersRepo()

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
				mockDB.ExpectQuery("UPDATE trading.orders").
					WithArgs(
						order.Filled, order.Remaining, order.AveragePrice, order.Cost, order.Status,
						order.ErrorMessage, order.ExchangeTimestamp, DefaultUser, order.ExchangeOrderID, order.ExchangeName,
					).WillReturnRows(rows)
			},
			expectedID: 1,
		},
		{
			name: "DB Error",
			setupMock: func(mockDB pgxmock.PgxPoolIface, order OrderData) {
				mockDB.ExpectQuery("UPDATE trading.orders").
					WithArgs(
						order.Filled, order.Remaining, order.AveragePrice, order.Cost, order.Status,
						order.ErrorMessage, order.ExchangeTimestamp, DefaultUser, order.ExchangeOrderID, order.ExchangeName,
					).WillReturnError(errors.New("db update error"))
			},
			expectedErrContains: "failed to update order",
		},
		{
			name: "Not Found",
			setupMock: func(mockDB pgxmock.PgxPoolIface, order OrderData) {
				mockDB.ExpectQuery("UPDATE trading.orders").
					WithArgs(
						order.Filled, order.Remaining, order.AveragePrice, order.Cost, order.Status,
						order.ErrorMessage, order.ExchangeTimestamp, DefaultUser, order.ExchangeOrderID, order.ExchangeName,
					).WillReturnError(pgx.ErrNoRows)
			},
			expectedErrContains: "failed to update order",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDB, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mockDB.Close()

			order := getSampleOrder()
			order.ID = 1

			tc.setupMock(mockDB, order)

			id, err := repo.UpdateOrder(context.Background(), mockDB, order)

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

func TestPgOrdersRepo_GetOrder(t *testing.T) {
	repo := NewOrdersRepo()
	order := getSampleOrder()
	order.ID = 1

	mockDB, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockDB.Close()

	// Success Case
	t.Run("Success", func(t *testing.T) {
		rows := pgxmock.NewRows([]string{
			"id", "exchange_name", "instrument_symbol", "exchange_order_id", "client_order_id",
			"side", "order_type", "price", "amount", "filled", "remaining", "average_price",
			"cost", "order_status", "error_message", "exchange_timestamp", "created_at", "updated_at",
		}).AddRow(
			order.ID, order.ExchangeName, order.InstrumentSymbol, order.ExchangeOrderID, order.ClientOrderID,
			order.Side, order.OrderType, order.Price, order.Amount, order.Filled, order.Remaining, order.AveragePrice,
			order.Cost, order.Status, order.ErrorMessage, order.ExchangeTimestamp, time.Now(), sql.NullTime{},
		)
		mockDB.ExpectQuery("SELECT o.id, e.name AS exchange_name").
			WithArgs(order.ExchangeOrderID, order.ExchangeName).
			WillReturnRows(rows)

		result, err := repo.GetOrder(context.Background(), mockDB, order.ExchangeOrderID, order.ExchangeName)
		require.NoError(t, err)
		assert.Equal(t, order.ID, result.ID)
		assert.Equal(t, order.ExchangeOrderID, result.ExchangeOrderID)
	})

	// Success Case (With Nulls)
	t.Run("Success_WithNulls", func(t *testing.T) {
		rows := pgxmock.NewRows([]string{
			"id", "exchange_name", "instrument_symbol", "exchange_order_id", "client_order_id",
			"side", "order_type", "price", "amount", "filled", "remaining", "average_price",
			"cost", "order_status", "error_message", "exchange_timestamp", "created_at", "updated_at",
		}).AddRow(
			order.ID, order.ExchangeName, order.InstrumentSymbol, order.ExchangeOrderID, nil,
			order.Side, order.OrderType, nil, order.Amount, order.Filled, order.Remaining, nil,
			order.Cost, order.Status, nil, nil, time.Now(), nil,
		)
		mockDB.ExpectQuery("SELECT o.id, e.name AS exchange_name").
			WithArgs(order.ExchangeOrderID, order.ExchangeName).
			WillReturnRows(rows)

		result, err := repo.GetOrder(context.Background(), mockDB, order.ExchangeOrderID, order.ExchangeName)
		require.NoError(t, err)
		assert.False(t, result.ClientOrderID.Valid)
		assert.False(t, result.Price.Valid)
		assert.False(t, result.AveragePrice.Valid)
		assert.False(t, result.ErrorMessage.Valid)
		assert.False(t, result.ExchangeTimestamp.Valid)
		assert.False(t, result.UpdatedAt.Valid)
	})

	// Not Found Case
	t.Run("Not Found", func(t *testing.T) {
		mockDB.ExpectQuery("SELECT o.id, e.name AS exchange_name").
			WithArgs(order.ExchangeOrderID, order.ExchangeName).
			WillReturnError(pgx.ErrNoRows)

		_, err := repo.GetOrder(context.Background(), mockDB, order.ExchangeOrderID, order.ExchangeName)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get order")
	})

	// DB Error Case
	t.Run("DB Error", func(t *testing.T) {
		mockDB.ExpectQuery("SELECT o.id, e.name AS exchange_name").
			WithArgs(order.ExchangeOrderID, order.ExchangeName).
			WillReturnError(errors.New("db query error"))

		_, err := repo.GetOrder(context.Background(), mockDB, order.ExchangeOrderID, order.ExchangeName)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db query error")
	})

	require.NoError(t, mockDB.ExpectationsWereMet())
}

func TestPgOrdersRepo_GetOrders(t *testing.T) {
	repo := NewOrdersRepo()
	order1 := getSampleOrder()
	order1.ID = 1
	order2 := getSampleOrder()
	order2.ID = 2
	order2.ExchangeOrderID = "dummy-order-456"

	columns := []string{
		"id", "exchange_name", "instrument_symbol", "exchange_order_id", "client_order_id",
		"side", "order_type", "price", "amount", "filled", "remaining", "average_price",
		"cost", "order_status", "error_message", "exchange_timestamp", "created_at", "updated_at",
	}

	testCases := []struct {
		name                string
		exchangeName        string
		symbolFilter        string
		limit               int
		setupMock           func(mockDB pgxmock.PgxPoolIface)
		expectedErrContains string
		assertResult        func(t *testing.T, result []OrderData)
	}{
		{
			name:         "Success with symbol filter",
			exchangeName: "dummy",
			symbolFilter: "BTC/USDT",
			limit:        10,
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(columns).AddRow(
					order1.ID, order1.ExchangeName, order1.InstrumentSymbol, order1.ExchangeOrderID, order1.ClientOrderID,
					order1.Side, order1.OrderType, order1.Price, order1.Amount, order1.Filled, order1.Remaining, order1.AveragePrice,
					order1.Cost, order1.Status, order1.ErrorMessage, order1.ExchangeTimestamp, time.Now(), sql.NullTime{},
				)
				mockDB.ExpectQuery("SELECT o.id, e.name AS exchange_name").
					WithArgs("dummy", "BTC/USDT", 10).
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
			limit:        10,
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(columns).AddRow(
					order1.ID, order1.ExchangeName, order1.InstrumentSymbol, order1.ExchangeOrderID, order1.ClientOrderID,
					order1.Side, order1.OrderType, order1.Price, order1.Amount, order1.Filled, order1.Remaining, order1.AveragePrice,
					order1.Cost, order1.Status, order1.ErrorMessage, order1.ExchangeTimestamp, time.Now(), sql.NullTime{},
				).AddRow(
					order2.ID, order2.ExchangeName, order2.InstrumentSymbol, order2.ExchangeOrderID, order2.ClientOrderID,
					order2.Side, order2.OrderType, order2.Price, order2.Amount, order2.Filled, order2.Remaining, order2.AveragePrice,
					order2.Cost, order2.Status, order2.ErrorMessage, order2.ExchangeTimestamp, time.Now(), sql.NullTime{},
				)
				mockDB.ExpectQuery("SELECT o.id, e.name AS exchange_name").
					WithArgs("dummy", "", 10).
					WillReturnRows(rows)
			},
			assertResult: func(t *testing.T, result []OrderData) {
				require.Len(t, result, 2)
			},
		},
		{
			name:         "DB Error",
			exchangeName: "dummy",
			symbolFilter: "",
			limit:        10,
			setupMock: func(mockDB pgxmock.PgxPoolIface) {
				mockDB.ExpectQuery("SELECT o.id, e.name AS exchange_name").
					WithArgs("dummy", "", 10).
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

			result, err := repo.GetOrders(context.Background(), mockDB, tc.exchangeName, tc.symbolFilter, tc.limit)

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
