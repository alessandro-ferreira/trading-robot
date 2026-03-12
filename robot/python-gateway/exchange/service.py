import logging
from typing import Any

import grpc
import ccxt

from v1 import exchange_pb2
from v1 import exchange_pb2_grpc
from exchange.factory import (
    ExchangeConfigurationError,
    ExchangeFactory,
    ExchangeNotConfigured,
)
from exchange.exchanges.base import Exchange
from core.config import Config


class ExchangeService(exchange_pb2_grpc.ExchangeServiceServicer):
    """
    Implements the gRPC service for the exchange gateway.
    This class handles incoming gRPC requests and interacts with the ExchangeFactory
    to perform operations on the configured exchanges.
    """

    def __init__(self, cfg: Config, factory: ExchangeFactory):
        """Initializes the service with configuration and exchange factory."""
        self.cfg = cfg
        self.factory = factory
        self.default_exchange = (
            self.cfg.exchanges[0].name if self.cfg.exchanges else None
        )
        # initialize all exchanges at startup to catch configuration errors early
        for exchange in self.cfg.exchanges:
            try:
                self.factory.get(exchange.name)
                logging.info(f"Successfully initialized exchange: {exchange.name}")
            except Exception as e:
                logging.exception(f"Failed to initialize exchange {exchange.name}: {e}")

    def _getExchange(self, request: Any, context: grpc.ServicerContext) -> Exchange:
        """Helper method to retrieve the exchange instance based on the request or default."""
        ex_name = request.exchange or self.default_exchange
        try:
            exchange = self.factory.get(ex_name)
        except ExchangeNotConfigured as e:
            logging.exception(f"Exchange not configured: {e}")
            context.abort(grpc.StatusCode.NOT_FOUND, str(e))
        except ExchangeConfigurationError as e:
            logging.exception(f"Exchange configuration error: {e}")
            context.abort(grpc.StatusCode.FAILED_PRECONDITION, str(e))

        return exchange

    def _handle_exchange_error(
        self, context: grpc.ServicerContext, e: Exception, action: str
    ):
        """Helper to map ccxt exceptions to gRPC status codes."""
        logging.exception(f"Error {action}: {e}")

        if isinstance(e, ccxt.NetworkError):
            context.abort(grpc.StatusCode.UNAVAILABLE, f"Exchange network error: {e}")
        elif isinstance(e, ccxt.AuthenticationError):
            context.abort(
                grpc.StatusCode.UNAUTHENTICATED, f"Exchange authentication failed: {e}"
            )
        elif isinstance(e, ccxt.InsufficientFunds):
            context.abort(
                grpc.StatusCode.FAILED_PRECONDITION, f"Insufficient funds: {e}"
            )
        elif isinstance(e, ccxt.InvalidOrder):
            context.abort(
                grpc.StatusCode.INVALID_ARGUMENT, f"Invalid order parameters: {e}"
            )
        else:
            context.abort(grpc.StatusCode.INTERNAL, str(e))

    def Ping(
        self, request: Any, context: grpc.ServicerContext
    ) -> exchange_pb2.PingResponse:
        """Handles the Ping RPC. This is a simple health check."""
        logging.info("Received Ping request from Go client.")
        return exchange_pb2.PingResponse(message="Pong from Python gateway!")

    def GetTicker(
        self, request: Any, context: grpc.ServicerContext
    ) -> exchange_pb2.TickerResponse:
        """Handles the GetTicker RPC."""
        logging.info(
            f"Received GetTicker request for exchange: {request.exchange}, symbol: {request.symbol}"
        )

        exchange = self._getExchange(request, context)
        try:
            ticker = exchange.fetch_ticker(request.symbol)
            price = float(ticker.last)
        except Exception as e:
            self._handle_exchange_error(context, e, "fetching ticker")

        return exchange_pb2.TickerResponse(symbol=ticker.symbol, price=price)

    def GetBalance(
        self, request: Any, context: grpc.ServicerContext
    ) -> exchange_pb2.BalanceResponse:
        """Handles the GetBalance RPC."""
        logging.info(
            f"Received GetBalance request for exchange: {request.exchange}, currency: {request.currency}"
        )

        exchange = self._getExchange(request, context)
        try:
            balance = exchange.fetch_balance()
            free = balance.get("free", {})
            used = balance.get("used", {})
            total = balance.get("total", {})
            if request.currency:
                currency = request.currency
                free = {currency: free.get(currency, 0)}
                used = {currency: used.get(currency, 0)}
                total = {currency: total.get(currency, 0)}
        except Exception as e:
            self._handle_exchange_error(context, e, "fetching balance")

        return exchange_pb2.BalanceResponse(free=free, used=used, total=total)

    def CreateOrder(
        self, request: Any, context: grpc.ServicerContext
    ) -> exchange_pb2.OrderResponse:
        """Handles the CreateOrder RPC."""
        logging.info(
            f"Received CreateOrder request for exchange: {request.exchange}, symbol: {request.symbol}, side: {request.side}, type: {request.type}, amount: {request.amount}, price: {request.price}"
        )

        exchange = self._getExchange(request, context)
        try:
            order = exchange.create_order(
                symbol=request.symbol,
                type=request.type,
                side=request.side,
                amount=request.amount,
                price=request.price,
            )
        except Exception as e:
            self._handle_exchange_error(context, e, "creating order")

        return exchange_pb2.OrderResponse(
            id=str(order.get("id", "")),
            symbol=order.get("symbol", request.symbol),
            side=order.get("side", request.side),
            type=order.get("type", request.type),
            amount=order.get("amount", request.amount),
            price=order.get("price", request.price),
            status=order.get("status", ""),
            filled=order.get("filled", 0.0),
            remaining=order.get("remaining", 0.0),
            cost=order.get("cost", 0.0),
            average=order.get("average", 0.0),
            client_order_id=str(order.get("clientOrderId") or ""),
            timestamp=int(order.get("timestamp") or 0),
        )

    def CancelOrder(
        self, request: Any, context: grpc.ServicerContext
    ) -> exchange_pb2.CancelOrderResponse:
        """Handles the CancelOrder RPC."""
        logging.info(
            f"Received CancelOrder request for exchange: {request.exchange}, ID: {request.id}, symbol: {request.symbol}"
        )

        exchange = self._getExchange(request, context)
        try:
            result = exchange.cancel_order(request.id, symbol=request.symbol)
        except Exception as e:
            self._handle_exchange_error(context, e, "canceling order")

        return exchange_pb2.CancelOrderResponse(
            id=str(result.get("id", request.id)), status=result.get("status", "")
        )

    def GetOrder(
        self, request: Any, context: grpc.ServicerContext
    ) -> exchange_pb2.OrderResponse:
        """Handles the GetOrder RPC."""
        logging.info(
            f"Received GetOrder request for exchange: {request.exchange}, ID: {request.id}"
        )

        exchange = self._getExchange(request, context)
        try:
            order = exchange.fetch_order(request.id, symbol=request.symbol)
        except Exception as e:
            self._handle_exchange_error(context, e, "fetching order")

        return exchange_pb2.OrderResponse(
            id=str(order.get("id", request.id)),
            symbol=order.get("symbol", request.symbol),
            side=order.get("side", ""),
            type=order.get("type", ""),
            amount=order.get("amount", 0.0),
            price=order.get("price", 0.0),
            status=order.get("status", ""),
            filled=order.get("filled", 0.0),
            remaining=order.get("remaining", 0.0),
            cost=order.get("cost", 0.0),
            average=order.get("average", 0.0),
            client_order_id=str(order.get("clientOrderId") or ""),
            timestamp=int(order.get("timestamp") or 0),
        )

    def GetOpenOrders(
        self, request: Any, context: grpc.ServicerContext
    ) -> exchange_pb2.OpenOrdersResponse:
        """Handles the GetOpenOrders RPC."""
        logging.info(
            f"Received GetOpenOrders request for exchange: {request.exchange}, symbol: {request.symbol}"
        )

        exchange = self._getExchange(request, context)
        try:
            orders = exchange.fetch_open_orders(request.symbol)
        except Exception as e:
            self._handle_exchange_error(context, e, "fetching open orders")

        resp_orders = [
            exchange_pb2.OrderResponse(
                id=str(order.get("id", "")),
                symbol=order.get("symbol", request.symbol),
                side=order.get("side", ""),
                type=order.get("type", ""),
                amount=order.get("amount", 0.0),
                price=order.get("price", 0.0),
                status=order.get("status", ""),
                filled=order.get("filled", 0.0),
                remaining=order.get("remaining", 0.0),
                cost=order.get("cost", 0.0),
                average=order.get("average", 0.0),
                client_order_id=str(order.get("clientOrderId") or ""),
                timestamp=int(order.get("timestamp") or 0),
            )
            for order in orders
        ]
        return exchange_pb2.OpenOrdersResponse(orders=resp_orders)

    def ResetState(
        self, request: Any, context: grpc.ServicerContext
    ) -> exchange_pb2.ResetStateResponse:
        """Resets the state of the dummy exchange for testing purposes."""
        logging.info("Received ResetState request.")

        try:
            exchange = self.factory.get("dummy")
            exchange.reset()  # Directly call reset, will raise AttributeError if not present
            logging.info("Dummy exchange state has been reset.")
            return exchange_pb2.ResetStateResponse(status="OK")
        except Exception as e:
            logging.warning(f"ResetState ignored: {e}")

        return exchange_pb2.ResetStateResponse(status="IGNORED")
