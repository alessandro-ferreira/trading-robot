-- Insert Strategy Pairs for Dummy Exchange
DO $$
DECLARE
    v_dummy_id BIGINT;
    v_btc_id BIGINT;
    v_eth_id BIGINT;
    v_ltc_id BIGINT;
    v_btc_pair_id BIGINT;
    v_eth_pair_id BIGINT;
    v_created_by TEXT := 'migration_000015';
BEGIN
    -- Fetch IDs for dummy exchange and instruments
    SELECT id INTO v_dummy_id FROM trading.exchanges WHERE name = 'dummy' AND active = TRUE;
    SELECT id INTO v_btc_id FROM trading.instruments WHERE name = 'BTC/USDT' AND exchange_id = v_dummy_id AND active = TRUE;
    SELECT id INTO v_eth_id FROM trading.instruments WHERE name = 'ETH/USDT' AND exchange_id = v_dummy_id AND active = TRUE;
    SELECT id INTO v_ltc_id FROM trading.instruments WHERE name = 'LTC/USDT' AND exchange_id = v_dummy_id AND active = TRUE;

    -- Insert Strategy Pairs
    INSERT INTO trading.strategy_pairs (exchange_id, instrument_id, strategy_type, created_by)
    VALUES (v_dummy_id, v_btc_id, 'momentum_trailing', v_created_by)
    RETURNING id INTO v_btc_pair_id;

    INSERT INTO trading.strategy_pairs (exchange_id, instrument_id, strategy_type, created_by)
    VALUES (v_dummy_id, v_eth_id, 'momentum_profit', v_created_by)
    RETURNING id INTO v_eth_pair_id;

    INSERT INTO trading.strategy_pairs (exchange_id, instrument_id, strategy_type, created_by)
    VALUES (v_dummy_id, v_ltc_id, 'dummy', v_created_by);

    -- Configure Momentum configurations
    -- BTC/BRL -> Momentum Trailing
    INSERT INTO trading.strategy_momentum (
        strategy_pair_id, strategy_type, window_seconds, momentum_windows,
        stop_loss_pct, activation_pct, trailing_stop_pct, created_by
    )
    VALUES (v_btc_pair_id, 'momentum_trailing', 10, ARRAY[(5, 0.0001)]::trading.momentum_window[], 0.1, 0.05, 0.02, v_created_by);

    -- ETH/BRL -> Momentum Profit
    INSERT INTO trading.strategy_momentum (
        strategy_pair_id, strategy_type, window_seconds, momentum_windows,
        stop_loss_pct, profit_target_pct, created_by
    )
    VALUES (v_eth_pair_id, 'momentum_profit', 10, ARRAY[(5, 0.0001)]::trading.momentum_window[], 0.1, 0.05, v_created_by);
END $$;
