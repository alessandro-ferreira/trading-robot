-- Table to track current wallet balances per exchange and asset
CREATE TABLE IF NOT EXISTS trading.balances (
    id BIGSERIAL PRIMARY KEY,
    exchange_id BIGINT NOT NULL,
    asset_id BIGINT NOT NULL,
    free NUMERIC NOT NULL DEFAULT 0,
    used NUMERIC NOT NULL DEFAULT 0,
    total NUMERIC NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by TEXT,
    updated_at TIMESTAMPTZ,
    updated_by TEXT,
    active BOOLEAN NOT NULL DEFAULT TRUE
);

ALTER TABLE trading.balances ADD CONSTRAINT fk_balances_exchange FOREIGN KEY (exchange_id) REFERENCES trading.exchanges(id) ON UPDATE CASCADE ON DELETE RESTRICT;
ALTER TABLE trading.balances ADD CONSTRAINT fk_balances_asset FOREIGN KEY (asset_id) REFERENCES trading.assets(id) ON UPDATE CASCADE ON DELETE RESTRICT;
-- Ensure unique balance record per asset per exchange
CREATE UNIQUE INDEX idx_balances_exchange_asset_active ON trading.balances(exchange_id, asset_id) WHERE active = TRUE;
