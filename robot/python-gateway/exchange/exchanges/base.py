import ccxt

from abc import ABC
from dataclasses import dataclass
from typing import Any, Dict, Optional

class ExchangeError(Exception):
    pass

@dataclass
class Ticker:
    symbol: str
    last: float
    bid: Optional[float] = None
    ask: Optional[float] = None
    timestamp: Optional[int] = None
    info: Dict[str, Any] = None

class Exchange(ABC):
    """
    Abstract base class for exchange implementations.
    """

    def __init__(self, cfg=None):
        self._cfg = cfg
        self._ccxt = None
        # Try to instantiate ccxt client if possible
        try:
            if cfg and hasattr(cfg, 'name') and hasattr(ccxt, cfg.name):
                ccxt_cls = getattr(ccxt, cfg.name)
                # Prepare credentials and options if present
                params = {}
                if hasattr(cfg, 'apiKey') and cfg.apiKey:
                    params['apiKey'] = cfg.apiKey
                if hasattr(cfg, 'secret') and cfg.secret:
                    params['secret'] = cfg.secret
                if hasattr(cfg, 'password') and cfg.password:
                    params['password'] = cfg.password
                # Add any other config fields as needed
                self._ccxt = ccxt_cls(params)
        except ImportError:
            pass  # ccxt not installed, ignore
        except Exception:
            pass  # Not a ccxt-supported exchange or bad config, ignore

    def set_sandbox_mode(self, enabled: bool):
        """Enables or disables sandbox mode if supported by the exchange."""
        if not self._ccxt:
            raise ExchangeError("Underlying ccxt exchange not available to set sandbox mode")
        try:
            self._ccxt.set_sandbox_mode(enabled)
        except Exception as e:
            raise ExchangeError(str(e))

    def fetch_ticker(self, symbol: str) -> Ticker:
        """Fetches the ticker for the given symbol."""
        if not self._ccxt:
            raise ExchangeError("Underlying exchange not available to fetch ticker")
        raw = self._ccxt.fetch_ticker(symbol)
        # normalization: prefer last -> close -> derived from info
        last = None
        for key in ("last", "close"):
            if key in raw and raw[key] is not None:
                last = raw[key]
                break
        if last is None and isinstance(raw.get("info"), dict):
            info = raw["info"]
            for fk in ("price", "last", "close"):
                if fk in info:
                    last = info[fk]
                    break
        if last is None:
            raise ExchangeError(f"No price available in ticker for {symbol}")
        try:
            last_f = float(last)
        except Exception as e:
            raise ExchangeError(f"Invalid price format for {symbol}: {e}")
        bid = raw.get("bid")
        ask = raw.get("ask")
        timestamp = raw.get("timestamp")
        return Ticker(symbol=symbol, last=last_f, bid=(float(bid) if bid is not None else None), ask=(float(ask) if ask is not None else None), timestamp=timestamp, info=raw.get("info", {}))

    def fetch_balance(self) -> Dict[str, Dict[str, float]]:
        """Fetches the account balance."""
        if not self._ccxt:
            raise ExchangeError("Underlying exchange not available")
        return self._ccxt.fetch_balance()

    def create_order(self, symbol: str, type: str, side: str, amount: float, price: Optional[float] = None) -> Dict[str, Any]:
        """Creates a new order."""
        if not self._ccxt:
            raise ExchangeError("Underlying exchange not available")
        return self._ccxt.create_order(symbol, type, side, amount, price)

    def cancel_order(self, id: str, symbol: Optional[str] = None) -> Dict[str, Any]:
        """Cancels an existing order."""
        if not self._ccxt:
            raise ExchangeError("Underlying exchange not available")
        return self._ccxt.cancel_order(id, symbol)

    def fetch_order(self, id: str, symbol: Optional[str] = None) -> Dict[str, Any]:
        """Fetches an existing order."""
        if not self._ccxt:
            raise ExchangeError("Underlying exchange not available")
        return self._ccxt.fetch_order(id, symbol)

    def fetch_open_orders(self, symbol: Optional[str] = None) -> list:
        """Fetches open orders for the given symbol."""
        if not self._ccxt:
            raise ExchangeError("Underlying exchange not available")
        return self._ccxt.fetch_open_orders(symbol)