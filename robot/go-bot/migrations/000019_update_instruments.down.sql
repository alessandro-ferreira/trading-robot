-- Delete instruments
DELETE FROM trading.instruments
WHERE name IN (
    'LTC/USDT',
    'XRP/USDT',
    'BCH/USDT',
    'ADA/USDT',
    'DOGE/USDT',
    'SOL/USDT',
    'LINK/USDT',
    'XLM/USDT',
    'AVAX/USDT',
    'BNB/USDT',
    'ALGO/USDT',
    'SUI/USDT',
    'XMR/USDT',
    'DOT/USDT',
    'FLOW/USDT',
    'APT/USDT',
    'ARB/USDT',
    'OP/USDT',
    'TRX/USDT',
    'HYPE/USDT',
    'HBAR/USDT',
    'ZEC/USDT',
    'SHIB/USDT',
    'TON/USDT',
    'TAO/USDT',
    'UNI/USDT',
    'AAVE/USDT',
    'NEAR/USDT'
) AND exchange_id IN (SELECT id FROM trading.exchanges WHERE name = 'binance');

-- Revert updates in BTC/USDT, BTC/BRL, ETH/USDT, ETH/BRL, LTC/USDT
UPDATE trading.instruments SET price_precision = 2, amount_precision = 2, min_amount = 0.01,
    updated_by = 'migration_000019-rollback', updated_at = NOW()
WHERE name IN ('BTC/USDT', 'BTC/BRL', 'ETH/USDT', 'ETH/BRL', 'LTC/USDT') AND exchange_id IN
    (SELECT id FROM trading.exchanges WHERE name IN ('binance', 'mercadobitcoin', 'dummy'));
