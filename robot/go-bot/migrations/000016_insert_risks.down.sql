-- Delete Risk Pairs for Dummy exchange
DELETE FROM trading.risk_pairs WHERE exchange_id =
    (SELECT id FROM trading.exchanges WHERE name = 'dummy');
