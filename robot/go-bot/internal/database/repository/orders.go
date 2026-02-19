package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// OrderData represents the order details persisted in the database.
type OrderData struct {
	ID                int64
	ExchangeName      string
	InstrumentSymbol  string
	ExchangeOrderID   string
	ClientOrderID     sql.NullString
	Side              string
	OrderType         string
	Price             sql.NullFloat64
	Amount            float64
	Filled            float64
	Remaining         float64
	AveragePrice      sql.NullFloat64
	Cost              float64
	Status            string
	ErrorMessage      sql.NullString
	ExchangeTimestamp sql.NullTime
	CreatedAt         time.Time
	UpdatedAt         sql.NullTime
}

// OrdersRepo defines the interface for interacting with orders.
type OrdersRepo interface {
	GetOrder(ctx context.Context, db DBExecutor, exchangeOrderID, exchangeName string) (OrderData, error)
	GetOrders(ctx context.Context, db DBExecutor, exchangeName, symbol string, limit int) ([]OrderData, error)
	CreateOrder(ctx context.Context, db DBExecutor, order OrderData) (int64, error)
	UpdateOrder(ctx context.Context, db DBExecutor, order OrderData) (int64, error)
}

// pgOrdersRepo is the PostgreSQL implementation of OrdersRepo.
type pgOrdersRepo struct{}

// NewOrdersRepo creates a new PostgreSQL OrdersRepo.
func NewOrdersRepo() OrdersRepo {
	return &pgOrdersRepo{}
}

// GetOrder retrieves a specific order by its exchange order ID.
func (r *pgOrdersRepo) GetOrder(ctx context.Context, db DBExecutor, exchangeOrderID, exchangeName string) (OrderData, error) {
	query := `
		SELECT
			o.id,
			e.name AS exchange_name,
			i.name  AS instrument_symbol,
			o.exchange_order_id,
			o.client_order_id,
			o.side,
			o.order_type,
			o.price,
			o.amount,
			o.filled,
			o.remaining,
			o.average_price,
			o.cost,
			o.order_status,
			o.error_message,
			o.exchange_timestamp,
			o.created_at,
			o.updated_at
		FROM trading.orders o
		INNER JOIN trading.exchanges e ON o.exchange_id = e.id AND e.name = $2 AND e.active = TRUE
		INNER JOIN trading.instruments i ON o.instrument_id = i.id AND i.active = TRUE
		WHERE o.exchange_order_id = $1 AND o.active = TRUE
	`

	var order OrderData
	err := db.QueryRow(ctx, query, exchangeOrderID, exchangeName).Scan(
		&order.ID,
		&order.ExchangeName,
		&order.InstrumentSymbol,
		&order.ExchangeOrderID,
		&order.ClientOrderID,
		&order.Side,
		&order.OrderType,
		&order.Price,
		&order.Amount,
		&order.Filled,
		&order.Remaining,
		&order.AveragePrice,
		&order.Cost,
		&order.Status,
		&order.ErrorMessage,
		&order.ExchangeTimestamp,
		&order.CreatedAt,
		&order.UpdatedAt,
	)
	if err != nil {
		return OrderData{}, fmt.Errorf("failed to get order: %w", err)
	}

	return order, nil
}

// GetOrders retrieves a list of orders, optionally filtered by symbol.
func (r *pgOrdersRepo) GetOrders(ctx context.Context, db DBExecutor, exchangeName, symbol string, limit int) ([]OrderData, error) {
	query := `
		SELECT
			o.id,
			e.name AS exchange_name,
			i.name  AS instrument_symbol,
			o.exchange_order_id,
			o.client_order_id,
			o.side,
			o.order_type,
			o.price,
			o.amount,
			o.filled,
			o.remaining,
			o.average_price,
			o.cost,
			o.order_status,
			o.error_message,
			o.exchange_timestamp,
			o.created_at,
			o.updated_at
		FROM trading.orders o
		INNER JOIN trading.exchanges e ON o.exchange_id = e.id AND e.name = $1 AND e.active = TRUE
		INNER JOIN trading.instruments i ON o.instrument_id = i.id AND ($2 = '' OR i.name = $2) AND i.active = TRUE
		WHERE o.active = TRUE
		ORDER BY o.created_at DESC
		LIMIT $3
	`

	rows, err := db.Query(ctx, query, exchangeName, symbol, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get orders: %w", err)
	}
	defer rows.Close()

	var orders []OrderData
	for rows.Next() {
		var o OrderData
		if err := rows.Scan(
			&o.ID,
			&o.ExchangeName,
			&o.InstrumentSymbol,
			&o.ExchangeOrderID,
			&o.ClientOrderID,
			&o.Side,
			&o.OrderType,
			&o.Price,
			&o.Amount,
			&o.Filled,
			&o.Remaining,
			&o.AveragePrice,
			&o.Cost,
			&o.Status,
			&o.ErrorMessage,
			&o.ExchangeTimestamp,
			&o.CreatedAt,
			&o.UpdatedAt,
		); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}

	return orders, rows.Err()
}

// CreateOrder inserts a new order into the database.
func (r *pgOrdersRepo) CreateOrder(ctx context.Context, db DBExecutor, order OrderData) (int64, error) {
	query := `
		INSERT INTO trading.orders (
			exchange_order_id,
			client_order_id,
			exchange_id,
			instrument_id,
			side,
			order_type,
			price,
			amount,
			filled,
			remaining,
			average_price,
			cost,
			order_status,
			error_message,
			exchange_timestamp,
			created_at,
			created_by
		) VALUES (
			$1, $2,
			(SELECT id FROM trading.exchanges WHERE name = $3 AND active = TRUE),
			(SELECT id FROM trading.instruments WHERE symbol = $4 AND active = TRUE),
			$5, $6::trading.order_type, $7, $8, $9, $10, $11, $12, $13::trading.order_status, $14, $15, NOW(), $16
		)
		RETURNING id
	`

	var id int64
	err := db.QueryRow(ctx, query,
		order.ExchangeOrderID,
		order.ClientOrderID,
		order.ExchangeName,
		order.InstrumentSymbol,
		order.Side,
		order.OrderType,
		order.Price,
		order.Amount,
		order.Filled,
		order.Remaining,
		order.AveragePrice,
		order.Cost,
		order.Status,
		order.ErrorMessage,
		order.ExchangeTimestamp,
		DefaultUser,
	).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("failed to create order: %w", err)
	}

	return id, nil
}

// UpdateOrder updates an existing order in the database.
func (r *pgOrdersRepo) UpdateOrder(ctx context.Context, db DBExecutor, order OrderData) (int64, error) {
	query := `
		UPDATE trading.orders
		SET
			filled = $1,
			remaining = $2,
			average_price = $3,
			cost = $4,
			order_status = $5::trading.order_status,
			error_message = $6,
			exchange_timestamp = $7,
			updated_at = NOW(),
			updated_by = $8
		WHERE
			exchange_order_id = $9
			AND exchange_id = (SELECT id FROM trading.exchanges WHERE name = $10 AND active = TRUE)
		RETURNING id
	`

	var id int64
	err := db.QueryRow(ctx, query,
		order.Filled,
		order.Remaining,
		order.AveragePrice,
		order.Cost,
		order.Status,
		order.ErrorMessage,
		order.ExchangeTimestamp,
		DefaultUser,
		order.ExchangeOrderID,
		order.ExchangeName,
	).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("failed to update order: %w", err)
	}

	return id, nil
}
