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