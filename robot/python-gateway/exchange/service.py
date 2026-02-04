import logging

# Import the generated classes
from v1 import exchange_pb2
from v1 import exchange_pb2_grpc

import grpc
from typing import Any
from exchange.factory import ExchangeNotConfigured, ExchangeFactory
from exchange.exchanges.base import Exchange


class ExchangeService(exchange_pb2_grpc.ExchangeServiceServicer):
    """
    Implements the gRPC service for the exchange gateway.
    This class handles incoming gRPC requests and interacts with the ExchangeFactory
    to perform operations on the configured exchanges.
    """

    def __init__(self, cfg: object, factory: 'ExchangeFactory'):
        """Initializes the service with configuration and exchange factory."""
        self.cfg = cfg
        self.factory = factory
        self.default_exchange = self.cfg.exchanges[0].name if self.cfg.exchanges else None

    def _getExchange(self, request: Any, context: grpc.ServicerContext) -> Exchange:
        """Helper method to retrieve the exchange instance based on the request or default."""
        ex_name = getattr(request, 'exchange', None) or self.default_exchange
        try:
            exchange = self.factory.get(ex_name)
        except ExchangeNotConfigured as e:
            logging.error(f"Exchange not configured: {e}")
            context.abort(grpc.StatusCode.NOT_FOUND, str(e))

        return exchange

    def Ping(self, request: Any, context: grpc.ServicerContext) -> exchange_pb2.PingResponse:
        """Handles the Ping RPC. This is a simple health check."""
        logging.info("Received Ping request from Go client.")
        return exchange_pb2.PingResponse(message="Pong from Python gateway!")

    def GetTicker(self, request: Any, context: grpc.ServicerContext) -> exchange_pb2.TickerResponse:
        """Handles the GetTicker RPC."""
        logging.info(f"Received GetTicker request for {request.symbol}")
        
        exchange = self._getExchange(request, context)
        try:
            ticker = exchange.fetch_ticker(request.symbol)
            price = float(ticker.last)
        except Exception as e:
            logging.error(f"Error fetching ticker: {e}")
            context.abort(grpc.StatusCode.INTERNAL, str(e))

        return exchange_pb2.TickerResponse(symbol=ticker.symbol, price=price)


    def GetBalance(self, request: Any, context: grpc.ServicerContext) -> exchange_pb2.BalanceResponse:
        """Handles the GetBalance RPC."""
        logging.info(f"Received GetBalance request for {request.currency}")
        
        exchange = self._getExchange(request, context)
        try:
            balance = exchange.fetch_balance()
            # balance is expected to be a dict with 'free', 'used', 'total' keys
            free = balance.get('free', {})
            used = balance.get('used', {})
            total = balance.get('total', {})
        except Exception as e:
            logging.error(f"Error fetching balance: {e}")
            context.abort(grpc.StatusCode.INTERNAL, str(e))

        return exchange_pb2.BalanceResponse(free=free, used=used, total=total)

    def CreateOrder(self, request: Any, context: grpc.ServicerContext) -> exchange_pb2.OrderResponse:
        """Handles the CreateOrder RPC."""
        logging.info(f"Received CreateOrder request for {request.symbol} {request.side} {request.type} {request.amount} @ {request.price}")
        
        exchange = self._getExchange(request, context)
        try:
            order = exchange.create_order(
                symbol=request.symbol,
                type=request.type,
                side=request.side,
                amount=request.amount,
                price=request.price
            )
        except Exception as e:
            logging.error(f"Error creating order: {e}")
            context.abort(grpc.StatusCode.INTERNAL, str(e))

        return exchange_pb2.OrderResponse(
            id=str(order.get('id', '')),
            symbol=order.get('symbol', request.symbol),
            side=order.get('side', request.side),
            type=order.get('type', request.type),
            amount=order.get('amount', request.amount),
            price=order.get('price', request.price),
            status=order.get('status', ''),
            filled=order.get('filled', 0.0),
            remaining=order.get('remaining', 0.0),
            cost=order.get('cost', 0.0),
            average=order.get('average', 0.0)
        )

    def CancelOrder(self, request: Any, context: grpc.ServicerContext) -> exchange_pb2.CancelOrderResponse:
        """Handles the CancelOrder RPC."""
        logging.info(f"Received CancelOrder request for ID: {request.id} symbol: {request.symbol}")

        exchange = self._getExchange(request, context)
        try:
            result = exchange.cancel_order(request.id, symbol=request.symbol)
        except Exception as e:
            logging.error(f"Error canceling order: {e}")
            context.abort(grpc.StatusCode.INTERNAL, str(e))

        return exchange_pb2.CancelOrderResponse(
            id=str(result.get('id', request.id)),
            status=result.get('status', '')
        )

    def GetOrder(self, request: Any, context: grpc.ServicerContext) -> exchange_pb2.OrderResponse:
        """Handles the GetOrder RPC."""
        logging.info(f"Received GetOrder request for ID: {request.id}")

        exchange = self._getExchange(request, context)
        try:
            order = exchange.fetch_order(request.id, symbol=request.symbol)
        except Exception as e:
            logging.error(f"Error fetching order: {e}")
            context.abort(grpc.StatusCode.INTERNAL, str(e))

        return exchange_pb2.OrderResponse(
            id=str(order.get('id', request.id)),
            symbol=order.get('symbol', request.symbol),
            side=order.get('side', ''),
            type=order.get('type', ''),
            amount=order.get('amount', 0.0),
            price=order.get('price', 0.0),
            status=order.get('status', ''),
            filled=order.get('filled', 0.0),
            remaining=order.get('remaining', 0.0),
            cost=order.get('cost', 0.0),
            average=order.get('average', 0.0)
        )

    def GetOpenOrders(self, request: Any, context: grpc.ServicerContext) -> exchange_pb2.OpenOrdersResponse:
        """Handles the GetOpenOrders RPC."""
        logging.info(f"Received GetOpenOrders request for {request.symbol}")
        
        exchange = self._getExchange(request, context)
        try:
            orders = exchange.fetch_open_orders(request.symbol)
        except Exception as e:
            logging.error(f"Error fetching open orders: {e}")
            context.abort(grpc.StatusCode.INTERNAL, str(e))

        resp_orders = [
            exchange_pb2.OrderResponse(
                id=str(order.get('id', '')),
                symbol=order.get('symbol', request.symbol),
                side=order.get('side', ''),
                type=order.get('type', ''),
                amount=order.get('amount', 0.0),
                price=order.get('price', 0.0),
                status=order.get('status', ''),
                filled=order.get('filled', 0.0),
                remaining=order.get('remaining', 0.0),
                cost=order.get('cost', 0.0),
                average=order.get('average', 0.0)
            ) for order in orders
        ]
        return exchange_pb2.OpenOrdersResponse(orders=resp_orders)
