package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// BalanceData represents a user's balance for a specific asset on an exchange.
type BalanceData struct {
	ID           int64
	ExchangeName string
	AssetSymbol  string
	Free         float64
	Used         float64
	Total        float64
	CreatedAt    time.Time
	UpdatedAt    sql.NullTime
}

// BalancesRepo defines the interface for interacting with the balances data.
type BalancesRepo interface {
	GetBalance(ctx context.Context, db DBExecutor, exchange, asset string) (BalanceData, error)
	GetAllBalances(ctx context.Context, db DBExecutor, exchange string) ([]BalanceData, error)
	UpsertBalance(ctx context.Context, db DBExecutor, balance BalanceData) (int64, error)
}

// pgBalancesRepo is the PostgreSQL implementation of BalancesRepo.
type pgBalancesRepo struct {
}

// NewBalancesRepo creates a new PostgreSQL BalancesRepo.
func NewBalancesRepo() BalancesRepo {
	return &pgBalancesRepo{}
}

// GetBalance retrieves a specific balance for an exchange and asset.
func (r *pgBalancesRepo) GetBalance(
	ctx context.Context, db DBExecutor, exchange, asset string,
) (BalanceData, error) {
	query := `
		SELECT
			b.id,
			e.name AS exchange_name,
			a.symbol AS asset_symbol,
			b.free,
			b.used,
			b.total,
			b.created_at,
			b.updated_at
		FROM trading.balances b
		INNER JOIN trading.exchanges e ON e.id = b.exchange_id AND e.active
		INNER JOIN trading.assets a ON a.id = b.asset_id AND a.active
		WHERE b.active AND e.name = $1 AND a.symbol = $2
	`
	var b BalanceData
	err := db.QueryRow(ctx, query, exchange, asset).Scan(
		&b.ID,
		&b.ExchangeName,
		&b.AssetSymbol,
		&b.Free,
		&b.Used,
		&b.Total,
		&b.CreatedAt,
		&b.UpdatedAt,
	)
	if err != nil {
		return BalanceData{}, fmt.Errorf("failed to get balance: %w", err)
	}
	return b, nil
}

// GetAllBalances retrieves all balances from the database, optionally filtered by exchange,
// joined with their respective exchange and asset names for easy display.
func (r *pgBalancesRepo) GetAllBalances(
	ctx context.Context, db DBExecutor, exchange string,
) ([]BalanceData, error) {
	query := `
		SELECT
			b.id,
			e.name AS exchange_name,
			a.symbol AS asset_symbol,
			b.free,
			b.used,
			b.total,
			b.created_at,
			b.updated_at
		FROM trading.balances b
		INNER JOIN trading.exchanges e ON e.id = b.exchange_id AND ($1 = '' OR e.name = $1) AND e.active
		INNER JOIN trading.assets a ON a.id = b.asset_id AND a.active
		WHERE b.active
		ORDER BY e.name ASC, a.symbol ASC
	`
	rows, err := db.Query(ctx, query, exchange)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var balances []BalanceData
	for rows.Next() {
		var b BalanceData
		if err := rows.Scan(
			&b.ID,
			&b.ExchangeName,
			&b.AssetSymbol,
			&b.Free,
			&b.Used,
			&b.Total,
			&b.CreatedAt,
			&b.UpdatedAt,
		); err != nil {
			return nil, err
		}
		balances = append(balances, b)
	}

	return balances, rows.Err()
}

// UpsertBalance updates the balance for a given asset and exchange, or inserts it if it doesn't exist.
func (r *pgBalancesRepo) UpsertBalance(
	ctx context.Context, db DBExecutor, balance BalanceData,
) (int64, error) {
	// Try to Update first
	updateQuery := `
		UPDATE trading.balances
		SET
			free = $3,
			used = $4,
			total = $5,
			updated_at = NOW(),
			updated_by = $6
		WHERE
			active
			AND exchange_id = (SELECT id FROM trading.exchanges WHERE name = $1 AND active)
			AND asset_id = (SELECT id FROM trading.assets WHERE symbol = $2 AND active)
		RETURNING id
	`
	var id int64
	err := db.
		QueryRow(
			ctx,
			updateQuery,
			balance.ExchangeName,
			balance.AssetSymbol,
			balance.Free,
			balance.Used,
			balance.Total,
			DefaultUser,
		).
		Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("failed to update balance: %w", err)
	}

	// If we get here, it means pgx.ErrNoRows was returned, so we should insert.
	insertQuery := `
		INSERT INTO trading.balances (exchange_id, asset_id, free, used, total, created_at, created_by)
		VALUES (
			(SELECT id FROM trading.exchanges WHERE name = $1 AND active),
			(SELECT id FROM trading.assets WHERE symbol = $2 AND active),
			$3, $4, $5, NOW(), $6
		)
		RETURNING id
	`
	err = db.
		QueryRow(
			ctx,
			insertQuery,
			balance.ExchangeName,
			balance.AssetSymbol,
			balance.Free,
			balance.Used,
			balance.Total,
			DefaultUser,
		).
		Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to insert balance: %w", err)
	}

	return id, nil
}
