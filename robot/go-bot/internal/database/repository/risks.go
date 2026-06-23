package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// RiskPair represents the risk parameters for a specific pair.
type RiskPair struct {
	ExchangeName     string
	InstrumentSymbol string
	AllocatedBudget  float64
	MaxAssetUnits    sql.NullFloat64
	CreatedAt        time.Time
	UpdatedAt        sql.NullTime
}

// RisksRepo defines the interface for managing pair risk configurations.
type RisksRepo interface {
	GetRiskPair(ctx context.Context, db DBExecutor, exchange, symbol string) (RiskPair, error)
	UpsertRiskPair(ctx context.Context, db DBExecutor, data RiskPair) error
}

type pgRisksRepo struct{}

// NewRisksRepo creates a new PostgreSQL RisksRepo.
func NewRisksRepo() RisksRepo {
	return &pgRisksRepo{}
}

// GetRiskPair retrieves the risk configuration for a specific exchange and instrument.
func (r *pgRisksRepo) GetRiskPair(
	ctx context.Context, db DBExecutor, exchange, symbol string,
) (RiskPair, error) {
	query := `
		SELECT
			e.name,
			i.name,
			rp.allocated_budget,
			rp.max_asset_units,
			rp.created_at,
			rp.updated_at
		FROM trading.risk_pairs rp
		INNER JOIN trading.exchanges e ON e.id = rp.exchange_id AND e.active
		INNER JOIN trading.instruments i ON i.id = rp.instrument_id AND i.exchange_id = e.id AND i.active
		WHERE e.name = $1 AND i.name = $2 AND rp.active
	`

	var data RiskPair
	err := db.QueryRow(ctx, query, exchange, symbol).Scan(
		&data.ExchangeName,
		&data.InstrumentSymbol,
		&data.AllocatedBudget,
		&data.MaxAssetUnits,
		&data.CreatedAt,
		&data.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RiskPair{}, fmt.Errorf("risk configuration not found for %s on %s", symbol, exchange)
		}
		return RiskPair{}, fmt.Errorf("failed to get risk pair: %w", err)
	}

	return data, nil
}

// UpsertRiskPair creates or updates a risk configuration for a specific pair.
func (r *pgRisksRepo) UpsertRiskPair(ctx context.Context, db DBExecutor, data RiskPair) error {
	// Select exchange_id and instrument_id
	selectQuery := `
		SELECT i.exchange_id, i.id
		FROM trading.instruments i
		INNER JOIN trading.exchanges e ON e.id = i.exchange_id
		WHERE e.name = $1 AND i.name = $2 AND e.active AND i.active
	`
	var exchangeID, instrumentID int64
	if err := db.QueryRow(
		ctx, selectQuery, data.ExchangeName, data.InstrumentSymbol).Scan(&exchangeID, &instrumentID); err != nil {
		return fmt.Errorf("failed to resolve exchange and instrument IDs for risk: %w", err)
	}

	// Try to Update first
	updateQuery := `
		UPDATE trading.risk_pairs
		SET allocated_budget = $3, max_asset_units = $4, updated_at = NOW(), updated_by = $5
		WHERE exchange_id = $1 AND instrument_id = $2 AND active
		RETURNING id
	`
	var id int64
	err := db.QueryRow(
		ctx, updateQuery, exchangeID, instrumentID, data.AllocatedBudget, data.MaxAssetUnits, DefaultUser,
	).Scan(&id)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to update risk pair: %w", err)
	}

	if errors.Is(err, sql.ErrNoRows) {
		// If we get here, it means no existing record was updated, so we should insert.
		insertQuery := `
			INSERT INTO trading.risk_pairs (exchange_id, instrument_id, allocated_budget, max_asset_units, created_by)
			VALUES ($1, $2, $3, $4, $5)
		`
		if _, err := db.Exec(
			ctx, insertQuery, exchangeID, instrumentID, data.AllocatedBudget, data.MaxAssetUnits, DefaultUser,
		); err != nil {
			return fmt.Errorf("failed to insert risk pair: %w", err)
		}
	}

	return nil
}
