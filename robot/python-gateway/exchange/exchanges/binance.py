from .base import Exchange, Ticker


class BinanceExchange(Exchange):
    """Binance specific customizations."""

    def fetch_ticker(self, symbol: str) -> Ticker:
        """Fetches the ticker for a given symbol and ensures the symbol is uppercase."""
        ticker = super().fetch_ticker(symbol)
        ticker.symbol = ticker.symbol.upper()
        return ticker
