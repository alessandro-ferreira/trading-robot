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
        # Test Limit Order (Open)
        order = self.exchange.create_order("BTC/USDT", "limit", "buy", 0.01, 20000)
        self.assertEqual(order["symbol"], "BTC/USDT")
        self.assertEqual(order["side"], "buy")
        self.assertEqual(order["type"], "limit")
        self.assertEqual(order["amount"], 0.01)
        self.assertEqual(order["price"], 20000)
        self.assertEqual(order["status"], "open")

        # Test Market Order (Closed)
        order_market = self.exchange.create_order(
            "BTC/USDT", "market", "buy", 0.01, 20000
        )
        self.assertEqual(order_market["status"], "closed")
        self.assertEqual(order_market["filled"], 0.01)
        self.assertIsNotNone(order_market["fee"])

    def test_cancel_order(self):
        # Create an order first
        order = self.exchange.create_order("BTC/USDT", "limit", "buy", 0.01, 20000)
        result = self.exchange.cancel_order(order["id"], "BTC/USDT")
        self.assertEqual(result["id"], order["id"])
        self.assertEqual(result["status"], "canceled")

    def test_fetch_order(self):
        # Create an order first
        created = self.exchange.create_order("BTC/USDT", "limit", "buy", 0.01, 20000)
        fetched = self.exchange.fetch_order(created["id"], "BTC/USDT")
        self.assertEqual(fetched["id"], created["id"])
        self.assertEqual(fetched["symbol"], "BTC/USDT")
        self.assertEqual(fetched["status"], "open")

    def test_fetch_open_orders(self):
        self.exchange.create_order("BTC/USDT", "limit", "buy", 0.01, 20000)
        open_orders = self.exchange.fetch_open_orders("BTC/USDT")
        self.assertIsInstance(open_orders, list)
        self.assertGreaterEqual(len(open_orders), 1)
        self.assertEqual(open_orders[0]["symbol"], "BTC/USDT")


# To run this test directly, use:
#   python -m tests.exchange.test_dummy_exchange
if __name__ == "__main__":
    unittest.main()
