import unittest
import logging
import io
import json
import os
import tempfile
from datetime import datetime
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
        self.assertIn("id", log_data)
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
        self.assertIn("(", output)  # For request ID
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
        self.assertIn("(", output)  # ID is still present
        self.assertNotIn("[", output)  # Source info is omitted

    @patch("sys.stdout", new_callable=io.StringIO)
    def test_default_stream(self, mock_stdout):
        """Tests that the logger defaults to sys.stdout when no stream is provided."""
        log_config = config.LogConfig(level="info", format="text")
        # Calling setup without stream hits the 'if stream is None' branch
        logger.setup(log_config)
        logging.info("covered default stream")
        self.assertIn("INFO", mock_stdout.getvalue())

    def test_file_logging(self):
        """Tests that the logger correctly writes to a file when configured."""
        with tempfile.NamedTemporaryFile(mode="w+", delete=False) as tmp_file:
            tmp_path = tmp_file.name

        try:
            log_config = config.LogConfig(level="info", format="text", path=tmp_path)
            # When path is set, it uses FileHandler
            logger.setup(log_config)
            logging.info("file log test")

            with open(tmp_path, "r") as f:
                content = f.read()

            self.assertIn("INFO", content)
            self.assertIn("file log test", content)
        finally:
            if os.path.exists(tmp_path):
                os.remove(tmp_path)

    def test_rotate_logging(self):
        """Tests that the logger appends the date to the filename when rotate is True."""
        with tempfile.TemporaryDirectory() as tmp_dir:
            base_path = os.path.join(tmp_dir, "test.log")
            log_config = config.LogConfig(
                level="info", format="text", path=base_path, rotate=True
            )

            logger.setup(log_config)
            logging.info("rotate test")

            date_str = datetime.now().strftime("%Y-%m-%d")
            expected_path = os.path.join(tmp_dir, f"test-{date_str}.log")

            self.assertTrue(
                os.path.exists(expected_path), f"File {expected_path} should exist"
            )
            with open(expected_path, "r") as f:
                content = f.read()
            self.assertIn("rotate test", content)


# To run this test directly, use:
#   python -m tests.core.test_logger
if __name__ == "__main__":
    unittest.main()
