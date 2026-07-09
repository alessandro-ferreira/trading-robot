-- Delete exchanges
DELETE FROM trading.exchanges
WHERE name IN (
    'kraken',
    'coinbase',
    'okx',
    'bybit',
    'kucoin'
);
