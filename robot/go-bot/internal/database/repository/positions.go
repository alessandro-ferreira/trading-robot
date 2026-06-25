package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Position Side constants
const (
	PositionSideLong = "long"
	// Go-robot does not support 'short' operations currently
	// PositionSideShort = "short"
)

// PositionData represents the position details persisted in the database.
type PositionData struct {
	ID               int64
	ExchangeName     string
	InstrumentSymbol string
	OrderID          sql.NullInt64
	Side             string
	Quantity         float64
	EntryPrice       float64
	HighestPrice     float64
	StopLossActive   bool
	UnknownOrigin    bool
	Active           bool
	CreatedAt        time.Time
	UpdatedAt        sql.NullTime
}

// PositionsRepo defines the interface for interacting with positions.
type PositionsRepo interface {
	GetPosition(
		ctx context.Context, db DBExecutor, exchangeName, instrumentSymbol string,
	) (PositionData, error)
	GetActivePositions(
		ctx context.Context, db DBExecutor, exchangeName, instrumentSymbol string,
	) ([]PositionData, error)
	UpsertPosition(ctx context.Context, db DBExecutor, pos PositionData) error
	DeletePosition(ctx context.Context, db DBExecutor, exchangeName, instrumentSymbol string) error
}

// pgPositionsRepo is the PostgreSQL implementation of PositionsRepo.
type pgPositionsRepo struct{}

// NewPositionsRepo creates a new PostgreSQL PositionsRepo.
func NewPositionsRepo() PositionsRepo {
	return &pgPositionsRepo{}
}

// GetPosition retrieves a specific position.
func (r *pgPositionsRepo) GetPosition(
	ctx context.Context, db DBExecutor, exchangeName, instrumentSymbol string,
) (PositionData, error) {
	query := `
		SELECT
			p.id,
			e.name AS exchange_name,
			i.name AS instrument_symbol,
			p.order_id,
			p.side,
			p.quantity,
			p.entry_price,
			p.highest_price,
			p.stop_loss_active,
			p.unknown_origin,
			p.active,
			p.created_at,
			p.updated_at
		FROM trading.positions p
		INNER JOIN trading.exchanges e ON e.id = p.exchange_id AND e.name = $1 AND e.active
		INNER JOIN trading.instruments i ON i.id = p.instrument_id AND i.exchange_id = p.exchange_id
			AND i.name = $2 AND i.active
		WHERE p.active
	`

	var pos PositionData
	err := db.QueryRow(ctx, query, exchangeName, instrumentSymbol).Scan(
		&pos.ID,
		&pos.ExchangeName,
		&pos.InstrumentSymbol,
		&pos.OrderID,
		&pos.Side,
		&pos.Quantity,
		&pos.EntryPrice,
		&pos.HighestPrice,
		&pos.StopLossActive,
		&pos.UnknownOrigin,
		&pos.Active,
		&pos.CreatedAt,
		&pos.UpdatedAt,
	)
	if err != nil {
		return PositionData{}, fmt.Errorf("failed to get position: %w", err)
	}

	return pos, nil
}

// GetOpenPositions retrieves all active positions across all exchanges.
func (r *pgPositionsRepo) GetActivePositions(
	ctx context.Context, db DBExecutor, exchangeName, instrumentSymbol string,
) ([]PositionData, error) {
	query := `
		SELECT
			p.id,
			e.name AS exchange_name,
			i.name AS instrument_symbol,
			p.order_id,
			p.side,
			p.quantity,
			p.entry_price,
			p.highest_price,
			p.stop_loss_active,
			p.unknown_origin,
			p.active,
			p.created_at,
			p.updated_at
		FROM trading.positions p
		INNER JOIN trading.exchanges e ON e.id = p.exchange_id AND ($1 = '' OR e.name = $1) AND e.active
		INNER JOIN trading.instruments i ON i.id = p.instrument_id AND i.exchange_id = p.exchange_id AND ($2 = '' OR i.name = $2) AND i.active
		WHERE p.active
	`

	rows, err := db.Query(ctx, query, exchangeName, instrumentSymbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get active positions: %w", err)
	}
	defer rows.Close()

	var positions []PositionData
	for rows.Next() {
		var pos PositionData
		if err := rows.Scan(
			&pos.ID, &pos.ExchangeName, &pos.InstrumentSymbol, &pos.OrderID, &pos.Side,
			&pos.Quantity, &pos.EntryPrice, &pos.HighestPrice, &pos.StopLossActive,
			&pos.UnknownOrigin, &pos.Active, &pos.CreatedAt, &pos.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan position: %w", err)
		}
		positions = append(positions, pos)
	}
	return positions, nil
}

// UpsertPosition inserts or updates a position.
func (r *pgPositionsRepo) UpsertPosition(ctx context.Context, db DBExecutor, pos PositionData) error {
	// Select exchange_id and instrument_id
	selectQuery := `
		SELECT i.exchange_id, i.id
		FROM trading.instruments i
		INNER JOIN trading.exchanges e ON e.id = i.exchange_id AND e.name = $1 AND e.active
		WHERE i.name = $2 AND i.active
	`
	var exchangeID, instrumentID int64
	err := db.QueryRow(ctx, selectQuery, pos.ExchangeName, pos.InstrumentSymbol).Scan(
		&exchangeID,
		&instrumentID,
	)
	if err != nil {
		return fmt.Errorf("failed to resolve exchange and instrument IDs for position: %w", err)
	}

	// Try to Update first
	updateQuery := `
		UPDATE trading.positions
		SET
			order_id = $3,
			quantity = $4,
			entry_price = $5,
			highest_price = $6,
			stop_loss_active = $7,
			unknown_origin = $8,
			updated_at = NOW(),
			updated_by = $9
		WHERE exchange_id = $1 AND instrument_id = $2 AND active
		RETURNING id
	`

	var id int64
	err = db.QueryRow(ctx, updateQuery,
		exchangeID,
		instrumentID,
		pos.OrderID,
		pos.Quantity,
		pos.EntryPrice,
		pos.HighestPrice,
		pos.StopLossActive,
		pos.UnknownOrigin,
		DefaultUser,
	).Scan(&id)

	if err == nil {
		return nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("failed to update position: %w", err)
	}

	// If we get here, it means pgx.ErrNoRows was returned, so we should insert.
	insertQuery := `
		INSERT INTO trading.positions (
			exchange_id,
			instrument_id,
			order_id,
			side,
			quantity,
			entry_price,
			highest_price,
			stop_loss_active,
			unknown_origin,
			active,
			created_at,
			created_by
		) VALUES (
			$1, $2, $3, $4::trading.position_side, $5, $6, $7, $8, $9, TRUE, NOW(), $10
		)
	`

	_, err = db.Exec(ctx, insertQuery,
		exchangeID,
		instrumentID,
		pos.OrderID,
		pos.Side,
		pos.Quantity,
		pos.EntryPrice,
		pos.HighestPrice,
		pos.StopLossActive,
		pos.UnknownOrigin,
		DefaultUser,
	)

	if err != nil {
		return fmt.Errorf("failed to insert position: %w", err)
	}

	return nil
}

// DeletePosition marks a position as inactive (soft delete).
func (r *pgPositionsRepo) DeletePosition(
	ctx context.Context, db DBExecutor, exchangeName, instrumentSymbol string,
) error {
	// Select exchange_id and instrument_id
	selectQuery := `
		SELECT i.exchange_id, i.id
		FROM trading.instruments i
		INNER JOIN trading.exchanges e ON e.id = i.exchange_id AND e.name = $1 AND e.active
		WHERE i.name = $2 AND i.active
	`
	var exchangeID, instrumentID int64
	err := db.QueryRow(ctx, selectQuery, exchangeName, instrumentSymbol).Scan(
		&exchangeID,
		&instrumentID,
	)
	if err != nil {
		return fmt.Errorf("failed to resolve exchange and instrument IDs for position: %w", err)
	}

	// Soft delete the position by setting active to FALSE
	updateQuery := `
		UPDATE trading.positions
		SET active = FALSE, updated_at = NOW(), updated_by = $3
		WHERE exchange_id = $1 AND instrument_id = $2 AND active
	`

	_, err = db.Exec(ctx, updateQuery, exchangeID, instrumentID, DefaultUser)
	if err != nil {
		return fmt.Errorf("failed to delete position: %w", err)
	}

	return nil
}
