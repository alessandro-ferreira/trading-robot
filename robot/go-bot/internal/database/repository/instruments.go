package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// InstrumentData represents a tradable instrument and its exchange rules.
type InstrumentData struct {
	ID               int64
	ExchangeName     string
	Name             string
	BaseAssetSymbol  string
	QuoteAssetSymbol string
	PricePrecision   int
	AmountPrecision  int
	MinAmount        float64
	CreatedAt        time.Time
	CreatedBy        sql.NullString
	UpdatedAt        sql.NullTime
	UpdatedBy        sql.NullString
	Active           bool
}

// InstrumentsRepo defines read operations for trading instruments.
type InstrumentsRepo interface {
	GetInstrument(ctx context.Context, db DBExecutor, exchange, symbol string) (InstrumentData, error)
}

type pgInstrumentsRepo struct{}

// NewInstrumentsRepo creates a new PostgreSQL InstrumentsRepo.
func NewInstrumentsRepo() InstrumentsRepo {
	return &pgInstrumentsRepo{}
}

// GetInstrument retrieves an active instrument for an exchange and symbol.
func (r *pgInstrumentsRepo) GetInstrument(
	ctx context.Context, db DBExecutor, exchange, symbol string,
) (InstrumentData, error) {
	query := `
		SELECT
			i.id,
			e.name AS exchange_name,
			i.name,
			base_asset.symbol AS base_asset_symbol,
			quote_asset.symbol AS quote_asset_symbol,
			i.price_precision,
			i.amount_precision,
			i.min_amount,
			i.created_at,
			i.created_by,
			i.updated_at,
			i.updated_by,
			i.active
		FROM trading.instruments i
		INNER JOIN trading.exchanges e ON e.id = i.exchange_id AND e.active
		INNER JOIN trading.assets base_asset ON base_asset.id = i.base_asset_id AND base_asset.active
		INNER JOIN trading.assets quote_asset ON quote_asset.id = i.quote_asset_id AND quote_asset.active
		WHERE e.name = $1 AND i.name = $2 AND i.active
	`

	var data InstrumentData
	err := db.QueryRow(ctx, query, exchange, symbol).Scan(
		&data.ID,
		&data.ExchangeName,
		&data.Name,
		&data.BaseAssetSymbol,
		&data.QuoteAssetSymbol,
		&data.PricePrecision,
		&data.AmountPrecision,
		&data.MinAmount,
		&data.CreatedAt,
		&data.CreatedBy,
		&data.UpdatedAt,
		&data.UpdatedBy,
		&data.Active,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return InstrumentData{}, fmt.Errorf("instrument not found for %s on %s", symbol, exchange)
		}
		return InstrumentData{}, fmt.Errorf("failed to get instrument: %w", err)
	}

	return data, nil
}
