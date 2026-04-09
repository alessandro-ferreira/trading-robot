-- Table to store historical price ticks
-- Used for strategy 'warm-up' during restarts or AI model hydration
CREATE TABLE IF NOT EXISTS trading.market_data_ticks (
    exchange_id BIGINT NOT NULL,
    instrument_id BIGINT NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL,
    price NUMERIC NOT NULL
);

-- Convert to hypertable for TimescaleDB optimization
SELECT create_hypertable('trading.market_data_ticks', 'timestamp', if_not_exists => TRUE);

-- Index for efficient lookup of historical data for a specific instrument
CREATE INDEX idx_market_data_ticks_lookup ON trading.market_data_ticks (exchange_id, instrument_id, timestamp DESC);
