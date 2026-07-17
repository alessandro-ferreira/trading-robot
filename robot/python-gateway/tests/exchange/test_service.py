import ccxt
import grpc
import os
import unittest

from unittest.mock import MagicMock

from core import config
from exchange.service import ExchangeService
from exchange.factory import (
    ExchangeNotConfigured,
    ExchangeConfigurationError,
)
from exchange.exchanges.base import Ticker
from exchange.exchanges.base import (
    ExchangeNetworkError,
    InsufficientFundsError,
    BadRequestError,
)
from v1 import exchange_pb2

TEST_DATA_DIR = "tests/core/testdata"


class TestExchangeService(unittest.TestCase):
    def setUp(self):
        cfg = config.load(os.path.join(TEST_DATA_DIR, "success.toml"))

        # Mock exchange instance with expected methods
        self.mock_exchange = MagicMock()
        self.mock_exchange.fetch_ticker.return_value = Ticker(
            symbol="BTC/USDT", last=50000.0
        )
        self.mock_exchange.fetch_balance.return_value = {
            "free": {"USDT": 1000.0},
            "used": {"USDT": 0.0},
            "total": {"USDT": 1000.0},
        }
        self.mock_exchange.create_order.return_value = {
            "id": "12345",
            "symbol": "BTC/USDT",
            "side": "buy",
            "type": "limit",
            "amount": 1.0,
            "price": 50000.0,
            "status": "open",
            "filled": 0.0,
            "remaining": 1.0,
            "cost": 0.0,
            "average": 0.0,
            "fee": 0.01,
            "fee_currency": "USDT",
        }
        self.mock_exchange.cancel_order.return_value = {
            "id": "12345",
            "status": "canceled",
        }
        self.mock_exchange.fetch_order.return_value = {
            "id": "12345",
            "symbol": "BTC/USDT",
            "status": "closed",
            "filled": 1.0,
            "remaining": 0.0,
            "price": 50000.0,
            "amount": 1.0,
            "cost": 50000.0,
            "average": 50000.0,
            "fee": 0.05,
            "fee_currency": "BRL",
        }
        self.mock_exchange.fetch_open_orders.return_value = [
            {
                "id": "101",
                "symbol": "BTC/USDT",
                "side": "buy",
                "type": "limit",
                "amount": 0.5,
                "price": 20000.0,
                "status": "open",
                "filled": 0.0,
                "remaining": 0.5,
                "cost": 0.0,
                "average": 0.0,
            }
        ]
        self.mock_exchange.fetch_my_trades.return_value = [
            {
                "id": "t1",
                "order": "101",
                "symbol": "BTC/USDT",
                "side": "buy",
                "price": 20000.0,
                "amount": 0.5,
                "cost": 10000.0,
                "timestamp": 1672531200000,
            }
        ]

        # Mock factory to return the mock exchange
        self.mock_factory = MagicMock()
        self.mock_factory.get.return_value = self.mock_exchange

        self.service = ExchangeService(cfg, self.mock_factory)
        self.context = MagicMock()

    def test_ping(self):
        """Verify ping-pong health check."""
        request = exchange_pb2.PingRequest()
        response = self.service.Ping(request, self.context)
        self.assertEqual(response.message, "Pong from Python gateway!")

    def test_get_ticker(self):
        """Verify basic ticker price retrieval."""
        request = exchange_pb2.GetTickerRequest(exchange="binance", symbol="BTC/USDT")
        response = self.service.GetTicker(request, self.context)
        self.assertEqual(response.symbol, "BTC/USDT")
        self.assertGreater(response.price, 0)

    def test_get_ticker_exchange_not_configured(self):
        """Verify error handling when an unknown exchange is requested."""
        self.mock_factory.get.side_effect = ExchangeNotConfigured(
            "Exchange not configured: testex"
        )
        self.context.abort.side_effect = Exception("Exchange not configured: testex")
        request = exchange_pb2.GetTickerRequest(exchange="testex", symbol="BTC/USDT")
        with self.assertRaises(Exception) as cm:
            self.service.GetTicker(request, self.context)
        self.assertIn("Exchange not configured", str(cm.exception))
        self.context.abort.side_effect = None
        self.mock_factory.get.side_effect = None

    def test_get_ticker_configuration_error(self):
        """Verify mapping of ExchangeConfigurationError in _getExchange."""
        self.mock_factory.get.side_effect = ExchangeConfigurationError(
            "Invalid credentials"
        )
        self.context.abort.side_effect = Exception("Aborted")
        request = exchange_pb2.GetTickerRequest(exchange="binance", symbol="BTC/USDT")
        with self.assertRaises(Exception):
            self.service.GetTicker(request, self.context)
        self.context.abort.assert_called_with(
            grpc.StatusCode.FAILED_PRECONDITION, "Invalid credentials"
        )

    def test_get_ticker_internal_error(self):
        """Verify internal exception mapping to gRPC INTERNAL status."""
        self.mock_exchange.fetch_ticker.side_effect = Exception("Internal error")
        self.context.abort.side_effect = Exception("Internal error")
        request = exchange_pb2.GetTickerRequest(exchange="binance", symbol="BTC/USDT")
        with self.assertRaises(Exception) as cm:
            self.service.GetTicker(request, self.context)
        self.assertIn("Internal error", str(cm.exception))
        self.context.abort.side_effect = None
        self.mock_exchange.fetch_ticker.side_effect = None

    def test_get_ticker_network_error(self):
        """Verify ccxt network error mapping to gRPC UNAVAILABLE status."""
        self.mock_exchange.fetch_ticker.side_effect = ccxt.NetworkError("Timeout")
        self.context.abort.side_effect = Exception("Aborted")
        request = exchange_pb2.GetTickerRequest(exchange="binance", symbol="BTC/USDT")
        with self.assertRaises(Exception):
            self.service.GetTicker(request, self.context)
        self.context.abort.assert_called_with(
            grpc.StatusCode.UNAVAILABLE, "Exchange network error: Timeout"
        )

    def test_get_ticker_exchange_network_error(self):
        """Verify custom ExchangeNetworkError mapping to gRPC UNAVAILABLE status."""
        self.mock_exchange.fetch_ticker.side_effect = ExchangeNetworkError(
            "Connection refused"
        )
        self.context.abort.side_effect = Exception("Aborted")
        request = exchange_pb2.GetTickerRequest(exchange="binance", symbol="BTC/USDT")
        with self.assertRaises(Exception):
            self.service.GetTicker(request, self.context)
        self.context.abort.assert_called_with(
            grpc.StatusCode.UNAVAILABLE, "Exchange network error: Connection refused"
        )

    def test_get_balance_filter(self):
        """Verify filtering by specific currency."""
        request = exchange_pb2.GetBalanceRequest(exchange="binance", currency="USDT")
        response = self.service.GetBalance(request, self.context)
        self.assertEqual(len(response.balances), 1)
        self.assertEqual(response.balances[0].asset, "USDT")
        self.assertEqual(response.balances[0].free, 1000.0)
        self.assertEqual(response.balances[0].total, 1000.0)

    def test_get_balance_all(self):
        """Verify all supported assets are returned when no filter is applied."""
        self.mock_exchange.fetch_balance.return_value = {
            "free": {"USDT": 1000.0, "BTC": 0.5},
            "used": {"USDT": 0.0, "BTC": 0.1},
            "total": {"USDT": 1000.0, "BTC": 0.6},
        }
        request = exchange_pb2.GetBalanceRequest(exchange="binance")
        response = self.service.GetBalance(request, self.context)
        assets = [b.asset for b in response.balances]
        self.assertIn("USDT", assets)
        self.assertIn("BTC", assets)
        self.assertEqual(len(response.balances), 2)

    def test_get_balance_whitelisting(self):
        """Verify that unsupported assets are filtered out."""
        self.mock_exchange.fetch_balance.return_value = {
            "free": {"USDT": 1000.0, "CRO": 9999999.0},
            "used": {"USDT": 0.0, "CRO": 0.0},
            "total": {"USDT": 1000.0, "CRO": 9999999.0},
        }
        request = exchange_pb2.GetBalanceRequest(exchange="binance")
        response = self.service.GetBalance(request, self.context)
        assets = [b.asset for b in response.balances]
        self.assertIn("USDT", assets)
        self.assertNotIn("CRO", assets)

    def test_get_balance_ignores_filter(self):
        """Verify that currency filter is ignored and all supported balances are returned."""
        request = exchange_pb2.GetBalanceRequest(exchange="binance", currency="ETH")
        response = self.service.GetBalance(request, self.context)
        self.assertEqual(len(response.balances), 1)
        self.assertEqual(response.balances[0].asset, "USDT")

    def test_get_balance_internal_error(self):
        """Verify balance error handling."""
        self.mock_exchange.fetch_balance.side_effect = Exception("Internal error")
        self.context.abort.side_effect = Exception("Internal error")
        request = exchange_pb2.GetBalanceRequest(exchange="binance", currency="USDT")
        with self.assertRaises(Exception) as cm:
            self.service.GetBalance(request, self.context)
        self.assertIn("Internal error", str(cm.exception))
        self.context.abort.side_effect = None
        self.mock_exchange.fetch_balance.side_effect = None

    def test_get_balance_authentication_error(self):
        """Verify mapping of authentication failures."""
        self.mock_exchange.fetch_balance.side_effect = ccxt.AuthenticationError(
            "Invalid API Key"
        )
        self.context.abort.side_effect = Exception("Aborted")
        request = exchange_pb2.GetBalanceRequest(exchange="binance", currency="USDT")
        with self.assertRaises(Exception):
            self.service.GetBalance(request, self.context)
        self.context.abort.assert_called_with(
            grpc.StatusCode.UNAUTHENTICATED,
            "Auth failed: Invalid API Key",
        )

    def test_create_order(self):
        """Verify successful order creation and mapping."""
        request = exchange_pb2.CreateOrderRequest(
            exchange="binance",
            symbol="BTC/USDT",
            side="buy",
            type="limit",
            amount=1.0,
            price=50000.0,
        )
        response = self.service.CreateOrder(request, self.context)
        self.assertEqual(response.id, "12345")
        self.assertEqual(response.symbol, "BTC/USDT")
        self.assertEqual(response.status, "open")
        self.assertEqual(response.fee, 0.01)
        self.assertEqual(response.fee_currency, "USDT")

    def test_create_order_internal_error(self):
        """Verify create order internal error handling."""
        self.mock_exchange.create_order.side_effect = Exception("Internal error")
        self.context.abort.side_effect = Exception("Internal error")
        request = exchange_pb2.CreateOrderRequest(
            exchange="binance",
            symbol="BTC/USDT",
            side="buy",
            type="limit",
            amount=1.0,
            price=50000.0,
        )
        with self.assertRaises(Exception) as cm:
            self.service.CreateOrder(request, self.context)
        self.assertIn("Internal error", str(cm.exception))
        self.context.abort.side_effect = None
        self.mock_exchange.create_order.side_effect = None

    def test_create_order_insufficient_funds(self):
        """Verify mapping of ccxt.InsufficientFunds."""
        self.mock_exchange.create_order.side_effect = ccxt.InsufficientFunds("No money")
        self.context.abort.side_effect = Exception("Aborted")
        request = exchange_pb2.CreateOrderRequest(
            exchange="binance",
            symbol="BTC/USDT",
            side="buy",
            type="limit",
            amount=1.0,
            price=50000.0,
        )
        with self.assertRaises(Exception):
            self.service.CreateOrder(request, self.context)
        self.context.abort.assert_called_with(
            grpc.StatusCode.FAILED_PRECONDITION, "Insufficient funds: No money"
        )

    def test_create_order_invalid_order(self):
        """Verify mapping of ccxt.InvalidOrder parameters."""
        self.mock_exchange.create_order.side_effect = ccxt.InvalidOrder(
            "Order amount is too small"
        )
        self.context.abort.side_effect = Exception("Aborted")
        request = exchange_pb2.CreateOrderRequest(
            exchange="binance",
            symbol="BTC/USDT",
            side="buy",
            type="limit",
            amount=0.0001,
            price=50000.0,
        )
        with self.assertRaises(Exception):
            self.service.CreateOrder(request, self.context)
        self.context.abort.assert_called_with(
            grpc.StatusCode.INVALID_ARGUMENT,
            "Invalid parameters: Order amount is too small",
        )

    def test_create_order_bad_request(self):
        """Verify mapping of BadRequestError."""
        self.mock_exchange.create_order.side_effect = BadRequestError("Invalid price")
        self.context.abort.side_effect = Exception("Aborted")
        request = exchange_pb2.CreateOrderRequest(
            exchange="binance",
            symbol="BTC/USDT",
            side="buy",
            type="limit",
            amount=1.0,
            price=-1.0,
        )
        with self.assertRaises(Exception):
            self.service.CreateOrder(request, self.context)
        self.context.abort.assert_called_with(
            grpc.StatusCode.INVALID_ARGUMENT, "Invalid parameters: Invalid price"
        )

    def test_create_stop_order(self):
        """Verify successful stop order creation and mapping."""
        self.mock_exchange.create_stop_order.return_value = {
            "id": "stop-123",
            "status": "open",
            "filled": 0.0,
        }
        request = exchange_pb2.CreateStopOrderRequest(
            exchange="binance",
            symbol="BTC/USDT",
            side="sell",
            amount=0.1,
            stop_price=40000.0,
        )
        response = self.service.CreateStopOrder(request, self.context)
        self.assertEqual(response.id, "stop-123")
        self.mock_exchange.create_stop_order.assert_called_with(
            symbol="BTC/USDT",
            side="sell",
            amount=0.1,
            stop_price=40000.0,
            limit_price=None,
        )

    def test_create_stop_order_insufficient_funds(self):
        """Verify mapping of InsufficientFundsError in CreateStopOrder."""
        self.mock_exchange.create_stop_order.side_effect = InsufficientFundsError(
            "Insufficient balance"
        )
        self.context.abort.side_effect = Exception("Aborted")
        request = exchange_pb2.CreateStopOrderRequest(
            exchange="binance",
            symbol="BTC/USDT",
            side="sell",
            amount=1.0,
            stop_price=50000.0,
        )
        with self.assertRaises(Exception):
            self.service.CreateStopOrder(request, self.context)
        self.context.abort.assert_called_with(
            grpc.StatusCode.FAILED_PRECONDITION,
            "Insufficient funds: Insufficient balance",
        )

    def test_cancel_order(self):
        """Verify order cancellation response."""
        request = exchange_pb2.CancelOrderRequest(
            exchange="binance", id="12345", symbol="BTC/USDT"
        )
        response = self.service.CancelOrder(request, self.context)
        self.assertEqual(response.id, "12345")
        self.assertEqual(response.status, "canceled")

    def test_cancel_order_internal_error(self):
        """Verify cancel order internal error handling."""
        self.mock_exchange.cancel_order.side_effect = Exception("Internal error")
        self.context.abort.side_effect = Exception("Internal error")
        request = exchange_pb2.CancelOrderRequest(
            exchange="binance", id="12345", symbol="BTC/USDT"
        )
        with self.assertRaises(Exception) as cm:
            self.service.CancelOrder(request, self.context)
        self.assertIn("Internal error", str(cm.exception))
        self.context.abort.side_effect = None
        self.mock_exchange.cancel_order.side_effect = None

    def test_get_order(self):
        """Verify order status retrieval."""
        request = exchange_pb2.GetOrderRequest(
            exchange="binance", id="12345", symbol="BTC/USDT"
        )
        response = self.service.GetOrder(request, self.context)
        self.assertEqual(response.id, "12345")
        self.assertEqual(response.symbol, "BTC/USDT")
        self.assertEqual(response.status, "closed")
        self.assertEqual(response.fee, 0.05)
        self.assertEqual(response.fee_currency, "BRL")

    def test_get_order_internal_error(self):
        """Verify get order internal error handling."""
        self.mock_exchange.fetch_order.side_effect = Exception("Internal error")
        self.context.abort.side_effect = Exception("Internal error")
        request = exchange_pb2.GetOrderRequest(
            exchange="binance", id="12345", symbol="BTC/USDT"
        )
        with self.assertRaises(Exception) as cm:
            self.service.GetOrder(request, self.context)
        self.assertIn("Internal error", str(cm.exception))
        self.context.abort.side_effect = None
        self.mock_exchange.fetch_order.side_effect = None

    def test_get_open_orders(self):
        """Verify open orders listing and parameter propagation."""
        self.mock_exchange.fetch_open_orders.return_value = [
            {
                "id": "101",
                "symbol": "BTC/USDT",
                "side": "buy",
                "type": "limit",
                "amount": 0.5,
                "price": 20000.0,
                "status": "open",
                "filled": 0.0,
                "remaining": 0.5,
                "cost": 0.0,
                "average": 0.0,
            },
            {
                "id": "102",
                "symbol": "BTC/USDT",
                "status": "open",
            },
        ]
        request = exchange_pb2.GetOpenOrdersRequest(
            exchange="binance", symbol="BTC/USDT", limit=2
        )
        response = self.service.GetOpenOrders(request, self.context)
        self.assertEqual(len(response.orders), 2)
        self.assertEqual(response.orders[0].id, "101")
        self.assertEqual(response.orders[1].id, "102")
        self.mock_exchange.fetch_open_orders.assert_called_with("BTC/USDT", limit=2)

        # Test that limit parameter is correctly applied
        request = exchange_pb2.GetOpenOrdersRequest(
            exchange="binance", symbol="BTC/USDT", limit=1
        )
        response = self.service.GetOpenOrders(request, self.context)
        self.assertEqual(len(response.orders), 1)
        self.assertEqual(response.orders[0].id, "101")
        self.mock_exchange.fetch_open_orders.assert_called_with("BTC/USDT", limit=1)

    def test_get_open_orders_no_params(self):
        """Verify that empty parameters are correctly converted to None."""
        request = exchange_pb2.GetOpenOrdersRequest(
            exchange="binance", symbol="", limit=0
        )
        self.service.GetOpenOrders(request, self.context)
        self.mock_exchange.fetch_open_orders.assert_called_with(None, limit=None)

    def test_get_open_orders_internal_error(self):
        """Verify open orders error handling."""
        self.mock_exchange.fetch_open_orders.side_effect = Exception("Internal error")
        self.context.abort.side_effect = Exception("Internal error")
        request = exchange_pb2.GetOpenOrdersRequest(
            exchange="binance", symbol="BTC/USDT"
        )
        with self.assertRaises(Exception) as cm:
            self.service.GetOpenOrders(request, self.context)
        self.assertIn("Internal error", str(cm.exception))
        self.context.abort.side_effect = None
        self.mock_exchange.fetch_open_orders.side_effect = None

    def test_get_recent_trades_internal_error(self):
        """Verify internal exception mapping in GetRecentTrades."""
        self.mock_exchange.fetch_my_trades.side_effect = Exception("Database failure")
        self.context.abort.side_effect = Exception("Aborted")
        request = exchange_pb2.GetRecentTradesRequest(
            exchange="binance", symbol="BTC/USDT"
        )
        with self.assertRaises(Exception):
            self.service.GetRecentTrades(request, self.context)
        self.context.abort.assert_called_with(
            grpc.StatusCode.INTERNAL, "Internal gateway error: Database failure"
        )

    def test_get_recent_trades(self):
        """Verify historical trade history retrieval and parameter propagation."""
        self.mock_exchange.fetch_my_trades.return_value = [
            {
                "id": "t1",
                "order": "101",
                "symbol": "BTC/USDT",
                "side": "buy",
                "price": 20000.0,
                "amount": 0.5,
                "cost": 10000.0,
                "timestamp": 1672531200000,
            },
            {
                "id": "t2",
                "symbol": "BTC/USDT",
                "side": "sell",
                "amount": 0.2,
                "price": 21000.0,
                "timestamp": 1672531300000,
            },
        ]
        request = exchange_pb2.GetRecentTradesRequest(
            exchange="binance", symbol="BTC/USDT", since=1672531200000, limit=2
        )
        response = self.service.GetRecentTrades(request, self.context)
        self.assertEqual(len(response.orders), 2)
        self.assertEqual(response.orders[0].id, "101")
        self.assertEqual(response.orders[1].id, "t2")
        self.mock_exchange.fetch_my_trades.assert_called_with(
            "BTC/USDT", since=1672531200000, limit=2
        )

        # Test that limit parameter is correctly applied
        request = exchange_pb2.GetRecentTradesRequest(
            exchange="binance", symbol="BTC/USDT", since=1672531200000, limit=1
        )
        response = self.service.GetRecentTrades(request, self.context)
        self.assertEqual(len(response.orders), 1)
        self.assertEqual(response.orders[0].id, "101")
        self.mock_exchange.fetch_my_trades.assert_called_with(
            "BTC/USDT", since=1672531200000, limit=1
        )

    def test_get_recent_trades_no_params(self):
        """Verify that empty trade audit parameters are converted to None."""
        request = exchange_pb2.GetRecentTradesRequest(
            exchange="binance", symbol="", since=0, limit=0
        )
        self.service.GetRecentTrades(request, self.context)
        self.mock_exchange.fetch_my_trades.assert_called_with(
            None, since=None, limit=None
        )

    def test_reset_state(self):
        """Verify state reset for testing purposes."""
        request = exchange_pb2.ResetStateRequest()
        response = self.service.ResetState(request, self.context)
        self.assertEqual(response.status, "OK")
        self.mock_exchange.reset.assert_called_once()

    def test_reset_state_exception_returns_ignored(self):
        """Verify that ResetState returns IGNORED when an exception occurs."""
        # Force an exception when accessing the factory or calling reset
        self.mock_factory.get.side_effect = Exception("Factory failure")
        request = exchange_pb2.ResetStateRequest()
        response = self.service.ResetState(request, self.context)
        self.assertEqual(response.status, "IGNORED")


# To run this test directly, use:
#   python -m tests.exchange.test_service
if __name__ == "__main__":
    unittest.main()
