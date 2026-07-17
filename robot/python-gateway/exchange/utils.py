import ccxt
import grpc
import logging
import time
import traceback

from typing import Any

from exchange.factory import (
    ExchangeConfigurationError,
    ExchangeFactory,
    ExchangeNotConfigured,
)
from .exchanges.base import (
    Exchange,
    ExchangeNetworkError,
    AuthenticationError,
    InsufficientFundsError,
    BadRequestError,
)
from v1 import exchange_pb2

# Maximum number of stack trace entries (or frames) from the active exception.
TRACEBACK_LIMIT = 3


def get_exchange(
    factory: ExchangeFactory, request: Any, context: grpc.ServicerContext
) -> Exchange:
    """Retrieves the exchange instance based on the request."""
    ex_name = request.exchange
    if not ex_name:
        context.abort(grpc.StatusCode.INVALID_ARGUMENT, "Exchange name is required")
    try:
        return factory.get(ex_name)
    except ExchangeNotConfigured as e:
        logging.error(f"Exchange not configured: {e}")
        context.abort(grpc.StatusCode.NOT_FOUND, str(e))
    except ExchangeConfigurationError as e:
        logging.error(f"Exchange configuration error: {e}")
        context.abort(grpc.StatusCode.FAILED_PRECONDITION, str(e))


def retry_network_call(func, *args, **kwargs):
    """Internal helper to retry network-related exchange calls."""
    retries = 3
    delay = 0.5
    for i in range(retries):
        try:
            return func(*args, **kwargs)
        except (ccxt.NetworkError, ExchangeNetworkError) as e:
            if i == retries - 1:
                raise e
            logging.warning(
                f"Network error, retrying in {delay}s... " f"(Attempt {i+1}/{retries})"
            )
            time.sleep(delay)
            delay *= 2


def handle_exchange_error(context: grpc.ServicerContext, e: Exception, action: str):
    """Maps CCXT and requests exceptions to gRPC status codes."""
    if isinstance(e, (ccxt.NetworkError, ExchangeNetworkError)):
        logging.error(
            f"Network error during {action}: {traceback.format_exc(limit=TRACEBACK_LIMIT)}"
        )
        context.abort(grpc.StatusCode.UNAVAILABLE, f"Exchange network error: {e}")
    elif isinstance(e, (ccxt.AuthenticationError, AuthenticationError)):
        logging.error(f"Authentication error during {action}: {e}")
        context.abort(grpc.StatusCode.UNAUTHENTICATED, f"Auth failed: {e}")
    elif isinstance(e, (ccxt.InsufficientFunds, InsufficientFundsError)):
        logging.error(f"Insufficient funds during {action}: {e}")
        context.abort(grpc.StatusCode.FAILED_PRECONDITION, f"Insufficient funds: {e}")
    elif isinstance(e, (ccxt.InvalidOrder, ccxt.BadRequest, BadRequestError)):
        logging.error(f"Invalid parameters during {action}: {e}")
        context.abort(grpc.StatusCode.INVALID_ARGUMENT, f"Invalid parameters: {e}")
    else:
        logging.error(
            f"Internal error during {action}: {traceback.format_exc(limit=TRACEBACK_LIMIT)}"
        )
        context.abort(grpc.StatusCode.INTERNAL, f"Internal gateway error: {e}")


def map_order(
    order: dict, req: Any = None, is_trade: bool = False
) -> exchange_pb2.OrderResponse:
    """Maps a CCXT order or trade dictionary to a gRPC OrderResponse."""
    if not order:
        return exchange_pb2.OrderResponse()

    # Keep the original ID logic for trade executions vs standard orders
    if is_trade:
        oid = str(order.get("order") or order.get("id") or "")
    else:
        oid = str(order.get("id") or "")

    # Normalize Status
    status = str(order.get("status") or ("closed" if "order" in order else "open"))

    # Determine order properties (Stop vs Regular, Limit vs Market)
    raw_type = str(order.get("type") or "").lower()
    if not raw_type and req:
        raw_type = str(getattr(req, "type", "")).lower()

    is_limit = "limit" in raw_type
    is_stop = "stop" in raw_type or "trigger" in raw_type or hasattr(req, "stop_price")

    if is_stop:
        otype = "stop_limit" if is_limit else "stop_market"
        # For stop orders, the price field represents the trigger price
        price = float(
            order.get("triggerPrice")
            or order.get("stopPrice")
            or order.get("price")
            or 0.0
        )
        if price == 0 and req:
            price = float(getattr(req, "stop_price", 0.0))
    else:
        otype = "limit" if is_limit else "market"
        price = float(order.get("price") or getattr(req, "price", 0.0) or 0.0)

    # Handle fee information, which may be a dict or a simple value
    fee = order.get("fee")
    if isinstance(fee, dict):
        fee_cost = float(fee.get("cost") or 0.0)
        fee_currency = str(fee.get("currency") or "")
    else:
        fee_cost = float(fee or 0.0)
        fee_currency = str(order.get("fee_currency") or "")

    return exchange_pb2.OrderResponse(
        id=oid,
        symbol=order.get("symbol", getattr(req, "symbol", "")),
        side=order.get("side", getattr(req, "side", "")),
        type=otype,
        amount=float(order.get("amount") or getattr(req, "amount", 0.0)),
        price=price,
        status=status,
        filled=float(order.get("filled") or 0.0),
        remaining=float(order.get("remaining") or 0.0),
        cost=float(order.get("cost") or 0.0),
        average=float(order.get("average") or 0.0),
        client_order_id=str(order.get("clientOrderId") or ""),
        timestamp=int(order.get("timestamp") or 0),
        fee=fee_cost,
        fee_currency=fee_currency,
    )
