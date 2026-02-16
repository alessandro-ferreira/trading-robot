-- Insert instruments
INSERT INTO trading.instruments (
  exchange_id,
  name,
  base_asset_id,
  quote_asset_id,
  price_precision,
  amount_precision,
  min_amount
)
VALUES
(
    (SELECT id FROM trading.exchanges WHERE name = 'binance'),
    'BTC/USDT',
    (SELECT id FROM trading.assets WHERE symbol = 'BTC'),
    (SELECT id FROM trading.assets WHERE symbol = 'USDT'),
    2, 2, 0.01
),
(
    (SELECT id FROM trading.exchanges WHERE name = 'mercadobitcoin'),
    'BTC/BRL',
    (SELECT id FROM trading.assets WHERE symbol = 'BTC'),
    (SELECT id FROM trading.assets WHERE symbol = 'BRL'),
    2, 2, 0.01
),
(
    (SELECT id FROM trading.exchanges WHERE name = 'dummy'),
    'BTC/BRL',
    (SELECT id FROM trading.assets WHERE symbol = 'BTC'),
    (SELECT id FROM trading.assets WHERE symbol = 'BRL'),
    2, 2, 0.01
),
(
    (SELECT id FROM trading.exchanges WHERE name = 'binance'),
    'ETH/USDT',
    (SELECT id FROM trading.assets WHERE symbol = 'ETH'),
    (SELECT id FROM trading.assets WHERE symbol = 'USDT'),
    2, 2, 0.01
),
(
    (SELECT id FROM trading.exchanges WHERE name = 'mercadobitcoin'),
    'ETH/BRL',
    (SELECT id FROM trading.assets WHERE symbol = 'ETH'),
    (SELECT id FROM trading.assets WHERE symbol = 'BRL'),
    2, 2, 0.01
),
(
    (SELECT id FROM trading.exchanges WHERE name = 'dummy'),
    'ETH/BRL',
    (SELECT id FROM trading.assets WHERE symbol = 'ETH'),
    (SELECT id FROM trading.assets WHERE symbol = 'BRL'),
    2, 2, 0.01
);

---NOTE: The price_precision, amount_precision and min_amount should be based on your exchange configuration
---These values are examples
---If you are getting an error, check that those values exists before trying to insert the instruments
-- NOTE: The price_precision, amount_precision and min_amount should be based on your exchange configuration.
-- These values are examples. The exchange_id, base_asset_id, and quote_asset_id should match the existing records.
