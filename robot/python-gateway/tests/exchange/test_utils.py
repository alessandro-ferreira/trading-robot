import ccxt
import grpc
import unittest

from unittest.mock import MagicMock, patch


from exchange import utils
from exchange.exchanges.base import (
    ExchangeNetworkError,
    AuthenticationError,
    InsufficientFundsError,
    BadRequestError,
)
from exchange.factory import (
    ExchangeNotConfigured,
    ExchangeConfigurationError,
)
from v1 import exchange_pb2


class TestUtils(unittest.TestCase):
    def setUp(self):
        self.context = MagicMock()

    def test_get_exchange_success(self):
        """Vefify exchange retrieval from the factory works correctly."""
        factory = MagicMock()
        request = MagicMock(exchange="binance")
        utils.get_exchange(factory, request, self.context)
        factory.get.assert_called_with("binance")

    def test_get_exchange_error_handling(self):
        """Verift error handling when exchange retrieval fails."""
        factory = MagicMock()
        test_cases = [
            (ExchangeNotConfigured("Not configured"), grpc.StatusCode.NOT_FOUND),
            (
                ExchangeConfigurationError("Bad config"),
                grpc.StatusCode.FAILED_PRECONDITION,
            ),
        ]

        for error, expected_code in test_cases:
            with self.subTest(error=error):
                factory.get.side_effect = error
                self.context.abort.side_effect = Exception("Aborted")
                request = MagicMock(exchange="test")
                with self.assertRaises(Exception):
                    utils.get_exchange(factory, request, self.context)
                self.context.abort.assert_called_with(expected_code, str(error))

    def test_retry_network_call_success(self):
        """Verify successful retry of a network call after an initial failure."""
        mock_func = MagicMock()
        mock_func.side_effect = [ccxt.NetworkError("Fail"), "Success"]
        with patch("time.sleep"):
            result = utils.retry_network_call(mock_func, "arg")
        self.assertEqual(result, "Success")
        self.assertEqual(mock_func.call_count, 2)

    def test_handle_exchange_error_mapping(self):
        """Verify handling of various exchange errors and their mapping to gRPC status codes."""
        test_cases = [
            (
                ccxt.NetworkError("Timeout"),
                grpc.StatusCode.UNAVAILABLE,
                "Exchange network error: Timeout",
            ),
            (
                ExchangeNetworkError("Refused"),
                grpc.StatusCode.UNAVAILABLE,
                "Exchange network error: Refused",
            ),
            (
                ccxt.AuthenticationError("Invalid Key"),
                grpc.StatusCode.UNAUTHENTICATED,
                "Auth failed: Invalid Key",
            ),
            (
                AuthenticationError("Expired"),
                grpc.StatusCode.UNAUTHENTICATED,
                "Auth failed: Expired",
            ),
            (
                ccxt.InsufficientFunds("No money"),
                grpc.StatusCode.FAILED_PRECONDITION,
                "Insufficient funds: No money",
            ),
            (
                InsufficientFundsError("No BRL"),
                grpc.StatusCode.FAILED_PRECONDITION,
                "Insufficient funds: No BRL",
            ),
            (
                ccxt.InvalidOrder("Small amount"),
                grpc.StatusCode.INVALID_ARGUMENT,
                "Invalid parameters: Small amount",
            ),
            (
                BadRequestError("Bad price"),
                grpc.StatusCode.INVALID_ARGUMENT,
                "Invalid parameters: Bad price",
            ),
            (
                Exception("DB error"),
                grpc.StatusCode.INTERNAL,
                "Internal gateway error: DB error",
            ),
        ]

        for error, expected_code, expected_msg in test_cases:
            with self.subTest(error=error):
                self.context.abort.reset_mock()
                self.context.abort.side_effect = Exception("Aborted")
                with self.assertRaises(Exception):
                    utils.handle_exchange_error(self.context, error, "action")
                self.context.abort.assert_called_with(expected_code, expected_msg)

    def test_map_order_properties(self):
        """Verify that every exchange field is mapped to OrderResponse."""
        request = exchange_pb2.CreateOrderRequest(
            exchange="binance",
            symbol="BTC/USDT",
            side="buy",
            type="limit",
            amount=1.0,
            price=45000.0,
        )
        order = {
            "id": "order-123",
            "symbol": "BTC/USDT",
            "side": "sell",
            "type": "limit",
            "amount": 1.5,
            "price": 50000.0,
            "status": "closed",
            "filled": 1.25,
            "remaining": 0.25,
            "cost": 62500.0,
            "average": 49995.0,
            "clientOrderId": "client-123",
            "timestamp": 1678886400000,
            "fee": {"cost": 0.1, "currency": "USDT"},
        }

        result = utils.map_order(request, order)

        self.assertEqual(result.id, "order-123")
        self.assertEqual(result.symbol, "BTC/USDT")
        self.assertEqual(result.side, "sell")
        self.assertEqual(result.type, "limit")
        self.assertEqual(result.amount, 1.5)
        self.assertEqual(result.price, 50000.0)
        self.assertEqual(result.status, "closed")
        self.assertEqual(result.filled, 1.25)
        self.assertEqual(result.remaining, 0.25)
        self.assertEqual(result.cost, 62500.0)
        self.assertEqual(result.average, 49995.0)
        self.assertEqual(result.client_order_id, "client-123")
        self.assertEqual(result.timestamp, 1678886400000)
        self.assertEqual(result.fee, 0.1)
        self.assertEqual(result.fee_currency, "USDT")

    def test_map_order_requires_request(self):
        with self.assertRaisesRegex(ValueError, "Request object is required"):
            utils.map_order(None, {"id": "order-123"})

    def test_map_order_empty_order_returns_empty_response(self):
        request = exchange_pb2.GetOrderRequest(symbol="BTC/USDT")

        result = utils.map_order(request, {})

        self.assertEqual(result, exchange_pb2.OrderResponse())

    def test_map_order_stop_and_scalar_fee(self):
        """Verify stop-limit mapping and scalar fee fields."""
        request = exchange_pb2.CreateStopOrderRequest(
            symbol="ETH/USDT", side="sell", amount=0.5, stop_price=30000.0
        )
        order = {
            "id": "stop-123",
            "type": "stop_loss_limit",
            "price": 29000.0,
            "stopPrice": 30000.0,
            "side": "sell",
            "status": "open",
            "fee": 0.05,
            "fee_currency": "USDT",
        }

        result = utils.map_order(request, order)

        self.assertEqual(result.id, "stop-123")
        self.assertEqual(result.symbol, "ETH/USDT")
        self.assertEqual(result.side, "sell")
        self.assertEqual(result.type, "stop_limit")
        self.assertEqual(result.amount, 0.5)
        self.assertEqual(result.price, 30000.0)
        self.assertEqual(result.status, "open")
        self.assertEqual(result.filled, 0.0)
        self.assertEqual(result.remaining, 0.0)
        self.assertEqual(result.cost, 0.0)
        self.assertEqual(result.average, 0.0)
        self.assertEqual(result.client_order_id, "")
        self.assertEqual(result.timestamp, 0)
        self.assertEqual(result.fee, 0.05)
        self.assertEqual(result.fee_currency, "USDT")

    def test_map_order_missing_fields(self):
        """Verify mapping when some fields are missing in the order."""
        request = exchange_pb2.CreateOrderRequest(
            symbol="LTC/USDT", side="buy", amount=2.0
        )
        order = {
            "id": "order-456",
            "symbol": "LTC/USDT",
            "side": "buy",
            # Missing type, price, status, filled, remaining, cost, average
        }

        result = utils.map_order(request, order)

        self.assertEqual(result.id, "order-456")
        self.assertEqual(result.symbol, "LTC/USDT")
        self.assertEqual(result.side, "buy")
        self.assertEqual(result.type, "market")  # Default to market if type missing
        self.assertEqual(result.amount, 2.0)
        self.assertEqual(result.price, 0.0)  # Default to 0.0 if price missing
        self.assertEqual(result.status, "")  # Default to empty string if status missing
        self.assertEqual(result.filled, 0.0)
        self.assertEqual(result.remaining, 0.0)
        self.assertEqual(result.cost, 0.0)
        self.assertEqual(result.average, 0.0)
        self.assertEqual(result.client_order_id, "")
        self.assertEqual(result.timestamp, 0)
        self.assertEqual(result.fee, 0.0)
        self.assertEqual(result.fee_currency, "")

    def test_map_order_creation_fallback(self):
        """Verify fallback mapping for order creation when type/price info is missing."""
        # Simulation of a creation response missing type/price info
        order = {"id": "ord-1", "status": "open"}
        request = exchange_pb2.CreateStopOrderRequest(
            symbol="BTC/USDT", side="sell", amount=0.5, stop_price=30000.0
        )
        res = utils.map_order(request, order)
        self.assertEqual(res.id, "ord-1")
        self.assertEqual(res.symbol, "BTC/USDT")
        self.assertEqual(res.side, "sell")
        self.assertEqual(res.type, "stop_market")
        self.assertEqual(res.amount, 0.5)
        self.assertEqual(res.price, 30000.0)
        self.assertEqual(res.status, "open")
        self.assertEqual(res.filled, 0.0)
        self.assertEqual(res.remaining, 0.0)
        self.assertEqual(res.cost, 0.0)
        self.assertEqual(res.average, 0.0)
        self.assertEqual(res.client_order_id, "")
        self.assertEqual(res.timestamp, 0)
        self.assertEqual(res.fee, 0.0)
        self.assertEqual(res.fee_currency, "")


# To run this test directly, use:
#   python -m tests.exchange.test_utils
if __name__ == "__main__":
    unittest.main()
