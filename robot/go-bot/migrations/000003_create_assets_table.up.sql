-- Table to store canonical assets (e.g., 'BTC', 'USDT', 'BRL')
CREATE TABLE IF NOT EXISTS trading.assets (
    id BIGSERIAL PRIMARY KEY,
    symbol TEXT NOT NULL,
    name TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by TEXT,
    updated_at TIMESTAMPTZ,
    updated_by TEXT,
    active BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE UNIQUE INDEX idx_assets_symbol_active ON trading.assets(symbol) WHERE active = TRUE;