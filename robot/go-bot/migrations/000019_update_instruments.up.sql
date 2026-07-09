-- Insert instruments
DO $$
DECLARE
    v_binance_id BIGINT;

    v_btc_id BIGINT;
    v_eth_id BIGINT;
    v_ltc_id BIGINT;
    v_xrp_id BIGINT;
    v_bch_id BIGINT;
    v_ada_id BIGINT;
    v_doge_id BIGINT;
    v_sol_id BIGINT;
    v_link_id BIGINT;
    v_xlm_id BIGINT;
    v_avax_id BIGINT;
    v_bnb_id BIGINT;
    v_algo_id BIGINT;
    v_sui_id BIGINT;
    v_xmr_id BIGINT;
    v_dot_id BIGINT;
    v_flow_id BIGINT;
    v_apt_id BIGINT;
    v_arb_id BIGINT;
    v_op_id BIGINT;
    v_trx_id BIGINT;
    v_hype_id BIGINT;
    v_hbar_id BIGINT;
    v_zec_id BIGINT;
    v_shib_id BIGINT;
    v_ton_id BIGINT;
    v_tao_id BIGINT;
    v_uni_id BIGINT;
    v_aave_id BIGINT;
    v_near_id BIGINT;
    v_usdt_id BIGINT;

    v_created_by TEXT := 'migration_000019';
BEGIN
    -- Fetch Exchange IDs
    SELECT id INTO v_binance_id FROM trading.exchanges WHERE name = 'binance' AND active;

    -- Fetch Asset IDs
    SELECT id INTO v_btc_id FROM trading.assets WHERE symbol = 'BTC' AND active;
    SELECT id INTO v_eth_id FROM trading.assets WHERE symbol = 'ETH' AND active;
    SELECT id INTO v_ltc_id FROM trading.assets WHERE symbol = 'LTC' AND active;
    SELECT id INTO v_xrp_id FROM trading.assets WHERE symbol = 'XRP' AND active;
    SELECT id INTO v_bch_id FROM trading.assets WHERE symbol = 'BCH' AND active;
    SELECT id INTO v_ada_id FROM trading.assets WHERE symbol = 'ADA' AND active;
    SELECT id INTO v_doge_id FROM trading.assets WHERE symbol = 'DOGE' AND active;
    SELECT id INTO v_sol_id FROM trading.assets WHERE symbol = 'SOL' AND active;
    SELECT id INTO v_link_id FROM trading.assets WHERE symbol = 'LINK' AND active;
    SELECT id INTO v_xlm_id FROM trading.assets WHERE symbol = 'XLM' AND active;
    SELECT id INTO v_avax_id FROM trading.assets WHERE symbol = 'AVAX' AND active;
    SELECT id INTO v_bnb_id FROM trading.assets WHERE symbol = 'BNB' AND active;
    SELECT id INTO v_algo_id FROM trading.assets WHERE symbol = 'ALGO' AND active;
    SELECT id INTO v_sui_id FROM trading.assets WHERE symbol = 'SUI' AND active;
    SELECT id INTO v_xmr_id FROM trading.assets WHERE symbol = 'XMR' AND active;
    SELECT id INTO v_dot_id FROM trading.assets WHERE symbol = 'DOT' AND active;
    SELECT id INTO v_flow_id FROM trading.assets WHERE symbol = 'FLOW' AND active;
    SELECT id INTO v_apt_id FROM trading.assets WHERE symbol = 'APT' AND active;
    SELECT id INTO v_arb_id FROM trading.assets WHERE symbol = 'ARB' AND active;
    SELECT id INTO v_op_id FROM trading.assets WHERE symbol = 'OP' AND active;
    SELECT id INTO v_trx_id FROM trading.assets WHERE symbol = 'TRX' AND active;
    SELECT id INTO v_hype_id FROM trading.assets WHERE symbol = 'HYPE' AND active;
    SELECT id INTO v_hbar_id FROM trading.assets WHERE symbol = 'HBAR' AND active;
    SELECT id INTO v_zec_id FROM trading.assets WHERE symbol = 'ZEC' AND active;
    SELECT id INTO v_shib_id FROM trading.assets WHERE symbol = 'SHIB' AND active;
    SELECT id INTO v_ton_id FROM trading.assets WHERE symbol = 'TON' AND active;
    SELECT id INTO v_tao_id FROM trading.assets WHERE symbol = 'TAO' AND active;
    SELECT id INTO v_uni_id FROM trading.assets WHERE symbol = 'UNI' AND active;
    SELECT id INTO v_aave_id FROM trading.assets WHERE symbol = 'AAVE' AND active;
    SELECT id INTO v_near_id FROM trading.assets WHERE symbol = 'NEAR' AND active;
    SELECT id INTO v_usdt_id FROM trading.assets WHERE symbol = 'USDT' AND active;

    -- Update existing instruments inserted in migration_000010
    UPDATE trading.instruments SET price_precision = 2, amount_precision = 5, min_amount = 0.00001,
        updated_by = v_created_by, updated_at = NOW() WHERE base_asset_id = v_btc_id;
    UPDATE trading.instruments SET price_precision = 2, amount_precision = 3, min_amount = 0.001,
        updated_by = v_created_by, updated_at = NOW() WHERE base_asset_id = v_eth_id;
    UPDATE trading.instruments SET price_precision = 2, amount_precision = 3, min_amount = 0.001,
        updated_by = v_created_by, updated_at = NOW() WHERE base_asset_id = v_ltc_id;


    -- Insert new instruments
    INSERT INTO trading.instruments (
        exchange_id, name, base_asset_id, quote_asset_id, price_precision, amount_precision, min_amount, created_by
    )
    VALUES
        (v_binance_id, 'LTC/USDT', v_ltc_id, v_usdt_id, 2, 3, 0.001, v_created_by),
        (v_binance_id, 'XRP/USDT', v_xrp_id, v_usdt_id, 4, 2, 1, v_created_by),
        (v_binance_id, 'BCH/USDT', v_bch_id, v_usdt_id, 2, 3, 0.001, v_created_by),
        (v_binance_id, 'ADA/USDT', v_ada_id, v_usdt_id, 4, 2, 1, v_created_by),
        (v_binance_id, 'DOGE/USDT', v_doge_id, v_usdt_id, 4, 2, 1, v_created_by),
        (v_binance_id, 'SOL/USDT', v_sol_id, v_usdt_id, 2, 2, 0.01, v_created_by),
        (v_binance_id, 'LINK/USDT', v_link_id, v_usdt_id, 2, 2, 0.01, v_created_by),
        (v_binance_id, 'XLM/USDT', v_xlm_id, v_usdt_id, 4, 2, 1, v_created_by),
        (v_binance_id, 'AVAX/USDT', v_avax_id, v_usdt_id, 2, 2, 0.01, v_created_by),
        (v_binance_id, 'BNB/USDT', v_bnb_id, v_usdt_id, 2, 3, 0.001, v_created_by),
        (v_binance_id, 'ALGO/USDT', v_algo_id, v_usdt_id, 4, 2, 1, v_created_by),
        (v_binance_id, 'SUI/USDT', v_sui_id, v_usdt_id, 4, 2, 0.01, v_created_by),
        (v_binance_id, 'XMR/USDT', v_xmr_id, v_usdt_id, 2, 3, 0.001, v_created_by),
        (v_binance_id, 'DOT/USDT', v_dot_id, v_usdt_id, 4, 2, 0.01, v_created_by),
        (v_binance_id, 'FLOW/USDT', v_flow_id, v_usdt_id, 4, 2, 1, v_created_by),
        (v_binance_id, 'APT/USDT', v_apt_id, v_usdt_id, 2, 2, 0.01, v_created_by),
        (v_binance_id, 'ARB/USDT', v_arb_id, v_usdt_id, 4, 2, 1, v_created_by),
        (v_binance_id, 'OP/USDT', v_op_id, v_usdt_id, 4, 2, 1, v_created_by),
        (v_binance_id, 'TRX/USDT', v_trx_id, v_usdt_id, 4, 2, 1, v_created_by),
        (v_binance_id, 'HYPE/USDT', v_hype_id, v_usdt_id, 2, 2, 0.01, v_created_by),
        (v_binance_id, 'HBAR/USDT', v_hbar_id, v_usdt_id, 4, 2, 1, v_created_by),
        (v_binance_id, 'ZEC/USDT', v_zec_id, v_usdt_id, 2, 3, 0.001, v_created_by),
        (v_binance_id, 'SHIB/USDT', v_shib_id, v_usdt_id, 8, 2, 1, v_created_by),
        (v_binance_id, 'TON/USDT', v_ton_id, v_usdt_id, 2, 2, 0.01, v_created_by),
        (v_binance_id, 'TAO/USDT', v_tao_id, v_usdt_id, 2, 3, 0.001, v_created_by),
        (v_binance_id, 'UNI/USDT', v_uni_id, v_usdt_id, 2, 2, 0.01, v_created_by),
        (v_binance_id, 'AAVE/USDT', v_aave_id, v_usdt_id, 2, 3, 0.001, v_created_by),
        (v_binance_id, 'NEAR/USDT', v_near_id, v_usdt_id, 4, 2, 0.01, v_created_by);

END $$;
