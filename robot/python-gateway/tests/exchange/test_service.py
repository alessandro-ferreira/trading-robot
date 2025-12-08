import unittest

from exchange.service import ExchangeService
from v1 import exchange_pb2


class MockContext:
    """A mock gRPC context for testing."""
    def is_active(self):
        return True


class ExchangeServiceTest(unittest.TestCase):
    def test_ping(self):
        """Tests the Ping RPC method."""
        service = ExchangeService()
        request = exchange_pb2.PingRequest()
        response = service.Ping(request, MockContext())

        self.assertEqual(response.message, "Pong from Python gateway!")
