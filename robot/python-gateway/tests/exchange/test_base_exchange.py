import os
import unittest
from core import config
from exchange.exchanges.base import Exchange, Ticker, ExchangeError

TEST_DATA_DIR = "tests/exchange/testdata"


class TestBaseExchange(unittest.TestCase):
    def setUp(self):
        cfg = config.load(os.path.join(TEST_DATA_DIR, "config.toml"))
        self.ex_cfg = cfg.exchanges[0]  # Use the first exchange config for testing
        self.exchange = Exchange(self.ex_cfg)

    def test_ticker_dataclass(self):
        ticker = Ticker(symbol="BTC/USDT", last=50000.0)
        self.assertEqual(ticker.symbol, "BTC/USDT")
        self.assertEqual(ticker.last, 50000.0)
        self.assertIsNone(ticker.bid)
        self.assertIsNone(ticker.ask)
        self.assertIsNone(ticker.timestamp)
        self.assertIsNone(ticker.info)

    def test_error_on_invalid_exchange_name(self):
        invalidCfg = self.ex_cfg
        invalidCfg.name = "exchange_not_in_ccxt"
        exchange = Exchange(invalidCfg)
        with self.assertRaises(ExchangeError):
            exchange.fetch_ticker("BTC/USDT")
        with self.assertRaises(ExchangeError):
            exchange.fetch_balance()
        with self.assertRaises(ExchangeError):
            exchange.create_order("BTC/USDT", "limit", "buy", 1.0, 50000.0)
        with self.assertRaises(ExchangeError):
            exchange.cancel_order("1", symbol="BTC/USDT")
        with self.assertRaises(ExchangeError):
            exchange.fetch_order("1", symbol="BTC/USDT")
        with self.assertRaises(ExchangeError):
            exchange.fetch_open_orders("BTC/USDT")


# To run this test directly, use:
#   python -m tests.exchange.test_base_exchange
if __name__ == "__main__":
    unittest.main()
