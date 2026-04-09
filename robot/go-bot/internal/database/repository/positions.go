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
	PositionSideLong  = "long"
	PositionSideShort = "short"
)

// PositionData represents the position details persisted in the database.
type PositionData struct {
	ID               int64
	ExchangeName     string
	InstrumentSymbol string
	Side             string
	Quantity         float64
	EntryPrice       float64
	HighestPrice     float64
	StrategyState    string
	Active           bool
	CreatedAt        time.Time
	UpdatedAt        sql.NullTime
}

// PositionsRepo defines the interface for interacting with positions.
type PositionsRepo interface {
	GetPosition(ctx context.Context, db DBExecutor, exchangeName, symbol string) (PositionData, error)
	GetOpenPositions(ctx context.Context, db DBExecutor) ([]PositionData, error)
	UpsertPosition(ctx context.Context, db DBExecutor, pos PositionData) error
	DeletePosition(ctx context.Context, db DBExecutor, exchangeName, symbol string) error
}

// pgPositionsRepo is the PostgreSQL implementation of PositionsRepo.
type pgPositionsRepo struct{}

// NewPositionsRepo creates a new PostgreSQL PositionsRepo.
func NewPositionsRepo() PositionsRepo {
	return &pgPositionsRepo{}
}

// GetPosition retrieves a specific position.
func (r *pgPositionsRepo) GetPosition(ctx context.Context, db DBExecutor, exchangeName, symbol string) (PositionData, error) {
	query := `
		SELECT
			p.id,
			e.name AS exchange_name,
			i.name AS instrument_symbol,
			p.side,
			p.quantity,
			p.entry_price,
			p.highest_price,
			p.strategy_state,
			p.active,
			p.created_at,
			p.updated_at
		FROM trading.positions p
		INNER JOIN trading.exchanges e ON p.exchange_id = e.id AND e.name = $1 AND e.active = TRUE
		INNER JOIN trading.instruments i ON p.instrument_id = i.id AND i.name = $2 AND i.active = TRUE
		WHERE p.active = TRUE
	`

	var pos PositionData
	err := db.QueryRow(ctx, query, exchangeName, symbol).Scan(
		&pos.ID,
		&pos.ExchangeName,
		&pos.InstrumentSymbol,
		&pos.Side,
		&pos.Quantity,
		&pos.EntryPrice,
		&pos.HighestPrice,
		&pos.StrategyState,
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
func (r *pgPositionsRepo) GetOpenPositions(ctx context.Context, db DBExecutor) ([]PositionData, error) {
	query := `
		SELECT
			p.id,
			e.name AS exchange_name,
			i.name AS instrument_symbol,
			p.side,
			p.quantity,
			p.entry_price,
			p.highest_price,
			p.strategy_state,
			p.active,
			p.created_at,
			p.updated_at
		FROM trading.positions p
		INNER JOIN trading.exchanges e ON p.exchange_id = e.id AND e.active = TRUE
		INNER JOIN trading.instruments i ON p.instrument_id = i.id AND i.active = TRUE
		WHERE p.active = TRUE
	`

	rows, err := db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get open positions: %w", err)
	}
	defer rows.Close()

	var positions []PositionData
	for rows.Next() {
		var pos PositionData
		if err := rows.Scan(
			&pos.ID, &pos.ExchangeName, &pos.InstrumentSymbol, &pos.Side,
			&pos.Quantity, &pos.EntryPrice, &pos.HighestPrice,
			&pos.StrategyState, &pos.Active, &pos.CreatedAt, &pos.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan position: %w", err)
		}
		positions = append(positions, pos)
	}
	return positions, nil
}

// UpsertPosition inserts or updates a position.
func (r *pgPositionsRepo) UpsertPosition(ctx context.Context, db DBExecutor, pos PositionData) error {
	// Try to Update first
	updateQuery := `
		UPDATE trading.positions
		SET
			quantity = $3,
			entry_price = $4,
			highest_price = $5,
			strategy_state = $6::trading.strategy_state,
			updated_at = NOW(),
			updated_by = $7
		WHERE
			exchange_id = (SELECT id FROM trading.exchanges WHERE name = $1 AND active = TRUE)
			AND instrument_id = (SELECT id FROM trading.instruments WHERE name = $2 AND active = TRUE)
			AND active = TRUE
		RETURNING id
	`

	var id int64
	err := db.QueryRow(ctx, updateQuery,
		pos.ExchangeName,
		pos.InstrumentSymbol,
		pos.Quantity,
		pos.EntryPrice,
		pos.HighestPrice,
		pos.StrategyState,
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
			side,
			quantity,
			entry_price,
			highest_price,
			strategy_state,
			active,
			created_at,
			created_by
		) VALUES (
			(SELECT id FROM trading.exchanges WHERE name = $1 AND active = TRUE),
			(SELECT id FROM trading.instruments WHERE name = $2 AND active = TRUE),
			$3::trading.position_side, $4, $5, $6, $7::trading.strategy_state, TRUE, NOW(), $8
		)
	`

	_, err = db.Exec(ctx, insertQuery,
		pos.ExchangeName,
		pos.InstrumentSymbol,
		pos.Side,
		pos.Quantity,
		pos.EntryPrice,
		pos.HighestPrice,
		pos.StrategyState,
		DefaultUser,
	)

	if err != nil {
		return fmt.Errorf("failed to insert position: %w", err)
	}

	return nil
}

// DeletePosition marks a position as inactive (soft delete).
func (r *pgPositionsRepo) DeletePosition(ctx context.Context, db DBExecutor, exchangeName, symbol string) error {
	query := `
		UPDATE trading.positions
		SET active = FALSE, updated_at = NOW(), updated_by = $3
		WHERE exchange_id = (SELECT id FROM trading.exchanges WHERE name = $1 AND active = TRUE)
		  AND instrument_id = (SELECT id FROM trading.instruments WHERE name = $2 AND active = TRUE)
		  AND active = TRUE
	`

	_, err := db.Exec(ctx, query, exchangeName, symbol, DefaultUser)
	if err != nil {
		return fmt.Errorf("failed to delete position: %w", err)
	}

	return nil
}
