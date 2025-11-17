import grpc
from concurrent import futures
import logging

# Import the generated classes
from v1 import exchange_pb2_grpc

# Import the service implementation from its dedicated module
from exchange.service import ExchangeService

logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')


def serve():
    """Initializes and runs the gRPC server."""
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    exchange_pb2_grpc.add_ExchangeServiceServicer_to_server(ExchangeService(), server)

    # Listen on port 50051, as defined in config.toml.example
    server.add_insecure_port('[::]:50051')
    logging.info("Python gRPC gateway started on port 50051")
    server.start()
    server.wait_for_termination()

if __name__ == '__main__':
    serve()
