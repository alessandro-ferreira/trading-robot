import unittest
import os
import io
import tomllib

from unittest.mock import patch

from core import config, logger

TEST_DATA_DIR = "tests/core/testdata"


class TestConfig(unittest.TestCase):
    def setUp(self):
        """Silences logging during configuration tests."""
        self.log_stream = io.StringIO()
        logger.setup(config.LogConfig(), stream=self.log_stream)

    def test_load_full_config(self):
        """Tests loading a complete configuration file."""
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
        """Tests loading a configuration file with missing values."""
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
        """Tests that loading a non-existent configuration file raises FileNotFoundError."""
        with self.assertRaises(FileNotFoundError):
            config.load("nonexistent.toml")

    def test_load_invalid_toml(self):
        """Tests that a malformed TOML file raises TOMLDecodeError."""
        invalid_path = "tests/core/testdata/invalid.toml"
        # Create a tiny invalid TOML file
        with open(invalid_path, "w") as f:
            f.write("invalid = [toml")  # Missing closing bracket

        try:
            with self.assertRaises(tomllib.TOMLDecodeError):
                config.load(invalid_path)
        finally:
            if os.path.exists(invalid_path):
                os.remove(invalid_path)

    def test_load_unexpected_exception(self):
        """Tests that unexpected exceptions during load are re-raised."""
        # Using patch to trigger an exception during open()
        with patch("builtins.open", side_effect=RuntimeError("unexpected error")):
            with self.assertRaises(RuntimeError):
                config.load("somefile.toml")


# To run this test directly, use:
#   python -m tests.core.test_config
if __name__ == "__main__":
    unittest.main()
