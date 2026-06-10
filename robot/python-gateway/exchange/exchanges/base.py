import logging
from abc import ABC
from dataclasses import dataclass
from typing import Any, Dict, List, Optional

import ccxt

from core.config import ExchangeConfig


class ExchangeError(Exception):
    """Base class for all exceptions raised by exchange connectors."""

    pass


class ExchangeNetworkError(ExchangeError):
    """Raised for retryable network errors, typically during GET requests."""

    pass


class AuthenticationError(ExchangeError):
    """Raised when the exchange returns 401 or 403."""

    pass


class InsufficientFundsError(ExchangeError):
    """Raised when an order fails due to lack of balance."""

    pass


class BadRequestError(ExchangeError):
    """Raised for 400 errors like invalid symbols or prices."""

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
    STOP_MARKET = "stop_market"
    STOP_LIMIT = "stop_limit"


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

                    if cfg and cfg.timeout is not None:
                        # CCXT timeout is defined in milliseconds
                        params["timeout"] = int(cfg.timeout * 1000)

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
        """Enables or disables sandbox mode if supported by the exchange."""
        raise NotImplementedError("set_sandbox_mode not implemented for this exchange")

    def fetch_ticker(self, symbol: str) -> Ticker:
        """
        Fetches the ticker for the given symbol.
        :param symbol: The symbol to fetch (e.g., 'BTC/USDT').
        :return: A Ticker object containing market data.
        """
        raise NotImplementedError("fetch_ticker not implemented for this exchange")

    def fetch_balance(self) -> Dict[str, Dict[str, float]]:
        """
        Fetches the account balance.
        :return: A dictionary containing 'free', 'used', and 'total' balances.
        """
        raise NotImplementedError("fetch_balance not implemented for this exchange")

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
        raise NotImplementedError("create_order not implemented for this exchange")

    def create_stop_order(
        self,
        symbol: str,
        side: str,
        amount: float,
        stop_price: float,
        limit_price: Optional[float] = None,
    ) -> Dict[str, Any]:
        """
        Creates a new stop order (market or limit trigger).
        :param stop_price: The price that triggers the order.
        :param limit_price: The execution price (optional, makes it a stop-limit).
        """
        raise NotImplementedError("create_stop_order not implemented for this exchange")

    def cancel_order(self, id: str, symbol: Optional[str] = None) -> Dict[str, Any]:
        """
        Cancels an existing order.
        :param id: The order ID.
        :param symbol: The symbol of the order (optional but recommended).
        :return: A dictionary containing the cancellation details.
        """
        raise NotImplementedError("cancel_order not implemented for this exchange")

    def fetch_order(self, id: str, symbol: Optional[str] = None) -> Dict[str, Any]:
        """
        Fetches an existing order's details.
        :param id: The order ID.
        :param symbol: The symbol of the order (optional but recommended).
        :return: A dictionary containing the order details.
        """
        raise NotImplementedError("fetch_order not implemented for this exchange")

    def fetch_open_orders(
        self, symbol: Optional[str] = None, limit: Optional[int] = None
    ) -> List[Dict[str, Any]]:
        """
        Fetches open orders for the given symbol.
        :param symbol: The symbol to filter by (optional).
        :param limit: The maximum number of orders to fetch (optional).
        :return: A list of open orders.
        """
        raise NotImplementedError("fetch_open_orders not implemented for this exchange")

    def fetch_my_trades(
        self,
        symbol: Optional[str] = None,
        since: Optional[int] = None,
        limit: Optional[int] = None,
    ) -> List[Dict[str, Any]]:
        """
        Fetches the user's trades (executions) for the given symbol.
        :param symbol: The symbol to filter by (optional).
        :param since: Millisecond timestamp for pagination (optional).
        :param limit: The maximum number of trades to fetch (optional).
        :return: A list of trade details.
        """
        raise NotImplementedError("fetch_my_trades not implemented for this exchange")
