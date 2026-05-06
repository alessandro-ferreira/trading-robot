-- Define ENUM for supported strategy types
CREATE TYPE trading.strategy_type AS ENUM ('dummy', 'momentum_profit', 'momentum_trailing');

-- Define ENUM for strategy operational status
CREATE TYPE trading.strategy_status AS ENUM ('enabled', 'pending_disabled', 'disabled');

-- Define composite type for momentum windows
CREATE TYPE trading.momentum_window AS (
    lookback_seconds INT,
    threshold NUMERIC
);

-- Table to link instruments to specific strategy types
CREATE TABLE IF NOT EXISTS trading.strategy_pairs (
    id BIGSERIAL PRIMARY KEY,
    exchange_id BIGINT NOT NULL,
    instrument_id BIGINT NOT NULL,
    strategy_type trading.strategy_type NOT NULL,
    status trading.strategy_status NOT NULL DEFAULT 'enabled',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by TEXT,
    updated_at TIMESTAMPTZ,
    updated_by TEXT,
    active BOOLEAN NOT NULL DEFAULT TRUE
);

ALTER TABLE trading.strategy_pairs ADD CONSTRAINT fk_strategy_pairs_exchange FOREIGN KEY (exchange_id) REFERENCES trading.exchanges(id) ON UPDATE CASCADE ON DELETE RESTRICT;
ALTER TABLE trading.strategy_pairs ADD CONSTRAINT fk_strategy_pairs_instrument FOREIGN KEY (instrument_id) REFERENCES trading.instruments(id) ON UPDATE CASCADE ON DELETE RESTRICT;

-- Ensure we have only one active strategy type per exchange/instrument pair
CREATE UNIQUE INDEX idx_strategy_pairs_exchange_instrument_type_active 
    ON trading.strategy_pairs(exchange_id, instrument_id, strategy_type) WHERE active;
-- Allow only one strategy to be in a runnable state (enabled or pending_disabled) per exchange/instrument pair
CREATE UNIQUE INDEX idx_strategy_pairs_exchange_instrument_runnable 
    ON trading.strategy_pairs(exchange_id, instrument_id) WHERE status IN ('enabled', 'pending_disabled') AND active;

-- Table to store momentum-specific parameters linked directly to a strategy pair
CREATE TABLE IF NOT EXISTS trading.strategy_momentum (
    id BIGSERIAL PRIMARY KEY,
    label TEXT NOT NULL,
    strategy_pair_id BIGINT NOT NULL,
    strategy_type trading.strategy_type NOT NULL,
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    window_seconds INT NOT NULL,
    momentum_windows trading.momentum_window[] NOT NULL,
    require_all BOOLEAN NOT NULL DEFAULT FALSE,
    stop_loss_pct NUMERIC NOT NULL,
    profit_target_pct NUMERIC, -- Specific to momentum_profit
    activation_pct NUMERIC,    -- Specific to momentum_trailing
    trailing_stop_pct NUMERIC, -- Specific to momentum_trailing
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by TEXT,
    updated_at TIMESTAMPTZ,
    updated_by TEXT,
    active BOOLEAN NOT NULL DEFAULT TRUE
);

ALTER TABLE trading.strategy_momentum ADD CONSTRAINT fk_strategy_momentum_pair FOREIGN KEY (strategy_pair_id)
    REFERENCES trading.strategy_pairs(id) ON UPDATE CASCADE ON DELETE CASCADE;

-- Ensure we have only one enabled configuration and unique labels per strategy pair and type
CREATE UNIQUE INDEX idx_strategy_momentum_pair_type_label
    ON trading.strategy_momentum(strategy_pair_id, strategy_type, label) WHERE active;
CREATE UNIQUE INDEX idx_strategy_momentum_pair_type_enabled
    ON trading.strategy_momentum(strategy_pair_id, strategy_type) WHERE is_enabled AND active;

-- Add constraints to ensure consistency of strategy parameters based on the strategy type
ALTER TABLE trading.strategy_momentum ADD CONSTRAINT check_momentum_type
    CHECK (strategy_type IN ('momentum_profit', 'momentum_trailing'));
ALTER TABLE trading.strategy_momentum ADD CONSTRAINT check_momentum_params_consistency CHECK (
    (strategy_type = 'momentum_profit' AND profit_target_pct IS NOT NULL AND activation_pct IS NULL AND trailing_stop_pct IS NULL) OR
    (strategy_type = 'momentum_trailing' AND profit_target_pct IS NULL AND activation_pct IS NOT NULL AND trailing_stop_pct IS NOT NULL)
);
