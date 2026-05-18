import time
from typing import Any, Dict, List, Optional
from .base import Exchange, Ticker, OrderType


class DummyExchange(Exchange):
    """
    A stateful dummy exchange that simulates order creation and management for testing.
    """

    def __init__(self, cfg=None):
        super().__init__(cfg)
        self.reset()

    def reset(self):
        """Resets the in-memory state of the exchange."""
        self._orders: Dict[str, Dict[str, Any]] = {}
        self._balances = {
            "free": {"USDT": 10000.0, "BTC": 0.5, "ETH": 10.0},
            "used": {"USDT": 0.0, "BTC": 0.0, "ETH": 0.0},
            "total": {"USDT": 10000.0, "BTC": 0.5, "ETH": 10.0},
        }
        self._prices = {
            "BTC/USDT": 42500.50,
            "ETH/USDT": 2250.75,
            "SOL/USDT": 98.30,
        }
        self._order_id_counter = 0

    def set_sandbox_mode(self, enabled: bool):
        pass

    def fetch_ticker(self, symbol: str) -> Ticker:
        """Return a fixed ticker for any symbol."""
        price = self._prices.get(symbol, 100.0)

        # Simulate an upward drift (0.1%) for the next fetch.
        # This allows momentum strategies to eventually trigger signals in integration tests.
        self._prices[symbol] = price * 1.001

        return Ticker(
            symbol=symbol,
            last=price,
            bid=price * 0.9999,
            ask=price * 1.0001,
            timestamp=int(time.time() * 1000),
            info={"exchange": "dummy", "quote": "USDT"},
        )

    def fetch_balance(self) -> Dict[str, Dict[str, float]]:
        """Return the current in-memory balance."""
        return self._balances

    def create_order(
        self,
        symbol: str,
        type: str,
        side: str,
        amount: float,
        price: Optional[float] = None,
    ) -> Dict[str, Any]:
        """Simulates creating an order and stores it in memory."""
        self._order_id_counter += 1
        # Use millisecond timestamp + counter to ensure unique, strictly increasing timestamps
        # even when called in rapid succession during tests.
        timestamp = int(time.time() * 1000) + self._order_id_counter
        order_id = f"dummy-{timestamp}-{self._order_id_counter}"

        # For simplicity, limit orders are 'open', market orders are 'closed'
        status = "open" if type == "limit" else "closed"
        filled = amount if status == "closed" else 0.0
        remaining = 0.0 if status == "closed" else amount
        cost = (price or 0) * filled

        fee = None
        trades = []
        if status == "closed":
            fee_cost = cost * 0.001
            fee = {"cost": fee_cost, "currency": "USDT", "rate": 0.001}
            trades = [
                {
                    "id": f"dummy-trade-{self._order_id_counter}",
                    "timestamp": timestamp,
                    "datetime": time.strftime(
                        "%Y-%m-%dT%H:%M:%SZ", time.gmtime(timestamp / 1000)
                    ),
                    "symbol": symbol,
                    "type": type,
                    "side": side,
                    "price": price,
                    "amount": amount,
                    "cost": cost,
                    "fee": fee,
                    "order": order_id,
                }
            ]

        order = {
            "id": order_id,
            "clientOrderId": f"dummy-client-{self._order_id_counter}",
            "timestamp": timestamp,
            "datetime": time.strftime(
                "%Y-%m-%dT%H:%M:%SZ", time.gmtime(timestamp / 1000)
            ),
            "lastTradeTimestamp": timestamp if status == "closed" else None,
            "symbol": symbol,
            "type": type,
            "side": side,
            "price": price,
            "amount": amount,
            "cost": cost,
            "average": price if status == "closed" else None,
            "filled": filled,
            "remaining": remaining,
            "status": status,
            "fee": fee,
            "trades": trades,
            "info": {"status": status},
        }
        self._orders[order_id] = order
        return order

    def create_stop_order(
        self,
        symbol: str,
        side: str,
        amount: float,
        stop_price: float,
        limit_price: Optional[float] = None,
    ) -> Dict[str, Any]:
        """Simulates creating a stop order and stores it in memory."""
        self._order_id_counter += 1
        timestamp = int(time.time() * 1000) + self._order_id_counter
        order_id = f"dummy-stop-{timestamp}-{self._order_id_counter}"

        order_type = OrderType.STOP_LIMIT if limit_price else OrderType.STOP_MARKET
        status = "open"

        order = {
            "id": order_id,
            "clientOrderId": f"dummy-client-{self._order_id_counter}",
            "timestamp": timestamp,
            "datetime": time.strftime(
                "%Y-%m-%dT%H:%M:%SZ", time.gmtime(timestamp / 1000)
            ),
            "lastTradeTimestamp": None,
            "symbol": symbol,
            "type": order_type,
            "side": side,
            "price": limit_price or stop_price,
            "amount": amount,
            "cost": 0.0,
            "average": None,
            "filled": 0.0,
            "remaining": amount,
            "status": status,
            "fee": None,
            "trades": [],
            "info": {"status": status, "stopPrice": stop_price},
        }
        self._orders[order_id] = order
        return order

    def cancel_order(self, id: str, symbol: Optional[str] = None) -> Dict[str, Any]:
        """Simulates canceling an order by updating its status."""
        order = self._orders.get(id)
        if order and order["status"] == "open":
            order["status"] = "canceled"
            # No partial fills in this simple simulation
            order["remaining"] = order["amount"] - order["filled"]
            self._orders[id] = order
            return order

        # If order not found or not open, fetch it to return a consistent state
        return self.fetch_order(id, symbol)

    def fetch_order(self, id: str, symbol: Optional[str] = None) -> Dict[str, Any]:
        """Fetches an order from the in-memory store."""
        order = self._orders.get(id)
        if not order:
            # To handle cancel->get flow, if not found, return a canceled state
            # This simulates an order that was already canceled and removed or never existed
            return {
                "id": id,
                "symbol": symbol,
                "status": "canceled",
                "filled": 0.0,
                "remaining": 0.0,
                "cost": 0.0,
                "timestamp": int(time.time() * 1000),
                "info": {"error": "Order not found"},
            }
        return order

    def fetch_open_orders(
        self, symbol: Optional[str] = None, limit: Optional[int] = None
    ) -> list:
        """
        Returns a list of open orders from the in-memory store.
        """
        open_orders = []
        for order in self._orders.values():
            if order["status"] == "open":
                if symbol is None or order["symbol"] == symbol:
                    open_orders.append(order)

        return open_orders[:limit] if limit else open_orders

    def fetch_my_trades(
        self,
        symbol: Optional[str] = None,
        since: Optional[int] = None,
        limit: Optional[int] = None,
    ) -> List[Dict[str, Any]]:
        """
        Returns a list of trades from the in-memory store.
        """
        trades = []
        # In DummyExchange, trades are stored within the simulated order object.
        for order in self._orders.values():
            for t in order.get("trades", []):
                if symbol is None or t["symbol"] == symbol:
                    trades.append(t)

        # Sort trades by timestamp descending (newest first)
        trades.sort(key=lambda x: x["timestamp"], reverse=True)

        return trades[:limit] if limit else trades
