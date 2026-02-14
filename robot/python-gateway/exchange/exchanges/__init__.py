from .dummy import DummyExchange

from .binance import BinanceExchange
from .coinbase import CoinbaseExchange
from .mercadobitcoin import MercadoBitcoinExchange


REGISTRY = {
    "dummy": DummyExchange,
    "binance": BinanceExchange,
    "coinbase": CoinbaseExchange,
    "mercadobitcoin": MercadoBitcoinExchange,
}

__all__ = [
    "DummyExchange",
    "BinanceExchange",
    "CoinbaseExchange",
    "MercadoBitcoinExchange",
    "REGISTRY",
]
