-- Update client_order_id for orders where it is NULL or empty, prefixing with 'deprecated-'
UPDATE trading.orders SET client_order_id = CONCAT('deprecated-', id::text),
    updated_by = 'migration_000022', updated_at = NOW()
    WHERE client_order_id IS NULL OR TRIM(client_order_id) = '';

-- Set client_order_id column to NOT NULL after updating existing rows
ALTER TABLE trading.orders ALTER COLUMN client_order_id SET NOT NULL;
