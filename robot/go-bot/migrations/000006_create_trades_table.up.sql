-- Table to record actual trade executions (fills)
CREATE TABLE IF NOT EXISTS trading.trades (
    id BIGSERIAL PRIMARY KEY,
    exchange_trade_id TEXT NOT NULL,
    order_id BIGINT NOT NULL,
    exchange_id BIGINT NOT NULL,
    instrument_id BIGINT NOT NULL,
    side TEXT NOT NULL,
    price NUMERIC NOT NULL,
    amount NUMERIC NOT NULL,
    cost NUMERIC NOT NULL,
    fee NUMERIC,
    fee_asset_id BIGINT,
    trade_timestamp TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by TEXT,
    updated_at TIMESTAMPTZ,
    updated_by TEXT,
    active BOOLEAN NOT NULL DEFAULT TRUE
);

-- Foreign keys linking trades to orders, exchanges, and instruments
ALTER TABLE trading.trades ADD CONSTRAINT fk_trades_order FOREIGN KEY (order_id) REFERENCES trading.orders(id) ON UPDATE CASCADE ON DELETE RESTRICT;
ALTER TABLE trading.trades ADD CONSTRAINT fk_trades_exchange FOREIGN KEY (exchange_id) REFERENCES trading.exchanges(id) ON UPDATE CASCADE ON DELETE RESTRICT;
ALTER TABLE trading.trades ADD CONSTRAINT fk_trades_instrument FOREIGN KEY (instrument_id) REFERENCES trading.instruments(id) ON UPDATE CASCADE ON DELETE RESTRICT;
ALTER TABLE trading.trades ADD CONSTRAINT fk_trades_fee_asset FOREIGN KEY (fee_asset_id) REFERENCES trading.assets(id) ON UPDATE CASCADE ON DELETE RESTRICT;

-- Indexes for querying trades by order, time, or external ID
CREATE INDEX idx_trades_order_id ON trading.trades(order_id);
CREATE INDEX idx_trades_timestamp ON trading.trades(trade_timestamp DESC);
CREATE INDEX idx_trades_active ON trading.trades(active);
CREATE UNIQUE INDEX idx_trades_exchange_trade_id_active ON trading.trades(exchange_id, exchange_trade_id) WHERE active = TRUE;