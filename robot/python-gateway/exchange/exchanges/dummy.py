from typing import Any, Dict, Optional
from .base import Exchange, Ticker


class DummyExchange(Exchange):
    """A dummy exchange that returns fixed values for testing without API credentials."""

    def set_sandbox_mode(self, enabled: bool):
        # No-op for dummy exchange
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
            timestamp=1707000000000,
            info={"exchange": "dummy", "quote": "USDT"},
        )

    def fetch_balance(self) -> Dict[str, Dict[str, float]]:
        """Return a fixed balance for testing."""
        return {
            "free": {"USDT": 10000.0, "BTC": 0.5, "ETH": 10.0},
            "used": {"USDT": 0.0, "BTC": 1.0, "ETH": 0.0},
            "total": {"USDT": 10000.0, "BTC": 1.5, "ETH": 10.0},
        }

    def create_order(
        self,
        symbol: str,
        type: str,
        side: str,
        amount: float,
        price: Optional[float] = None,
    ) -> Dict[str, Any]:
        """Return a mock order response."""
        cost = (price or 100.0) * amount
        return {
            "id": "dummy-order-20260203001",
            "clientOrderId": "dummy-client-20260203001",
            "timestamp": 1707000000000,
            "datetime": "2026-02-03T17:00:00Z",
            "lastTradeTimestamp": 1707000000000,
            "symbol": symbol,
            "type": type,
            "side": side,
            "price": price or 100.0,
            "amount": amount,
            "cost": cost,
            "average": price or 100.0,
            "filled": amount,
            "remaining": 0.0,
            "status": "closed",
            "fee": {"cost": cost * 0.001, "currency": "USDT", "rate": 0.001},
            "trades": [
                {
                    "id": "dummy-trade-20260203001",
                    "timestamp": 1707000000000,
                    "datetime": "2026-02-03T17:00:00Z",
                    "symbol": symbol,
                    "type": type,
                    "side": side,
                    "price": price or 100.0,
                    "amount": amount,
                    "cost": cost,
                    "fee": {"cost": cost * 0.001, "currency": "USDT", "rate": 0.001},
                }
            ],
            "info": {"status": "filled"},
        }

    def cancel_order(self, id: str, symbol: Optional[str] = None) -> Dict[str, Any]:
        """Return a mock cancel order response."""
        return {
            "id": id,
            "clientOrderId": "dummy-client-canceled",
            "timestamp": 1707000000000,
            "datetime": "2026-02-03T17:00:00Z",
            "lastTradeTimestamp": 1707000000000,
            "symbol": symbol,
            "type": "limit",
            "side": "buy",
            "price": 100.0,
            "amount": 1.0,
            "cost": 100.0,
            "average": 100.0,
            "filled": 0.0,
            "remaining": 1.0,
            "status": "canceled",
            "fee": {"cost": 0.0, "currency": "USDT", "rate": 0.001},
            "trades": [],
            "info": {"status": "cancelled_by_user"},
        }

    def fetch_order(self, id: str, symbol: Optional[str] = None) -> Dict[str, Any]:
        """Return a mock order."""
        return {
            "id": id,
            "clientOrderId": "dummy-client-20260203002",
            "timestamp": 1707000000000,
            "datetime": "2026-02-03T17:00:00Z",
            "lastTradeTimestamp": 1707000000000,
            "symbol": symbol,
            "type": "limit",
            "side": "buy",
            "price": 42500.0,
            "amount": 0.5,
            "cost": 21250.0,
            "average": 42500.0,
            "filled": 0.5,
            "remaining": 0.0,
            "status": "closed",
            "fee": {"cost": 21.25, "currency": "USDT", "rate": 0.001},
            "trades": [
                {
                    "id": "dummy-trade-20260203002",
                    "timestamp": 1707000000000,
                    "datetime": "2026-02-03T17:00:00Z",
                    "symbol": symbol,
                    "type": "limit",
                    "side": "buy",
                    "price": 42500.0,
                    "amount": 0.5,
                    "cost": 21250.0,
                    "fee": {"cost": 21.25, "currency": "USDT", "rate": 0.001},
                }
            ],
            "info": {"status": "filled"},
        }

    def fetch_open_orders(self, symbol: Optional[str] = None) -> list:
        """Return a list of mock open orders."""
        return [
            {
                "id": "dummy-open-order-1",
                "clientOrderId": "dummy-client-open-1",
                "timestamp": 1706999000000,
                "datetime": "2026-02-03T16:43:20Z",
                "lastTradeTimestamp": None,
                "symbol": symbol or "BTC/USDT",
                "type": "limit",
                "side": "buy",
                "price": 41500.0,
                "amount": 0.2,
                "cost": 8300.0,
                "average": None,
                "filled": 0.0,
                "remaining": 0.2,
                "status": "open",
                "fee": None,
                "trades": [],
                "info": {"status": "open"},
            },
            {
                "id": "dummy-open-order-2",
                "clientOrderId": "dummy-client-open-2",
                "timestamp": 1706999000000,
                "datetime": "2026-02-03T16:43:20Z",
                "lastTradeTimestamp": None,
                "symbol": symbol or "ETH/USDT",
                "type": "limit",
                "side": "sell",
                "price": 2300.0,
                "amount": 1.0,
                "cost": 2300.0,
                "average": None,
                "filled": 0.0,
                "remaining": 1.0,
                "status": "open",
                "fee": None,
                "trades": [],
                "info": {"status": "open"},
            },
        ]
