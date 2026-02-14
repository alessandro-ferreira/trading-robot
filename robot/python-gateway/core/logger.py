import logging
import sys
import json

from core.config import LogConfig


class JSONFormatter(logging.Formatter):
    """
    A custom formatter to output logs in JSON format.
    """

    def __init__(self, *args, source: bool = False, **kwargs):
        super().__init__(*args, **kwargs)
        self.include_source = source

    def format(self, record):
        log_record = {
            "time": self.formatTime(record, self.datefmt),
            "level": record.levelname,
            "msg": record.getMessage(),
        }
        if record.exc_info:
            log_record["exc_info"] = self.formatException(record.exc_info)
        if record.stack_info:
            log_record["stack_info"] = self.formatStack(record.stack_info)
        if self.include_source:
            log_record["source"] = f"{record.pathname}:{record.lineno}"

        return json.dumps(log_record)


def setup(cfg: LogConfig, stream=None):
    """
    Sets up the root logger. This function can be called multiple times
    to reconfigure the logger.
    """
    root_logger = logging.getLogger()
    root_logger.setLevel(cfg.level.upper())

    # Remove all existing handlers to prevent duplicate logs
    for handler in root_logger.handlers[:]:
        root_logger.removeHandler(handler)

    if stream is None:
        stream = sys.stdout

    handler = logging.StreamHandler(stream)

    formatter: logging.Formatter
    if cfg.format.lower() == "json":
        formatter = JSONFormatter(source=cfg.source)
    else:  # Default to text
        log_format = "%(asctime)s - %(levelname)s - %(message)s"
        if cfg.source:
            log_format = (
                "%(asctime)s - %(levelname)s - [%(pathname)s:%(lineno)d] - %(message)s"
            )
        formatter = logging.Formatter(log_format)

    handler.setFormatter(formatter)
    root_logger.addHandler(handler)
