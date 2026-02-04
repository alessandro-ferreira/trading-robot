from .base import Exchange, Ticker


class CoinbaseExchange(Exchange):
    """Coinbase specific customizations."""

    def fetch_ticker(self, symbol: str) -> Ticker:
        """Fetches the ticker for a given symbol and ensures the symbol is uppercase."""
        ticker = super().fetch_ticker(symbol)
        # Coinbase tends to use uppercase with - or /; normalize to standard X/Y
        ticker.symbol = ticker.symbol.replace('-', '/').upper()
        return ticker
