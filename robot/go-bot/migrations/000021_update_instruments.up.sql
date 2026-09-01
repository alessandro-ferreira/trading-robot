-- Update Binance instrument metadata from the Binance (api.binance.com/api/v3/exchangeInfo) PRICE_FILTER and LOT_SIZE filters.
DO $$
DECLARE
    v_binance_id BIGINT;
    v_created_by TEXT := 'migration_000021';
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
        ('BTC/USDT', 2, 5, 0.00001), ('ETH/USDT', 2, 4, 0.0001),
        ('LTC/USDT', 2, 3, 0.001), ('XRP/USDT', 4, 1, 0.1),
        ('BCH/USDT', 1, 3, 0.001), ('ADA/USDT', 4, 1, 0.1),
        ('DOGE/USDT', 5, 0, 1), ('SOL/USDT', 2, 3, 0.001),
        ('LINK/USDT', 3, 2, 0.01), ('XLM/USDT', 4, 0, 1),
        ('AVAX/USDT', 3, 2, 0.01), ('BNB/USDT', 2, 3, 0.001),
        ('ALGO/USDT', 4, 0, 1), ('SUI/USDT', 4, 1, 0.1),
        ('XMR/USDT', 1, 3, 0.001), ('DOT/USDT', 3, 2, 0.01),
        ('FLOW/USDT', 5, 2, 0.01), ('APT/USDT', 3, 2, 0.01),
        ('ARB/USDT', 4, 1, 0.1), ('OP/USDT', 4, 2, 0.01),
        ('TRX/USDT', 4, 1, 0.1), ('HBAR/USDT', 5, 0, 1),
        ('ZEC/USDT', 2, 3, 0.001), ('SHIB/USDT', 8, 0, 1),
        ('TON/USDT', 3, 2, 0.01), ('TAO/USDT', 1, 4, 0.0001),
        ('UNI/USDT', 3, 2, 0.01), ('AAVE/USDT', 2, 3, 0.001),
        ('NEAR/USDT', 3, 1, 0.1)
    ) AS values_data(name, price_precision, amount_precision, min_amount)
    WHERE exchange_id = v_binance_id
      AND instruments.name = values_data.name;
END $$;
