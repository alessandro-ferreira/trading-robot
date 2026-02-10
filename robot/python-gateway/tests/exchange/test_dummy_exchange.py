import unittest
from exchange.exchanges.dummy import DummyExchange

class TestDummyExchange(unittest.TestCase):
    def setUp(self):
        self.exchange = DummyExchange()

    def test_fetch_ticker(self):
        ticker = self.exchange.fetch_ticker("BTC/USDT")
        self.assertEqual(ticker.symbol, "BTC/USDT")
        self.assertEqual(ticker.last, 42500.50)
        self.assertIn("exchange", ticker.info)
        self.assertEqual(ticker.info["exchange"], "dummy")

    def test_fetch_balance(self):
        balance = self.exchange.fetch_balance()
        self.assertIn("free", balance)
        self.assertIn("BTC", balance["free"])
        self.assertEqual(balance["free"]["BTC"], 0.5)
        self.assertIn("USDT", balance["free"])
        self.assertEqual(balance["free"]["USDT"], 10000.0)

    def test_create_order(self):
        order = self.exchange.create_order("BTC/USDT", "limit", "buy", 0.01, 20000)
        self.assertEqual(order["symbol"], "BTC/USDT")
        self.assertEqual(order["side"], "buy")
        self.assertEqual(order["type"], "limit")
        self.assertEqual(order["amount"], 0.01)
        self.assertEqual(order["price"], 20000)
        self.assertEqual(order["status"], "closed")

    def test_cancel_order(self):
        result = self.exchange.cancel_order("order-id-123", "BTC/USDT")
        self.assertEqual(result["id"], "order-id-123")
        self.assertEqual(result["status"], "canceled")

    def test_fetch_order(self):
        order = self.exchange.fetch_order("order-id-123", "BTC/USDT")
        self.assertEqual(order["id"], "order-id-123")
        self.assertEqual(order["symbol"], "BTC/USDT")
        self.assertEqual(order["status"], "closed")

    def test_fetch_open_orders(self):
        open_orders = self.exchange.fetch_open_orders("BTC/USDT")
        self.assertIsInstance(open_orders, list)
        self.assertGreaterEqual(len(open_orders), 1)
        self.assertEqual(open_orders[0]["symbol"], "BTC/USDT")

# To run this test directly, use:
#   python -m tests.exchange.test_dummy_exchange
if __name__ == "__main__":
    unittest.main()
