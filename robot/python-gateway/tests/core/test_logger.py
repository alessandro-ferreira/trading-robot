import unittest
import logging
import io
import json
import os
import tempfile

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

    def test_json_format_with_exception(self):
        """Tests that the JSON formatter includes exception and stack information."""
        log_config = config.LogConfig(level="error", format="json")
        logger.setup(log_config, stream=self.log_stream)

        try:
            1 / 0
        except ZeroDivisionError:
            logging.error("error occurred", exc_info=True, stack_info=True)

        output = self.log_stream.getvalue()
        log_data = json.loads(output)

        self.assertIn("exc_info", log_data)
        self.assertIn("ZeroDivisionError: division by zero", log_data["exc_info"])
        self.assertIn("stack_info", log_data)

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

    def test_daily_file_rotation(self):
        """Tests that DailyFileHandler rotates the log file when the date changes."""
        with tempfile.TemporaryDirectory() as tmp_dir:
            base_path = os.path.join(tmp_dir, "test.log")

            with patch("core.logger.datetime") as mock_datetime:
                # First call to now().strftime() for __init__
                mock_datetime.now.return_value.strftime.return_value = "2023-01-01"
                handler = logger.DailyFileHandler(base_path)

                # Second call for the first emit (same date)
                log_record = logging.LogRecord(
                    "test", logging.INFO, "test.py", 10, "msg1", None, None
                )
                handler.emit(log_record)

                # Third call for the second emit (new date)
                mock_datetime.now.return_value.strftime.return_value = "2023-01-02"
                log_record2 = logging.LogRecord(
                    "test", logging.INFO, "test.py", 11, "msg2", None, None
                )
                handler.emit(log_record2)

                initial_path = os.path.join(tmp_dir, "test-2023-01-01.log")
                new_path = os.path.join(tmp_dir, "test-2023-01-02.log")

                self.assertTrue(os.path.exists(initial_path))
                self.assertTrue(os.path.exists(new_path))

                handler.close()

    @patch("sys.stderr", new_callable=io.StringIO)
    def test_setup_file_error(self, mock_stderr):
        """Tests the exception handling in setup when DailyFileHandler fails."""
        # Using a directory that doesn't exist to trigger an exception
        log_config = config.LogConfig(
            level="info", path="/non/existent/path/test.log", rotate=True
        )
        logger.setup(log_config)

        self.assertIn("Failed to setup file logger", mock_stderr.getvalue())


# To run this test directly, use:
#   python -m tests.core.test_logger
if __name__ == "__main__":
    unittest.main()
