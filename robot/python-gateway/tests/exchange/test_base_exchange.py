import unittest

from exchange.exchanges.base import (
    Exchange,
    ExchangeConfig,
    OrderType,
    ExchangeError,
    ExchangeNetworkError,
    AuthenticationError,
    InsufficientFundsError,
    BadRequestError,
)
from exchange.exchanges.ccxt import CCXTExchange


class TestExchangeBase(unittest.TestCase):
    def setUp(self):
        """Initialize shared configuration for base exchange tests."""
        self.base_cfg = ExchangeConfig(name="binance", ccxt=True)

    def test_interface_not_implemented(self):
        """Verify that all virtual methods in the base Exchange class enforce the interface contract."""
        ex = Exchange()

        with self.assertRaises(NotImplementedError):
            ex.set_sandbox_mode(True)
        with self.assertRaises(NotImplementedError):
            ex.fetch_ticker("BTC/USDT")
        with self.assertRaises(NotImplementedError):
            ex.fetch_balance()
        with self.assertRaises(NotImplementedError):
            ex.create_order("BTC/USDT", OrderType.LIMIT, "buy", 1.0, 50000.0)
        with self.assertRaises(NotImplementedError):
            ex.create_stop_order("BTC/USDT", "sell", 1.0, 40000.0)
        with self.assertRaises(NotImplementedError):
            ex.cancel_order("order_id", "BTC/USDT")
        with self.assertRaises(NotImplementedError):
            ex.fetch_order("order_id", "BTC/USDT")
        with self.assertRaises(NotImplementedError):
            ex.fetch_open_orders("BTC/USDT")
        with self.assertRaises(NotImplementedError):
            ex.fetch_my_trades("BTC/USDT")

    def test_exception_hierarchy(self):
        """Verify that custom exceptions correctly inherit from ExchangeError."""
        self.assertTrue(issubclass(ExchangeNetworkError, ExchangeError))
        self.assertTrue(issubclass(AuthenticationError, ExchangeError))
        self.assertTrue(issubclass(InsufficientFundsError, ExchangeError))
        self.assertTrue(issubclass(BadRequestError, ExchangeError))
        self.assertIsInstance(ExchangeError(), Exception)

    def test_base_init_ccxt_mapping(self):
        """
        Verify that the base constructor correctly handles CCXT flag and instantiation.

        This ensures that the shared logic in the base class correctly maps configuration
        to the internal _ccxt attribute.
        """
        # Test with CCXT enabled
        ex = CCXTExchange(self.base_cfg)
        self.assertIsNotNone(ex._ccxt)

        # Test with CCXT disabled (native)
        native_cfg = ExchangeConfig(name="mercadobitcoin", ccxt=False)
        ex_native = Exchange(native_cfg)
        self.assertIsNone(ex_native._ccxt)

    def test_base_init_invalid_ccxt_name(self):
        """Verify that base constructor handles invalid CCXT names without crashing (logs a warning)."""
        invalid_cfg = ExchangeConfig(name="invalid_exchange_name", ccxt=True)
        ex = Exchange(invalid_cfg)
        self.assertIsNone(ex._ccxt)


# To run this test directly, use:
#   python -m tests.exchange.test_base_exchange
if __name__ == "__main__":
    unittest.main()
