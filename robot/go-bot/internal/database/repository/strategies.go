package repository

import (
	"context"
	"database/sql"
	"errors"
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
	GetStrategyPairs(ctx context.Context, db DBExecutor, onlyEnabled bool) ([]StrategyPair, error)
	UpsertEnabledStrategy(ctx context.Context, db DBExecutor, exchangeName, symbol, strategyType, label string, momentum StrategyMomentum) error
}

type pgStrategiesRepo struct{}

// NewStrategiesRepo creates a new PostgreSQL StrategiesRepo.
func NewStrategiesRepo() StrategiesRepo {
	return &pgStrategiesRepo{}
}

// GetStrategyPairs retrieves strategy pairs with optional filtering by enabled status.
func (r *pgStrategiesRepo) GetStrategyPairs(ctx context.Context, db DBExecutor, onlyEnabled bool) ([]StrategyPair, error) {
	query := `
		SELECT
			e.name AS exchange_name,
			i.name AS instrument_symbol,
			sp.strategy_type,
			sm.window_seconds,
			ARRAY(SELECT (m).lookback_seconds FROM unnest(sm.momentum_windows) AS m ORDER BY (m).lookback_seconds) AS lookbacks,
			ARRAY(SELECT (m).threshold FROM unnest(sm.momentum_windows) AS m ORDER BY (m).lookback_seconds) AS thresholds,
			sm.require_all,
			sm.stop_loss_pct,
			sm.profit_target_pct,
			sm.activation_pct,
			sm.trailing_stop_pct
		FROM trading.strategy_pairs sp
		INNER JOIN trading.exchanges e ON e.id = sp.exchange_id AND e.active
		INNER JOIN trading.instruments i ON i.id = sp.instrument_id AND i.exchange_id = sp.exchange_id AND i.active
		LEFT JOIN trading.strategy_momentum sm ON sp.id = sm.strategy_pair_id AND sm.is_enabled AND sm.active
		WHERE (sp.is_enabled OR NOT $1) AND sp.active
	`

	rows, err := db.Query(ctx, query, onlyEnabled)
	if err != nil {
		return nil, fmt.Errorf("failed to query strategy pairs: %w", err)
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

// UpsertEnabledStrategy dynamically changes or creates the enabled strategy and configuration for a pair.
func (r *pgStrategiesRepo) UpsertEnabledStrategy(ctx context.Context, db DBExecutor, exchangeName, symbol, strategyType, label string, momentum StrategyMomentum) (err error) {
	// Select exchange_id and instrument_id
	selectQuery := `
		SELECT i.exchange_id, i.id
		FROM trading.instruments i
		INNER JOIN trading.exchanges e ON e.id = i.exchange_id AND e.name = $1 AND e.active
		WHERE i.name = $2 AND i.active
	`
	var exchangeID, instrumentID int64
	err = db.QueryRow(ctx, selectQuery, exchangeName, symbol).Scan(
		&exchangeID,
		&instrumentID,
	)
	if err != nil {
		return fmt.Errorf("failed to resolve exchange and instrument IDs: %w", err)
	}

	// Begin a transaction to ensure atomicity of the upsert operation.
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	// Disable the currently enabled strategy for this specific pair to prevent unique index collisions.
	disablePair := `
		UPDATE trading.strategy_pairs
		SET is_enabled = FALSE, updated_at = NOW(), updated_by = $3
		WHERE exchange_id = $1 AND instrument_id = $2 AND is_enabled AND active
	`
	if _, err = tx.Exec(ctx, disablePair, exchangeID, instrumentID, DefaultUser); err != nil {
		return fmt.Errorf("failed to disable current strategy pair: %w", err)
	}

	// Try to update the target strategy pair to enabled.
	updatePair := `
		UPDATE trading.strategy_pairs
		SET is_enabled = TRUE, updated_at = NOW(), updated_by = $4
		WHERE exchange_id = $1 AND instrument_id = $2 AND strategy_type = $3 AND active
		RETURNING id
	`
	var pairID int64
	err = tx.QueryRow(ctx, updatePair, exchangeID, instrumentID, strategyType, DefaultUser).Scan(&pairID)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to update strategy pair: %w", err)
	}

	if errors.Is(err, sql.ErrNoRows) {
		// Insert the strategy pair if it does not exist.
		insertPair := `
			INSERT INTO trading.strategy_pairs (exchange_id, instrument_id, strategy_type, is_enabled, active, created_by)
			VALUES ($1, $2, $3, TRUE, TRUE, $4)
			RETURNING id
		`
		if err = tx.QueryRow(ctx, insertPair, exchangeID, instrumentID, strategyType, DefaultUser).Scan(&pairID); err != nil {
			return fmt.Errorf("failed to insert strategy pair: %w", err)
		}
	}

	if strategyType == StrategyDummy {
		return tx.Commit(ctx)
	}

	// Disable any currently enabled momentum configuration for this strategy pair and type.
	disableMom := `
		UPDATE trading.strategy_momentum
		SET is_enabled = FALSE, updated_at = NOW(), updated_by = $3
		WHERE strategy_pair_id = $1 AND strategy_type = $2 AND is_enabled AND active
	`
	if _, err = tx.Exec(ctx, disableMom, pairID, strategyType, DefaultUser); err != nil {
		return fmt.Errorf("failed to disable current momentum config: %w", err)
	}

	// Resolve momentum window data for the SQL arrays.
	lookbacks := make([]int32, len(momentum.Windows))
	thresholds := make([]float64, len(momentum.Windows))
	for i, w := range momentum.Windows {
		lookbacks[i] = int32(w.LookbackSeconds)
		thresholds[i] = w.Threshold
	}

	// Try to update an existing momentum configuration with new values.
	updateMom := `
		UPDATE trading.strategy_momentum
		SET
			is_enabled = TRUE,
			window_seconds = $4,
			momentum_windows = ARRAY(SELECT ROW(l, t)::trading.momentum_window FROM unnest($5::int[], $6::numeric[]) AS x(l, t)),
			require_all = $7,
			stop_loss_pct = $8,
			profit_target_pct = $9,
			activation_pct = $10,
			trailing_stop_pct = $11,
			updated_at = NOW(),
			updated_by = $12
		WHERE strategy_pair_id = $1 AND strategy_type = $2 AND label = $3 AND active
		RETURNING id
	`
	var momID int64
	err = tx.QueryRow(ctx, updateMom,
		pairID, strategyType, label,
		momentum.WindowSeconds, lookbacks, thresholds,
		momentum.RequireAll, momentum.StopLossPct,
		sql.NullFloat64{Float64: momentum.ProfitTargetPct, Valid: strategyType == StrategyMomentumProfit},
		sql.NullFloat64{Float64: momentum.ActivationPct, Valid: strategyType == StrategyMomentumTrailing},
		sql.NullFloat64{Float64: momentum.TrailingStopPct, Valid: strategyType == StrategyMomentumTrailing},
		DefaultUser,
	).Scan(&momID)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to update momentum configuration: %w", err)
	}

	if errors.Is(err, sql.ErrNoRows) {
		// Insert the new momentum configuration.
		insertMom := `
			INSERT INTO trading.strategy_momentum (
				label, strategy_pair_id, strategy_type, is_enabled, window_seconds,
				momentum_windows, require_all, stop_loss_pct, profit_target_pct,
				activation_pct, trailing_stop_pct, active, created_by
			)
			VALUES (
				$1, $2, $3, TRUE, $4,
				ARRAY(SELECT ROW(l, t)::trading.momentum_window FROM unnest($5::int[], $6::numeric[]) AS x(l, t)),
				$7, $8, $9, $10, $11, TRUE, $12
			)
		`
		_, err = tx.Exec(ctx, insertMom,
			label, pairID, strategyType, momentum.WindowSeconds, lookbacks, thresholds,
			momentum.RequireAll, momentum.StopLossPct,
			sql.NullFloat64{Float64: momentum.ProfitTargetPct, Valid: strategyType == StrategyMomentumProfit},
			sql.NullFloat64{Float64: momentum.ActivationPct, Valid: strategyType == StrategyMomentumTrailing},
			sql.NullFloat64{Float64: momentum.TrailingStopPct, Valid: strategyType == StrategyMomentumTrailing},
			DefaultUser,
		)
		if err != nil {
			return fmt.Errorf("failed to insert momentum configuration: %w", err)
		}
	}

	err = tx.Commit(ctx)
	return err
}
