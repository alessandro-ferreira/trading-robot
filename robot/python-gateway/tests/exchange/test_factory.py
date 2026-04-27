import unittest
from unittest.mock import patch, MagicMock

from exchange.factory import (
    ExchangeFactory,
    ExchangeNotConfigured,
    ExchangeConfigurationError,
)
from exchange.exchanges.ccxt import CCXTExchange
from core import config
from core.config import ExchangeConfig


class TestExchangeFactory(unittest.TestCase):
    def setUp(self):
        cfg = config.Config()
        cfg.exchanges = [
            config.ExchangeConfig(name="binance", ccxt=True),
            config.ExchangeConfig(name="mercadobitcoin"),
            config.ExchangeConfig(name="coinbase", ccxt=True),
        ]
        self.factory = ExchangeFactory(cfg.exchanges)

    def test_list_exchanges(self):
        """Verify listing of configured exchanges."""
        names = self.factory.list_exchanges()
        self.assertIn("binance", names)
        self.assertIn("mercadobitcoin", names)
        self.assertIn("coinbase", names)

    def test_get_instantiates_providers(self):
        """Verify that factory correctly instantiates configured providers."""
        bin_ex = self.factory.get("binance")
        mb_ex = self.factory.get("mercadobitcoin")
        cb_ex = self.factory.get("coinbase")
        self.assertIsNotNone(bin_ex)
        self.assertIsNotNone(mb_ex)
        self.assertIsNotNone(cb_ex)

    def test_get_ccxt_implementation(self):
        """Verify that any exchange with ccxt=True uses the generic CCXTExchange."""
        # binance was configured with ccxt=True in setUp
        bin_ex = self.factory.get("binance")
        self.assertIsInstance(bin_ex, CCXTExchange)

    def test_get_unknown_exchange_raises(self):
        """Verify that requesting an unknown exchange name raises ExchangeNotConfigured."""
        with self.assertRaises(ExchangeNotConfigured):
            self.factory.get("unknown-exchange")

    @patch("exchange.factory.REGISTRY", {})
    def test_get_provider_not_found_raises(self):
        """Verify that a configured exchange with no implementation in REGISTRY raises ExchangeNotConfigured."""
        # The factory has configs, but the registry is mocked as empty
        with self.assertRaises(ExchangeNotConfigured) as cm:
            self.factory.get("binance")
        self.assertIn("No provider found", str(cm.exception))

    def test_get_initialization_failure_raises(self):
        """Verify that an exception during provider instantiation is wrapped in ExchangeConfigurationError."""
        cfg = config.Config()
        cfg.exchanges = [ExchangeConfig(name="crash-exchange", ccxt=False)]

        # Mock a provider that crashes on __init__
        mock_provider = MagicMock(side_effect=RuntimeError("Constructor failed"))

        with patch("exchange.factory.REGISTRY", {"crash-exchange": mock_provider}):
            factory = ExchangeFactory(cfg.exchanges)
            with self.assertRaises(ExchangeConfigurationError) as cm:
                factory.get("crash-exchange")
            self.assertIn("Failed to initialize exchange", str(cm.exception))

    def test_get_sandbox_mode_not_supported_raises(self):
        """Verify that enabling sandbox on unsupported native exchanges raises ExchangeConfigurationError."""
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
