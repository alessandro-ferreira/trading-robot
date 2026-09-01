import http.client
import logging
import requests
import threading
import time

from datetime import datetime, timezone
from decimal import Decimal, ROUND_DOWN
from typing import Any, Dict, List, Optional

from .base import (
    Exchange,
    ExchangeError,
    ExchangeNetworkError,
    AuthenticationError,
    BadRequestError,
    Ticker,
    OrderType,
)


class MercadoBitcoinExchange(Exchange):
    """
    MercadoBitcoin implementation using native API v4 instead of ccxt.
    https://api.mercadobitcoin.net/api/v4/docs
    """

    BASE_URL = "https://api.mercadobitcoin.net/api/v4"

    PATH_OAUTH_TOKEN = "/authorize"
    PATH_ACCOUNTS = "/accounts"
    PATH_ACCOUNT_BALANCES = "/accounts/{}/balances"
    PATH_TICKERS = "/tickers"
    PATH_PLACE_ORDER = "/accounts/{}/{}/orders"
    PATH_CANCEL_ORDER = "/accounts/{}/{}/orders/{}"
    PATH_GET_ORDER = "/accounts/{}/{}/orders/{}"
    PATH_GET_ORDERS = "/accounts/{}/{}/orders"

    TIMEOUT_SECONDS = 10
    STOP_MARKET_SLIPPAGE = 0.02
    # Quantity precision from MB v4 /symbols output for SUPPORTED_ASSETS grouped by round-lot precision.
    QUANTITY_DECIMALS_BY_ASSET = {
        0: {"SHIB"},
        3: {"ALGO", "ARB", "DOGE", "HBAR", "OP", "XLM"},
        4: {"ADA", "APT", "DOT", "NEAR", "SUI", "TRX", "USDT", "XRP"},
        5: {"AVAX", "LINK", "UNI"},
        6: {"AAVE", "HYPE", "LTC", "SOL"},
        7: {"BCH", "TAO", "ZEC"},
        8: {"BTC", "ETH"},
    }

    def __init__(self, cfg=None):
        super().__init__(cfg)
        self._account_id: Optional[str] = None
        self._token: Optional[str] = None
        self._token_expiry: float = 0
        self._auth_lock = threading.RLock()

    def _authenticate(self):
        """
        Authenticates using the API key and secret to obtain a Bearer token.
        """
        with self._auth_lock:
            if not self._cfg or not self._cfg.secret or not self._cfg.api_key:
                raise ExchangeError(
                    "API key and Secret are required for MercadoBitcoin private API"
                )

            url = f"{self.BASE_URL}{self.PATH_OAUTH_TOKEN}"
            payload = {"login": self._cfg.api_key, "password": self._cfg.secret}

            timeout = self.TIMEOUT_SECONDS
            if self._cfg and self._cfg.timeout:
                timeout = self._cfg.timeout

            try:
                response = requests.post(url, json=payload, timeout=timeout)

                if response.status_code != http.client.OK:
                    raise ExchangeError(
                        f"Authentication failed: {response.status_code} - {response.text}"
                    )

                data = response.json()

                self._token = data.get("access_token")
                # 'expiration' is already in seconds since epoch; we subtract 60s for a safety buffer.
                raw_exp = data.get("expiration")
                if raw_exp:
                    self._token_expiry = float(raw_exp) - 60
                else:
                    self._token_expiry = time.time() + 1800 - 60
            except ExchangeError:
                raise
            except Exception as e:
                raise ExchangeError(f"Authentication failed: {e}")

    def _request(
        self, method: str, path: str, data: Optional[Dict[str, Any]] = None
    ) -> Any:
        with self._auth_lock:
            if not self._token or time.time() >= self._token_expiry:
                self._authenticate()
            request_token = self._token

        url = f"{self.BASE_URL}{path}"
        timeout = self.TIMEOUT_SECONDS
        if self._cfg and self._cfg.timeout:
            timeout = self._cfg.timeout
        headers = {"Authorization": f"Bearer {self._token}"}

        if method == "GET":
            try:
                response = requests.request(
                    method,
                    url,
                    headers=headers,
                    params=data,
                    timeout=timeout,
                )
            except requests.exceptions.RequestException as e:
                # GET requests are safe to retry
                raise ExchangeNetworkError(f"Network error during GET: {e}")
        else:
            try:
                response = requests.request(
                    method,
                    url,
                    headers=headers,
                    json=data,
                    timeout=timeout,
                )
            except requests.exceptions.RequestException as e:
                # Non-GET requests should NOT be automatically retried
                raise ExchangeError(f"Request failed during {method}: {e}")

        if response.status_code == http.client.NO_CONTENT:
            return {}

        if response.status_code not in [http.client.OK, http.client.CREATED]:
            if response.status_code == http.client.UNAUTHORIZED:
                # If we get a 401, clear the token and to force re-authentication on the next request.
                with self._auth_lock:
                    if self._token == request_token:
                        self._token = None
                        self._token_expiry = 0
            self._handle_http_errors(response)

        return response.json()

    def _handle_http_errors(self, response: requests.Response):
        """Maps HTTP status codes to specific ExchangeError subclasses."""
        error_msg = (
            f"MercadoBitcoin API Error: {response.status_code} - {response.text}"
        )

        if response.status_code in (http.client.UNAUTHORIZED, http.client.FORBIDDEN):
            raise AuthenticationError(error_msg)

        if response.status_code == http.client.BAD_REQUEST:
            raise BadRequestError(error_msg)

        raise ExchangeError(error_msg)

    def _get_account_id(self) -> str:
        """
        Fetches and caches the account ID.

        :return: The account ID string.
        """
        if self._account_id is None:
            try:
                data = self._request("GET", self.PATH_ACCOUNTS)
                # EAFP: Try to access the first element and its 'id' key.
                self._account_id = data[0]["id"]
            except Exception:
                raise ExchangeError("Failed to retrieve account ID from MercadoBitcoin")

        return self._account_id

    def _get_base_quote(self, symbol: str) -> tuple[str, str]:
        """Returns the base and quote assets from a bot symbol."""
        parts = symbol.split("/")
        if len(parts) != 2 or not all(parts):
            raise ExchangeError(f"Invalid symbol format for MercadoBitcoin: {symbol}")
        return parts[0].upper(), parts[1].upper()

    def _normalize_symbol(self, symbol: str) -> str:
        """
        Converts a symbol like 'BTC/BRL' to 'BTC-BRL'.

        :param symbol: The symbol to normalize.
        :return: The normalized symbol string.
        """
        base, quote = self._get_base_quote(symbol)
        return f"{base}-{quote}"

    def _map_status(self, mb_status: Optional[str]) -> str:
        """Maps Mercado Bitcoin status to standard CCXT terms."""
        if not mb_status:
            return ""
        # MB v4 uses mixed casing (uppercase in POST, lowercase in GET).
        s = mb_status.lower()
        if s in ("created", "working"):
            return "open"
        if s == "filled":
            return "closed"
        if s == "cancelled":
            return "canceled"
        return s

    def _map_type(self, mb_type: Optional[str]) -> str:
        """Maps Mercado Bitcoin types to standard constants."""
        if not mb_type:
            return ""
        t = mb_type.lower()
        # Mercado Bitcoin uses 'stoplimit' for both stop-limit and simulated
        # stop-market orders. We map it to 'stop_market' as requested to
        # align with the bot's internal database constants.
        if t == "stoplimit":
            return OrderType.STOP_MARKET
        return t

    def _calculate_fees(self, response: Dict[str, Any], symbol: str) -> Dict[str, Any]:
        """Aggregates fees from executions and determines currency for Mercado Bitcoin."""
        executions = response.get("executions", [])
        raw_fee = response.get("fee")
        if not raw_fee and isinstance(response.get("info"), dict):
            raw_fee = response["info"].get("fee")
        total_fee = float(raw_fee or 0.0)
        if total_fee == 0.0:
            total_fee = sum(float(e.get("fee") or 0.0) for e in executions)

        # Determine currency based on side.
        # MB v4 typically: Buy fees in base asset, Sell fees in quote asset (BRL).
        side = response.get("side")
        fee_currency = ""
        if symbol and "/" in symbol:
            base, quote = self._get_base_quote(symbol)
            fee_currency = base if side == "buy" else quote
        elif not symbol and response.get("instrument"):
            _, quote = self._get_base_quote(response["instrument"].replace("-", "/", 1))
            fee_currency = quote

        return {"fee": total_fee, "fee_currency": fee_currency}

    def _format_quantity(self, amount: float, asset_or_symbol: str) -> str:
        """Formats an order or balance quantity using the asset's supported precision."""
        base_asset = asset_or_symbol.split("/", 1)[0].upper()
        quantity_decimals = {
            asset: decimals
            for decimals, assets in self.QUANTITY_DECIMALS_BY_ASSET.items()
            for asset in assets
        }
        if base_asset not in quantity_decimals:
            return str(amount)
        decimals = quantity_decimals[base_asset]

        quantum = Decimal("1").scaleb(-decimals)
        quantity = Decimal(str(amount)).quantize(quantum, rounding=ROUND_DOWN)
        return format(quantity, f".{decimals}f")

    def _map_order(self, symbol: str, response: Dict[str, Any]) -> Dict[str, Any]:
        """Maps a Mercado Bitcoin order response to a standardized order format."""
        # Map status to standard ccxt terms
        status = self._map_status(response.get("status"))
        fees = self._calculate_fees(response, symbol)

        timestamp = (
            int(response.get("created_at")) * 1000
            if response.get("created_at")
            else None
        )
        dt = (
            datetime.fromtimestamp(timestamp / 1000, timezone.utc).isoformat()
            if timestamp
            else None
        )

        return {
            "id": response.get("id"),
            "clientOrderId": response.get("externalId")
            or response.get("clientOrderId"),
            "symbol": symbol,
            "type": self._map_type(response.get("type")),
            "side": response.get("side"),
            "price": float(
                response.get("stopPrice") or response.get("limitPrice") or 0.0
            ),
            "average": float(response.get("avgPrice"))
            if response.get("avgPrice")
            else None,
            "amount": float(response.get("qty") or 0.0),
            "filled": float(response.get("filledQty") or 0.0),
            "remaining": (
                float(response.get("qty") or 0.0)
                - float(response.get("filledQty") or 0.0)
            ),
            "cost": float(response.get("cost"))
            if response.get("cost") is not None
            else None,
            "fee": fees["fee"],
            "fee_currency": fees["fee_currency"],
            "status": status,
            "timestamp": timestamp,
            "datetime": dt,
            "info": response,
        }

    def fetch_ticker(self, symbol: str) -> Ticker:
        """
        Fetches the ticker for a given symbol using the public API.

        :param symbol: The symbol to fetch (e.g., 'BTC/BRL').
        :return: A Ticker object.
        """
        pair = self._normalize_symbol(symbol)
        data = self._request("GET", self.PATH_TICKERS, data={"symbols": pair})

        ticker_data = data[0]

        return Ticker(
            symbol=symbol,
            last=float(ticker_data["last"]),
            bid=float(ticker_data["buy"]) if ticker_data.get("buy") else None,
            ask=float(ticker_data["sell"]) if ticker_data.get("sell") else None,
            timestamp=int(int(ticker_data["date"]) / 1000000),  # Convert ns to ms
            info=ticker_data,
        )

    def fetch_balance(self) -> Dict[str, Dict[str, float]]:
        """
        Fetches account balances.

        :return: A dictionary containing 'free', 'used', and 'total' balances.
        """
        account_id = self._get_account_id()
        path = self.PATH_ACCOUNT_BALANCES.format(account_id)
        balances = self._request("GET", path)

        result = {"free": {}, "used": {}, "total": {}}

        for b in balances:
            currency = b["symbol"].upper()
            available = float(self._format_quantity(b["available"], currency))
            used = float(self._format_quantity(b["on_hold"], currency))
            total = float(self._format_quantity(b["total"], currency))

            result["free"][currency] = available
            result["used"][currency] = used
            result["total"][currency] = total

        return result

    def create_order(
        self,
        symbol: str,
        type: str,
        side: str,
        amount: float,
        client_order_id: str,
        price: Optional[float] = None,
    ) -> Dict[str, Any]:
        """
        Creates a new order.

        :param symbol: Instrument symbol (e.g. BTC/BRL).
        :param type: 'market' or 'limit'.
        :param side: 'buy' or 'sell'.
        :param amount: Amount of base currency.
        :param client_order_id: The client order ID.
        :param price: Price per unit (required for limit orders).
        :return: A dictionary containing the order details.
        """
        if type == OrderType.LIMIT and price is None:
            raise ExchangeError("Price is required for limit orders")

        account_id = self._get_account_id()
        pair = self._normalize_symbol(symbol)
        path = self.PATH_PLACE_ORDER.format(account_id, pair)
        qty_str = self._format_quantity(amount, symbol)
        if Decimal(qty_str) <= 0:
            raise ExchangeError(
                f"Order quantity is too small for MercadoBitcoin: {amount}"
            )

        payload = {
            "qty": qty_str,
            "side": side,
            "type": type,
            "externalId": client_order_id,
        }

        if type == OrderType.LIMIT:
            payload["limitPrice"] = float(price)

        logging.info(f"Creating order with payload: {payload}")

        response = self._request("POST", path, data=payload)
        status = self._map_status(response.get("status"))
        fees = self._calculate_fees(response, symbol)

        return {
            "id": response.get("orderId"),
            "clientOrderId": response.get("clientOrderId")
            or response.get("externalId"),
            "symbol": symbol,
            "type": self._map_type(type),
            "side": side,
            "amount": amount,
            "price": price,
            "filled": amount if status == "closed" else 0.0,
            "remaining": 0.0 if status == "closed" else amount,
            "status": status,
            "fee": fees["fee"],
            "fee_currency": fees["fee_currency"],
            "info": response,
        }

    def create_stop_order(
        self,
        symbol: str,
        side: str,
        amount: float,
        client_order_id: str,
        stop_price: float,
        limit_price: Optional[float] = None,
    ) -> Dict[str, Any]:
        """
        Creates a new stop order on Mercado Bitcoin v4.
        Note: MB v4 only supports 'stoplimit'. To simulate stop-market,
        we use stopPrice as the trigger and ensure a limitPrice is provided.
        """
        account_id = self._get_account_id()
        pair = self._normalize_symbol(symbol)
        path = self.PATH_PLACE_ORDER.format(account_id, pair)

        # MB V4 API literal is 'stoplimit'
        mb_type = "stoplimit"
        qty_str = self._format_quantity(amount, symbol)
        if Decimal(qty_str) <= 0:
            raise ExchangeError(
                f"Order quantity is too small for MercadoBitcoin: {amount}"
            )

        if limit_price and limit_price > 0:
            effective_limit = float(limit_price)
        else:
            # Since MercadoBitcoin does not support stop-market orders, we define
            # the limit price based on the stop price and a small slippage factor.
            if side == "sell":
                effective_limit = float(stop_price) * (1.0 - self.STOP_MARKET_SLIPPAGE)
            else:
                effective_limit = float(stop_price) * (1.0 + self.STOP_MARKET_SLIPPAGE)

        formatted_stop = float(f"{float(stop_price):.2f}")
        formatted_limit = float(f"{effective_limit:.2f}")
        if formatted_stop <= 0 or formatted_limit <= 0:
            raise ExchangeError(
                "Stop and limit prices must be positive for MercadoBitcoin"
            )
        if side == "sell" and formatted_limit >= formatted_stop:
            raise ExchangeError("Sell stop-limit price must be below stop price")
        if side == "buy" and formatted_limit <= formatted_stop:
            raise ExchangeError("Buy stop-limit price must be above stop price")

        payload = {
            "qty": qty_str,
            "side": side,
            "type": mb_type,
            "stopPrice": float(f"{stop_price:.2f}"),
            "limitPrice": float(f"{effective_limit:.2f}"),
            "externalId": client_order_id,
        }

        logging.info(f"Creating stop order with payload: {payload}")

        response = self._request("POST", path, data=payload)
        status = self._map_status(response.get("status"))
        fees = self._calculate_fees(response, symbol)

        return {
            "id": response.get("orderId"),
            "clientOrderId": response.get("clientOrderId")
            or response.get("externalId"),
            "symbol": symbol,
            "type": self._map_type(mb_type),
            "side": side,
            "amount": amount,
            "price": float(stop_price),
            "filled": amount if status == "closed" else 0.0,
            "remaining": 0.0 if status == "closed" else amount,
            "status": status,
            "fee": fees["fee"],
            "fee_currency": fees["fee_currency"],
            "info": response,
        }

    def cancel_order(self, id: str, symbol: Optional[str] = None) -> Dict[str, Any]:
        """
        Cancels an existing order.

        :param id: The order ID.
        :param symbol: The symbol of the order (required for Mercado Bitcoin).
        :return: A dictionary containing the cancellation details.
        """
        if not symbol:
            raise ExchangeError(
                "Symbol is required to cancel an order on Mercado Bitcoin"
            )

        account_id = self._get_account_id()
        pair = self._normalize_symbol(symbol)
        path = self.PATH_CANCEL_ORDER.format(account_id, pair, id)

        # Use async=false to wait for cancellation confirmation
        path += "?async=false"

        response = self._request("DELETE", path)

        return {"id": id, "symbol": symbol, "status": "canceled", "info": response}

    def fetch_order(self, id: str, symbol: Optional[str] = None) -> Dict[str, Any]:
        """
        Fetches an existing order.

        :param id: The order ID.
        :param symbol: The symbol of the order (required for Mercado Bitcoin).
        :return: A dictionary containing the order details.
        """
        if not symbol:
            raise ExchangeError(
                "Symbol is required to fetch an order on Mercado Bitcoin"
            )

        account_id = self._get_account_id()
        pair = self._normalize_symbol(symbol)
        path = self.PATH_GET_ORDER.format(account_id, pair, id)

        response = self._request("GET", path)

        return self._map_order(symbol, response)

    def fetch_orders(self, symbol: str) -> List[Dict[str, Any]]:
        """
        Fetches a list of orders for the given symbol.

        :param symbol: The symbol to filter by.
        :return: A list of open orders.
        """
        return self._fetch_orders(symbol)

    def fetch_open_orders(self, symbol: str) -> List[Dict[str, Any]]:
        """
        Fetches a list of open orders for the given symbol.

        :param symbol: The symbol to filter by.
        :return: A list of open orders.
        """
        return self._fetch_orders(symbol, params={"status": "created,working"})

    def _fetch_orders(
        self, symbol: str, params: Optional[Dict[str, Any]] = None
    ) -> List[Dict[str, Any]]:
        """
        Fetches orders for the given symbol, optionally also filtered by params.

        :param symbol: The symbol to filter by.
        :param params: Additional parameters for filtering (optional).
        :return: A list of filtered orders.
        """
        account_id = self._get_account_id()
        params = dict(params or {})

        pair = self._normalize_symbol(symbol)
        # Use the market-specific endpoint for higher rate limits (10 req/s).
        # Note: MB v4 documentation does not list 'size' as a parameter for this path.
        path = self.PATH_GET_ORDERS.format(account_id, pair)
        response = self._request("GET", path, data=params)

        result = []
        for order in response:
            result.append(self._map_order(symbol, order))

        return result
