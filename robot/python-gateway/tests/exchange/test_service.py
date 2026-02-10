import os
import unittest
from unittest.mock import MagicMock

from exchange.service import ExchangeService, ExchangeNotConfigured
from v1 import exchange_pb2
from exchange.exchanges.base import Ticker
from core import config

TEST_DATA_DIR = "tests/core/testdata"

class TestExchangeService(unittest.TestCase):
    def setUp(self):
        cfg = config.load(os.path.join(TEST_DATA_DIR, "success.toml"))

        # Mock exchange instance with expected methods
        self.mock_exchange = MagicMock()
        self.mock_exchange.fetch_ticker.return_value = Ticker(symbol='BTC/USDT', last=50000.0)
        self.mock_exchange.fetch_balance.return_value = {
            'free': {'USDT': 1000.0},
            'used': {'USDT': 0.0},
            'total': {'USDT': 1000.0}
        }
        self.mock_exchange.create_order.return_value = {
            'id': '12345', 'symbol': 'BTC/USDT', 'side': 'buy', 'type': 'limit', 'amount': 1.0, 'price': 50000.0, 'status': 'open', 'filled': 0.0, 'remaining': 1.0, 'cost': 0.0, 'average': 0.0
        }
        self.mock_exchange.cancel_order.return_value = {'id': '12345', 'status': 'canceled'}
        self.mock_exchange.fetch_order.return_value = {'id': '12345', 'symbol': 'BTC/USDT', 'status': 'closed', 'filled': 1.0, 'remaining': 0.0, 'price': 50000.0, 'amount': 1.0, 'cost': 50000.0, 'average': 50000.0}
        self.mock_exchange.fetch_open_orders.return_value = [
            {'id': '101', 'symbol': 'BTC/USDT', 'side': 'buy', 'type': 'limit', 'amount': 0.5, 'price': 20000.0, 'status': 'open', 'filled': 0.0, 'remaining': 0.5, 'cost': 0.0, 'average': 0.0}
        ]

        # Mock factory to return the mock exchange
        self.mock_factory = MagicMock()
        self.mock_factory.get.return_value = self.mock_exchange
        self.mock_factory.get_or_raise.return_value = self.mock_exchange

        self.service = ExchangeService(cfg, self.mock_factory)
        self.context = MagicMock()

    def test_ping(self):
        request = exchange_pb2.PingRequest()
        response = self.service.Ping(request, self.context)
        self.assertEqual(response.message, "Pong from Python gateway!")

    def test_get_ticker(self):
        request = exchange_pb2.GetTickerRequest(symbol="BTC/USDT", exchange='binance')
        response = self.service.GetTicker(request, self.context)
        self.assertEqual(response.symbol, "BTC/USDT")
        self.assertGreater(response.price, 0)

    def test_get_ticker_exchange_not_configured(self):
        self.mock_factory.get.side_effect = ExchangeNotConfigured("Exchange not configured: testex")
        self.context.abort.side_effect = Exception("Exchange not configured: testex")
        request = exchange_pb2.GetTickerRequest(symbol="BTC/USDT", exchange="testex")
        with self.assertRaises(Exception) as cm:
            self.service.GetTicker(request, self.context)
        self.assertIn("Exchange not configured", str(cm.exception))
        self.context.abort.side_effect = None
        self.mock_factory.get.side_effect = None

    def test_get_ticker_internal_error(self):
        self.mock_exchange.fetch_ticker.side_effect = Exception("Internal error")
        self.context.abort.side_effect = Exception("Internal error")
        request = exchange_pb2.GetTickerRequest(symbol="BTC/USDT", exchange='binance')
        with self.assertRaises(Exception) as cm:
            self.service.GetTicker(request, self.context)
        self.assertIn("Internal error", str(cm.exception))
        self.context.abort.side_effect = None
        self.mock_exchange.fetch_ticker.side_effect = None

    def test_get_balance(self):
        request = exchange_pb2.GetBalanceRequest(currency="USDT", exchange='binance')
        response = self.service.GetBalance(request, self.context)
        self.assertEqual(response.free["USDT"], 1000.0)
        self.assertEqual(response.total["USDT"], 1000.0)

    def test_get_balance_internal_error(self):
        self.mock_exchange.fetch_balance.side_effect = Exception("Internal error")
        self.context.abort.side_effect = Exception("Internal error")
        request = exchange_pb2.GetBalanceRequest(currency="USDT", exchange='binance')
        with self.assertRaises(Exception) as cm:
            self.service.GetBalance(request, self.context)
        self.assertIn("Internal error", str(cm.exception))
        self.context.abort.side_effect = None
        self.mock_exchange.fetch_balance.side_effect = None

    def test_create_order(self):
        request = exchange_pb2.CreateOrderRequest(symbol="BTC/USDT", side="buy", type="limit", amount=1.0, price=50000.0, exchange='binance')
        response = self.service.CreateOrder(request, self.context)
        self.assertEqual(response.id, "12345")
        self.assertEqual(response.symbol, "BTC/USDT")
        self.assertEqual(response.status, "open")
    
    def test_create_order_internal_error(self):
        self.mock_exchange.create_order.side_effect = Exception("Internal error")
        self.context.abort.side_effect = Exception("Internal error")
        request = exchange_pb2.CreateOrderRequest(symbol="BTC/USDT", side="buy", type="limit", amount=1.0, price=50000.0, exchange='binance')
        with self.assertRaises(Exception) as cm:
            self.service.CreateOrder(request, self.context)
        self.assertIn("Internal error", str(cm.exception))
        self.context.abort.side_effect = None
        self.mock_exchange.create_order.side_effect = None

    def test_cancel_order(self):
        request = exchange_pb2.CancelOrderRequest(id="12345", symbol="BTC/USDT", exchange='binance')
        response = self.service.CancelOrder(request, self.context)
        self.assertEqual(response.id, "12345")
        self.assertEqual(response.status, "canceled")

    def test_cancel_order_internal_error(self):
        self.mock_exchange.cancel_order.side_effect = Exception("Internal error")
        self.context.abort.side_effect = Exception("Internal error")
        request = exchange_pb2.CancelOrderRequest(id="12345", symbol="BTC/USDT", exchange='binance')
        with self.assertRaises(Exception) as cm:
            self.service.CancelOrder(request, self.context)
        self.assertIn("Internal error", str(cm.exception))
        self.context.abort.side_effect = None
        self.mock_exchange.cancel_order.side_effect = None

    def test_get_order(self):
        request = exchange_pb2.GetOrderRequest(id="12345", symbol="BTC/USDT", exchange='binance')
        response = self.service.GetOrder(request, self.context)
        self.assertEqual(response.id, "12345")
        self.assertEqual(response.symbol, "BTC/USDT")
        self.assertEqual(response.status, "closed")

    def test_get_order_internal_error(self):
        self.mock_exchange.fetch_order.side_effect = Exception("Internal error")
        self.context.abort.side_effect = Exception("Internal error")
        request = exchange_pb2.GetOrderRequest(id="12345", symbol="BTC/USDT", exchange='binance')
        with self.assertRaises(Exception) as cm:
            self.service.GetOrder(request, self.context)
        self.assertIn("Internal error", str(cm.exception))
        self.context.abort.side_effect = None
        self.mock_exchange.fetch_order.side_effect = None

    def test_get_open_orders(self):
        request = exchange_pb2.GetOpenOrdersRequest(symbol="BTC/USDT", exchange='binance')
        response = self.service.GetOpenOrders(request, self.context)
        self.assertEqual(len(response.orders), 1)
        self.assertEqual(response.orders[0].id, "101")
        self.assertEqual(response.orders[0].symbol, "BTC/USDT")

    def test_get_open_orders_internal_error(self):
        self.mock_exchange.fetch_open_orders.side_effect = Exception("Internal error")
        self.context.abort.side_effect = Exception("Internal error")
        request = exchange_pb2.GetOpenOrdersRequest(symbol="BTC/USDT", exchange='binance')
        with self.assertRaises(Exception) as cm:
            self.service.GetOpenOrders(request, self.context)
        self.assertIn("Internal error", str(cm.exception))
        self.context.abort.side_effect = None
        self.mock_exchange.fetch_open_orders.side_effect = None

# To run this test directly, use:
#   python -m tests.exchange.test_service
if __name__ == "__main__":
    unittest.main()

