-- Allow exchange_order_id to be NULL while an order is not yet linked to an exchange order.
ALTER TABLE trading.orders ALTER COLUMN exchange_order_id DROP NOT NULL;

-- Ensure an exchange order ID is unique among active orders for each exchange.
DROP INDEX IF EXISTS trading.idx_orders_exchange_order_id_active;
CREATE UNIQUE INDEX idx_orders_exchange_order_id_active ON trading.orders(exchange_id, exchange_order_id)
    WHERE active AND exchange_order_id IS NOT NULL;

-- Ensure only one unresolved 'new' order exists per exchange/instrument.
CREATE UNIQUE INDEX idx_orders_new_without_exchange_order_id ON trading.orders(exchange_id, instrument_id)
    WHERE active AND order_status = 'new' AND exchange_order_id IS NULL;
