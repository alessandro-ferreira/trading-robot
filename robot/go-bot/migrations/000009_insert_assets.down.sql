-- Delete assets
DELETE FROM trading.assets
WHERE symbol IN ('BTC', 'ETH', 'LTC', 'XRP', 'BCH', 'ADA', 'DOGE', 'SOL', 'LINK', 'XLM', 'USDT', 'BRL', 'USD');
