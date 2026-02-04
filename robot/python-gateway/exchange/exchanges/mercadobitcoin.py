from .base import Exchange, Ticker


class MercadoBitcoinExchange(Exchange):
    """MercadoBitcoin specific customizations."""

    def fetch_ticker(self, symbol: str) -> Ticker:
        """Fetches the ticker for a given symbol and ensures the symbol is uppercase."""
        ticker = super().fetch_ticker(symbol)
        # MercadoBitcoin sometimes uses different separators; normalize symbol
        ticker.symbol = ticker.symbol.replace('-', '/').upper()
        return ticker
