from datetime import datetime, timezone
import time
from typing import Any, Dict, List, Optional
from .base import Exchange, ExchangeError, InsufficientFundsError, Ticker, OrderType


class DummyExchange(Exchange):
    """
    A stateful dummy exchange that simulates order creation and management for testing.
    """

    AGING_LIMIT = 5

    def __init__(self, cfg=None):
        super().__init__(cfg)
        self.reset()

    def reset(self):
        """Resets the in-memory state of the exchange."""
        self._orders: Dict[str, Dict[str, Any]] = {}
        self._balances = {
            "free": {"USDT": 10000.0, "BTC": 0.0, "ETH": 0.0, "LTC": 0.0, "SOL": 0.0},
            "used": {"USDT": 0.0, "BTC": 0.0, "ETH": 0.0, "LTC": 0.0, "SOL": 0.0},
            "total": {"USDT": 10000.0, "BTC": 0.0, "ETH": 0.0, "LTC": 0.0, "SOL": 0.0},
        }
        self._prices = {
            "BTC/USDT": 42500.50,
            "ETH/USDT": 2250.75,
            "LTC/USDT": 48.20,
            "SOL/USDT": 98.30,
        }
        self._order_id_counter = 0

    def _update_total(self, asset: str):
        """Syncs the total balance for an asset."""
        free = self._balances["free"].get(asset, 0.0)
        used = self._balances["used"].get(asset, 0.0)
        self._balances["total"][asset] = free + used

    def _get_datetime(self, timestamp_ms: int) -> str:
        """Converts a millisecond timestamp to an ISO8601 string."""
        return datetime.fromtimestamp(timestamp_ms / 1000, timezone.utc).isoformat()

    def _execute_trade(self, order_id: str):
        """Internal helper to settle an order and swap balances."""
        order = self._orders.get(order_id)
        if not order or order["status"] != "open":
            return

        symbol = order["symbol"]
        side = order["side"]
        amount = order["amount"]
        price = (
            order["price"] if order.get("price") else self._prices.get(symbol, 100.0)
        )
        base, quote = symbol.split("/")
        cost = amount * price

        # Transition order state
        order["status"] = "closed"
        order["filled"] = amount
        order["remaining"] = 0.0
        order["average"] = price
        order["cost"] = cost

        # Update Balances: Remove from 'used' (locked) or 'free' and add to 'received'
        # Regular orders lock funds in 'used' during create_order.
        # Stop orders (created via create_stop_order) do not lock funds until triggered.
        is_stop = order["type"] in (OrderType.STOP_MARKET, OrderType.STOP_LIMIT)

        if side == "buy":
            if is_stop:
                self._balances["free"][quote] -= order["cost"]
            else:
                self._balances["used"][quote] -= order["cost"]
            self._balances["free"][base] = (
                self._balances["free"].get(base, 0.0) + amount
            )
            self._update_total(quote)
            self._update_total(base)
        else:
            if is_stop:
                self._balances["free"][base] -= amount
            else:
                self._balances["used"][base] -= amount
            self._balances["free"][quote] = (
                self._balances["free"].get(quote, 0.0) + cost
            )
            self._update_total(base)
            self._update_total(quote)

        # Add execution details for fetch_my_trades
        trade_id = f"trade-{order_id}"
        # Use the parent order's timestamp (which includes a monotonic offset)
        # to ensure stable sorting in high-frequency test environments.
        ts = order["timestamp"]
        trade = {
            "id": trade_id,
            "order": order_id,
            "timestamp": ts,
            "datetime": self._get_datetime(ts),
            "symbol": symbol,
            "side": side,
            "price": price,
            "amount": amount,
            "filled": amount,
            "cost": cost,
            "fee": order.get("fee", 0.0),
            "fee_currency": order.get("fee_currency", ""),
            "info": {},
        }
        order["trades"].append(trade)

    def set_sandbox_mode(self, enabled: bool):
        pass

    def fetch_ticker(self, symbol: str) -> Ticker:
        """Return a fixed ticker for any symbol."""
        price = self._prices.get(symbol, 100.0)

        # Aging-Based Drift:
        # 1. Identify open stop orders for this symbol.
        # 2. Increment their age.
        # 3. If any stop order is "mature" (age >= AGING_LIMIT), flip the trend DOWN.
        # 4. Otherwise, keep drifting UP to allow Happy Path profit targets to hit first.

        stop_orders = [
            o
            for o in self._orders.values()
            if o["status"] == "open"
            and o["symbol"] == symbol
            and o["type"] in (OrderType.STOP_MARKET, OrderType.STOP_LIMIT)
        ]

        drift = 1 + (0.01 * 0.01)
        for o in stop_orders:
            o["_age"] += 1
            if o["_age"] >= self.AGING_LIMIT:
                drift = 1 - (0.5 * 0.01)

        self._prices[symbol] = price * drift
        current_price = self._prices[symbol]

        # Check for triggered stop orders
        for oid, o in list(self._orders.items()):
            if o["status"] == "open" and o["symbol"] == symbol:
                if o["type"] in (OrderType.STOP_MARKET, OrderType.STOP_LIMIT):
                    trigger = o["stop_price"]
                    if (o["side"] == "buy" and current_price >= trigger) or (
                        o["side"] == "sell" and current_price <= trigger
                    ):
                        self._execute_trade(oid)

        return Ticker(
            symbol=symbol,
            last=current_price,
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
        """Simulates creating an order with balance validation and fund locking."""
        base, quote = symbol.split("/")
        exec_price = price if price else self._prices.get(symbol, 100.0)
        cost = amount * exec_price

        # Balance Check & Locking
        if side == "buy":
            if self._balances["free"].get(quote, 0.0) < cost:
                raise InsufficientFundsError(f"Insufficient funds: {quote}")
            self._balances["free"][quote] -= cost
            self._balances["used"][quote] = (
                self._balances["used"].get(quote, 0.0) + cost
            )
            self._update_total(quote)
        else:
            if self._balances["free"].get(base, 0.0) < amount:
                raise InsufficientFundsError(f"Insufficient funds: {base}")
            self._balances["free"][base] -= amount
            self._balances["used"][base] = (
                self._balances["used"].get(base, 0.0) + amount
            )
            self._update_total(base)

        self._order_id_counter += 1
        # Use millisecond timestamp + counter to ensure unique, strictly increasing timestamps
        # even when called in rapid succession during tests.
        timestamp = int(time.time() * 1000) + self._order_id_counter
        order_id = f"dummy-{timestamp}-{self._order_id_counter}"

        order = {
            "id": order_id,
            "clientOrderId": f"dummy-client-{timestamp}-{self._order_id_counter}",
            "timestamp": timestamp,
            "datetime": self._get_datetime(timestamp),
            "symbol": symbol,
            "type": type,
            "side": side,
            "price": price,
            "amount": amount,
            "cost": cost,
            "filled": 0.0,
            "remaining": amount,
            "status": "open",
            "fee": 0.0,
            "fee_currency": quote if side == "buy" else base,
            "average": 0.0,
            "trades": [],
            "_age": 0,
            "info": {},
        }
        self._orders[order_id] = order

        # Market orders fill immediately
        if type == "market":
            self._execute_trade(order_id)

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
        base, quote = symbol.split("/")
        # Stop orders typically don't lock funds until triggered
        self._order_id_counter += 1
        timestamp = int(time.time() * 1000) + self._order_id_counter
        order_id = f"dummy-stop-{timestamp}-{self._order_id_counter}"

        order_type = OrderType.STOP_LIMIT if limit_price else OrderType.STOP_MARKET

        order = {
            "id": order_id,
            "clientOrderId": f"dummy-client-{timestamp}-{self._order_id_counter}",
            "timestamp": timestamp,
            "datetime": self._get_datetime(timestamp),
            "symbol": symbol,
            "type": order_type,
            "side": side,
            "price": limit_price or stop_price,
            "stop_price": stop_price,
            "amount": amount,
            "cost": 0.0,
            "filled": 0.0,
            "remaining": amount,
            "status": "open",
            "fee": 0.0,
            "fee_currency": quote if side == "buy" else base,
            "average": 0.0,
            "trades": [],
            "_age": 0,
            "info": {},
        }
        self._orders[order_id] = order
        return order

    def cancel_order(self, id: str, symbol: Optional[str] = None) -> Dict[str, Any]:
        """Simulates canceling an order by updating its status."""
        order = self._orders.get(id)
        if order and order["status"] == "open":
            # Release locked funds (stop orders don't lock funds here)
            if order["type"] not in (OrderType.STOP_MARKET, OrderType.STOP_LIMIT):
                base, quote = order["symbol"].split("/")
                if order["side"] == "buy":
                    self._balances["used"][quote] -= order["cost"]
                    self._balances["free"][quote] += order["cost"]
                    self._update_total(quote)
                else:
                    self._balances["used"][base] -= order["amount"]
                    self._balances["free"][base] += order["amount"]
                    self._update_total(base)

            order["status"] = "canceled"
            order["remaining"] = order["amount"] - order["filled"]
            self._orders[id] = order
            return order

        # If order not found or not open, fetch it to return a consistent state
        return self.fetch_order(id, symbol)

    def fetch_order(self, id: str, symbol: Optional[str] = None) -> Dict[str, Any]:
        """Fetches an order from the in-memory store."""
        order = self._orders.get(id)

        # Simulate aging: fill limit/market orders after reaching the AGING_LIMIT.
        # Stop orders do not fill based on age; they remain open until triggered or canceled.
        if (
            order
            and order["status"] == "open"
            and order["type"] not in (OrderType.STOP_MARKET, OrderType.STOP_LIMIT)
        ):
            order["_age"] += 1
            if order["_age"] >= self.AGING_LIMIT:
                self._execute_trade(id)

        if not order:
            raise ExchangeError(f"Order not found: {id}")

        return order

    def fetch_open_orders(
        self, symbol: Optional[str] = None, limit: Optional[int] = None
    ) -> list:
        """Returns a list of open orders from the in-memory store."""
        open_orders = []
        for order in self._orders.values():
            if order["status"] == "open":
                # Also apply aging when listing open orders, excluding stop orders.
                if order["type"] not in (OrderType.STOP_MARKET, OrderType.STOP_LIMIT):
                    order["_age"] += 1
                    if order["_age"] >= self.AGING_LIMIT:
                        self._execute_trade(order["id"])
                        continue

                if symbol is None or order["symbol"] == symbol:
                    open_orders.append(order)

        return open_orders[:limit] if limit else open_orders

    def fetch_my_trades(
        self,
        symbol: Optional[str] = None,
        since: Optional[int] = None,
        limit: Optional[int] = None,
    ) -> List[Dict[str, Any]]:
        """Returns a list of trades from the in-memory store."""
        trades = []
        # In DummyExchange, trades are stored within the simulated order object.
        for order in self._orders.values():
            for t in order.get("trades", []):
                if symbol is None or t["symbol"] == symbol:
                    trades.append(t)

        # Sort trades by timestamp descending (newest first)
        trades.sort(key=lambda x: x["timestamp"], reverse=True)

        return trades[:limit] if limit else trades
