-- Table to store supported exchanges (e.g., 'binance', 'mercadobitcoin')
CREATE TABLE IF NOT EXISTS trading.exchanges (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by TEXT,
    updated_at TIMESTAMPTZ,
    updated_by TEXT,
    active BOOLEAN NOT NULL DEFAULT TRUE
);

-- Ensure exchange names are unique among active records
CREATE UNIQUE INDEX idx_exchanges_name_active ON trading.exchanges(name) WHERE active = TRUE;
