import logging
import tomllib
from dataclasses import dataclass, field
from typing import List, Union


@dataclass
class ServerConfig:
    """Server configuration."""

    shutdown_timeout: int = 10  # seconds


@dataclass
class GRPCConfig:
    """gRPC server configuration."""

    python_gateway_address: str = "[::]:50051"


@dataclass
class LogConfig:
    """Logging configuration."""

    level: str = "INFO"
    format: str = "text"
    source: bool = False


@dataclass
class ExchangeConfig:
    """Exchange configuration."""

    name: str = ""
    api_key: str = ""
    secret: str = ""
    sandbox_mode: bool = False
    ccxt: bool = True  # Indicates if this exchange should be handled by ccxt


@dataclass
class Config:
    """Root configuration object."""

    server: ServerConfig = field(default_factory=ServerConfig)
    grpc: GRPCConfig = field(default_factory=GRPCConfig)
    log: LogConfig = field(default_factory=LogConfig)
    exchanges: List[ExchangeConfig] = field(default_factory=list)


def _parse_duration_to_seconds(duration: Union[str, int], default: int) -> int:
    """Parses a duration value which can be an integer (seconds) or a string ending with 's'."""
    if isinstance(duration, int):
        return duration
    if isinstance(duration, str) and duration.lower().endswith("s"):
        try:
            return int(duration[:-1])
        except (ValueError, TypeError):
            return default
    return default


def load_from_dict(data: dict) -> Config:
    """Loads configuration from a dictionary, applying defaults."""
    cfg = Config()

    # Server/GRPC/Log loading
    if server_data := data.get("server"):
        timeout_val = server_data.get("shutdown_timeout", cfg.server.shutdown_timeout)
        cfg.server.shutdown_timeout = _parse_duration_to_seconds(
            timeout_val, cfg.server.shutdown_timeout
        )

    if grpc_data := data.get("grpc"):
        cfg.grpc.python_gateway_address = grpc_data.get(
            "python_gateway_address", cfg.grpc.python_gateway_address
        )

    if log_data := data.get("log"):
        cfg.log.level = log_data.get("level", cfg.log.level)
        cfg.log.format = log_data.get("format", cfg.log.format)
        cfg.log.source = log_data.get("source", cfg.log.source)

    for entry in data.get("exchange", []):
        ex_cfg = ExchangeConfig(
            name=entry.get("name", None),
            api_key=entry.get("api_key", None),
            secret=entry.get("api_secret", None),
            sandbox_mode=entry.get("sandbox_mode", False),
            ccxt=entry.get("ccxt", False),
        )
        cfg.exchanges.append(ex_cfg)

    return cfg


def load(path: str) -> Config:
    """Loads configuration from a TOML file."""
    logging.info(f"Loading configuration from {path}")
    try:
        with open(path, "rb") as f:
            data = tomllib.load(f)
            return load_from_dict(data)
    except FileNotFoundError:
        logging.warning(f"Config file not found at {path}. Using default values.")
        return Config()
    except tomllib.TOMLDecodeError as e:
        logging.error(f"Failed to decode TOML file: {e}")
        raise
