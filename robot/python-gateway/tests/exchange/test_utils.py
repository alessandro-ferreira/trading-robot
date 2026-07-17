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
        """Verify that order properties are correctly mapped from the exchange response."""
        test_cases = [
            # Market Order
            ({"id": "1", "type": "market", "price": 0.0}, "market", 0.0),
            # Limit Order
            ({"id": "2", "type": "limit", "price": 50000.0}, "limit", 50000.0),
            # Stop Market (Binance style)
            (
                {"id": "3", "type": "stop_loss", "triggerPrice": 70000.0},
                "stop_market",
                70000.0,
            ),
            # Stop Limit
            (
                {
                    "id": "4",
                    "type": "stop_loss_limit",
                    "price": 69000.0,
                    "stopPrice": 70000.0,
                },
                "stop_limit",
                70000.0,
            ),
        ]

        for order, expected_type, expected_price in test_cases:
            with self.subTest(order_id=order["id"]):
                res = utils.map_order(order)
                self.assertEqual(res.type, expected_type)
                self.assertEqual(res.price, expected_price)

    def test_map_order_creation_fallback(self):
        """Verify fallback mapping for order creation when type/price info is missing."""
        # Simulation of a creation response missing type/price info
        order = {"id": "ord-1", "status": "open"}
        request = exchange_pb2.CreateStopOrderRequest(
            symbol="BTC/USDT", side="sell", amount=0.5, stop_price=30000.0
        )
        res = utils.map_order(order, req=request)
        self.assertEqual(res.type, "stop_market")
        self.assertEqual(res.price, 30000.0)

    def test_map_order_identifiers(self):
        """Verify that order identifiers are correctly mapped to the response."""
        # Standard order
        res = utils.map_order({"id": "123", "clientOrderId": "cid-1"})
        self.assertEqual(res.id, "123")
        self.assertEqual(res.client_order_id, "cid-1")

        # Trade execution
        res = utils.map_order({"id": "t1", "order": "123"}, is_trade=True)
        self.assertEqual(res.id, "123")


# To run this test directly, use:
#   python -m tests.exchange.test_utils
if __name__ == "__main__":
    unittest.main()
