import unittest
from unittest.mock import MagicMock, patch
import ccxt
from core import config
from exchange.exchanges.ccxt import CCXTExchange
from exchange.exchanges.base import ExchangeError


class TestCCXTExchange(unittest.TestCase):
    def setUp(self):
        """Initialize a mocked CCXT library for exchange testing."""
        self.cfg = config.ExchangeConfig(
            name="binance", api_key="key", secret="secret", ccxt=True
        )
        # Mock the ccxt library before instantiating
        with patch("ccxt.binance") as mock_binance:
            self.mock_ccxt = MagicMock()
            mock_binance.return_value = self.mock_ccxt
            self.exchange = CCXTExchange(self.cfg)

    def test_fetch_ticker_standard(self):
        """Tests fetch_ticker using the 'last' field."""
        self.mock_ccxt.fetch_ticker.return_value = {
            "symbol": "BTC/USDT",
            "last": 50000.0,
            "bid": 49990.0,
            "ask": 50010.0,
            "timestamp": 1600000000000,
            "info": {"raw": "data"},
        }
        ticker = self.exchange.fetch_ticker("BTC/USDT")
        self.assertEqual(ticker.last, 50000.0)
        self.assertEqual(ticker.bid, 49990.0)
        self.assertEqual(ticker.ask, 50010.0)

    def test_fetch_ticker_fallback_close(self):
        """Tests fetch_ticker fallback to 'close' when 'last' is missing."""
        self.mock_ccxt.fetch_ticker.return_value = {
            "symbol": "BTC/USDT",
            "last": None,
            "close": 49500.0,
            "bid": None,
            "ask": None,
            "info": {},
        }
        ticker = self.exchange.fetch_ticker("BTC/USDT")
        self.assertEqual(ticker.last, 49500.0)

    def test_fetch_ticker_fallback_info(self):
        """Tests fetch_ticker fallback to nested 'info' fields."""
        self.mock_ccxt.fetch_ticker.return_value = {
            "symbol": "BTC/USDT",
            "last": None,
            "close": None,
            "info": {"price": "49000.0"},
        }
        ticker = self.exchange.fetch_ticker("BTC/USDT")
        self.assertEqual(ticker.last, 49000.0)

    def test_fetch_ticker_no_price_error(self):
        """Tests that fetch_ticker raises ExchangeError if no price is found."""
        self.mock_ccxt.fetch_ticker.return_value = {
            "symbol": "BTC/USDT",
            "last": None,
            "info": {},
        }
        with self.assertRaises(ExchangeError) as cm:
            self.exchange.fetch_ticker("BTC/USDT")
        self.assertIn("No price available", str(cm.exception))

    def test_fetch_balance(self):
        """Tests standard balance fetching."""
        self.mock_ccxt.fetch_balance.return_value = {"total": {"BTC": 1.0}}
        balance = self.exchange.fetch_balance()
        self.assertEqual(balance["total"]["BTC"], 1.0)
        self.mock_ccxt.fetch_balance.assert_called_once()

    def test_create_order_limit(self):
        """Tests creating a limit order with all parameters."""
        self.mock_ccxt.create_order.return_value = {"id": "123"}
        order = self.exchange.create_order("BTC/USDT", "limit", "buy", 0.1, 50000.0)
        self.mock_ccxt.create_order.assert_called_with(
            "BTC/USDT", "limit", "buy", 0.1, 50000.0
        )
        self.assertEqual(order["id"], "123")

    def test_fetch_my_trades_with_params(self):
        """Tests trades fetching with since and limit parameters."""
        self.mock_ccxt.fetch_my_trades.return_value = [{"id": "t1"}]
        trades = self.exchange.fetch_my_trades("BTC/USDT", since=1600000000, limit=10)
        self.mock_ccxt.fetch_my_trades.assert_called_with(
            "BTC/USDT", since=1600000000, limit=10
        )
        self.assertEqual(len(trades), 1)

    def test_fetch_my_trades_no_symbol_exception(self):
        """
        Tests that if fetch_my_trades fails without a symbol (common on Binance/Coinbase),
        it returns an empty list and logs a warning instead of crashing.
        """
        self.mock_ccxt.fetch_my_trades.side_effect = ccxt.ArgumentsRequired(
            "Symbol required"
        )

        # Use a real logger to verify warning if needed, but here we just check output
        trades = self.exchange.fetch_my_trades(symbol=None)

        self.assertEqual(trades, [])
        self.mock_ccxt.fetch_my_trades.assert_called_once()

    def test_fetch_my_trades_with_symbol_exception(self):
        """
        Tests that if fetch_my_trades fails WHEN a symbol IS provided,
        it propagates the exception as it indicates a real failure.
        """
        self.mock_ccxt.fetch_my_trades.side_effect = ccxt.NetworkError("Timeout")

        with self.assertRaises(ccxt.NetworkError):
            self.exchange.fetch_my_trades(symbol="BTC/USDT")

    def test_fetch_open_orders_filtering(self):
        """Tests that limit and symbol are passed correctly to open orders."""
        self.mock_ccxt.fetch_open_orders.return_value = []
        self.exchange.fetch_open_orders(symbol="ETH/USDT", limit=50)
        self.mock_ccxt.fetch_open_orders.assert_called_with("ETH/USDT", limit=50)

    def test_set_sandbox_mode(self):
        """Tests sandbox mode propagation."""
        self.exchange.set_sandbox_mode(True)
        self.mock_ccxt.set_sandbox_mode.assert_called_with(True)

    def test_set_sandbox_mode_not_supported(self):
        """Tests handling of set_sandbox_mode when underlying exchange raises error."""
        self.mock_ccxt.set_sandbox_mode.side_effect = Exception("Not supported")
        with self.assertRaises(ExchangeError):
            self.exchange.set_sandbox_mode(True)


# To run this test directly, use:
#   python -m tests.exchange.test_base_exchange
if __name__ == "__main__":
    unittest.main()
