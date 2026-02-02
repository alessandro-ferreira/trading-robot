import logging

# Import the generated classes
from v1 import exchange_pb2
from v1 import exchange_pb2_grpc


class ExchangeService(exchange_pb2_grpc.ExchangeServiceServicer):
    """
    Implements the gRPC service for the exchange gateway.
    This class contains the logic that translates gRPC calls into actions,
    such as interacting with the ccxt library.
    """

    def Ping(self, request, context):
        """
        Handles the Ping RPC. This is a simple health check.
        """
        logging.info("Received Ping request from Go client.")
        # In the future, this could also check connectivity to the actual exchange.
        return exchange_pb2.PingResponse(message="Pong from Python gateway!")

    def GetTicker(self, request, context):
        logging.info(f"Received GetTicker request for {request.symbol}")
        # Mock data for connectivity verification
        return exchange_pb2.TickerResponse(symbol=request.symbol, price=98000.50)

    def GetBalance(self, request, context):
        logging.info(f"Received GetBalance request for {request.currency}")
        # Mock data for connectivity verification
        return exchange_pb2.BalanceResponse(
            free={request.currency: 1000.0},
            used={request.currency: 0.0},
            total={request.currency: 1000.0}
        )

    def CreateOrder(self, request, context):
        logging.info(f"Received CreateOrder request: {request}")
        # Mock response
        return exchange_pb2.OrderResponse(
            id="12345",
            symbol=request.symbol,
            side=request.side,
            type=request.type,
            amount=request.amount,
            price=request.price,
            status="open",
            filled=0.0,
            remaining=request.amount,
            cost=0.0,
            average=0.0
        )

    def CancelOrder(self, request, context):
        logging.info(f"Received CancelOrder request for ID: {request.id}")
        # Mock response
        return exchange_pb2.CancelOrderResponse(
            id=request.id,
            status="canceled"
        )

    def GetOrder(self, request, context):
        logging.info(f"Received GetOrder request for ID: {request.id}")
        # Mock response
        return exchange_pb2.OrderResponse(
            id=request.id,
            symbol=request.symbol,
            side="buy",
            type="limit",
            amount=1.0,
            price=50000.0,
            status="closed",
            filled=1.0,
            remaining=0.0,
            cost=50000.0,
            average=50000.0
        )

    def GetOpenOrders(self, request, context):
        logging.info(f"Received GetOpenOrders request for {request.symbol}")
        # Mock response
        order1 = exchange_pb2.OrderResponse(
            id="101",
            symbol=request.symbol,
            side="buy",
            type="limit",
            amount=0.5,
            price=20000.0,
            status="open",
            filled=0.0,
            remaining=0.5,
            cost=0.0,
            average=0.0
        )
        return exchange_pb2.OpenOrdersResponse(orders=[order1])