import json
import logging
import os
import sys

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


class DailyFileHandler(logging.FileHandler):
    """
    A thread-safe FileHandler that automatically rotates log files daily.

    Rotation is performed inside emit(), which is already protected by
    logging.Handler's internal lock.
    """

    def __init__(self, base_path, mode="a", encoding="utf-8", delay=False):
        self.base_path = base_path
        self.current_date = datetime.now().strftime("%Y-%m-%d")
        super().__init__(
            self._get_log_path(self.current_date),
            mode=mode,
            encoding=encoding,
            delay=delay,
        )

    def _get_log_path(self, date_str):
        base, ext = os.path.splitext(self.base_path)
        return f"{base}-{date_str}{ext}"

    def emit(self, record):
        new_date = datetime.now().strftime("%Y-%m-%d")

        if new_date != self.current_date:
            if self.stream:
                self.stream.flush()
                self.stream.close()

            self.current_date = new_date
            self.baseFilename = os.path.abspath(self._get_log_path(self.current_date))
            self.stream = self._open()

        super().emit(record)


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
        try:
            if cfg.rotate:
                handler = DailyFileHandler(cfg.path)
            else:
                handler = logging.FileHandler(cfg.path)
        except Exception as e:
            print(f"Failed to setup file logger at {cfg.path}: {e}", file=sys.stderr)

    handler.addFilter(RequestIDFilter())

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
