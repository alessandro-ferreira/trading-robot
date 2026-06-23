-- Insert instruments
DO $$
DECLARE
    v_binance_id BIGINT;
    v_mb_id BIGINT;
    v_dummy_id BIGINT;
    v_btc_id BIGINT;
    v_eth_id BIGINT;
    v_ltc_id BIGINT;
    v_sol_id BIGINT;
    v_usdt_id BIGINT;
    v_brl_id BIGINT;
    v_created_by TEXT := 'migration_000010';
BEGIN
    -- Fetch Exchange IDs
    SELECT id INTO v_binance_id FROM trading.exchanges WHERE name = 'binance' AND active;
    SELECT id INTO v_mb_id FROM trading.exchanges WHERE name = 'mercadobitcoin' AND active;
    SELECT id INTO v_dummy_id FROM trading.exchanges WHERE name = 'dummy' AND active;

    -- Fetch Asset IDs
    SELECT id INTO v_btc_id FROM trading.assets WHERE symbol = 'BTC' AND active;
    SELECT id INTO v_eth_id FROM trading.assets WHERE symbol = 'ETH' AND active;
    SELECT id INTO v_ltc_id FROM trading.assets WHERE symbol = 'LTC' AND active;
    SELECT id INTO v_sol_id FROM trading.assets WHERE symbol = 'SOL' AND active;
    SELECT id INTO v_usdt_id FROM trading.assets WHERE symbol = 'USDT' AND active;
    SELECT id INTO v_brl_id FROM trading.assets WHERE symbol = 'BRL' AND active;

    -- Insert instruments
    INSERT INTO trading.instruments (
        exchange_id, name, base_asset_id, quote_asset_id, price_precision, amount_precision, min_amount, created_by
    )
    VALUES
        (v_binance_id, 'BTC/USDT', v_btc_id, v_usdt_id, 2, 2, 0.01, v_created_by),
        (v_mb_id,      'BTC/BRL',  v_btc_id, v_brl_id,  2, 2, 0.01, v_created_by),
        (v_dummy_id,   'BTC/USDT',  v_btc_id, v_usdt_id,  2, 2, 0.01, v_created_by),
        (v_binance_id, 'ETH/USDT', v_eth_id, v_usdt_id, 2, 2, 0.01, v_created_by),
        (v_mb_id,      'ETH/BRL',  v_eth_id, v_brl_id,  2, 2, 0.01, v_created_by),
        (v_dummy_id,   'ETH/USDT',  v_eth_id, v_usdt_id,  2, 2, 0.01, v_created_by),
        (v_dummy_id,   'LTC/USDT',  v_ltc_id, v_usdt_id,  2, 2, 0.01, v_created_by),
        (v_dummy_id,   'SOL/USDT',  v_sol_id, v_usdt_id,  2, 2, 0.01, v_created_by);
END $$;
