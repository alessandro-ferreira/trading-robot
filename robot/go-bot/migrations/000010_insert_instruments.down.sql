-- Delete instruments
DELETE FROM trading.instruments
WHERE name IN ('BTC/USDT', 'BTC/BRL', 'ETH/USDT', 'ETH/BRL', 'LTC/USDT') AND exchange_id IN
    (SELECT id FROM trading.exchanges WHERE name IN ('binance', 'mercadobitcoin', 'dummy'));
