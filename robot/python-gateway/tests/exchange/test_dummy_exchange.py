import unittest
from exchange.exchanges.dummy import DummyExchange


class TestDummyExchange(unittest.TestCase):
    def setUp(self):
        self.exchange = DummyExchange()

    def test_set_sandbox_mode(self):
        """Verify that set_sandbox_mode is accepted (no-op)."""
        # Should not raise any exception
        self.exchange.set_sandbox_mode(True)

    def test_fetch_ticker(self):
        """Verify ticker fetching and simulated price drift."""
        ticker = self.exchange.fetch_ticker("BTC/USDT")
        self.assertEqual(ticker.symbol, "BTC/USDT")
        self.assertEqual(ticker.last, 42500.50)

        # Test drift on subsequent call
        ticker2 = self.exchange.fetch_ticker("BTC/USDT")
        self.assertGreater(ticker2.last, ticker.last)
        self.assertAlmostEqual(ticker2.last, ticker.last * 1.001)

        self.assertIn("exchange", ticker.info)
        self.assertEqual(ticker.info["exchange"], "dummy")

    def test_fetch_balance(self):
        """Verify default starting balances."""
        balance = self.exchange.fetch_balance()
        self.assertIn("free", balance)
        self.assertIn("BTC", balance["free"])
        self.assertEqual(balance["free"]["BTC"], 0.5)
        self.assertIn("USDT", balance["free"])
        self.assertEqual(balance["free"]["USDT"], 10000.0)

    def test_create_order(self):
        """Verify creation of limit (open) and market (closed) orders."""
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
        """Verify that an open order can be canceled."""
        # Create an order first
        order = self.exchange.create_order("BTC/USDT", "limit", "buy", 0.01, 20000)
        result = self.exchange.cancel_order(order["id"], "BTC/USDT")
        self.assertEqual(result["id"], order["id"])
        self.assertEqual(result["status"], "canceled")

    def test_cancel_order_not_found(self):
        """Verify that canceling a non-existent order returns a canceled state via fetch_order."""
        result = self.exchange.cancel_order("non-existent", "BTC/USDT")
        self.assertEqual(result["status"], "canceled")

    def test_fetch_order(self):
        """Verify that an existing order can be fetched by ID."""
        created = self.exchange.create_order("BTC/USDT", "limit", "buy", 0.01, 20000)
        fetched = self.exchange.fetch_order(created["id"], "BTC/USDT")
        self.assertEqual(fetched["id"], created["id"])
        self.assertEqual(fetched["symbol"], "BTC/USDT")
        self.assertEqual(fetched["status"], "open")

    def test_fetch_order_not_found(self):
        """Verify non-existent order returns a simulated canceled state."""
        fetched = self.exchange.fetch_order("non-existent-id", "ETH/USDT")
        self.assertEqual(fetched["id"], "non-existent-id")
        self.assertEqual(fetched["status"], "canceled")
        self.assertEqual(fetched["filled"], 0.0)

    def test_fetch_open_orders(self):
        """Verify open orders listing and limit filtering."""
        self.exchange.create_order("BTC/USDT", "limit", "buy", 0.01, 20000)
        self.exchange.create_order("BTC/USDT", "limit", "buy", 0.01, 21000)
        self.exchange.create_order("BTC/USDT", "limit", "buy", 0.01, 22000)

        all_orders = self.exchange.fetch_open_orders("BTC/USDT")
        self.assertGreaterEqual(len(all_orders), 3)

        # Verify strict limit
        limited_orders = self.exchange.fetch_open_orders("BTC/USDT", limit=2)
        self.assertEqual(len(limited_orders), 2)

        # Test symbol filtering
        self.exchange.create_order("ETH/USDT", "limit", "buy", 1.0, 2000)
        eth_orders = self.exchange.fetch_open_orders("ETH/USDT")
        self.assertEqual(len(eth_orders), 1)
        self.assertEqual(eth_orders[0]["symbol"], "ETH/USDT")

    def test_fetch_my_trades(self):
        """Verify trade history listing and descending sort order."""
        # Market orders in dummy exchange create trades immediately
        self.exchange.create_order("BTC/USDT", "market", "buy", 0.05, 40000)
        self.exchange.create_order("BTC/USDT", "market", "buy", 0.10, 41000)
        self.exchange.create_order("BTC/USDT", "market", "buy", 0.15, 42000)

        all_trades = self.exchange.fetch_my_trades("BTC/USDT")
        self.assertGreaterEqual(len(all_trades), 3)

        # Verify strict limit and descending sort (newest first)
        limited_trades = self.exchange.fetch_my_trades("BTC/USDT", limit=2)
        self.assertEqual(len(limited_trades), 2)
        self.assertEqual(limited_trades[0]["amount"], 0.15)
        self.assertIn("order", limited_trades[0])  # Verify ID mapping


# To run this test directly, use:
#   python -m tests.exchange.test_dummy_exchange
if __name__ == "__main__":
    unittest.main()
