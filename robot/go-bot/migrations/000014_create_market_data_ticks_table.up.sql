-- Table to store historical price ticks
-- Used for strategy 'warm-up' during restarts or AI model hydration
CREATE TABLE IF NOT EXISTS trading.market_data_ticks (
    exchange_id BIGINT NOT NULL,
    instrument_id BIGINT NOT NULL,
    tick_unix_at BIGINT NOT NULL,
    price NUMERIC NOT NULL
);

-- Convert to hypertable for TimescaleDB optimization
SELECT create_hypertable('trading.market_data_ticks', 'tick_unix_at', chunk_time_interval => 86400, if_not_exists => TRUE);

-- Index for efficient lookup of historical data for a specific instrument
CREATE INDEX idx_market_data_ticks_lookup ON trading.market_data_ticks (exchange_id, instrument_id, tick_unix_at DESC);
