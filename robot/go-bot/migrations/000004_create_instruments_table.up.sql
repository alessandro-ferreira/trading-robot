-- Table to store tradable instruments (pairs) and their specific rules per exchange
CREATE TABLE IF NOT EXISTS trading.instruments (
    id BIGSERIAL PRIMARY KEY,
    exchange_id BIGINT NOT NULL,
    name TEXT NOT NULL,
    base_asset_id BIGINT NOT NULL,
    quote_asset_id BIGINT NOT NULL,
    price_precision INT NOT NULL,
    amount_precision INT NOT NULL,
    min_amount NUMERIC NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by TEXT,
    updated_at TIMESTAMPTZ,
    updated_by TEXT,
    active BOOLEAN NOT NULL DEFAULT TRUE
);

ALTER TABLE trading.instruments ADD CONSTRAINT fk_instruments_exchange FOREIGN KEY (exchange_id) REFERENCES trading.exchanges(id) ON UPDATE CASCADE ON DELETE RESTRICT;
ALTER TABLE trading.instruments ADD CONSTRAINT fk_instruments_base_asset FOREIGN KEY (base_asset_id) REFERENCES trading.assets(id) ON UPDATE CASCADE ON DELETE RESTRICT;
ALTER TABLE trading.instruments ADD CONSTRAINT fk_instruments_quote_asset FOREIGN KEY (quote_asset_id) REFERENCES trading.assets(id) ON UPDATE CASCADE ON DELETE RESTRICT;

-- Ensure symbols are unique per exchange among active records
CREATE UNIQUE INDEX idx_instruments_exchange_name_active ON trading.instruments(exchange_id, name) WHERE active;
