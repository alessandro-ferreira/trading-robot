-- Define ENUMs for order types and statuses to ensure data consistency
CREATE TYPE trading.order_type AS ENUM ('limit', 'market', 'stop_limit', 'stop_market');
CREATE TYPE trading.order_status AS ENUM ('new', 'open', 'closed', 'canceled', 'rejected', 'expired');

-- Core table to track the lifecycle of orders
CREATE TABLE IF NOT EXISTS trading.orders (
    id BIGSERIAL PRIMARY KEY,
    exchange_order_id TEXT NOT NULL,
    client_order_id TEXT,
    exchange_id BIGINT NOT NULL,
    instrument_id BIGINT NOT NULL,
    side TEXT NOT NULL,
    order_type trading.order_type NOT NULL,
    price NUMERIC,
    amount NUMERIC NOT NULL,
    filled NUMERIC NOT NULL DEFAULT 0,
    remaining NUMERIC NOT NULL DEFAULT 0,
    average_price NUMERIC,
    cost NUMERIC NOT NULL DEFAULT 0,
    order_status trading.order_status NOT NULL,
    error_message TEXT,
    exchange_timestamp TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by TEXT,
    updated_at TIMESTAMPTZ,
    updated_by TEXT,
    active BOOLEAN NOT NULL DEFAULT TRUE
);

-- Foreign keys for data integrity
ALTER TABLE trading.orders ADD CONSTRAINT fk_orders_exchange FOREIGN KEY (exchange_id)
    REFERENCES trading.exchanges(id) ON UPDATE CASCADE ON DELETE RESTRICT;
ALTER TABLE trading.orders ADD CONSTRAINT fk_orders_instrument FOREIGN KEY (instrument_id)
    REFERENCES trading.instruments(id) ON UPDATE CASCADE ON DELETE RESTRICT;

-- Indexes for performance and uniqueness
CREATE UNIQUE INDEX idx_orders_exchange_order_id_active ON trading.orders(exchange_id, exchange_order_id) WHERE active;
CREATE UNIQUE INDEX idx_orders_client_order_id_active ON trading.orders(exchange_id, client_order_id) WHERE active;
CREATE INDEX idx_orders_status ON trading.orders(order_status);
CREATE INDEX idx_orders_active ON trading.orders(active);
