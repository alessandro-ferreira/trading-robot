import http.client
import os
import requests
import unittest

from unittest.mock import MagicMock, patch

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

    @patch("requests.post")
    @patch("requests.request")
    def test_request_unauthorized_clears_token_and_reauthenticates(
        self, mock_request, mock_post
    ):
        """Verify that receiving 401 clears token, re-authenticates on next call, and succeeds."""
        # Initial valid token
        self.exchange._token = "initial_token"
        self.exchange._token_expiry = 9999999999

        # First request receives 401 Unauthorized
        resp_401 = MagicMock()
        resp_401.status_code = http.client.UNAUTHORIZED
        resp_401.text = "Unauthorized"

        # Second request (after re-authentication) succeeds
        resp_200 = MagicMock()
        resp_200.status_code = http.client.OK
        resp_200.json.return_value = [{"id": "acc_456"}]

        mock_request.side_effect = [resp_401, resp_200]

        # Auth response for re-authentication
        auth_resp = MagicMock()
        auth_resp.status_code = http.client.OK
        auth_resp.json.return_value = {
            "access_token": "new_token",
            "expiration": 9999999999,
        }
        mock_post.return_value = auth_resp

        # First request fails with AuthenticationError and resets token
        with self.assertRaises(AuthenticationError):
            self.exchange._request("GET", "/accounts")

        self.assertIsNone(self.exchange._token)
        self.assertEqual(self.exchange._token_expiry, 0)

        # Second request triggers re-authentication and succeeds
        data = self.exchange._request("GET", "/accounts")
        self.assertEqual(data, [{"id": "acc_456"}])
        self.assertEqual(self.exchange._token, "new_token")
        mock_post.assert_called_once()

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
    @patch("requests.post")
    def test_fetch_balance_truncates_known_assets_and_preserves_unknown_assets(
        self, mock_post, mock_request
    ):
        """Verify balances use executable precision while unknown assets remain unchanged."""
        auth_resp = MagicMock()
        auth_resp.status_code = http.client.OK
        auth_resp.json.return_value = {"access_token": "t", "expiration": 3600}
        mock_post.return_value = auth_resp

        accounts_resp = MagicMock()
        accounts_resp.status_code = http.client.OK
        accounts_resp.json.return_value = [{"id": "acc_123"}]

        balances_resp = MagicMock()
        balances_resp.status_code = http.client.OK
        balances_resp.json.return_value = [
            {
                "symbol": "XLM",
                "available": "26.65675789",
                "on_hold": "0.00075789",
                "total": "26.65751578",
            },
            {
                "symbol": "BTC",
                "available": "0.123456789",
                "on_hold": "0.000000009",
                "total": "0.123456798",
            },
            {
                "symbol": "UNKNOWN",
                "available": "1.23456789",
                "on_hold": "0.00000001",
                "total": "1.23456790",
            },
        ]
        mock_request.side_effect = [accounts_resp, balances_resp]

        balance = self.exchange.fetch_balance()

        self.assertEqual(balance["free"]["XLM"], 26.656)
        self.assertEqual(balance["used"]["XLM"], 0.0)
        self.assertEqual(balance["total"]["XLM"], 26.657)
        self.assertEqual(balance["free"]["BTC"], 0.12345678)
        self.assertEqual(balance["used"]["BTC"], 0.0)
        self.assertEqual(balance["total"]["BTC"], 0.12345679)
        self.assertEqual(balance["total"]["UNKNOWN"], 1.2345679)

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
            "executions": [{"fee": "0.001"}],
        }
        mock_request.return_value = mock_response

        order = self.exchange.create_order(
            "BTC/BRL", OrderType.LIMIT, "buy", 0.1, 100000.0
        )

        self.assertEqual(order["id"], "ord_123")
        self.assertEqual(order["symbol"], "BTC/BRL")
        self.assertEqual(order["status"], "open")
        self.assertEqual(order["fee"], 0.001)
        self.assertEqual(order["fee_currency"], "BTC")

        args, kwargs = mock_request.call_args
        self.assertEqual(args[0], "POST")
        self.assertIn("/accounts/acc_123/BTC-BRL/orders", args[1])
        self.assertEqual(kwargs["json"]["qty"], "0.10000000")
        self.assertEqual(kwargs["json"]["limitPrice"], 100000.0)

    @patch("requests.request")
    def test_create_order_market_success(self, mock_request):
        """Verify market order formatting uses XLM's MercadoBitcoin lot size."""
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

        order = self.exchange.create_order(
            "XLM/BRL", OrderType.MARKET, "sell", 26.65675789
        )

        self.assertEqual(order["id"], "ord_market")
        args, kwargs = mock_request.call_args
        payload = kwargs["json"]
        self.assertEqual(payload["type"], "market")
        self.assertEqual(payload["qty"], "26.656")
        self.assertNotIn("limitPrice", payload)

    def test_format_quantity_for_all_supported_mercadobitcoin_assets(self):
        amount = 1.23456789

        expected_by_decimals = {
            0: "1",
            3: "1.234",
            4: "1.2345",
            5: "1.23456",
            6: "1.234567",
            7: "1.2345678",
            8: "1.23456789",
        }
        for decimals, assets in self.exchange.QUANTITY_DECIMALS_BY_ASSET.items():
            expected = expected_by_decimals[decimals]
            for asset in assets:
                with self.subTest(asset=asset, decimals=decimals):
                    self.assertEqual(
                        self.exchange._format_quantity(amount, f"{asset}/BRL"),
                        expected,
                    )

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
        self.assertEqual(
            payload["limitPrice"], 100000.0 * (1.0 - self.exchange.STOP_MARKET_SLIPPAGE)
        )
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
        """Verify individual order fetching maps every available field."""
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
            "avgPrice": "99500.0",
            "cost": "4975.0",
            "status": "working",
            "created_at": 1672531200,
            "executions": [{"fee": "0.001"}],
        }
        mock_request.return_value = mock_response

        order = self.exchange.fetch_order("ord_123", "BTC/BRL")

        self.assertEqual(order["id"], "ord_123")
        self.assertEqual(order["clientOrderId"], "client_123")
        self.assertEqual(order["symbol"], "BTC/BRL")
        self.assertEqual(order["type"], "limit")
        self.assertEqual(order["side"], "buy")
        self.assertEqual(order["price"], 100000.0)
        self.assertEqual(order["average"], 99500.0)
        self.assertEqual(order["amount"], 0.1)
        self.assertEqual(order["status"], "open")
        self.assertEqual(order["filled"], 0.05)
        self.assertEqual(order["remaining"], 0.05)
        self.assertEqual(order["cost"], 4975.0)
        self.assertEqual(order["fee"], 0.001)
        self.assertEqual(order["fee_currency"], "BTC")
        self.assertEqual(order["timestamp"], 1672531200000)
        self.assertEqual(order["datetime"], "2023-01-01T00:00:00+00:00")
        self.assertEqual(order["info"], mock_response.json.return_value)

    @patch("requests.request")
    def test_fetch_order_with_missing_fields(self, mock_request):
        """Verify individual order fetching preserves defaults for missing fields."""
        self.exchange._account_id = "acc_123"
        self.exchange._token = "mock_token"
        self.exchange._token_expiry = 9999999999

        mock_response = MagicMock()
        mock_response.status_code = http.client.OK
        mock_response.json.return_value = {}
        mock_request.return_value = mock_response

        order = self.exchange.fetch_order("ord_123", "BTC/BRL")

        self.assertIsNone(order["id"])
        self.assertIsNone(order["clientOrderId"])
        self.assertEqual(order["symbol"], "BTC/BRL")
        self.assertEqual(order["type"], "")
        self.assertIsNone(order["side"])
        self.assertEqual(order["price"], 0.0)
        self.assertIsNone(order["average"])
        self.assertEqual(order["amount"], 0.0)
        self.assertEqual(order["status"], "")
        self.assertEqual(order["filled"], 0.0)
        self.assertEqual(order["remaining"], 0.0)
        self.assertIsNone(order["cost"])
        self.assertEqual(order["fee"], 0.0)
        self.assertEqual(order["fee_currency"], "BRL")
        self.assertIsNone(order["timestamp"])
        self.assertIsNone(order["datetime"])
        self.assertEqual(order["info"], {})

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
    def test_fetch_orders_symbol_success(self, mock_request):
        """Verify symbol-specific orders retrieval."""
        self.exchange._account_id = "acc_123"
        self.exchange._token = "mock_token"
        self.exchange._token_expiry = 9999999999

        mock_response = MagicMock()
        mock_response.status_code = http.client.OK
        mock_response.json.return_value = [
            {
                "id": "ord_1",
                "status": "closed",
                "instrument": "BTC-BRL",
                "qty": "0.1",
                "filledQty": "0.0",
                "created_at": 1672531200,
            }
        ]
        mock_request.return_value = mock_response

        orders = self.exchange.fetch_orders("BTC/BRL")

        self.assertEqual(len(orders), 1)
        self.assertEqual(orders[0]["id"], "ord_1")
        self.assertEqual(orders[0]["symbol"], "BTC/BRL")

        args, kwargs = mock_request.call_args
        self.assertEqual(args[0], "GET")
        self.assertIn("/accounts/acc_123/BTC-BRL/orders", args[1])
        # Verify 'size' is NOT passed for symbol-specific path as per MB doc
        self.assertNotIn("size", kwargs.get("params", {}))

    @patch("requests.request")
    def test_fetch_orders_all_success(self, mock_request):
        """Verify orders retrieval for the requested symbol."""
        self.exchange._account_id = "acc_123"
        self.exchange._token = "mock_token"
        self.exchange._token_expiry = 9999999999

        mock_response = MagicMock()
        mock_response.status_code = http.client.OK
        mock_response.json.return_value = [
            {
                "id": "ord_2",
                "status": "working",
                "instrument": "ETH-BRL",
                "qty": "1.0",
                "filledQty": "0.0",
                "created_at": 1672531200,
            }
        ]
        mock_request.return_value = mock_response

        orders = self.exchange.fetch_orders("ETH/BRL")

        self.assertEqual(len(orders), 1)
        self.assertEqual(orders[0]["id"], "ord_2")
        self.assertEqual(orders[0]["symbol"], "ETH/BRL")

        args, kwargs = mock_request.call_args
        self.assertEqual(args[0], "GET")
        self.assertIn("/accounts/acc_123/ETH-BRL/orders", args[1])
        self.assertNotIn("size", kwargs.get("params", {}))

    @patch("requests.request")
    def test_fetch_orders_api_failure(self, mock_request):
        """Verify error handling for the orders list."""
        self.exchange._account_id = "acc_123"
        self.exchange._token = "mock_token"
        self.exchange._token_expiry = 9999999999

        mock_response = MagicMock()
        mock_response.status_code = http.client.INTERNAL_SERVER_ERROR
        mock_response.text = "Internal Server Error"
        mock_request.return_value = mock_response

        with self.assertRaises(ExchangeError) as cm:
            self.exchange.fetch_orders("BTC/BRL")
        self.assertIn("MercadoBitcoin API Error: 500", str(cm.exception))

    @patch("requests.request")
    def test_fetch_open_orders_all_success(self, mock_request):
        """Verify open orders retrieval for the requested symbol."""
        self.exchange._account_id = "acc_123"
        self.exchange._token = "mock_token"
        self.exchange._token_expiry = 9999999999

        mock_response = MagicMock()
        mock_response.status_code = http.client.OK
        mock_response.json.return_value = [
            {
                "id": "ord_2",
                "status": "working",
                "instrument": "ETH-BRL",
                "qty": "1.0",
                "filledQty": "0.0",
                "created_at": 1672531200,
            }
        ]
        mock_request.return_value = mock_response

        orders = self.exchange.fetch_open_orders("ETH/BRL")

        self.assertEqual(len(orders), 1)
        self.assertEqual(orders[0]["id"], "ord_2")
        self.assertEqual(orders[0]["symbol"], "ETH/BRL")

        args, kwargs = mock_request.call_args
        self.assertEqual(args[0], "GET")
        self.assertIn("/accounts/acc_123/ETH-BRL/orders", args[1])
        self.assertEqual(kwargs["params"]["status"], "created,working")
        self.assertNotIn("size", kwargs.get("params", {}))

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


# To run this test directly, use:
#   python -m tests.exchange.test_mb_exchange
if __name__ == "__main__":
    unittest.main()
