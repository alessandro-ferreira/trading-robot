-- Revert the client_order_id column to allow NULL values
ALTER TABLE trading.orders ALTER COLUMN client_order_id DROP NOT NULL;

-- Revert client_order_id for orders that were prefixed with 'deprecated-'
UPDATE trading.orders SET client_order_id = NULL,
    updated_by = 'migration_000022-rollback', updated_at = NOW()
    WHERE client_order_id LIKE 'deprecated-%';
