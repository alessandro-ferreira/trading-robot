import unittest
import os

from core import config

TEST_DATA_DIR = "tests/core/testdata"


class TestConfig(unittest.TestCase):
    def test_load_full_config(self):
        cfg = config.load(os.path.join(TEST_DATA_DIR, "success.toml"))

        self.assertEqual(cfg.server.default_exchange_timeout, 5)
        self.assertEqual(cfg.server.shutdown_timeout, 2)
        self.assertEqual(cfg.grpc.python_gateway_address, "localhost:9999")
        self.assertEqual(cfg.log.level, "debug")
        self.assertEqual(cfg.log.format, "json")
        self.assertEqual(cfg.log.rotate, True)
        self.assertEqual(cfg.log.source, True)

        # Accessing the first exchange in the list
        self.assertTrue(len(cfg.exchanges) > 0)
        first_ex = cfg.exchanges[0]
        self.assertEqual(first_ex.name, "mercadobitcoin")
        self.assertEqual(first_ex.api_key, "test_api_key")
        self.assertEqual(first_ex.secret, "test_secret")
        self.assertFalse(first_ex.sandbox_mode)

    def test_load_defaults(self):
        cfg = config.load(os.path.join(TEST_DATA_DIR, "partial.toml"))

        self.assertEqual(cfg.server.default_exchange_timeout, 10)
        self.assertEqual(cfg.server.shutdown_timeout, 10)
        self.assertEqual(cfg.grpc.python_gateway_address, "[::]:50051")
        self.assertEqual(cfg.log.level, "info")
        self.assertEqual(cfg.log.format, "text")
        self.assertEqual(cfg.log.path, "")
        self.assertEqual(cfg.log.rotate, False)
        self.assertEqual(cfg.log.source, False)

        self.assertEqual(len(cfg.exchanges), 1)
        first_ex = cfg.exchanges[0]
        self.assertEqual(first_ex.name, "mercadobitcoin")

    def test_load_nonexistent_file(self):
        with self.assertRaises(FileNotFoundError):
            config.load("nonexistent.toml")


# To run this test directly, use:
#   python -m tests.core.test_config
if __name__ == "__main__":
    unittest.main()
