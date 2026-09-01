import unittest

from unittest.mock import MagicMock, patch

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
        # Create a non supported ccxt instance to raise not available ExchangeError
        self.cfg = config.ExchangeConfig(
            name="mercadobitcoin", api_key="", secret="", ccxt=True, sandbox_mode=False
        )
        self.non_ccxt_exchange = CCXTExchange(self.cfg)

    def test_set_sandbox_mode(self):
        """Tests sandbox mode propagation."""
        self.exchange.set_sandbox_mode(True)
        self.mock_ccxt.set_sandbox_mode.assert_called_with(True)

    def test_set_sandbox_mode_not_supported(self):
        """Tests handling of set_sandbox_mode when exchange raises error."""
        self.mock_ccxt.set_sandbox_mode.side_effect = Exception("Not supported")
        with self.assertRaises(ExchangeError):
            self.exchange.set_sandbox_mode(True)

    def test_set_sandbox_mode_not_available(self):
        """Tests sandbox mode raises Underlying ccxt exchange not available."""
        with self.assertRaises(ExchangeError):
            self.non_ccxt_exchange.set_sandbox_mode(True)

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

    def test_fetch_ticker_not_available(self):
        """Tests fetch_ticker raises Underlying ccxt exchange not available."""
        with self.assertRaises(ExchangeError):
            self.non_ccxt_exchange.fetch_ticker("BTC/USDT")

    def test_fetch_balance(self):
        """Tests standard balance fetching."""
        self.mock_ccxt.fetch_balance.return_value = {"total": {"BTC": 1.0}}
        balance = self.exchange.fetch_balance()
        self.assertEqual(balance["total"]["BTC"], 1.0)
        self.mock_ccxt.fetch_balance.assert_called_once()

    def test_fetch_balance_not_available(self):
        """Tests fetch_balance raises Underlying ccxt exchange not available."""
        with self.assertRaises(ExchangeError):
            self.non_ccxt_exchange.fetch_balance()

    def test_create_order_limit(self):
        """Tests creating a limit order with all parameters."""
        self.mock_ccxt.create_order.return_value = {"id": "123"}
        order = self.exchange.create_order(
            "BTC/USDT", "limit", "buy", 0.1, "client-123", 50000.0
        )
        self.mock_ccxt.create_order.assert_called_with(
            "BTC/USDT", "limit", "buy", 0.1, 50000.0, {"clientOrderId": "client-123"}
        )
        self.assertEqual(order["id"], "123")

    def test_create_order_not_available(self):
        """Tests create_order raises Underlying ccxt exchange not available."""
        with self.assertRaises(ExchangeError):
            self.non_ccxt_exchange.create_order(
                "BTC/USDT", "limit", "buy", 0.1, "client-123", 50000.0
            )

    def test_create_stop_order_market(self):
        """Tests creating a stop market order via CCXT."""
        self.mock_ccxt.create_order.return_value = {"id": "stop-123"}
        order = self.exchange.create_stop_order(
            "BTC/USDT", "sell", 0.1, "client-123", 40000.0
        )

        self.mock_ccxt.create_order.assert_called_with(
            "BTC/USDT",
            "market",
            "sell",
            0.1,
            None,
            {
                "triggerPrice": 40000.0,
                "stopPrice": 40000.0,
                "clientOrderId": "client-123",
            },
        )
        self.assertEqual(order["id"], "stop-123")

    def test_create_stop_order_limit(self):
        """Tests creating a stop limit order via CCXT."""
        self.mock_ccxt.create_order.return_value = {"id": "stop-limit-123"}
        order = self.exchange.create_stop_order(
            "BTC/USDT", "sell", 0.1, "client-123", 40000.0, 39500.0
        )

        self.mock_ccxt.create_order.assert_called_with(
            "BTC/USDT",
            "limit",
            "sell",
            0.1,
            39500.0,
            {
                "triggerPrice": 40000.0,
                "stopPrice": 40000.0,
                "clientOrderId": "client-123",
            },
        )
        self.assertEqual(order["id"], "stop-limit-123")

    def test_create_stop_order_not_available(self):
        """Tests create_stop_order raises Underlying ccxt exchange not available."""
        with self.assertRaises(ExchangeError):
            self.non_ccxt_exchange.create_stop_order(
                "BTC/USDT", "sell", 0.1, "client-123", 40000.0
            )

    def test_cancel_order(self):
        """Tests canceling an order via CCXT."""
        self.mock_ccxt.cancel_order.return_value = {
            "id": "ord-123",
            "status": "canceled",
        }
        result = self.exchange.cancel_order("ord-123", "BTC/USDT")
        self.mock_ccxt.cancel_order.assert_called_with("ord-123", "BTC/USDT")
        self.assertEqual(result["id"], "ord-123")
        self.assertEqual(result["status"], "canceled")

    def test_cancel_order_not_available(self):
        """Tests cancel_order raises Underlying ccxt exchange not available."""
        with self.assertRaises(ExchangeError):
            self.non_ccxt_exchange.cancel_order("ord-123", "BTC/USDT")

    def test_fetch_order(self):
        """Tests fetching an order via CCXT."""
        self.mock_ccxt.fetch_order.return_value = {
            "id": "ord-123",
            "symbol": "BTC/USDT",
            "status": "closed",
        }
        result = self.exchange.fetch_order("ord-123", "BTC/USDT")
        self.mock_ccxt.fetch_order.assert_called_with("ord-123", "BTC/USDT")
        self.assertEqual(result["id"], "ord-123")
        self.assertEqual(result["status"], "closed")

    def test_fetch_order_not_available(self):
        """Tests fetch_order raises Underlying ccxt exchange not available."""
        with self.assertRaises(ExchangeError):
            self.non_ccxt_exchange.fetch_order("ord-123", "BTC/USDT")

    def test_fetch_orders_filtering(self):
        """Tests that the required symbol is passed to orders."""
        self.mock_ccxt.fetch_orders.return_value = []
        self.exchange.fetch_orders(symbol="ETH/USDT")
        self.mock_ccxt.fetch_orders.assert_called_with("ETH/USDT")

    def test_fetch_orders_not_available(self):
        """Tests fetch_orders raises Underlying ccxt exchange not available."""
        with self.assertRaises(ExchangeError):
            self.non_ccxt_exchange.fetch_orders(symbol="ETH/USDT")

    def test_fetch_open_orders_filtering(self):
        """Tests that the required symbol is passed to open orders."""
        self.mock_ccxt.fetch_open_orders.return_value = []
        self.exchange.fetch_open_orders(symbol="ETH/USDT")
        self.mock_ccxt.fetch_open_orders.assert_called_with("ETH/USDT")

    def test_fetch_open_orders_not_available(self):
        """Tests fetch_open_orders raises Underlying ccxt exchange not available."""
        with self.assertRaises(ExchangeError):
            self.non_ccxt_exchange.fetch_open_orders(symbol="ETH/USDT")


# To run this test directly, use:
#   python -m tests.exchange.test_ccxt_exchange
if __name__ == "__main__":
    unittest.main()
