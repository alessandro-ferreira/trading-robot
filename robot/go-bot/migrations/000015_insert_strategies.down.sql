-- Delete Momentum strategies and Strategy Pairs for Dummy exchange
DELETE FROM trading.strategy_momentum WHERE strategy_pair_id IN
    (SELECT id FROM trading.strategy_pairs WHERE exchange_id =
        (SELECT id FROM trading.exchanges WHERE name = 'dummy'));

DELETE FROM trading.strategy_pairs WHERE exchange_id =
    (SELECT id FROM trading.exchanges WHERE name = 'dummy');
