import unittest
import logging
import io
import json
from unittest.mock import patch

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

    def test_text_format_no_source(self):
        """Tests text formatting without source information to cover the default Formatter branch."""
        log_config = config.LogConfig(level="info", format="text", source=False)
        logger.setup(log_config, stream=self.log_stream)

        logging.info("no source message")
        output = self.log_stream.getvalue()

        self.assertIn("INFO", output)
        self.assertIn("no source message", output)
        self.assertNotIn("[", output)  # Source info is omitted

    @patch("sys.stdout", new_callable=io.StringIO)
    def test_default_stream(self, mock_stdout):
        """Tests that the logger defaults to sys.stdout when no stream is provided."""
        log_config = config.LogConfig(level="info", format="text")
        # Calling setup without stream hits the 'if stream is None' branch
        logger.setup(log_config)
        logging.info("covered default stream")
        self.assertIn("INFO", mock_stdout.getvalue())


# To run this test directly, use:
#   python -m tests.core.test_logger
if __name__ == "__main__":
    unittest.main()
