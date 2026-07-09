-- Delete exchanges
DELETE FROM trading.exchanges
WHERE name IN ('binance', 'mercadobitcoin', 'dummy');
