import unittest
from unittest.mock import MagicMock

from exchange.service import ExchangeService
from v1 import exchange_pb2


class TestExchangeService(unittest.TestCase):
    def setUp(self):
        self.service = ExchangeService()
        self.context = MagicMock()

    def test_ping(self):
        request = exchange_pb2.PingRequest()
        response = self.service.Ping(request, self.context)
        self.assertEqual(response.message, "Pong from Python gateway!")

    def test_get_ticker(self):
        request = exchange_pb2.GetTickerRequest(symbol="BTC/USDT")
        response = self.service.GetTicker(request, self.context)
        self.assertEqual(response.symbol, "BTC/USDT")
        self.assertGreater(response.price, 0)

    def test_get_balance(self):
        request = exchange_pb2.GetBalanceRequest(currency="USDT")
        response = self.service.GetBalance(request, self.context)
        self.assertEqual(response.total["USDT"], 1000.0)

    def test_create_order(self):
        request = exchange_pb2.CreateOrderRequest(
            symbol="BTC/USDT",
            side="buy",
            type="limit",
            amount=1.0,
            price=50000.0
        )
        response = self.service.CreateOrder(request, self.context)
        self.assertEqual(response.id, "12345")
        self.assertEqual(response.symbol, "BTC/USDT")
        self.assertEqual(response.status, "open")

    def test_cancel_order(self):
        request = exchange_pb2.CancelOrderRequest(id="12345", symbol="BTC/USDT")
        response = self.service.CancelOrder(request, self.context)
        self.assertEqual(response.id, "12345")
        self.assertEqual(response.status, "canceled")

    def test_get_order(self):
        request = exchange_pb2.GetOrderRequest(id="12345", symbol="BTC/USDT")
        response = self.service.GetOrder(request, self.context)
        self.assertEqual(response.id, "12345")
        self.assertEqual(response.status, "closed")

    def test_get_open_orders(self):
        request = exchange_pb2.GetOpenOrdersRequest(symbol="BTC/USDT")
        response = self.service.GetOpenOrders(request, self.context)
        self.assertTrue(len(response.orders) > 0)
        self.assertEqual(response.orders[0].symbol, "BTC/USDT")
        self.assertEqual(response.orders[0].status, "open")
