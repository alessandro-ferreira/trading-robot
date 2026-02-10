import argparse
import logging
import signal
import time
from concurrent import futures

import grpc
from grpc_reflection.v1alpha import reflection

from v1 import exchange_pb2, exchange_pb2_grpc
from core import config, logger
from exchange import ExchangeFactory, ExchangeService


def serve():
    """Initializes and runs the gRPC server."""
    parser = argparse.ArgumentParser(description="Python Exchange Gateway gRPC Server")
    parser.add_argument(
        "--config",
        type=str,
        required=True,
        help="Path to the configuration file (e.g., ../config.toml)",
    )
    args = parser.parse_args()

    # Load configuration and setup logging.
    try:
        cfg = config.load(args.config)
    except Exception as e:
        # If config loading fails before logging is set up, use basicConfig to log the error.
        logging.basicConfig()
        logging.critical(f"Failed to load configuration, cannot start: {e}")
        return
    
    logger.setup(cfg.log)

    # Initialize the exchange factory and gRPC server.
    factory = ExchangeFactory(cfg.exchanges)
    server = grpc.server(futures.ThreadPoolExecutor())
    exchange_pb2_grpc.add_ExchangeServiceServicer_to_server(ExchangeService(cfg, factory), server)

    # Enable reflection for the service, which allows clients to query the server for available services and methods.
    SERVICE_NAMES = (
        exchange_pb2.DESCRIPTOR.services_by_name['ExchangeService'].full_name,
        reflection.SERVICE_NAME,
    )
    reflection.enable_server_reflection(SERVICE_NAMES, server)

    # Start the server.
    server.add_insecure_port(cfg.grpc.python_gateway_address)
    server.start()
    logging.info(f"Python gRPC gateway started on {cfg.grpc.python_gateway_address}")
    logging.info("Press Ctrl+C to stop the server.")

    def handle_shutdown(signum, frame):
        logging.info(f"Shutdown signal received ({signal.Signals(signum).name}). Stopping server...")
        # This is a non-blocking call that causes wait_for_termination() to unblock.
        server.stop(grace=1)

    # Register the handler for SIGINT (Ctrl+C) and SIGTERM.
    signal.signal(signal.SIGINT, handle_shutdown)
    signal.signal(signal.SIGTERM, handle_shutdown)

    # Block the main thread until the server is stopped by the signal handler.
    server.wait_for_termination()

    # Wait for a configurable period to allow for any final logs to be processed.
    if cfg.server.shutdown_timeout > 0:
        logging.info(f"Waiting for shutdown delay of {cfg.server.shutdown_timeout}s...")
        time.sleep(cfg.server.shutdown_timeout)

    logging.info("Server stopped.")

if __name__ == '__main__':
    serve()
