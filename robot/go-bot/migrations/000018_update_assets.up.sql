-- Set name for symbols that were inserted in migration_000009
UPDATE trading.assets SET name = 'Bitcoin', updated_by = 'migration_000018', updated_at = NOW() WHERE symbol = 'BTC';
UPDATE trading.assets SET name = 'Ethereum', updated_by = 'migration_000018', updated_at = NOW() WHERE symbol = 'ETH';
UPDATE trading.assets SET name = 'Litecoin', updated_by = 'migration_000018', updated_at = NOW() WHERE symbol = 'LTC';
UPDATE trading.assets SET name = 'Ripple', updated_by = 'migration_000018', updated_at = NOW() WHERE symbol = 'XRP';
UPDATE trading.assets SET name = 'Bitcoin Cash', updated_by = 'migration_000018', updated_at = NOW() WHERE symbol = 'BCH';
UPDATE trading.assets SET name = 'Cardano', updated_by = 'migration_000018', updated_at = NOW() WHERE symbol = 'ADA';
UPDATE trading.assets SET name = 'Dogecoin', updated_by = 'migration_000018', updated_at = NOW() WHERE symbol = 'DOGE';
UPDATE trading.assets SET name = 'Solana', updated_by = 'migration_000018', updated_at = NOW() WHERE symbol = 'SOL';
UPDATE trading.assets SET name = 'Chainlink', updated_by = 'migration_000018', updated_at = NOW() WHERE symbol = 'LINK';
UPDATE trading.assets SET name = 'Stellar', updated_by = 'migration_000018', updated_at = NOW() WHERE symbol = 'XLM';
UPDATE trading.assets SET name = 'Tether', updated_by = 'migration_000018', updated_at = NOW() WHERE symbol = 'USDT';
UPDATE trading.assets SET name = 'Brazilian Real', updated_by = 'migration_000018', updated_at = NOW() WHERE symbol = 'BRL';
UPDATE trading.assets SET name = 'US Dollar', updated_by = 'migration_000018', updated_at = NOW() WHERE symbol = 'USD';


-- Insert new assets
INSERT INTO trading.assets (symbol, name, created_by)
VALUES
    ('BNB', 'Binance Coin', 'migration_000018'),
    ('AVAX', 'Avalanche', 'migration_000018'),
    ('ALGO', 'Algorand', 'migration_000018'),
    ('SUI', 'Sui', 'migration_000018'),
    ('XMR', 'Monero', 'migration_000018'),
    ('DOT', 'Polkadot', 'migration_000018'),
    ('FLOW', 'Flow', 'migration_000018'),
    ('APT', 'Aptos', 'migration_000018'),
    ('ARB', 'Arbitrum', 'migration_000018'),
    ('OP', 'Optimism', 'migration_000018'),
    ('TRX', 'Tron', 'migration_000018'),
    ('HYPE', 'Hyperion', 'migration_000018'),
    ('HBAR', 'Hedera', 'migration_000018'),
    ('ZEC', 'Zcash', 'migration_000018'),
    ('SHIB', 'Shiba Inu', 'migration_000018'),
    ('TON', 'Toncoin', 'migration_000018'),
    ('TAO', 'Taos', 'migration_000018'),
    ('UNI', 'Uniswap', 'migration_000018'),
    ('AAVE', 'Aave', 'migration_000018'),
    ('NEAR', 'Near', 'migration_000018');
