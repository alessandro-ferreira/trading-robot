package repository

import (
	"context"
	"database/sql"
	"fmt"
)

const (
	StrategyDummy            = "dummy"
	StrategyMomentumProfit   = "momentum_profit"
	StrategyMomentumTrailing = "momentum_trailing"
)

// DefaultWarmupWindow is used when a strategy configuration (like dummy) doesn't specify a window.
const DefaultWarmupWindow = 300

// MomentumWindow represents the trading.momentum_window composite type.
type MomentumWindow struct {
	LookbackSeconds int
	Threshold       float64
}

// StrategyMomentum represents parameters from the trading.strategy_momentum table.
type StrategyMomentum struct {
	WindowSeconds   int
	Windows         []MomentumWindow
	RequireAll      bool
	StopLossPct     float64
	ProfitTargetPct float64
	ActivationPct   float64
	TrailingStopPct float64
}

// StrategyPair represents metadata from the trading.strategy_pairs table.
type StrategyPair struct {
	ExchangeName     string
	InstrumentSymbol string
	Type             string
	WarmupWindow     int
	Momentum         StrategyMomentum
}

// StrategiesRepo defines the interface for managing strategy configurations.
type StrategiesRepo interface {
	GetActiveStrategyPairs(ctx context.Context, db DBExecutor) ([]StrategyPair, error)
}

type pgStrategiesRepo struct{}

// NewStrategiesRepo creates a new PostgreSQL StrategiesRepo.
func NewStrategiesRepo() StrategiesRepo {
	return &pgStrategiesRepo{}
}

func (r *pgStrategiesRepo) GetActiveStrategyPairs(ctx context.Context, db DBExecutor) ([]StrategyPair, error) {
	query := `
		SELECT
			e.name AS exchange_name,
			i.name AS instrument_symbol,
			sp.strategy_type,
			sm.window_seconds,
			ARRAY(SELECT (m).lookback_seconds FROM unnest(sm.momentum_windows) AS m) AS lookbacks,
			ARRAY(SELECT (m).threshold FROM unnest(sm.momentum_windows) AS m) AS thresholds,
			sm.require_all,
			sm.stop_loss_pct,
			sm.profit_target_pct,
			sm.activation_pct,
			sm.trailing_stop_pct
		FROM trading.strategy_pairs sp
		INNER JOIN trading.exchanges e ON sp.exchange_id = e.id AND e.active = TRUE
		INNER JOIN trading.instruments i ON sp.instrument_id = i.id AND i.active = TRUE
		LEFT JOIN trading.strategy_momentum sm ON sp.id = sm.strategy_pair_id AND sm.active = TRUE AND sm.is_enabled = TRUE
		WHERE sp.active = TRUE
	`

	rows, err := db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query active strategy pairs: %w", err)
	}
	defer rows.Close()

	var pairs []StrategyPair
	for rows.Next() {
		var p StrategyPair
		var windowSeconds sql.NullInt32
		var lookbacks []int32
		var thresholds []float64
		var requireAll sql.NullBool
		var stopLoss, profitTarget, activationPct, trailingStopPct sql.NullFloat64

		err := rows.Scan(
			&p.ExchangeName,
			&p.InstrumentSymbol,
			&p.Type,
			&windowSeconds,
			&lookbacks,
			&thresholds,
			&requireAll,
			&stopLoss,
			&profitTarget,
			&activationPct,
			&trailingStopPct,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan strategy pair: %w", err)
		}

		if windowSeconds.Valid {
			p.Momentum.WindowSeconds = int(windowSeconds.Int32)
			p.WarmupWindow = p.Momentum.WindowSeconds
		} else {
			p.WarmupWindow = DefaultWarmupWindow
		}

		p.Momentum.RequireAll = requireAll.Bool
		p.Momentum.StopLossPct = stopLoss.Float64

		if len(lookbacks) == len(thresholds) {
			p.Momentum.Windows = make([]MomentumWindow, len(lookbacks))
			for i := range lookbacks {
				p.Momentum.Windows[i] = MomentumWindow{
					LookbackSeconds: int(lookbacks[i]),
					Threshold:       thresholds[i],
				}
			}
		}

		// Map nullable DB fields to config struct
		p.Momentum.ProfitTargetPct = profitTarget.Float64
		p.Momentum.ActivationPct = activationPct.Float64
		p.Momentum.TrailingStopPct = trailingStopPct.Float64

		pairs = append(pairs, p)
	}
	return pairs, rows.Err()
}
