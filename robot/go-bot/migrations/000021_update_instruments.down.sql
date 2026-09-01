-- Restore Binance instrument metadata from migration_000019.
DO $$
DECLARE
    v_binance_id BIGINT;
    v_created_by TEXT := 'migration_000021-rollback';
BEGIN
    SELECT id INTO v_binance_id
    FROM trading.exchanges
    WHERE name = 'binance' AND active;

    UPDATE trading.instruments AS instruments
    SET (price_precision, amount_precision, min_amount) =
        (values_data.price_precision, values_data.amount_precision, values_data.min_amount),
        updated_by = v_created_by,
        updated_at = NOW()
    FROM (VALUES
        ('BTC/USDT', 2, 5, 0.00001), ('ETH/USDT', 2, 2, 0.01),
        ('LTC/USDT', 2, 3, 0.001), ('XRP/USDT', 4, 2, 1),
        ('BCH/USDT', 2, 3, 0.001), ('ADA/USDT', 4, 2, 1),
        ('DOGE/USDT', 4, 2, 1), ('SOL/USDT', 2, 2, 0.01),
        ('LINK/USDT', 2, 2, 0.01), ('XLM/USDT', 4, 2, 1),
        ('AVAX/USDT', 2, 2, 0.01), ('BNB/USDT', 2, 3, 0.001),
        ('ALGO/USDT', 4, 2, 1), ('SUI/USDT', 4, 2, 0.01),
        ('XMR/USDT', 2, 3, 0.001), ('DOT/USDT', 4, 2, 0.01),
        ('FLOW/USDT', 4, 2, 1), ('APT/USDT', 2, 2, 0.01),
        ('ARB/USDT', 4, 2, 1), ('OP/USDT', 4, 2, 1),
        ('TRX/USDT', 4, 2, 1), ('HBAR/USDT', 4, 2, 1),
        ('ZEC/USDT', 2, 3, 0.001), ('SHIB/USDT', 8, 2, 1),
        ('TON/USDT', 2, 2, 0.01), ('TAO/USDT', 2, 3, 0.001),
        ('UNI/USDT', 2, 2, 0.01), ('AAVE/USDT', 2, 3, 0.001),
        ('NEAR/USDT', 4, 2, 0.01)
    ) AS values_data(name, price_precision, amount_precision, min_amount)
    WHERE instruments.exchange_id = v_binance_id
      AND instruments.name = values_data.name;
END $$;
