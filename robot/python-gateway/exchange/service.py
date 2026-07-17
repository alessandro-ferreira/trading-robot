import grpc
import logging

from typing import Any

from . import utils
from .exchanges import SUPPORTED_ASSETS
from core.config import Config
from exchange.factory import ExchangeFactory
from v1 import exchange_pb2
from v1 import exchange_pb2_grpc


class ExchangeService(exchange_pb2_grpc.ExchangeServiceServicer):
    """
    Implements the gRPC service for the exchange gateway.
    This class handles incoming gRPC requests and interacts with the ExchangeFactory
    to perform operations on the configured exchanges.
    """

    def __init__(self, cfg: Config, factory: ExchangeFactory):
        """Initializes the service with configuration and exchange factory."""
        self.cfg = cfg
        self.factory = factory
        # initialize all exchanges at startup to catch configuration errors early
        for exchange in self.cfg.exchanges:
            try:
                self.factory.get(exchange.name)
                logging.info(f"Successfully initialized exchange: {exchange.name}")
            except Exception as e:
                logging.exception(f"Failed to initialize exchange {exchange.name}: {e}")

    def Ping(
        self, request: Any, context: grpc.ServicerContext
    ) -> exchange_pb2.PingResponse:
        """Handles the Ping RPC. This is a simple health check."""
        logging.info("Ping: received")
        return exchange_pb2.PingResponse(message="Pong from Python gateway!")

    def GetTicker(
        self, request: Any, context: grpc.ServicerContext
    ) -> exchange_pb2.TickerResponse:
        """Handles the GetTicker RPC."""
        logging.debug(f"GetTicker: {request.exchange} {request.symbol}")
        exchange = utils.get_exchange(self.factory, request, context)
        ticker, price = None, None
        try:
            ticker = utils.retry_network_call(exchange.fetch_ticker, request.symbol)
            price = float(ticker.last)
        except Exception as e:
            utils.handle_exchange_error(context, e, "fetching ticker")

        logging.info(
            f"GetTicker Success: {request.exchange} symbol={request.symbol} price={price}"
        )
        logging.debug(f"raw_ticker: {ticker}")
        return exchange_pb2.TickerResponse(symbol=ticker.symbol, price=price)

    def GetBalance(
        self, request: Any, context: grpc.ServicerContext
    ) -> exchange_pb2.BalanceResponse:
        """Handles the GetBalance RPC."""
        logging.info(
            f"GetBalance: {request.exchange} asset={request.currency or 'ALL'}"
        )
        exchange = utils.get_exchange(self.factory, request, context)
        balances = []
        try:
            balance = utils.retry_network_call(exchange.fetch_balance)
            free_map = balance.get("free", {})
            used_map = balance.get("used", {})
            total_map = balance.get("total", {})

            balances = []
            # Use the total map as the source of truth for assets returned by the exchange
            for asset, total_val in total_map.items():
                if asset not in SUPPORTED_ASSETS:
                    continue

                f = float(free_map.get(asset, 0.0))
                u = float(used_map.get(asset, 0.0))
                t = float(total_val)

                balances.append(
                    exchange_pb2.BalanceObject(asset=asset, free=f, used=u, total=t)
                )
        except Exception as e:
            utils.handle_exchange_error(context, e, "fetching balance")

        logging.info(
            f"GetBalance Success: {request.exchange} assets_count={len(balances)}"
        )

        # Filter raw balance for logging to include only supported assets.
        pruned = {
            cat: {a: v for a, v in d.items() if a in SUPPORTED_ASSETS}
            for cat, d in balance.items()
            if isinstance(d, dict) and cat in ("free", "used", "total")
        }
        logging.info(f"raw_balance: {pruned}")
        return exchange_pb2.BalanceResponse(balances=balances)

    def CreateOrder(
        self, request: Any, context: grpc.ServicerContext
    ) -> exchange_pb2.OrderResponse:
        """Handles the CreateOrder RPC."""
        p_val = f" price={request.price}" if request.HasField("price") else ""
        logging.info(
            f"CreateOrder: {request.exchange} sym={request.symbol} "
            f"side={request.side} type={request.type} qty={request.amount}{p_val}"
        )

        exchange = utils.get_exchange(self.factory, request, context)
        order = None
        try:
            order = exchange.create_order(
                symbol=request.symbol,
                type=request.type,
                side=request.side,
                amount=request.amount,
                price=request.price if request.HasField("price") else None,
            )
        except Exception as e:
            utils.handle_exchange_error(context, e, "creating order")

        logging.info(
            f"CreateOrder Success: {request.exchange} id={order.get('id')} "
            f"status={order.get('status')} fill={order.get('filled', 0.0)}"
        )
        logging.info(f"raw_order: {order}")
        return utils.map_order(order, request)

    def CreateStopOrder(
        self, request: Any, context: grpc.ServicerContext
    ) -> exchange_pb2.OrderResponse:
        """Handles the CreateStopOrder RPC."""
        l_val = (
            f" limit={request.limit_price}" if request.HasField("limit_price") else ""
        )
        logging.info(
            f"CreateStopOrder: {request.exchange} sym={request.symbol} "
            f"side={request.side} qty={request.amount} stop={request.stop_price}{l_val}"
        )

        exchange = utils.get_exchange(self.factory, request, context)
        order = None
        try:
            order = exchange.create_stop_order(
                symbol=request.symbol,
                side=request.side,
                amount=request.amount,
                stop_price=request.stop_price,
                limit_price=request.limit_price
                if request.HasField("limit_price")
                else None,
            )
        except Exception as e:
            utils.handle_exchange_error(context, e, "creating stop order")

        logging.info(
            f"CreateStopOrder Success: {request.exchange} id={order.get('id')} "
            f"status={order.get('status')} fill={order.get('filled', 0.0)}"
        )
        logging.info(f"raw_stop_order: {order}")
        return utils.map_order(order, request)

    def CancelOrder(
        self, request: Any, context: grpc.ServicerContext
    ) -> exchange_pb2.CancelOrderResponse:
        """Handles the CancelOrder RPC."""
        logging.info(
            f"CancelOrder: {request.exchange} id={request.id} sym={request.symbol}"
        )
        exchange = utils.get_exchange(self.factory, request, context)
        result = {}
        try:
            result = exchange.cancel_order(request.id, symbol=request.symbol)
        except Exception as e:
            utils.handle_exchange_error(context, e, "canceling order")

        logging.info(
            f"CancelOrder Success: {request.exchange} id={request.id} status={result.get('status')}"
        )
        logging.info(f"raw_response: {result}")
        return exchange_pb2.CancelOrderResponse(
            id=str(result.get("id", request.id)), status=result.get("status", "")
        )

    def GetOrder(
        self, request: Any, context: grpc.ServicerContext
    ) -> exchange_pb2.OrderResponse:
        """Handles the GetOrder RPC."""
        logging.info(f"GetOrder: {request.exchange} id={request.id}")
        exchange = utils.get_exchange(self.factory, request, context)
        order = None
        try:
            order = utils.retry_network_call(
                exchange.fetch_order, request.id, symbol=request.symbol
            )
        except Exception as e:
            utils.handle_exchange_error(context, e, "fetching order")

        logging.info(
            f"GetOrder Success: {request.exchange} id={order.get('id')} "
            f"status={order.get('status')} fill={order.get('filled', 0.0)}"
        )
        logging.info(f"raw_order: {order}")
        return utils.map_order(order, request)

    def GetOpenOrders(
        self, request: Any, context: grpc.ServicerContext
    ) -> exchange_pb2.OrdersResponse:
        """Handles the GetOpenOrders RPC."""
        logging.info(f"GetOpenOrders: {request.exchange} sym={request.symbol or 'ALL'}")
        exchange = utils.get_exchange(self.factory, request, context)
        orders = []
        try:
            symbol = request.symbol if request.symbol else None
            limit = request.limit if request.limit > 0 else None
            orders = utils.retry_network_call(
                exchange.fetch_open_orders, symbol, limit=limit
            )
        except Exception as e:
            utils.handle_exchange_error(context, e, "fetching open orders")

        logging.info(f"GetOpenOrders Success: {request.exchange} count={len(orders)}")
        return exchange_pb2.OrdersResponse(
            orders=[utils.map_order(o, request) for o in orders[:limit]]
        )

    def GetRecentTrades(
        self, request: Any, context: grpc.ServicerContext
    ) -> exchange_pb2.OrdersResponse:
        """Handles the GetRecentTrades RPC. Fetches historical executions."""
        logging.info(
            f"GetRecentTrades: {request.exchange} {request.symbol or 'ALL'} "
            f"since:{request.since} limit:{request.limit}"
        )
        exchange = utils.get_exchange(self.factory, request, context)
        trades = []
        try:
            symbol = request.symbol if request.symbol else None
            since = request.since if request.since > 0 else None
            limit = request.limit if request.limit > 0 else None
            trades = utils.retry_network_call(
                exchange.fetch_my_trades, symbol, since=since, limit=limit
            )
        except Exception as e:
            utils.handle_exchange_error(context, e, "fetching recent trades")

        logging.info(f"GetRecentTrades Success: {request.exchange} count={len(trades)}")
        return exchange_pb2.OrdersResponse(
            orders=[utils.map_order(t, request, True) for t in trades[:limit]]
        )

    def ResetState(
        self, request: Any, context: grpc.ServicerContext
    ) -> exchange_pb2.ResetStateResponse:
        """Resets the state of the dummy exchange for testing purposes."""
        logging.info("Received ResetState request.")

        try:
            exchange = self.factory.get("dummy")
            exchange.reset()  # Directly call reset, will raise AttributeError if not present
            logging.info("Dummy exchange state has been reset.")
            return exchange_pb2.ResetStateResponse(status="OK")
        except Exception as e:
            logging.warning(f"ResetState ignored: {e}")

        return exchange_pb2.ResetStateResponse(status="IGNORED")
