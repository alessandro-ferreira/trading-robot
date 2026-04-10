package repository

import (
	"context"
	"fmt"
)

// MarketDataTick represents a historical price point for warming up strategies.
type MarketDataTick struct {
	ExchangeName string
	Symbol       string
	TickUnixAt   int64
	Price        float64
}

// MarketDataRepo defines the interface for interacting with market data.
type MarketDataRepo interface {
	GetMarketDataTicks(ctx context.Context, db DBExecutor, exchangeName, symbol string, limit int) ([]MarketDataTick, error)
	InsertTick(ctx context.Context, db DBExecutor, tick MarketDataTick) error
}

type pgMarketDataRepo struct{}

// NewMarketDataRepo creates a new PostgreSQL MarketDataRepo.
func NewMarketDataRepo() MarketDataRepo {
	return &pgMarketDataRepo{}
}

func (r *pgMarketDataRepo) GetMarketDataTicks(ctx context.Context, db DBExecutor, exchangeName, symbol string, limit int) ([]MarketDataTick, error) {
	query := `
		SELECT tick_unix_at, price
		FROM (
			SELECT t.tick_unix_at, t.price
			FROM trading.market_data_ticks t
			INNER JOIN trading.exchanges e ON t.exchange_id = e.id
			INNER JOIN trading.instruments i ON t.instrument_id = i.id AND i.exchange_id = e.id
			WHERE e.name = $1 AND e.active = TRUE
			  AND i.name = $2 AND i.active = TRUE
			ORDER BY t.tick_unix_at DESC
			LIMIT $3
		) sub
		ORDER BY tick_unix_at ASC
	`

	rows, err := db.Query(ctx, query, exchangeName, symbol, limit)
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
	// Check for existence first to avoid sequence increment and unnecessary overwrites
	checkQuery := `
		SELECT 1
		FROM trading.market_data_ticks t
		INNER JOIN trading.exchanges e ON t.exchange_id = e.id
		INNER JOIN trading.instruments i ON t.instrument_id = i.id
		WHERE e.name = $1 AND i.name = $2 AND t.tick_unix_at = $3
		  AND e.active = TRUE AND i.active = TRUE
	`
	var exists int
	err := db.QueryRow(ctx, checkQuery, tick.ExchangeName, tick.Symbol, tick.TickUnixAt).Scan(&exists)
	if err == nil {
		return nil // Tick already exists at this timestamp
	}

	insertQuery := `
		INSERT INTO trading.market_data_ticks (exchange_id, instrument_id, tick_unix_at, price)
		SELECT i.exchange_id, i.id, $3, $4
		FROM trading.instruments i
		INNER JOIN trading.exchanges e ON i.exchange_id = e.id
		WHERE e.name = $1 AND i.name = $2
		  AND e.active = TRUE AND i.active = TRUE
	`

	_, err = db.Exec(ctx, insertQuery,
		tick.ExchangeName,
		tick.Symbol,
		tick.TickUnixAt,
		tick.Price,
	)
	return err
}
