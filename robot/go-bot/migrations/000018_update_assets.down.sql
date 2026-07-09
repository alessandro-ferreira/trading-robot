-- Delete assets
DELETE FROM trading.assets
WHERE symbol IN (
    'BNB',
    'AVAX',
    'ALGO',
    'SUI',
    'XMR',
    'DOT',
    'FLOW',
    'APT',
    'ARB',
    'OP',
    'TRX',
    'HYPE',
    'HBAR',
    'ZEC',
    'SHIB',
    'TON',
    'TAO',
    'UNI',
    'AAVE',
    'NEAR'
);

-- Reset the name for symbols that were inserted in migration_000009
UPDATE trading.assets SET name = NULL, updated_by = 'migration_000018-rollback', updated_at = NOW() WHERE symbol IN
    ('BTC', 'ETH', 'LTC', 'XRP', 'BCH', 'ADA', 'DOGE', 'SOL', 'LINK', 'XLM', 'USDT', 'BRL', 'USD');
