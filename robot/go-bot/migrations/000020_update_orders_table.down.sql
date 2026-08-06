-- Remove the unique index that allows multiple 'new' orders without an exchange order ID.
DROP INDEX IF EXISTS trading.idx_orders_new_without_exchange_order_id;

-- Restore the original requirement that every order has an exchange order ID.
ALTER TABLE trading.orders ALTER COLUMN exchange_order_id SET NOT NULL;

-- Restore the original uniqueness constraint for active orders.
DROP INDEX IF EXISTS trading.idx_orders_exchange_order_id_active;
CREATE UNIQUE INDEX idx_orders_exchange_order_id_active ON trading.orders(exchange_id, exchange_order_id) WHERE active;
