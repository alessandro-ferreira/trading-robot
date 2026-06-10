import logging
from typing import Any, Dict, List, Optional

from .base import Exchange, Ticker, ExchangeError, OrderType


class CCXTExchange(Exchange):
    """
    A generic exchange implementation that relies on the CCXT library.
    This is used for any exchange supported by CCXT that doesn't require
    specific custom normalization logic.
    """

    def __init__(self, cfg=None):
        super().__init__(cfg)
        if not self._cfg or not self._cfg.ccxt:
            raise NotImplementedError("CCXTExchange requires a ccxt configuration")

    def set_sandbox_mode(self, enabled: bool):
        """Enables or disables sandbox mode if supported by the exchange."""
        if not self._ccxt:
            raise ExchangeError("Underlying ccxt exchange not available")
        try:
            self._ccxt.set_sandbox_mode(enabled)
        except Exception as e:
            raise ExchangeError(str(e))

    def fetch_ticker(self, symbol: str) -> Ticker:
        """Fetches market data for a symbol via CCXT."""
        if not self._ccxt:
            raise ExchangeError("Underlying exchange not available")

        raw = self._ccxt.fetch_ticker(symbol)
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

        return Ticker(
            symbol=symbol,
            last=float(last),
            bid=(float(raw["bid"]) if raw.get("bid") is not None else None),
            ask=(float(raw["ask"]) if raw.get("ask") is not None else None),
            timestamp=raw.get("timestamp"),
            info=raw.get("info", {}),
        )

    def fetch_balance(self) -> Dict[str, Dict[str, float]]:
        """Fetches account balances via CCXT."""
        if not self._ccxt:
            raise ExchangeError("Underlying exchange not available")
        return self._ccxt.fetch_balance()

    def create_order(
        self,
        symbol: str,
        type: str,
        side: str,
        amount: float,
        price: Optional[float] = None,
    ) -> Dict[str, Any]:
        """Creates a new order via CCXT."""
        if not self._ccxt:
            raise ExchangeError("Underlying exchange not available")
        return self._ccxt.create_order(symbol, type, side, amount, price)

    def create_stop_order(
        self,
        symbol: str,
        side: str,
        amount: float,
        stop_price: float,
        limit_price: Optional[float] = None,
    ) -> Dict[str, Any]:
        """Creates a new stop order via CCXT using unified trigger parameters."""
        if not self._ccxt:
            raise ExchangeError("Underlying exchange not available")

        # CCXT uses 'triggerPrice' as the unified alias for stop/trigger prices.
        # We provide both 'triggerPrice' and 'stopPrice' to maximize compatibility
        # across older and newer exchange implementations.
        params = {"triggerPrice": stop_price, "stopPrice": stop_price}

        # CCXT requires base types for the request; the trigger intent is handled via params.
        request_type = OrderType.LIMIT if limit_price is not None else OrderType.MARKET

        return self._ccxt.create_order(
            symbol, request_type, side, amount, limit_price, params
        )

    def cancel_order(self, id: str, symbol: Optional[str] = None) -> Dict[str, Any]:
        """Cancels an existing order via CCXT."""
        if not self._ccxt:
            raise ExchangeError("Underlying exchange not available")
        return self._ccxt.cancel_order(id, symbol)

    def fetch_order(self, id: str, symbol: Optional[str] = None) -> Dict[str, Any]:
        """Fetches order details via CCXT."""
        if not self._ccxt:
            raise ExchangeError("Underlying exchange not available")
        return self._ccxt.fetch_order(id, symbol)

    def fetch_open_orders(
        self, symbol: Optional[str] = None, limit: Optional[int] = None
    ) -> List[Dict[str, Any]]:
        """Fetches current open orders via CCXT."""
        if not self._ccxt:
            raise ExchangeError("Underlying exchange not available")
        return self._ccxt.fetch_open_orders(symbol, limit=limit)

    def fetch_my_trades(
        self,
        symbol: Optional[str] = None,
        since: Optional[int] = None,
        limit: Optional[int] = None,
    ) -> List[Dict[str, Any]]:
        """Fetches personal trade history via CCXT."""
        if not self._ccxt:
            raise ExchangeError("Underlying exchange not available")

        try:
            return self._ccxt.fetch_my_trades(symbol, since=since, limit=limit)
        except Exception as e:
            # CCXT usually requires a symbol for private trade history on many exchanges.
            if not symbol:
                logging.warning(
                    f"Exchange {self._cfg.name} does not support fetch_my_trades without symbol: {e}"
                )
                return []
            raise
