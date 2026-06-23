-- Insert Risk Configurations for Dummy Exchange
DO $$
DECLARE
    v_dummy_id BIGINT;
    v_btc_id BIGINT;
    v_eth_id BIGINT;
    v_ltc_id BIGINT;
    v_created_by TEXT := 'migration_000016';
BEGIN
    -- Fetch IDs for dummy exchange and instruments
    SELECT id INTO v_dummy_id FROM trading.exchanges WHERE name = 'dummy' AND active;
    SELECT id INTO v_btc_id FROM trading.instruments WHERE name = 'BTC/USDT' AND exchange_id = v_dummy_id AND active;
    SELECT id INTO v_eth_id FROM trading.instruments WHERE name = 'ETH/USDT' AND exchange_id = v_dummy_id AND active;
    SELECT id INTO v_ltc_id FROM trading.instruments WHERE name = 'LTC/USDT' AND exchange_id = v_dummy_id AND active;

    -- Insert Risk Configurations for Dummy Exchange
    INSERT INTO trading.risk_pairs (exchange_id, instrument_id, allocated_budget, max_asset_units, created_by)
    VALUES
        (v_dummy_id, v_btc_id, 500.0, 0.01, v_created_by),
        (v_dummy_id, v_eth_id, 300.0, 1.0, v_created_by),
        (v_dummy_id, v_ltc_id, 200.0, 25.0, v_created_by);
END $$;
