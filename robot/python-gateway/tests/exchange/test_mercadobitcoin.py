import http.client
import os
import unittest
from unittest.mock import MagicMock, patch

from core import config
from exchange.exchanges.base import ExchangeError, OrderType
from exchange.exchanges.mercadobitcoin import MercadoBitcoinExchange

TEST_DATA_DIR = "tests/exchange/testdata"


class TestMercadoBitcoinExchange(unittest.TestCase):
    def setUp(self):
        cfg = config.load(os.path.join(TEST_DATA_DIR, "config.toml"))
        self.cfg = next(ex for ex in cfg.exchanges if ex.name == "mercadobitcoin")
        self.exchange = MercadoBitcoinExchange(self.cfg)

    @patch("requests.post")
    def test_authenticate_success(self, mock_post):
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
        # Mock failed authentication
        mock_response = MagicMock()
        mock_response.status_code = http.client.UNAUTHORIZED
        mock_response.text = "Unauthorized"
        mock_post.return_value = mock_response

        with self.assertRaises(ExchangeError):
            self.exchange._authenticate()

    @patch("requests.get")
    def test_fetch_ticker_success(self, mock_get):
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
        mock_get.return_value = mock_response

        ticker = self.exchange.fetch_ticker("BTC/BRL")

        self.assertEqual(ticker.symbol, "BTC/BRL")
        self.assertEqual(ticker.last, 195000.0)
        self.assertEqual(ticker.bid, 194900.0)
        self.assertEqual(ticker.ask, 195100.0)
        # Timestamp converted to ms: 1672531200000000 / 1000 = 1672531200000
        self.assertEqual(ticker.timestamp, 1672531200000)

    @patch("requests.get")
    def test_fetch_ticker_failure(self, mock_get):
        mock_response = MagicMock()
        mock_response.status_code = http.client.NOT_FOUND
        mock_response.text = "Not Found"
        mock_get.return_value = mock_response

        with self.assertRaises(ExchangeError):
            self.exchange.fetch_ticker("INVALID/PAIR")

    @patch("requests.request")
    @patch("requests.post")  # For authentication
    def test_fetch_balance_success(self, mock_post, mock_request):
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

    def test_create_order_missing_price_for_limit(self):
        with self.assertRaises(ExchangeError) as cm:
            self.exchange.create_order("BTC/BRL", OrderType.LIMIT, "buy", 0.1)
        self.assertIn("Price is required for limit orders", str(cm.exception))

    @patch("requests.request")
    def test_create_order_api_failure(self, mock_request):
        self.exchange._account_id = "acc_123"
        self.exchange._token = "mock_token"
        self.exchange._token_expiry = 9999999999

        mock_response = MagicMock()
        mock_response.status_code = http.client.BAD_REQUEST
        mock_response.text = "Invalid quantity"
        mock_request.return_value = mock_response

        with self.assertRaises(ExchangeError) as cm:
            self.exchange.create_order("BTC/BRL", OrderType.MARKET, "buy", 0.1)
        self.assertIn("MercadoBitcoin API Error: 400", str(cm.exception))

    @patch("requests.request")
    def test_cancel_order_success(self, mock_request):
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

    def test_cancel_order_missing_symbol(self):
        with self.assertRaises(ExchangeError) as cm:
            self.exchange.cancel_order("ord_123")
        self.assertIn("Symbol is required", str(cm.exception))

    @patch("requests.request")
    def test_cancel_order_api_failure(self, mock_request):
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
        with self.assertRaises(ExchangeError) as cm:
            self.exchange.fetch_order("ord_123")
        self.assertIn("Symbol is required", str(cm.exception))

    @patch("requests.request")
    def test_fetch_order_api_failure(self, mock_request):
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

        trades = self.exchange.fetch_my_trades("BTC/BRL", limit=5)

        self.assertEqual(len(trades), 1)
        self.assertEqual(trades[0]["id"], "exec_1")
        self.assertEqual(trades[0]["order"], "parent_ord")
        self.assertEqual(trades[0]["amount"], 0.05)

        args, kwargs = mock_request.call_args
        self.assertEqual(kwargs["params"]["has_executions"], "true")
        # Verify 'size' is NOT passed for symbol-specific path as per MB doc
        self.assertNotIn("size", kwargs.get("params", {}))


# To run this test directly, use:
#   python -m tests.exchange.test_mercadobitcoin
if __name__ == "__main__":
    unittest.main()
