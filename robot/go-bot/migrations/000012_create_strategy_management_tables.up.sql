-- Define ENUM for supported strategy types
CREATE TYPE trading.strategy_type AS ENUM ('dummy', 'momentum_profit', 'momentum_trailing');

-- Table to store strategy configurations (hyperparameters)
-- Parameters are stored as JSONB to allow flexibility between different strategy types (Profit vs Trailing)
CREATE TABLE IF NOT EXISTS trading.strategy_configs (
    id BIGSERIAL PRIMARY KEY,
    label TEXT NOT NULL,
    strategy_type trading.strategy_type NOT NULL,
    parameters JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by TEXT,
    updated_at TIMESTAMPTZ,
    updated_by TEXT,
    active BOOLEAN NOT NULL DEFAULT TRUE
);

-- Ensure config labels are unique
CREATE UNIQUE INDEX idx_strategy_configs_label_active ON trading.strategy_configs(label) WHERE active = TRUE;

-- Table to link instruments to specific strategy configurations
-- This allows the external AI engine to swap configs or toggle pairs on/off
CREATE TABLE IF NOT EXISTS trading.strategy_pairs (
    id BIGSERIAL PRIMARY KEY,
    exchange_id BIGINT NOT NULL,
    instrument_id BIGINT NOT NULL,
    strategy_config_id BIGINT NOT NULL,
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE, -- Operational toggle
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by TEXT,
    updated_at TIMESTAMPTZ,
    updated_by TEXT,
    active BOOLEAN NOT NULL DEFAULT TRUE -- Soft delete
);

ALTER TABLE trading.strategy_pairs ADD CONSTRAINT fk_strategy_pairs_exchange FOREIGN KEY (exchange_id) REFERENCES trading.exchanges(id) ON UPDATE CASCADE ON DELETE RESTRICT;
ALTER TABLE trading.strategy_pairs ADD CONSTRAINT fk_strategy_pairs_instrument FOREIGN KEY (instrument_id) REFERENCES trading.instruments(id) ON UPDATE CASCADE ON DELETE RESTRICT;
ALTER TABLE trading.strategy_pairs ADD CONSTRAINT fk_strategy_pairs_config FOREIGN KEY (strategy_config_id) REFERENCES trading.strategy_configs(id) ON UPDATE CASCADE ON DELETE RESTRICT;

-- Ensure an instrument has only one enabled strategy per exchange at a time
CREATE UNIQUE INDEX idx_strategy_pairs_instrument_enabled ON trading.strategy_pairs(exchange_id, instrument_id) WHERE active = TRUE AND is_enabled = TRUE;
