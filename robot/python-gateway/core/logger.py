import logging
import sys
import json
import os

from datetime import datetime
from core.config import LogConfig

# Determine the project root directory (one level up from core/)
_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))


class RequestIDFilter(logging.Filter):
    """
    Filters log records to inject a 6-digit request ID derived from the thread identity.
    """

    def filter(self, record):
        record.request_id = record.thread % 1000000
        return True


class JSONFormatter(logging.Formatter):
    """
    A custom formatter to output logs in JSON format.
    """

    def __init__(self, *args, source: bool = False, **kwargs):
        super().__init__(*args, **kwargs)
        self.include_source = source

    def format(self, record):
        log_record = {
            "id": getattr(record, "request_id", 0),
            "time": self.formatTime(record, self.datefmt),
            "level": record.levelname,
            "msg": record.getMessage(),
        }
        if record.exc_info:
            log_record["exc_info"] = self.formatException(record.exc_info)
        if record.stack_info:
            log_record["stack_info"] = self.formatStack(record.stack_info)
        if self.include_source:
            rel_path = os.path.relpath(record.pathname, _ROOT)
            log_record["source"] = f"{rel_path}:{record.lineno}"

        return json.dumps(log_record)


class TextFormatter(logging.Formatter):
    """
    A custom formatter to provide relative paths from project root.
    """

    def format(self, record):
        record.relpath = os.path.relpath(record.pathname, _ROOT)
        return super().format(record)


def setup(cfg: LogConfig, stream=None):
    """
    Sets up the root logger. This function can be called multiple times
    to reconfigure the logger.
    """
    root_logger = logging.getLogger()
    root_logger.setLevel(cfg.level.upper())

    # Remove and close all existing handlers to prevent duplicate logs
    for handler in root_logger.handlers[:]:
        root_logger.removeHandler(handler)
        handler.close()

    if stream is None:
        stream = sys.stdout

    handler = logging.StreamHandler(stream)
    # If a log file path is specified, add a FileHandler to log to that file.
    if cfg.path:
        log_path = cfg.path
        if cfg.rotate:
            base, ext = os.path.splitext(log_path)
            date_str = datetime.now().strftime("%Y-%m-%d")
            log_path = f"{base}-{date_str}{ext}"
        try:
            handler = logging.FileHandler(log_path)
        except Exception as e:
            print(f"Failed to setup file logger at {log_path}: {e}", file=sys.stderr)
    handler.addFilter(RequestIDFilter())

    formatter: logging.Formatter
    if cfg.format.lower() == "json":
        formatter = JSONFormatter(source=cfg.source)
    else:  # Default to text
        log_format = "(%(request_id)06d) %(asctime)s - %(levelname)s - %(message)s"
        if cfg.source:
            log_format = "(%(request_id)06d) %(asctime)s - %(levelname)s - [%(relpath)s:%(lineno)d] - %(message)s"
            formatter = TextFormatter(log_format)
        else:
            formatter = logging.Formatter(log_format)

    handler.setFormatter(formatter)
    root_logger.addHandler(handler)
