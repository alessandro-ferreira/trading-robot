import unittest
import os
from core import config

import logging
# Define a directory for test configuration files
TEST_DATA_DIR = "tests/core/testdata"
SUCCESS_CONFIG_PATH = os.path.join(TEST_DATA_DIR, "success.toml")
PARTIAL_CONFIG_PATH = os.path.join(TEST_DATA_DIR, "partial.toml")
NON_EXISTENT_PATH = os.path.join(TEST_DATA_DIR, "nonexistent.toml")


class ConfigTest(unittest.TestCase):
    def test_load_success(self):
        """Tests loading a complete and valid configuration file."""
        cfg = config.load(SUCCESS_CONFIG_PATH)
        self.assertEqual(cfg.server.shutdown_timeout, 5)
        self.assertEqual(cfg.grpc.python_gateway_address, "localhost:9999")
        self.assertEqual(cfg.log.level, "debug")
        self.assertEqual(cfg.log.format, "json")
        self.assertTrue(cfg.log.source)

    def test_load_partial_with_defaults(self):
        """Tests that defaults are applied for missing values in a partial config."""
        cfg = config.load(PARTIAL_CONFIG_PATH)
        # Values from file
        self.assertEqual(cfg.log.level, "warn")
        # Default values
        self.assertEqual(cfg.server.shutdown_timeout, 10)
        self.assertEqual(cfg.grpc.python_gateway_address, "[::]:50051")
        self.assertEqual(cfg.log.format, "text")
        self.assertFalse(cfg.log.source)

    def test_load_non_existent_file(self):
        """Tests that loading a non-existent file returns the default config."""
        # Temporarily disable logging for this test to avoid seeing the expected warning.
        logging.disable(logging.WARNING)
        try:
            cfg = config.load(NON_EXISTENT_PATH)
            self.assertIsInstance(cfg, config.Config)
            # Check a few default values to be sure
            self.assertEqual(cfg.server.shutdown_timeout, 10)
            self.assertEqual(cfg.log.level, "INFO")
        finally:
            # Re-enable logging so it doesn't affect other tests.
            logging.disable(logging.NOTSET)
