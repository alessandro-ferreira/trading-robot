import logging
from typing import Dict, List, Optional

from core.config import ExchangeConfig
from .exchanges.base import Exchange
from .exchanges import REGISTRY


class ExchangeNotConfigured(Exception):
    """Raised when an exchange is not configured or unknown."""

    pass


class ExchangeConfigurationError(Exception):
    """Raised when an exchange configuration is invalid (e.g., unsupported sandbox)."""

    pass


class ExchangeFactory:
    """
    Implements a factory for creating and managing exchange instances based on configuration.
    This class uses a registry pattern to instantiate exchange providers dynamically.
    """

    def __init__(self, exchanges_config: List[ExchangeConfig]):
        """Initializes the factory with a list of exchange configurations."""
        self._configs: Dict[str, ExchangeConfig] = {
            ex_cfg.name: ex_cfg for ex_cfg in exchanges_config
        }
        self._instances: Dict[str, Exchange] = {}

    def _create_instance(self, name: str) -> Exchange:
        """Creates an exchange instance based on its configuration."""
        cfg = self._configs.get(name)
        if not cfg:
            raise ExchangeNotConfigured(
                f"Exchange '{name}' is not configured or unknown"
            )

        # Check if exchange is registered
        if cfg.name not in REGISTRY:
            logging.error(f"Exchange '{cfg.name}' is not registered")
            raise ExchangeNotConfigured(f"Exchange '{cfg.name}' is not registered")

        # Instantiate provider from registry
        provider_cls = REGISTRY[cfg.name]
        try:
            provider = provider_cls(cfg)
        except Exception as e:
            raise ExchangeConfigurationError(
                f"Failed to initialize exchange '{cfg.name}': {e}"
            )

        # Enforce sandbox setup via provider API
        if cfg.sandbox_mode:
            try:
                provider.set_sandbox_mode(True)
                logging.info(f"Sandbox mode enabled for {cfg.name}")
            except Exception as e:
                logging.error(f"Failed to enable sandbox mode for {cfg.name}: {e}")
                raise ExchangeConfigurationError(
                    f"Sandbox mode not supported or failed for {cfg.name}: {e}"
                )

        return provider

    def list_exchanges(self):
        """Returns a list of configured exchange names."""
        return list(self._configs.keys())

    def get(self, name: str) -> Exchange:
        """Retrieves an exchange instance by name, creating it if it doesn't exist."""
        if name is None:
            raise ExchangeNotConfigured("Exchange name must be provided")
        if name not in self._instances:
            try:
                self._instances[name] = self._create_instance(name)
            except (ExchangeNotConfigured, ExchangeConfigurationError):
                raise
        return self._instances[name]

    def get_default(self) -> Optional[str]:
        """Returns the name of the default exchange (the first configured one), or None if none are configured."""
        return next(iter(self._configs.keys()), None)
