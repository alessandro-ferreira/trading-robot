import unittest
import logging
import io
import json

from core import config, logger


class LoggerTest(unittest.TestCase):
    def setUp(self):
        """Redirect the root logger to a string buffer for each test."""
        self.log_stream = io.StringIO()

    def test_json_format(self):
        """Tests that the logger produces valid JSON output."""
        log_config = config.LogConfig(level="info", format="json", source=True)
        logger.setup(log_config, stream=self.log_stream)

        logging.info("test message")

        output = self.log_stream.getvalue()
        log_data = json.loads(output)

        self.assertEqual(log_data["level"], "INFO")
        self.assertEqual(log_data["msg"], "test message")
        self.assertIn("source", log_data)
        self.assertIn("test_logger.py", log_data["source"])

    def test_text_format(self):
        """Tests that the logger produces standard text output."""
        log_config = config.LogConfig(level="debug", format="text", source=True)
        logger.setup(log_config, stream=self.log_stream)

        logging.debug("debug message")

        output = self.log_stream.getvalue()

        self.assertIn("DEBUG", output)
        self.assertIn("debug message", output)
        self.assertIn("[", output)  # For source
        self.assertIn("test_logger.py", output)


# To run this test directly, use:
#   python -m tests.core.test_logger
if __name__ == "__main__":
    unittest.main()
