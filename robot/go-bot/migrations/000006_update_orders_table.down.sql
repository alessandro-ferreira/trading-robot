-- Remove execution and fee tracking columns from the orders table

ALTER TABLE trading.orders
    DROP COLUMN IF EXISTS fee,
    DROP COLUMN IF EXISTS fee_asset_id;
