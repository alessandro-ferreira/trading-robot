-- Table to store pair-specific risk parameters
CREATE TABLE IF NOT EXISTS trading.risk_pairs (
    id BIGSERIAL PRIMARY KEY,
    exchange_id BIGINT NOT NULL,
    instrument_id BIGINT NOT NULL,
    risk_per_trade NUMERIC NOT NULL,
    max_position_size NUMERIC,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by TEXT,
    updated_at TIMESTAMPTZ,
    updated_by TEXT,
    active BOOLEAN NOT NULL DEFAULT TRUE
);

ALTER TABLE trading.risk_pairs ADD CONSTRAINT fk_risk_pairs_exchange FOREIGN KEY (exchange_id)
    REFERENCES trading.exchanges(id) ON UPDATE CASCADE ON DELETE RESTRICT;
ALTER TABLE trading.risk_pairs ADD CONSTRAINT fk_risk_pairs_instrument FOREIGN KEY (instrument_id)
    REFERENCES trading.instruments(id) ON UPDATE CASCADE ON DELETE RESTRICT;

-- Ensure one active risk configuration per pair per exchange
CREATE UNIQUE INDEX idx_risk_pairs_unique ON trading.risk_pairs(exchange_id, instrument_id) WHERE active;
