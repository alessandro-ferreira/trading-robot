package repository

import (
	"context"
	"database/sql"
	"fmt"
)

// RiskPairData represents the risk parameters for a specific pair.
type RiskPairData struct {
	ExchangeName     string
	InstrumentSymbol string
	RiskPerTrade     float64
	MaxPositionSize  sql.NullFloat64
}

// RisksRepo defines the interface for managing pair risk configurations.
type RisksRepo interface {
	GetRiskPair(ctx context.Context, db DBExecutor, exchange, symbol string) (RiskPairData, error)
}

type pgRisksRepo struct{}

// NewRisksRepo creates a new PostgreSQL RisksRepo.
func NewRisksRepo() RisksRepo {
	return &pgRisksRepo{}
}

func (r *pgRisksRepo) GetRiskPair(ctx context.Context, db DBExecutor, exchange, symbol string) (RiskPairData, error) {
	query := `
		SELECT
			e.name,
			i.name,
			rp.risk_per_trade,
			rp.max_position_size
		FROM trading.risk_pairs rp
		INNER JOIN trading.exchanges e ON rp.exchange_id = e.id
		INNER JOIN trading.instruments i ON rp.instrument_id = i.id
		WHERE e.name = $1 AND i.name = $2 AND rp.active = TRUE
	`

	var data RiskPairData
	err := db.QueryRow(ctx, query, exchange, symbol).Scan(
		&data.ExchangeName,
		&data.InstrumentSymbol,
		&data.RiskPerTrade,
		&data.MaxPositionSize,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return RiskPairData{}, fmt.Errorf("risk configuration not found for %s on %s", symbol, exchange)
		}
		return RiskPairData{}, fmt.Errorf("failed to get risk pair: %w", err)
	}

	return data, nil
}
