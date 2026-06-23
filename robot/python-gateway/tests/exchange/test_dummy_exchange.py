import unittest
from core.config import ExchangeConfig
from exchange.exchanges.base import OrderType, ExchangeError
from exchange.exchanges.dummy import DummyExchange


class TestDummyExchange(unittest.TestCase):
    def setUp(self):
        cfg = ExchangeConfig(name="dummy", ccxt=False)
        self.exchange = DummyExchange(cfg)

    def test_set_sandbox_mode(self):
        """Verify that set_sandbox_mode is accepted (no-op)."""
        # Should not raise any exception
        self.exchange.set_sandbox_mode(True)

    def test_fetch_ticker(self):
        """Verify ticker fetching and simulated price drift."""
        ticker = self.exchange.fetch_ticker("BTC/USDT")
        self.assertEqual(ticker.symbol, "BTC/USDT")
        # dummy.py now applies 0.1% drift BEFORE returning the price
        self.assertEqual(ticker.last, 42500.50 * 1.001)

        # Test drift on subsequent call
        ticker2 = self.exchange.fetch_ticker("BTC/USDT")
        self.assertGreater(ticker2.last, ticker.last)
        self.assertAlmostEqual(ticker2.last, ticker.last * 1.001)

        self.assertIn("exchange", ticker.info)
        self.assertEqual(ticker.info["exchange"], "dummy")

    def test_fetch_balance(self):
        """Verify default starting balances (zeroed base assets)."""
        balance = self.exchange.fetch_balance()
        self.assertIn("free", balance)
        self.assertIn("BTC", balance["free"])
        self.assertEqual(balance["free"]["BTC"], 0.0)
        self.assertIn("USDT", balance["free"])
        self.assertEqual(balance["free"]["USDT"], 10000.0)

    def test_create_order(self):
        """Verify creation of limit (open) with balance locking and market (closed) with settlement."""
        # Test Limit Order (Buy BTC with USDT)
        amount = 0.01
        price = 40000
        cost = amount * price

        order = self.exchange.create_order("BTC/USDT", "limit", "buy", amount, price)
        self.assertEqual(order["symbol"], "BTC/USDT")
        self.assertEqual(order["side"], "buy")
        self.assertEqual(order["type"], "limit")
        self.assertEqual(order["amount"], amount)
        self.assertEqual(order["price"], price)
        self.assertEqual(order["status"], "open")

        # Verify balance locking
        balance = self.exchange.fetch_balance()
        self.assertEqual(balance["free"]["USDT"], 10000.0 - cost)
        self.assertEqual(balance["used"]["USDT"], cost)

        # Test Market Order (Buy BTC - Immediate settlement)
        market_amount = 0.02

        order_market = self.exchange.create_order(
            "BTC/USDT", "market", "buy", market_amount
        )
        self.assertEqual(order_market["status"], "closed")
        self.assertEqual(order_market["filled"], market_amount)

        # Verify balance swap
        final_balance = self.exchange.fetch_balance()
        self.assertEqual(final_balance["free"]["BTC"], market_amount)
        self.assertEqual(final_balance["total"]["BTC"], market_amount)

    def test_create_order_insufficient_funds(self):
        """Verify that creating an order with insufficient funds raises an exception."""
        with self.assertRaisesRegex(Exception, "Insufficient funds"):
            self.exchange.create_order("BTC/USDT", "limit", "buy", 1000.0, 42000)

    def test_create_stop_order(self):
        """Verify creation of stop_market and stop_limit orders."""
        # Test Stop Market (Open)
        order = self.exchange.create_stop_order("BTC/USDT", "sell", 0.1, 40000.0)
        self.assertEqual(order["symbol"], "BTC/USDT")
        self.assertEqual(order["side"], "sell")
        self.assertEqual(order["type"], OrderType.STOP_MARKET)
        self.assertEqual(order["price"], 40000.0)
        self.assertEqual(order["status"], "open")

        # Test Stop Limit (Open)
        order_limit = self.exchange.create_stop_order(
            "BTC/USDT", "sell", 0.1, 40000.0, 39500.0
        )
        self.assertEqual(order_limit["type"], OrderType.STOP_LIMIT)
        self.assertEqual(order_limit["price"], 39500.0)

    def test_cancel_order(self):
        """Verify that canceling an open order releases locked funds."""
        order = self.exchange.create_order("BTC/USDT", "limit", "buy", 0.01, 20000)

        # Verify funds are locked before cancellation
        balance_before = self.exchange.fetch_balance()
        self.assertEqual(balance_before["used"]["USDT"], order["cost"])
        self.assertEqual(balance_before["free"]["USDT"], 10000.0 - order["cost"])

        result = self.exchange.cancel_order(order["id"], "BTC/USDT")
        self.assertEqual(result["id"], order["id"])
        self.assertEqual(result["status"], "canceled")

        # Verify balance unlocked
        balance = self.exchange.fetch_balance()
        self.assertEqual(balance["free"]["USDT"], 10000.0)
        self.assertEqual(balance["used"]["USDT"], 0.0)

    def test_cancel_order_not_found(self):
        """Verify that canceling a non-existent order raises an error."""
        with self.assertRaises(ExchangeError):
            self.exchange.cancel_order("non-existent", "BTC/USDT")

    def test_fetch_order(self):
        """Verify that an existing order can be fetched by ID."""
        created = self.exchange.create_order("BTC/USDT", "limit", "buy", 0.01, 20000)
        fetched = self.exchange.fetch_order(created["id"], "BTC/USDT")
        self.assertEqual(fetched["id"], created["id"])
        self.assertEqual(fetched["symbol"], "BTC/USDT")
        self.assertEqual(fetched["status"], "open")

    def test_order_aging(self):
        """Verify that limit orders are automatically filled after several fetch attempts."""
        order = self.exchange.create_order("BTC/USDT", "limit", "buy", 0.1, 40000)

        # Fetch until one before aging limit: still open
        self.exchange.fetch_order(order["id"], "BTC/USDT")
        for i in range(2, self.exchange.AGING_LIMIT):
            self.assertEqual(
                self.exchange.fetch_order(order["id"], "BTC/USDT")["status"], "open"
            )

        # Reaching AGING_LIMIT: should fill
        fetched = self.exchange.fetch_order(order["id"], "BTC/USDT")
        self.assertEqual(fetched["status"], "closed")
        self.assertEqual(fetched["filled"], 0.1)

        # Verify balance settled
        balance = self.exchange.fetch_balance()
        self.assertEqual(balance["free"]["BTC"], 0.1)

    def test_open_orders_aging(self):
        """Verify that fetch_open_orders also triggers order aging."""
        self.exchange.create_order("BTC/USDT", "limit", "buy", 0.1, 40000)

        # Fetch until one before aging limit
        for i in range(1, self.exchange.AGING_LIMIT):
            self.exchange.fetch_open_orders("BTC/USDT")
        open_orders = self.exchange.fetch_open_orders("BTC/USDT")

        # Should be empty as the order filled
        self.assertEqual(len(open_orders), 0)

    def test_fetch_order_not_found(self):
        """Verify non-existent order raises an error."""
        with self.assertRaises(ExchangeError):
            self.exchange.fetch_order("non-existent-id", "ETH/USDT")

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
        self.exchange.create_order("BTC/USDT", "market", "buy", 0.01, 40000)
        self.exchange.create_order("BTC/USDT", "market", "buy", 0.02, 41000)
        self.exchange.create_order("BTC/USDT", "market", "buy", 0.03, 42000)

        all_trades = self.exchange.fetch_my_trades("BTC/USDT")
        self.assertGreaterEqual(len(all_trades), 3)

        # Verify strict limit and descending sort (newest first)
        limited_trades = self.exchange.fetch_my_trades("BTC/USDT", limit=2)
        self.assertEqual(len(limited_trades), 2)
        self.assertEqual(limited_trades[0]["amount"], 0.03)
        self.assertIn("order", limited_trades[0])  # Verify ID mapping

    def test_reset(self):
        """Verify that reset clears orders and restores initial state."""
        self.exchange.create_order("BTC/USDT", "limit", "buy", 0.1, 20000)
        self.exchange.fetch_ticker("BTC/USDT")  # Triggers price drift

        self.exchange.reset()

        self.assertEqual(len(self.exchange.fetch_open_orders()), 0)
        ticker = self.exchange.fetch_ticker("BTC/USDT")
        # dummy.py now applies 0.1% drift BEFORE returning the price
        self.assertEqual(
            ticker.last, 42500.50 * 1.001
        )  # Verify original price restored


# To run this test directly, use:
#   python -m tests.exchange.test_dummy_exchange
if __name__ == "__main__":
    unittest.main()
