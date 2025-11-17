import logging
import time

from v1 import exchange_pb2
from v1 import exchange_pb2_grpc


class ExchangeService(exchange_pb2_grpc.ExchangeServiceServicer):
    """
    Implements the gRPC ExchangeService. This is where the core logic for
    interacting with the ccxt library will reside.
    """

    def CreateOrder(self, request, context):
        logging.info(f"Received CreateOrder request: symbol='{request.symbol}', side={exchange_pb2.OrderSide.Name(request.side)}, amount={request.amount}")

        # TODO: Add actual ccxt logic here to place the order.

        # For now, return a dummy response.
        response = exchange_pb2.CreateOrderResponse(order_id="py-order-12345", status="received", timestamp=int(time.time()))
        return response
