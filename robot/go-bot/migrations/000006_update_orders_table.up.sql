-- Add execution and fee tracking columns to the orders table

ALTER TABLE trading.orders
    ADD COLUMN fee NUMERIC,
    ADD COLUMN fee_asset_id BIGINT;

-- Link the fee asset to the assets table for referential integrity.
ALTER TABLE trading.orders ADD CONSTRAINT fk_orders_fee_asset FOREIGN KEY (fee_asset_id)
    REFERENCES trading.assets(id) ON UPDATE CASCADE ON DELETE RESTRICT;

-- Index to support fee-based reporting and audits.
CREATE INDEX idx_orders_fee_asset_id ON trading.orders(fee_asset_id);
