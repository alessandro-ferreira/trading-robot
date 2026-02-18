package repository

import (
	"context"
	"database/sql"
	"fmt"
)

// BalanceData represents a user's balance for a specific asset on an exchange.
type BalanceData struct {
	ID           int64
	ExchangeName string
	AssetSymbol  string
	Free         float64
	Used         float64
	Total        float64
	UpdatedAt    sql.NullTime
	UpdatedBy    sql.NullString
}

// BalancesRepo defines the interface for interacting with the balances data.
type BalancesRepo interface {
	GetAllBalances(ctx context.Context, db DBExecutor) ([]BalanceData, error)
	UpsertBalance(ctx context.Context, db DBExecutor, balance BalanceData) error
}

// pgBalancesRepo is the PostgreSQL implementation of BalancesRepo.
type pgBalancesRepo struct {
}

// NewBalancesRepo creates a new PostgreSQL BalancesRepo.
func NewBalancesRepo() BalancesRepo {
	return &pgBalancesRepo{}
}

// GetAllBalances retrieves all balances from the database, joined with their
// respective exchange and asset names for easy display.
func (r *pgBalancesRepo) GetAllBalances(ctx context.Context, db DBExecutor) ([]BalanceData, error) {
	query := `
		SELECT
			b.id,
			e.name AS exchange_name,
			a.symbol AS asset_symbol,
			b.free,
			b.used,
			b.total,
			b.updated_at,
			b.updated_by
		FROM trading.balances b
		INNER JOIN trading.exchanges e ON b.exchange_id = e.id
		INNER JOIN trading.assets a ON b.asset_id = a.id
		WHERE b.active = TRUE AND e.active = TRUE AND a.active = TRUE
		ORDER BY e.name ASC, a.symbol ASC
	`
	rows, err := db.Query(ctx, query)
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
			&b.UpdatedAt,
			&b.UpdatedBy,
		); err != nil {
			return nil, err
		}
		balances = append(balances, b)
	}

	return balances, rows.Err()
}

// UpsertBalance updates the balance for a given asset and exchange, or inserts it if it doesn't exist.
func (r *pgBalancesRepo) UpsertBalance(ctx context.Context, db DBExecutor, balance BalanceData) error {
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
			active = TRUE
			AND exchange_id = (SELECT id FROM trading.exchanges WHERE name = $1 AND active = TRUE)
			AND asset_id = (SELECT id FROM trading.assets WHERE symbol = $2 AND active = TRUE)
	`
	cmdTag, err := db.Exec(ctx, updateQuery, balance.ExchangeName, balance.AssetSymbol, balance.Free, balance.Used, balance.Total, DefaultUser)
	if err != nil {
		return fmt.Errorf("failed to update balance: %w", err)
	}

	// If the update affected rows, we are done.
	if cmdTag.RowsAffected() > 0 {
		return nil
	}

	// If no rows were updated, Insert
	insertQuery := `
		INSERT INTO trading.balances (exchange_id, asset_id, free, used, total, created_at, created_by)
		VALUES (
			(SELECT id FROM trading.exchanges WHERE name = $1 AND active = TRUE),
			(SELECT id FROM trading.assets WHERE symbol = $2 AND active = TRUE),
			$3, $4, $5, NOW(), $6
		)
	`
	_, err = db.Exec(ctx, insertQuery, balance.ExchangeName, balance.AssetSymbol, balance.Free, balance.Used, balance.Total, DefaultUser)
	if err != nil {
		return fmt.Errorf("failed to insert balance: %w", err)
	}

	return nil
}
