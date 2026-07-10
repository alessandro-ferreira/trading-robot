package repository

import (
	"context"
	"database/sql"
	"fmt"
)

const TICKS_LIMIT = 10000

// MarketDataTick represents a historical price point for warming up strategies.
type MarketDataTick struct {
	ExchangeName string
	Symbol       string
	TickUnixAt   int64
	Price        float64
}

// MarketDataRepo defines the interface for interacting with market data.
type MarketDataRepo interface {
	GetMarketDataTicks(
		ctx context.Context, db DBExecutor, exchangeName, symbol string, sinceEpoch int64,
	) ([]MarketDataTick, error)
	InsertTick(ctx context.Context, db DBExecutor, tick MarketDataTick) error
}

type pgMarketDataRepo struct{}

// NewMarketDataRepo creates a new PostgreSQL MarketDataRepo.
func NewMarketDataRepo() MarketDataRepo {
	return &pgMarketDataRepo{}
}

func (r *pgMarketDataRepo) GetMarketDataTicks(
	ctx context.Context, db DBExecutor, exchangeName, symbol string, sinceEpoch int64,
) ([]MarketDataTick, error) {
	query := `
		SELECT tick_unix_at, price
		FROM (
			SELECT t.tick_unix_at, t.price
			FROM trading.market_data_ticks t
			INNER JOIN trading.exchanges e ON e.id = t.exchange_id AND e.name = $1 AND e.active
			INNER JOIN trading.instruments i ON i.id = t.instrument_id AND i.exchange_id = t.exchange_id
				AND i.name = $2 AND i.active
			WHERE t.tick_unix_at >= $3 AND $3 > 0
			ORDER BY t.tick_unix_at DESC
			LIMIT $4
		) sub
		ORDER BY tick_unix_at ASC
	`

	rows, err := db.Query(ctx, query, exchangeName, symbol, sinceEpoch, TICKS_LIMIT)
	if err != nil {
		return nil, fmt.Errorf("failed to get market data ticks: %w", err)
	}
	defer rows.Close()

	var ticks []MarketDataTick
	for rows.Next() {
		var t MarketDataTick
		if err := rows.Scan(&t.TickUnixAt, &t.Price); err != nil {
			return nil, fmt.Errorf("failed to scan tick: %w", err)
		}
		ticks = append(ticks, t)
	}

	return ticks, rows.Err()
}

func (r *pgMarketDataRepo) InsertTick(ctx context.Context, db DBExecutor, tick MarketDataTick) error {
	// Select exchange_id and instrument_id and check for existence
	selectQuery := `
		SELECT i.exchange_id, i.id, t.tick_unix_at
		FROM trading.instruments i
		INNER JOIN trading.exchanges e ON e.id = i.exchange_id AND e.name = $1 AND e.active
		LEFT JOIN trading.market_data_ticks t ON t.exchange_id = i.exchange_id
			AND t.instrument_id = i.id AND t.tick_unix_at = $3
		WHERE i.name = $2 AND i.active
	`

	var exchangeID, instrumentID int64
	var existingTick sql.NullInt64

	err := db.QueryRow(ctx, selectQuery, tick.ExchangeName, tick.Symbol, tick.TickUnixAt).Scan(
		&exchangeID,
		&instrumentID,
		&existingTick,
	)
	if err != nil {
		return fmt.Errorf("failed to resolve exchange and instrument IDs for market tick: %w", err)
	}

	if existingTick.Valid {
		return nil // Tick already exists, skip
	}

	// Insert new tick
	insertQuery := `
		INSERT INTO trading.market_data_ticks (exchange_id, instrument_id, tick_unix_at, price)
	 	VALUES ($1, $2, $3, $4)
	`
	_, err = db.Exec(ctx, insertQuery, exchangeID, instrumentID, tick.TickUnixAt, tick.Price)
	if err != nil {
		return fmt.Errorf("failed to insert market data tick: %w", err)
	}

	return nil
}
