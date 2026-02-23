import time
from typing import Any, Dict, Optional
from .base import Exchange, Ticker


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
        self._order_id_counter = 0

    def set_sandbox_mode(self, enabled: bool):
        pass

    def fetch_ticker(self, symbol: str) -> Ticker:
        """Return a fixed ticker for any symbol."""
        # Fixed prices for testing
        fixed_prices = {
            "BTC/USDT": 42500.50,
            "ETH/USDT": 2250.75,
            "SOL/USDT": 98.30,
        }
        price = fixed_prices.get(symbol, 100.0)
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
        order_id = f"dummy-order-{self._order_id_counter}"
        timestamp = int(time.time() * 1000)

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

    def fetch_open_orders(self, symbol: Optional[str] = None) -> list:
        """Returns a list of open orders from the in-memory store."""
        open_orders = []
        for order in self._orders.values():
            if order["status"] == "open":
                if symbol is None or order["symbol"] == symbol:
                    open_orders.append(order)
        return open_orders
