import unittest
import os

from core import config

TEST_DATA_DIR = "tests/core/testdata"

class TestConfig(unittest.TestCase):
    def test_load_full_config(self):
        cfg = config.load(os.path.join(TEST_DATA_DIR, "success.toml"))

        self.assertEqual(cfg.server.shutdown_timeout, 5)
        self.assertEqual(cfg.grpc.python_gateway_address, "localhost:9999")
        self.assertEqual(cfg.log.level, "debug")
        
        # Accessing the first exchange in the list
        self.assertTrue(len(cfg.exchanges) > 0)
        first_ex = cfg.exchanges[0]
        self.assertEqual(first_ex.name, "mercadobitcoin")
        self.assertEqual(first_ex.api_key, "test_api_key")
        self.assertEqual(first_ex.secret, "test_secret")
        self.assertFalse(first_ex.sandbox_mode)

    def test_load_defaults(self):
        cfg = config.load(os.path.join(TEST_DATA_DIR, "partial.toml"))

        self.assertEqual(cfg.log.level, "warn")
        self.assertEqual(cfg.server.shutdown_timeout, 10)
        
        if not cfg.exchanges:
            self.assertEqual(len(cfg.exchanges), 0)

    def test_load_nonexistent_file(self):
        cfg = config.load("nonexistent.toml")
        self.assertIsInstance(cfg, config.Config)
        # Defaults for an empty config usually mean 0 exchanges
        self.assertEqual(len(cfg.exchanges), 0)

# To run this test directly, use:
#   python -m tests.core.test_config
if __name__ == "__main__":
    unittest.main()