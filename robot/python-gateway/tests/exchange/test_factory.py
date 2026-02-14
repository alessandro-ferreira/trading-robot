import unittest

from exchange.factory import (
    ExchangeFactory,
    ExchangeNotConfigured,
    ExchangeConfigurationError,
)
from core import config
from core.config import ExchangeConfig


class TestExchangeFactory(unittest.TestCase):
    def setUp(self):
        cfg = config.Config()
        cfg.exchanges = [
            config.ExchangeConfig(name="binance"),
            config.ExchangeConfig(name="mercadobitcoin"),
            config.ExchangeConfig(name="coinbase"),
        ]
        self.factory = ExchangeFactory(cfg.exchanges)

    def test_list_and_get_default(self):
        names = self.factory.list_exchanges()
        self.assertIn("binance", names)
        self.assertIn("mercadobitcoin", names)
        self.assertIn("coinbase", names)
        self.assertIsNotNone(self.factory.get_default())

    def test_get_instantiates_providers(self):
        bin_ex = self.factory.get("binance")
        mb_ex = self.factory.get("mercadobitcoin")
        cb_ex = self.factory.get("coinbase")
        self.assertIsNotNone(bin_ex)
        self.assertIsNotNone(mb_ex)
        self.assertIsNotNone(cb_ex)

    def test_get_unknown_exchange_raises(self):
        # Should raise ExchangeNotConfigured for unknown exchange
        with self.assertRaises(ExchangeNotConfigured):
            self.factory.get("unknown-exchange")

    def test_get_sandbox_mode_not_supported_raises(self):
        # Should raise ExchangeConfigurationError if sandbox_mode is True for an exchange that doesn't support it
        # Use a known exchange that does not support sandbox (e.g., mercadobitcoin)
        cfg = config.Config()
        cfg.exchanges = [
            ExchangeConfig(
                name="mercadobitcoin", api_key="", secret="", sandbox_mode=True
            ),
        ]
        factory = ExchangeFactory(cfg.exchanges)
        with self.assertRaises(ExchangeConfigurationError):
            factory.get("mercadobitcoin")


# To run this test directly, use:
#   python -m tests.exchange.test_factory
if __name__ == "__main__":
    unittest.main()
