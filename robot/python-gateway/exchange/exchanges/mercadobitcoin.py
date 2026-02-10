import http.client
import json
import time
from typing import Any, Dict, List, Optional

import requests

from .base import Exchange, Ticker, ExchangeError


class MercadoBitcoinExchange(Exchange):
    """
    MercadoBitcoin implementation using native API v4 instead of ccxt.
    https://api.mercadobitcoin.net/api/v4/docs
    """

    BASE_URL = "https://api.mercadobitcoin.net/api/v4"
    
    PATH_OAUTH_TOKEN = "/authorize"
    PATH_ACCOUNTS = "/accounts"
    PATH_ACCOUNT_BALANCES = "/accounts/{}/balances"
    PATH_TICKERS = "/tickers"

    TIMEOUT = 10  # seconds
    
    def __init__(self, cfg=None):
        super().__init__(cfg)
        self._account_id: Optional[str] = None
        self._token: Optional[str] = None
        self._token_expiry: float = 0

    def _authenticate(self):
        """Authenticates using the API key and secret to obtain a Bearer token."""
        if not self._cfg or not self._cfg.secret or not self._cfg.api_key:
            raise ExchangeError("API key and Secret are required for MercadoBitcoin private API")

        url = f"{self.BASE_URL}{self.PATH_OAUTH_TOKEN}"
        payload = {
            "login": self._cfg.api_key,
            "password": self._cfg.secret
        }

        try:
            response = requests.post(url, json=payload, timeout=self.TIMEOUT)
            
            if response.status_code != http.client.OK:
                raise ExchangeError(f"Authentication failed: {response.status_code} - {response.text}")

            data = response.json()
            
            self._token = data.get('access_token')
            # Expiration is in seconds (e.g., 1800). Add buffer.
            self._token_expiry = time.time() + int(data.get('expiration', 1800)) - 60
        except ExchangeError:
            raise
        except Exception as e:
            raise ExchangeError(f"Authentication failed: {e}")

    def _request(self, method: str, path: str, data: Optional[Dict[str, Any]] = None) -> Any:
        if not self._token or time.time() >= self._token_expiry:
            self._authenticate()

        url = f"{self.BASE_URL}{path}"
        # Let requests handle JSON serialization by using the `json` parameter.
        headers = {'Authorization': f'Bearer {self._token}'}

        try:
            response = requests.request(method, url, headers=headers, json=data, timeout=self.TIMEOUT)
            
            if response.status_code == http.client.NO_CONTENT:
                return {}
            elif response.status_code not in [http.client.OK, http.client.CREATED]:
                raise ExchangeError(f"MercadoBitcoin API Error: {response.status_code} - {response.text}")

            return response.json()
        except requests.exceptions.RequestException as e:
            raise ExchangeError(f"Request failed: {e}")

    def _get_account_id(self) -> str:
        """Fetches and caches the account ID."""
        if self._account_id is None:
            try:
                data = self._request('GET', self.PATH_ACCOUNTS)
                # EAFP: Try to access the first element and its 'id' key.
                self._account_id = data[0]['id']
            except ExchangeError:
                raise
            except Exception:
                raise ExchangeError(f"Failed to parse account ID. Response: {data}")
        return self._account_id

    def _normalize_symbol(self, symbol: str) -> str:
        """Converts a symbol like 'BTC/BRL' to 'BTC-BRL'."""
        parts = symbol.split('/')
        if len(parts) != 2:
            raise ExchangeError(f"Invalid symbol format for MercadoBitcoin: {symbol}")
        return f"{parts[0]}-{parts[1]}"

    def fetch_ticker(self, symbol: str) -> Ticker:
        """Fetches the ticker for a given symbol using the public API."""
        pair = self._normalize_symbol(symbol)
        url = f"{self.BASE_URL}{self.PATH_TICKERS}?symbols={pair}"
        
        data = None
        try:
            response = requests.get(url, timeout=self.TIMEOUT)
            
            if response.status_code != http.client.OK:
                raise ExchangeError(f"MercadoBitcoin API Error: {response.status_code} - {response.text}")

            data = response.json()
            ticker_data = data[0]
            
            return Ticker(
                symbol=symbol,
                last=float(ticker_data['last']),
                bid=float(ticker_data['buy']) if ticker_data.get('buy') else None,
                ask=float(ticker_data['sell']) if ticker_data.get('sell') else None,
                timestamp=int(int(ticker_data['date']) / 1000000), # Convert ns to ms
                info=ticker_data
            )

        except ExchangeError:
            raise
        except requests.exceptions.RequestException as e:
            raise ExchangeError(f"Request failed: {e}")
        except Exception:
            raise ExchangeError(f"Failed to parse ticker for {symbol}. Response: {data}")

    def fetch_balance(self) -> Dict[str, Dict[str, float]]:
        """Fetches account balances."""
        account_id = self._get_account_id()
        path = self.PATH_ACCOUNT_BALANCES.format(account_id)
        balances = self._request('GET', path)
        
        result = {
            'free': {},
            'used': {},
            'total': {}
        }

        for b in balances:
            currency = b['symbol'].upper()
            available = float(b['available'])
            used = float(b['on_hold'])
            total = float(b['total'])
            
            result['free'][currency] = available
            result['used'][currency] = used
            result['total'][currency] = total
                
        return result
