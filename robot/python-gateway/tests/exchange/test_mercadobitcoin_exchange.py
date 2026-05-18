import http.client
import os
import unittest
from unittest.mock import MagicMock, patch

import requests
from core import config
from exchange.exchanges.base import (
    ExchangeError,
    ExchangeNetworkError,
    AuthenticationError,
    BadRequestError,
    OrderType,
)
from exchange.exchanges.mercadobitcoin import MercadoBitcoinExchange

TEST_DATA_DIR = "tests/exchange/testdata"


class TestMercadoBitcoinExchange(unittest.TestCase):
    def setUp(self):
        cfg = config.load(os.path.join(TEST_DATA_DIR, "config.toml"))
        self.cfg = next(ex for ex in cfg.exchanges if ex.name == "mercadobitcoin")
        self.exchange = MercadoBitcoinExchange(self.cfg)

    @patch("requests.post")
    def test_authenticate_success(self, mock_post):
        """Verify successful OAuth2 token retrieval."""
        # Mock successful authentication response
        mock_response = MagicMock()
        mock_response.status_code = http.client.OK
        mock_response.json.return_value = {
            "access_token": "mock_token",
            "expiration": 1800,
        }
        mock_post.return_value = mock_response

        self.exchange._authenticate()

        self.assertEqual(self.exchange._token, "mock_token")
        self.assertGreater(self.exchange._token_expiry, 0)
        mock_post.assert_called_once()

    @patch("requests.post")
    def test_authenticate_failure(self, mock_post):
        """Verify error propagation on failed authentication."""
        # Mock failed authentication
        mock_response = MagicMock()
        mock_response.status_code = http.client.UNAUTHORIZED
        mock_response.text = "Unauthorized"
        mock_post.return_value = mock_response

        with self.assertRaises(ExchangeError):
            self.exchange._authenticate()

    @patch("requests.request")
    def test_fetch_ticker_success(self, mock_request):
        """Verify ticker fetching and nanosecond-to-millisecond timestamp conversion."""
        self.exchange._token = "mock_token"
        self.exchange._token_expiry = 9999999999

        # Mock ticker response
        mock_response = MagicMock()
        mock_response.status_code = http.client.OK
        mock_response.json.return_value = [
            {
                "pair": "BTC-BRL",
                "high": "200000.00000000",
                "low": "190000.00000000",
                "vol": "50.00000000",
                "last": "195000.00000000",
                "buy": "194900.00000000",
                "sell": "195100.00000000",
                "open": "192000.00000000",
                "date": 1672531200000000000,  # Nanoseconds
            }
        ]
        mock_request.return_value = mock_response

        ticker = self.exchange.fetch_ticker("BTC/BRL")

        self.assertEqual(ticker.symbol, "BTC/BRL")
        self.assertEqual(ticker.last, 195000.0)
        self.assertEqual(ticker.bid, 194900.0)
        self.assertEqual(ticker.ask, 195100.0)
        # Timestamp converted to ms: 1672531200000000 / 1000 = 1672531200000
        self.assertEqual(ticker.timestamp, 1672531200000)

    @patch("requests.request")
    def test_fetch_ticker_failure(self, mock_request):
        """Verify error handling for invalid market pairs."""
        self.exchange._token = "mock_token"
        self.exchange._token_expiry = 9999999999

        mock_response = MagicMock()
        mock_response.status_code = http.client.NOT_FOUND
        mock_response.text = "Not Found"
        mock_request.return_value = mock_response

        with self.assertRaises(ExchangeError):
            self.exchange.fetch_ticker("INVALID/PAIR")

    @patch("requests.request")
    def test_fetch_ticker_network_error(self, mock_request):
        """Verify mapping of requests.RequestException to ExchangeNetworkError."""
        self.exchange._token = "mock_token"
        self.exchange._token_expiry = 9999999999

        mock_request.side_effect = requests.exceptions.ConnectionError("Refused")
        with self.assertRaises(ExchangeNetworkError) as cm:
            self.exchange.fetch_ticker("BTC/BRL")
        self.assertIn("Network error during GET", str(cm.exception))

    @patch("requests.request")
    @patch("requests.post")  # For authentication
    def test_fetch_balance_success(self, mock_post, mock_request):
        """Verify multi-asset balance fetching and caching of account IDs."""
        # Mock Auth
        auth_resp = MagicMock()
        auth_resp.status_code = http.client.OK
        auth_resp.json.return_value = {"access_token": "t", "expiration": 3600}
        mock_post.return_value = auth_resp

        # Mock Accounts (for _get_account_id)
        accounts_resp = MagicMock()
        accounts_resp.status_code = http.client.OK
        accounts_resp.json.return_value = [{"id": "acc_123"}]

        # Mock Balances
        balances_resp = MagicMock()
        balances_resp.status_code = http.client.OK
        balances_resp.json.return_value = [
            {
                "symbol": "BRL",
                "available": "1000.0",
                "on_hold": "0.0",
                "total": "1000.0",
            },
            {"symbol": "BTC", "available": "0.5", "on_hold": "0.1", "total": "0.6"},
        ]

        # Configure side_effect to return accounts then balances
        mock_request.side_effect = [accounts_resp, balances_resp]

        balance = self.exchange.fetch_balance()

        self.assertEqual(balance["free"]["BRL"], 1000.0)
        self.assertEqual(balance["total"]["BTC"], 0.6)
        self.assertEqual(balance["used"]["BTC"], 0.1)

    @patch("requests.request")
    def test_create_order_success(self, mock_request):
        """Verify limit order creation with correct payload formatting."""
        # Pre-set account ID and token to skip auth/account calls
        self.exchange._account_id = "acc_123"
        self.exchange._token = "mock_token"
        self.exchange._token_expiry = 9999999999

        mock_response = MagicMock()
        mock_response.status_code = http.client.OK
        mock_response.json.return_value = {
            "orderId": "ord_123",
            "qty": "0.1",
            "limitPrice": "100000.0",
            "side": "buy",
            "type": "limit",
            "status": "created",
        }
        mock_request.return_value = mock_response

        order = self.exchange.create_order(
            "BTC/BRL", OrderType.LIMIT, "buy", 0.1, 100000.0
        )

        self.assertEqual(order["id"], "ord_123")
        self.assertEqual(order["symbol"], "BTC/BRL")
        self.assertEqual(order["status"], "open")

        args, kwargs = mock_request.call_args
        self.assertEqual(args[0], "POST")
        self.assertIn("/accounts/acc_123/BTC-BRL/orders", args[1])
        self.assertEqual(kwargs["json"]["qty"], "0.1")
        self.assertEqual(kwargs["json"]["limitPrice"], 100000.0)

    @patch("requests.request")
    def test_create_order_market_success(self, mock_request):
        """Verify market order omits limitPrice."""
        self.exchange._account_id = "acc_123"
        self.exchange._token = "mock_token"
        self.exchange._token_expiry = 9999999999

        mock_response = MagicMock()
        mock_response.status_code = http.client.OK
        mock_response.json.return_value = {
            "orderId": "ord_market",
            "qty": "0.1",
            "side": "sell",
            "type": "market",
            "status": "filled",
        }
        mock_request.return_value = mock_response

        order = self.exchange.create_order("BTC/BRL", OrderType.MARKET, "sell", 0.1)

        self.assertEqual(order["id"], "ord_market")
        args, kwargs = mock_request.call_args
        payload = kwargs["json"]
        self.assertEqual(payload["type"], "market")
        self.assertNotIn("limitPrice", payload)

    def test_create_order_missing_price_for_limit(self):
        """Verify client-side validation for missing limit prices."""
        with self.assertRaises(ExchangeError) as cm:
            self.exchange.create_order("BTC/BRL", OrderType.LIMIT, "buy", 0.1)
        self.assertIn("Price is required for limit orders", str(cm.exception))

    @patch("requests.request")
    def test_create_order_api_failure(self, mock_request):
        """Verify API error handling during order creation."""
        self.exchange._account_id = "acc_123"
        self.exchange._token = "mock_token"
        self.exchange._token_expiry = 9999999999

        mock_response = MagicMock()
        mock_response.status_code = http.client.BAD_REQUEST
        mock_response.text = "Invalid quantity"
        mock_request.return_value = mock_response

        with self.assertRaises(BadRequestError) as cm:
            self.exchange.create_order("BTC/BRL", OrderType.MARKET, "buy", 0.1)
        self.assertIn("MercadoBitcoin API Error: 400", str(cm.exception))

    @patch("requests.request")
    def test_cancel_order_success(self, mock_request):
        """Verify order cancellation with explicit symbol requirement."""
        self.exchange._account_id = "acc_123"
        self.exchange._token = "mock_token"
        self.exchange._token_expiry = 9999999999

        mock_response = MagicMock()
        mock_response.status_code = http.client.OK
        mock_response.json.return_value = {"status": "cancelled"}
        mock_request.return_value = mock_response

        result = self.exchange.cancel_order("ord_123", "BTC/BRL")

        self.assertEqual(result["id"], "ord_123")
        self.assertEqual(result["status"], "canceled")

        args, kwargs = mock_request.call_args
        self.assertEqual(args[0], "DELETE")
        self.assertIn("/accounts/acc_123/BTC-BRL/orders/ord_123?async=false", args[1])

    @patch("requests.request")
    def test_create_stop_order_market_simulation(self, mock_request):
        """Verify stop-market simulation with 40% slippage and rounding."""
        self.exchange._account_id = "acc_123"
        self.exchange._token = "mock_token"
        self.exchange._token_expiry = 9999999999

        mock_response = MagicMock()
        mock_response.status_code = http.client.OK
        mock_response.json.return_value = {
            "orderId": "stop_123",
            "status": "created",
            "clientOrderId": "c_123",
        }
        mock_request.return_value = mock_response

        # Stop price 100,000. Sell order -> limit = 100,000 * 0.6 = 60,000
        order = self.exchange.create_stop_order("BTC/BRL", "sell", 0.1, 100000.0)

        args, kwargs = mock_request.call_args
        payload = kwargs["json"]
        self.assertEqual(payload["stopPrice"], 100000.0)
        self.assertEqual(payload["limitPrice"], 60000)
        self.assertEqual(order["price"], 100000.0)
        self.assertEqual(order["clientOrderId"], "c_123")

    @patch("requests.request")
    def test_create_stop_order_limit(self, mock_request):
        """Verify explicit stop-limit creation without slippage override."""
        self.exchange._account_id = "acc_123"
        self.exchange._token = "mock_token"
        self.exchange._token_expiry = 9999999999

        mock_response = MagicMock()
        mock_response.status_code = http.client.OK
        mock_response.json.return_value = {"orderId": "sl_123", "status": "working"}
        mock_request.return_value = mock_response

        self.exchange.create_stop_order(
            "BTC/BRL", "buy", 0.1, 100000.0, limit_price=101000.0
        )

        kwargs = mock_request.call_args.kwargs
        self.assertEqual(kwargs["json"]["limitPrice"], 101000.0)

    def test_handle_http_errors_mapping(self):
        """Verify mapping of HTTP status codes to custom exceptions."""
        mock_resp = MagicMock()
        mock_resp.text = "Error"

        mock_resp.status_code = 401
        self.assertRaises(
            AuthenticationError, self.exchange._handle_http_errors, mock_resp
        )

        mock_resp.status_code = 400
        self.assertRaises(BadRequestError, self.exchange._handle_http_errors, mock_resp)

    def test_cancel_order_missing_symbol(self):
        """Verify that MB requires a symbol for cancellation."""
        with self.assertRaises(ExchangeError) as cm:
            self.exchange.cancel_order("ord_123")
        self.assertIn("Symbol is required", str(cm.exception))

    @patch("requests.request")
    def test_cancel_order_api_failure(self, mock_request):
        """Verify handling of 404 on order cancellation."""
        self.exchange._account_id = "acc_123"
        self.exchange._token = "mock_token"
        self.exchange._token_expiry = 9999999999

        mock_response = MagicMock()
        mock_response.status_code = http.client.NOT_FOUND
        mock_response.text = "Order not found"
        mock_request.return_value = mock_response

        with self.assertRaises(ExchangeError) as cm:
            self.exchange.cancel_order("ord_123", "BTC/BRL")
        self.assertIn("MercadoBitcoin API Error: 404", str(cm.exception))

    @patch("requests.request")
    def test_fetch_order_success(self, mock_request):
        """Verify individual order fetching and status mapping."""
        self.exchange._account_id = "acc_123"
        self.exchange._token = "mock_token"
        self.exchange._token_expiry = 9999999999

        mock_response = MagicMock()
        mock_response.status_code = http.client.OK
        mock_response.json.return_value = {
            "id": "ord_123",
            "externalId": "client_123",
            "type": "limit",
            "side": "buy",
            "limitPrice": "100000.0",
            "qty": "0.1",
            "filledQty": "0.05",
            "status": "working",
            "created_at": 1672531200,
        }
        mock_request.return_value = mock_response

        order = self.exchange.fetch_order("ord_123", "BTC/BRL")

        self.assertEqual(order["id"], "ord_123")
        self.assertEqual(order["clientOrderId"], "client_123")
        self.assertEqual(order["status"], "open")
        self.assertEqual(order["filled"], 0.05)
        self.assertEqual(order["remaining"], 0.05)
        self.assertEqual(order["timestamp"], 1672531200000)

    def test_fetch_order_missing_symbol(self):
        """Verify symbol requirement for individual order fetch."""
        with self.assertRaises(ExchangeError) as cm:
            self.exchange.fetch_order("ord_123")
        self.assertIn("Symbol is required", str(cm.exception))

    @patch("requests.request")
    def test_fetch_order_api_failure(self, mock_request):
        """Verify API error handling during order fetch."""
        self.exchange._account_id = "acc_123"
        self.exchange._token = "mock_token"
        self.exchange._token_expiry = 9999999999

        mock_response = MagicMock()
        mock_response.status_code = http.client.NOT_FOUND
        mock_response.text = "Order not found"
        mock_request.return_value = mock_response

        with self.assertRaises(ExchangeError) as cm:
            self.exchange.fetch_order("ord_123", "BTC/BRL")
        self.assertIn("MercadoBitcoin API Error: 404", str(cm.exception))

    @patch("requests.request")
    def test_fetch_open_orders_symbol_success(self, mock_request):
        """Verify symbol-specific open orders retrieval."""
        self.exchange._account_id = "acc_123"
        self.exchange._token = "mock_token"
        self.exchange._token_expiry = 9999999999

        mock_response = MagicMock()
        mock_response.status_code = http.client.OK
        mock_response.json.return_value = [
            {
                "id": "ord_1",
                "status": "created",
                "instrument": "BTC-BRL",
                "qty": "0.1",
                "filledQty": "0.0",
                "created_at": 1672531200,
            }
        ]
        mock_request.return_value = mock_response

        orders = self.exchange.fetch_open_orders("BTC/BRL", limit=5)

        self.assertEqual(len(orders), 1)
        self.assertEqual(orders[0]["id"], "ord_1")
        self.assertEqual(orders[0]["symbol"], "BTC/BRL")

        args, kwargs = mock_request.call_args
        self.assertEqual(args[0], "GET")
        self.assertIn("/accounts/acc_123/BTC-BRL/orders", args[1])
        self.assertEqual(kwargs["params"]["status"], "created,working")
        # Verify 'size' is NOT passed for symbol-specific path as per MB doc
        self.assertNotIn("size", kwargs.get("params", {}))

    @patch("requests.request")
    def test_fetch_open_orders_all_success(self, mock_request):
        """Verify account-wide open orders retrieval using the 'items' wrapper."""
        self.exchange._account_id = "acc_123"
        self.exchange._token = "mock_token"
        self.exchange._token_expiry = 9999999999

        mock_response = MagicMock()
        mock_response.status_code = http.client.OK
        mock_response.json.return_value = {
            "items": [
                {
                    "id": "ord_2",
                    "status": "working",
                    "instrument": "ETH-BRL",
                    "qty": "1.0",
                    "filledQty": "0.0",
                    "created_at": 1672531200,
                }
            ]
        }
        mock_request.return_value = mock_response

        orders = self.exchange.fetch_open_orders(limit=10)

        self.assertEqual(len(orders), 1)
        self.assertEqual(orders[0]["id"], "ord_2")
        self.assertEqual(orders[0]["symbol"], "ETH/BRL")

        args, kwargs = mock_request.call_args
        self.assertEqual(args[0], "GET")
        self.assertIn("/accounts/acc_123/orders", args[1])
        self.assertEqual(kwargs["params"]["status"], "created,working")
        self.assertEqual(kwargs["params"]["size"], 10)

    @patch("requests.request")
    def test_fetch_open_orders_api_failure(self, mock_request):
        """Verify error handling for the open orders list."""
        self.exchange._account_id = "acc_123"
        self.exchange._token = "mock_token"
        self.exchange._token_expiry = 9999999999

        mock_response = MagicMock()
        mock_response.status_code = http.client.INTERNAL_SERVER_ERROR
        mock_response.text = "Internal Server Error"
        mock_request.return_value = mock_response

        with self.assertRaises(ExchangeError) as cm:
            self.exchange.fetch_open_orders("BTC/BRL")
        self.assertIn("MercadoBitcoin API Error: 500", str(cm.exception))

    @patch("requests.request")
    def test_fetch_my_trades_success(self, mock_request):
        """Verify historical execution extraction from nested order data."""
        self.exchange._account_id = "acc_123"
        self.exchange._token = "mock_token"
        self.exchange._token_expiry = 9999999999

        # Mock order with nested executions
        mock_response = MagicMock()
        mock_response.status_code = http.client.OK
        mock_response.json.return_value = [
            {
                "id": "parent_ord",
                "instrument": "BTC-BRL",
                "executions": [
                    {
                        "id": "exec_1",
                        "side": "buy",
                        "qty": "0.05",
                        "price": "190000.0",
                        "executed_at": 1672531200,
                    }
                ],
            }
        ]
        mock_request.return_value = mock_response

        since_ms = 1672531200000
        trades = self.exchange.fetch_my_trades("BTC/BRL", since=since_ms, limit=5)

        self.assertEqual(len(trades), 1)
        self.assertEqual(trades[0]["id"], "exec_1")

        args, kwargs = mock_request.call_args
        self.assertEqual(kwargs["params"]["has_executions"], "true")
        # Verify 'since' ms is converted to 'created_at_from' seconds
        self.assertEqual(kwargs["params"]["created_at_from"], 1672531200)
        # Verify 'size' is NOT passed for symbol-specific path as per MB doc
        self.assertNotIn("size", kwargs.get("params", {}))

    @patch("requests.request")
    def test_fetch_my_trades_all_success(self, mock_request):
        """Verify account-wide trade history retrieval (no symbol provided)."""
        self.exchange._account_id = "acc_123"
        self.exchange._token = "mock_token"
        self.exchange._token_expiry = 9999999999

        # Mock response for account-wide orders with executions
        mock_response = MagicMock()
        mock_response.status_code = http.client.OK
        mock_response.json.return_value = {
            "items": [
                {
                    "id": "order_all",
                    "executions": [
                        {
                            "id": "exec_all",
                            "instrument": "ETH-BRL",
                            "side": "sell",
                            "qty": "1.0",
                            "price": "10000.0",
                            "executed_at": 1672531300,
                        }
                    ],
                }
            ]
        }
        mock_request.return_value = mock_response

        trades = self.exchange.fetch_my_trades(symbol=None, limit=10)

        self.assertEqual(len(trades), 1)
        self.assertEqual(trades[0]["id"], "exec_all")
        self.assertEqual(trades[0]["symbol"], "ETH/BRL")

        args, kwargs = mock_request.call_args
        self.assertEqual(args[0], "GET")
        self.assertIn("/accounts/acc_123/orders", args[1])
        self.assertEqual(kwargs["params"]["has_executions"], "true")
        self.assertEqual(kwargs["params"]["size"], 10)


# To run this test directly, use:
#   python -m tests.exchange.test_mercadobitcoin_exchange
if __name__ == "__main__":
    unittest.main()
