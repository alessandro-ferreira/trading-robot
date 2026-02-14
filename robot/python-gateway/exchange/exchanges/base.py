import logging
from abc import ABC
from dataclasses import dataclass
from typing import Any, Dict, List, Optional

import ccxt

from core.config import ExchangeConfig


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


class OrderType:
    MARKET = "market"
    LIMIT = "limit"


class OrderSide:
    BUY = "buy"
    SELL = "sell"


class Exchange(ABC):
    """
    Abstract base class for exchange implementations.
    """

    def __init__(self, cfg: ExchangeConfig = None):
        self._cfg = cfg
        self._ccxt = None
        # Try to instantiate ccxt client if possible
        try:
            if cfg and cfg.ccxt:
                if cfg.name and cfg.name in ccxt.exchanges:
                    ccxt_cls = getattr(ccxt, cfg.name)
                    # Prepare credentials and options if present
                    params = {}
                    if cfg.api_key is not None:
                        params["apiKey"] = cfg.api_key
                        logging.debug(f"API key provided for {cfg.name}")
                    if cfg.secret is not None:
                        params["secret"] = cfg.secret
                        logging.debug(f"Secret provided for {cfg.name}")
                    # Add any other config fields as needed
                    self._ccxt = ccxt_cls(params)
                    logging.debug(
                        f"Initialized ccxt exchange {cfg.name} with params: {params}"
                    )
                else:
                    logging.warning(
                        f"Exchange '{cfg.name}' configured with ccxt=True but not found in ccxt library"
                    )

            elif cfg and cfg.name:
                logging.debug(
                    f"Exchange '{cfg.name}' initialized without ccxt (native implementation)"
                )
            else:
                logging.warning("No exchange name provided in config")

        except Exception as e:
            logging.error(
                f"Error initializing exchange '{cfg.name if cfg else "unknown"}': {e}"
            )
            raise

    def set_sandbox_mode(self, enabled: bool):
        """
        Enables or disables sandbox mode if supported by the exchange.

        :param enabled: True to enable sandbox mode, False to disable.
        """
        if not self._cfg or not self._cfg.ccxt:
            raise NotImplementedError("set_sandbox_mode not implemented")
        if not self._ccxt:
            raise ExchangeError(
                "Underlying ccxt exchange not available to set sandbox mode"
            )

        try:
            self._ccxt.set_sandbox_mode(enabled)
        except Exception as e:
            raise ExchangeError(str(e))

    def fetch_ticker(self, symbol: str) -> Ticker:
        """
        Fetches the ticker for the given symbol.

        :param symbol: The symbol to fetch (e.g., 'BTC/USDT').
        :return: A Ticker object containing market data.
        """
        if not self._cfg or not self._cfg.ccxt:
            raise NotImplementedError("fetch_ticker not implemented")
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
        return Ticker(
            symbol=symbol,
            last=last_f,
            bid=(float(bid) if bid is not None else None),
            ask=(float(ask) if ask is not None else None),
            timestamp=timestamp,
            info=raw.get("info", {}),
        )

    def fetch_balance(self) -> Dict[str, Dict[str, float]]:
        """
        Fetches the account balance.

        :return: A dictionary containing 'free', 'used', and 'total' balances.
        """
        if not self._cfg or not self._cfg.ccxt:
            raise NotImplementedError("fetch_balance not implemented")
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
        """
        Creates a new order.

        :param symbol: The symbol to trade (e.g., 'BTC/USDT').
        :param type: The order type ('market' or 'limit').
        :param side: The order side ('buy' or 'sell').
        :param amount: The amount of base currency to trade.
        :param price: The price per unit (required for limit orders).
        :return: A dictionary containing the order details.
        """
        if not self._cfg or not self._cfg.ccxt:
            raise NotImplementedError("create_order not implemented")
        if not self._ccxt:
            raise ExchangeError("Underlying exchange not available")

        return self._ccxt.create_order(symbol, type, side, amount, price)

    def cancel_order(self, id: str, symbol: Optional[str] = None) -> Dict[str, Any]:
        """
        Cancels an existing order.

        :param id: The order ID.
        :param symbol: The symbol of the order (optional but recommended).
        :return: A dictionary containing the cancellation details.
        """
        if not self._cfg or not self._cfg.ccxt:
            raise NotImplementedError("cancel_order not implemented")
        if not self._ccxt:
            raise ExchangeError("Underlying exchange not available")

        return self._ccxt.cancel_order(id, symbol)

    def fetch_order(self, id: str, symbol: Optional[str] = None) -> Dict[str, Any]:
        """
        Fetches an existing order.

        :param id: The order ID.
        :param symbol: The symbol of the order (optional but recommended).
        :return: A dictionary containing the order details.
        """
        if not self._cfg or not self._cfg.ccxt:
            raise NotImplementedError("fetch_order not implemented")
        if not self._ccxt:
            raise ExchangeError("Underlying exchange not available")

        return self._ccxt.fetch_order(id, symbol)

    def fetch_open_orders(self, symbol: Optional[str] = None) -> List[Dict[str, Any]]:
        """
        Fetches open orders for the given symbol.

        :param symbol: The symbol to filter by (optional).
        :return: A list of open orders.
        """
        if not self._cfg or not self._cfg.ccxt:
            raise NotImplementedError("fetch_open_orders not implemented")
        if not self._ccxt:
            raise ExchangeError("Underlying exchange not available")

        return self._ccxt.fetch_open_orders(symbol)
