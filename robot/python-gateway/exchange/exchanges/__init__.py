from .ccxt import CCXTExchange
from .dummy import DummyExchange
from .mercadobitcoin import MercadoBitcoinExchange


REGISTRY = {
    "ccxt": CCXTExchange,
    "dummy": DummyExchange,
    "mercadobitcoin": MercadoBitcoinExchange,
}

# Whitelist of assets supported by the system's database schema.
SUPPORTED_ASSETS = {
    "BTC",
    "ETH",
    "LTC",
    "XRP",
    "BCH",
    "ADA",
    "DOGE",
    "SOL",
    "LINK",
    "XLM",
    "USDT",
    "BRL",
    "USD",
    "BNB",
    "AVAX",
    "ALGO",
    "SUI",
    "XMR",
    "DOT",
    "FLOW",
    "APT",
    "ARB",
    "OP",
    "TRX",
    "HYPE",
    "HBAR",
    "ZEC",
    "SHIB",
    "TON",
    "TAO",
    "UNI",
    "AAVE",
    "NEAR",
}

__all__ = [
    "CCXTExchange",
    "DummyExchange",
    "MercadoBitcoinExchange",
    "REGISTRY",
    "SUPPORTED_ASSETS",
]
