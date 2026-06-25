-- Define ENUMs for position sides to ensure data consistency
CREATE TYPE trading.position_side AS ENUM ('long', 'short');

-- Table to track open trading positions per exchange and instrument
CREATE TABLE IF NOT EXISTS trading.positions (
    id BIGSERIAL PRIMARY KEY,
    exchange_id BIGINT NOT NULL,
    instrument_id BIGINT NOT NULL,
    order_id BIGINT,
    side trading.position_side NOT NULL,
    quantity NUMERIC NOT NULL,
    entry_price NUMERIC NOT NULL,
    highest_price NUMERIC NOT NULL DEFAULT 0,
    stop_loss_active BOOLEAN NOT NULL DEFAULT FALSE,
    unknown_origin BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by TEXT,
    updated_at TIMESTAMPTZ,
    updated_by TEXT,
    active BOOLEAN NOT NULL DEFAULT TRUE
);

ALTER TABLE trading.positions ADD CONSTRAINT fk_positions_exchange FOREIGN KEY (exchange_id)
    REFERENCES trading.exchanges(id) ON UPDATE CASCADE ON DELETE RESTRICT;
ALTER TABLE trading.positions ADD CONSTRAINT fk_positions_instrument FOREIGN KEY (instrument_id)
    REFERENCES trading.instruments(id) ON UPDATE CASCADE ON DELETE RESTRICT;

-- Relational links to the execution lifecycle
ALTER TABLE trading.positions ADD CONSTRAINT fk_positions_order FOREIGN KEY (order_id)
    REFERENCES trading.orders(id) ON UPDATE CASCADE ON DELETE SET NULL;

-- Ensure only one active position per exchange and instrument
CREATE UNIQUE INDEX idx_positions_exchange_instrument_active
    ON trading.positions(exchange_id, instrument_id) WHERE active;

-- Ensure that if unknown_origin is FALSE, we require a valid order_id, quantity, and entry_price.
ALTER TABLE trading.positions ADD CONSTRAINT check_position_origin_completeness CHECK
    ( (unknown_origin = TRUE) OR (order_id IS NOT NULL AND order_id > 0 AND quantity > 0 AND entry_price > 0) );
