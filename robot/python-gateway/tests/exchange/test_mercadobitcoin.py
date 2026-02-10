import http.client
import os
import unittest
from unittest.mock import MagicMock, patch

from core import config
from exchange.exchanges.base import ExchangeError
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
            "expiration": 1800
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
        mock_response.json.return_value = [{
            "pair": "BTC-BRL",
            "high": "200000.00000000",
            "low": "190000.00000000",
            "vol": "50.00000000",
            "last": "195000.00000000",
            "buy": "194900.00000000",
            "sell": "195100.00000000",
            "open": "192000.00000000",
            "date": 1672531200000000000  # Nanoseconds
        }]
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
            {"symbol": "BRL", "available": "1000.0", "on_hold": "0.0", "total": "1000.0"},
            {"symbol": "BTC", "available": "0.5", "on_hold": "0.1", "total": "0.6"}
        ]

        # Configure side_effect to return accounts then balances
        mock_request.side_effect = [accounts_resp, balances_resp]

        balance = self.exchange.fetch_balance()

        self.assertEqual(balance['free']['BRL'], 1000.0)
        self.assertEqual(balance['total']['BTC'], 0.6)
        self.assertEqual(balance['used']['BTC'], 0.1)

# To run this test directly, use:
#   python -m tests.exchange.test_mercadobitcoin
if __name__ == "__main__":
    unittest.main()
