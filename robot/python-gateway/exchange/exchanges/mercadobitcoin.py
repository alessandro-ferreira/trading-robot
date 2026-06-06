from datetime import datetime, timezone
import http.client
import logging
import time
from typing import Any, Dict, List, Optional

import requests

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
    PATH_ORDERS_SYMBOL = "/accounts/{}/{}/orders"
    PATH_ORDERS_ALL = "/accounts/{}/orders"

    TIMEOUT_SECONDS = 10

    def __init__(self, cfg=None):
        super().__init__(cfg)
        self._account_id: Optional[str] = None
        self._token: Optional[str] = None
        self._token_expiry: float = 0

    def _authenticate(self):
        """
        Authenticates using the API key and secret to obtain a Bearer token.
        """
        if not self._cfg or not self._cfg.secret or not self._cfg.api_key:
            raise ExchangeError(
                "API key and Secret are required for MercadoBitcoin private API"
            )

        url = f"{self.BASE_URL}{self.PATH_OAUTH_TOKEN}"
        payload = {"login": self._cfg.api_key, "password": self._cfg.secret}

        try:
            response = requests.post(url, json=payload, timeout=self.TIMEOUT_SECONDS)

            if response.status_code != http.client.OK:
                raise ExchangeError(
                    f"Authentication failed: {response.status_code} - {response.text}"
                )

            data = response.json()

            self._token = data.get("access_token")
            # Expiration is in seconds (e.g., 1800). Add buffer.
            self._token_expiry = time.time() + int(data.get("expiration", 1800)) - 60
        except ExchangeError:
            raise
        except Exception as e:
            raise ExchangeError(f"Authentication failed: {e}")

    def _request(
        self, method: str, path: str, data: Optional[Dict[str, Any]] = None
    ) -> Any:
        if not self._token or time.time() >= self._token_expiry:
            self._authenticate()

        url = f"{self.BASE_URL}{path}"
        # Let requests handle JSON serialization by using the `json` parameter.
        headers = {"Authorization": f"Bearer {self._token}"}

        if method == "GET":
            try:
                response = requests.request(
                    method,
                    url,
                    headers=headers,
                    params=data,
                    timeout=self.TIMEOUT_SECONDS,
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
                    timeout=self.TIMEOUT_SECONDS,
                )
            except requests.exceptions.RequestException as e:
                # Non-GET requests should NOT be automatically retried
                raise ExchangeError(f"Request failed during {method}: {e}")

        if response.status_code == http.client.NO_CONTENT:
            return {}

        if response.status_code not in [http.client.OK, http.client.CREATED]:
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

    def _normalize_symbol(self, symbol: str) -> str:
        """
        Converts a symbol like 'BTC/BRL' to 'BTC-BRL'.

        :param symbol: The symbol to normalize.
        :return: The normalized symbol string.
        """
        parts = symbol.split("/")
        if len(parts) != 2:
            raise ExchangeError(f"Invalid symbol format for MercadoBitcoin: {symbol}")
        return f"{parts[0]}-{parts[1]}"

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
        total_fee = sum(float(e.get("fee") or 0.0) for e in executions)

        # Determine currency based on side.
        # MB v4 typically: Buy fees in base asset, Sell fees in quote asset (BRL).
        side = response.get("side")
        fee_currency = ""
        if symbol and "/" in symbol:
            base, quote = symbol.split("/")
            fee_currency = base if side == "buy" else quote
        elif not symbol and response.get("instrument"):
            fee_currency = response.get("instrument").split("-")[1]

        return {"fee": total_fee, "fee_currency": fee_currency}

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
            available = float(b["available"])
            used = float(b["on_hold"])
            total = float(b["total"])

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
        price: Optional[float] = None,
    ) -> Dict[str, Any]:
        """
        Creates a new order.

        :param symbol: Instrument symbol (e.g. BTC/BRL).
        :param type: 'market' or 'limit'.
        :param side: 'buy' or 'sell'.
        :param amount: Amount of base currency.
        :param price: Price per unit (required for limit orders).
        :return: A dictionary containing the order details.
        """
        if type == OrderType.LIMIT and price is None:
            raise ExchangeError("Price is required for limit orders")

        account_id = self._get_account_id()
        pair = self._normalize_symbol(symbol)
        path = self.PATH_PLACE_ORDER.format(account_id, pair)

        # Format amount to string to avoid scientific notation and ensure precision
        qty_str = "{:.8f}".format(amount).rstrip("0").rstrip(".")

        payload = {
            "qty": qty_str,
            "side": side,
            "type": type,
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
        qty_str = "{:.8f}".format(amount).rstrip("0").rstrip(".")

        # If the bot requested a Market Stop (limit_price=0/None), we must
        # still provide a limitPrice for the 'stoplimit' type.
        if limit_price and limit_price > 0:
            effective_limit = float(limit_price)
        else:
            # To simulate a stop-market on an exchange that only supports stop-limit,
            # we use a significant slippage margin (40%) to ensure execution.
            slippage_percentage = 0.40
            if side == "sell":
                effective_limit = float(stop_price) * (1.0 - slippage_percentage)
            else:
                effective_limit = float(stop_price) * (1.0 + slippage_percentage)

            # Rounding to the nearest integer to satisfy tick size requirements for BRL pairs.
            effective_limit = round(effective_limit)

        payload = {
            "qty": qty_str,
            "side": side,
            "type": mb_type,
            "stopPrice": float(stop_price),
            "limitPrice": effective_limit,
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
            "clientOrderId": response.get("clientOrderId")
            or response.get("externalId"),
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

    def fetch_open_orders(
        self, symbol: Optional[str] = None, limit: Optional[int] = None
    ) -> List[Dict[str, Any]]:
        """
        Fetches open orders for the given symbol.

        :param symbol: The symbol to filter by (optional).
        :param limit: The maximum number of orders to fetch (optional).
        :return: A list of open orders.
        """
        account_id = self._get_account_id()
        params = {"status": "created,working"}

        if symbol:
            pair = self._normalize_symbol(symbol)
            # Use the market-specific endpoint for higher rate limits (10 req/s).
            # Note: MB v4 documentation does not list 'size' as a parameter for this path.
            path = self.PATH_ORDERS_SYMBOL.format(account_id, pair)
            response = self._request("GET", path, data=params)
            # Endpoint 1 returns a list directly
            orders_data = response
        else:
            # The account-wide endpoint (3 req/s) supports 'size' for limiting results.
            if limit:
                params["size"] = limit
            # Use the all orders endpoint
            path = self.PATH_ORDERS_ALL.format(account_id)
            response = self._request("GET", path, data=params)
            # Endpoint 2 returns a dict with 'items'
            orders_data = response.get("items", [])

        result = []
        for order in orders_data:
            # Map status
            status = self._map_status(order.get("status"))

            timestamp = (
                int(order.get("created_at")) * 1000 if order.get("created_at") else None
            )
            dt = (
                datetime.fromtimestamp(timestamp / 1000, timezone.utc).isoformat()
                if timestamp
                else None
            )

            # Handle field discrepancies
            order_id = order.get("id")
            client_order_id = order.get("clientOrderId") or order.get("externalId")
            # If symbol was not provided, it should be in the order object as 'instrument'
            order_symbol = (
                symbol
                if symbol
                else (
                    order.get("instrument", "").replace("-", "/")
                    if order.get("instrument")
                    else None
                )
            )

            price = float(order.get("stopPrice") or order.get("limitPrice") or 0.0)
            avg_price = float(order.get("avgPrice")) if order.get("avgPrice") else None
            amount = float(order.get("qty") or 0.0)
            filled = float(order.get("filledQty") or 0.0)
            remaining = amount - filled
            cost = float(order.get("cost")) if order.get("cost") is not None else None
            fees = self._calculate_fees(order, order_symbol or "")

            result.append(
                {
                    "id": order_id,
                    "clientOrderId": client_order_id,
                    "symbol": order_symbol,
                    "type": self._map_type(order.get("type")),
                    "side": order.get("side"),
                    "price": price,
                    "average": avg_price,
                    "amount": amount,
                    "filled": filled,
                    "remaining": remaining,
                    "cost": cost,
                    "fee": fees["fee"],
                    "fee_currency": fees["fee_currency"],
                    "status": status,
                    "timestamp": timestamp,
                    "datetime": dt,
                    "info": order,
                }
            )

        return result[:limit] if limit else result

    def fetch_my_trades(
        self,
        symbol: Optional[str] = None,
        since: Optional[int] = None,
        limit: Optional[int] = None,
    ) -> List[Dict[str, Any]]:
        """
        Fetches the user's trades (executions) by listing orders with executions.
        Since there is no dedicated executions endpoint in MB v4, we extract
        nested executions from the orders list.

        :param symbol: The symbol to filter by (optional).
        :param since: Millisecond timestamp for pagination (optional).
        :param limit: The maximum number of trades to fetch (optional).
        :return: A list of trade details mapped to standard format.
        """
        account_id = self._get_account_id()
        # Filter for orders that have associated executions
        params = {"has_executions": "true"}
        if since:
            # Map CCXT millisecond 'since' to MB v4 'created_at_from' (seconds)
            params["created_at_from"] = int(since / 1000)

        if symbol:
            pair = self._normalize_symbol(symbol)
            # Use market-specific endpoint for better performance.
            # Server-side 'size' (limit) is not supported on this path per documentation.
            path = self.PATH_ORDERS_SYMBOL.format(account_id, pair)
            # List orders from specific market returns an Array directly.
            orders_data = self._request("GET", path, data=params)
        else:
            # Account-wide endpoint supports 'size' for server-side limiting.
            if limit:
                params["size"] = limit
            path = self.PATH_ORDERS_ALL.format(account_id)
            response = self._request("GET", path, data=params)
            # List all orders returns a dictionary containing 'items'.
            orders_data = response.get("items", [])

        result = []
        for order in orders_data:
            # In the orders list view (with has_executions=true), executions are
            # returned as a nested array within each order object.
            executions = order.get("executions", [])
            for ex in executions:
                # MB v4 uses Unix seconds for timestamps.
                ts_raw = ex.get("executed_at") or ex.get("created_at")
                timestamp = int(ts_raw) * 1000 if ts_raw else None
                dt = (
                    datetime.fromtimestamp(timestamp / 1000, timezone.utc).isoformat()
                    if timestamp
                    else None
                )

                trade_symbol = (
                    symbol if symbol else ex.get("instrument", "").replace("-", "/")
                )
                # For individual executions, MB provides the fee directly.
                fee_val = float(ex.get("fee") or 0.0)
                base, quote = (
                    trade_symbol.split("/") if "/" in trade_symbol else ("", "BRL")
                )
                fee_curr = base if ex.get("side") == "buy" else quote

                result.append(
                    {
                        "id": str(ex.get("id")),
                        "order": str(order.get("id")),  # Link back to parent order
                        "symbol": trade_symbol,
                        "type": self._map_type(order.get("type")),
                        "side": ex.get("side"),
                        "price": float(ex.get("price") or 0.0),
                        "amount": float(ex.get("qty") or 0.0),
                        "cost": float(ex.get("cost") or 0.0),
                        "fee": fee_val,
                        "fee_currency": fee_curr,
                        "timestamp": timestamp,
                        "datetime": dt,
                        "info": ex,
                    }
                )

        # CCXT-style local filtering by 'since' (millisecond timestamp)
        # to ensure the filter is strictly applied regardless of API behavior.
        if since:
            result = [t for t in result if (t.get("timestamp") or 0) >= since]

        # Aggregated executions from different orders must be re-sorted by execution time (descending).
        result.sort(key=lambda x: x["timestamp"] or 0, reverse=True)
        return result[:limit] if limit else result
