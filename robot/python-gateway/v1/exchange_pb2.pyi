from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class PingRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class PingResponse(_message.Message):
    __slots__ = ("message",)
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    message: str
    def __init__(self, message: _Optional[str] = ...) -> None: ...

class GetTickerRequest(_message.Message):
    __slots__ = ("exchange", "symbol")
    EXCHANGE_FIELD_NUMBER: _ClassVar[int]
    SYMBOL_FIELD_NUMBER: _ClassVar[int]
    exchange: str
    symbol: str
    def __init__(self, exchange: _Optional[str] = ..., symbol: _Optional[str] = ...) -> None: ...

class TickerResponse(_message.Message):
    __slots__ = ("symbol", "price")
    SYMBOL_FIELD_NUMBER: _ClassVar[int]
    PRICE_FIELD_NUMBER: _ClassVar[int]
    symbol: str
    price: float
    def __init__(self, symbol: _Optional[str] = ..., price: _Optional[float] = ...) -> None: ...

class GetBalanceRequest(_message.Message):
    __slots__ = ("exchange", "currency")
    EXCHANGE_FIELD_NUMBER: _ClassVar[int]
    CURRENCY_FIELD_NUMBER: _ClassVar[int]
    exchange: str
    currency: str
    def __init__(self, exchange: _Optional[str] = ..., currency: _Optional[str] = ...) -> None: ...

class BalanceObject(_message.Message):
    __slots__ = ("asset", "free", "used", "total")
    ASSET_FIELD_NUMBER: _ClassVar[int]
    FREE_FIELD_NUMBER: _ClassVar[int]
    USED_FIELD_NUMBER: _ClassVar[int]
    TOTAL_FIELD_NUMBER: _ClassVar[int]
    asset: str
    free: float
    used: float
    total: float
    def __init__(self, asset: _Optional[str] = ..., free: _Optional[float] = ..., used: _Optional[float] = ..., total: _Optional[float] = ...) -> None: ...

class BalanceResponse(_message.Message):
    __slots__ = ("balances",)
    BALANCES_FIELD_NUMBER: _ClassVar[int]
    balances: _containers.RepeatedCompositeFieldContainer[BalanceObject]
    def __init__(self, balances: _Optional[_Iterable[_Union[BalanceObject, _Mapping]]] = ...) -> None: ...

class CreateOrderRequest(_message.Message):
    __slots__ = ("exchange", "symbol", "side", "type", "amount", "price")
    EXCHANGE_FIELD_NUMBER: _ClassVar[int]
    SYMBOL_FIELD_NUMBER: _ClassVar[int]
    SIDE_FIELD_NUMBER: _ClassVar[int]
    TYPE_FIELD_NUMBER: _ClassVar[int]
    AMOUNT_FIELD_NUMBER: _ClassVar[int]
    PRICE_FIELD_NUMBER: _ClassVar[int]
    exchange: str
    symbol: str
    side: str
    type: str
    amount: float
    price: float
    def __init__(self, exchange: _Optional[str] = ..., symbol: _Optional[str] = ..., side: _Optional[str] = ..., type: _Optional[str] = ..., amount: _Optional[float] = ..., price: _Optional[float] = ...) -> None: ...

class CreateStopOrderRequest(_message.Message):
    __slots__ = ("exchange", "symbol", "side", "amount", "stop_price", "limit_price")
    EXCHANGE_FIELD_NUMBER: _ClassVar[int]
    SYMBOL_FIELD_NUMBER: _ClassVar[int]
    SIDE_FIELD_NUMBER: _ClassVar[int]
    AMOUNT_FIELD_NUMBER: _ClassVar[int]
    STOP_PRICE_FIELD_NUMBER: _ClassVar[int]
    LIMIT_PRICE_FIELD_NUMBER: _ClassVar[int]
    exchange: str
    symbol: str
    side: str
    amount: float
    stop_price: float
    limit_price: float
    def __init__(self, exchange: _Optional[str] = ..., symbol: _Optional[str] = ..., side: _Optional[str] = ..., amount: _Optional[float] = ..., stop_price: _Optional[float] = ..., limit_price: _Optional[float] = ...) -> None: ...

class OrderResponse(_message.Message):
    __slots__ = ("id", "symbol", "side", "type", "amount", "price", "status", "filled", "remaining", "cost", "average", "client_order_id", "timestamp")
    ID_FIELD_NUMBER: _ClassVar[int]
    SYMBOL_FIELD_NUMBER: _ClassVar[int]
    SIDE_FIELD_NUMBER: _ClassVar[int]
    TYPE_FIELD_NUMBER: _ClassVar[int]
    AMOUNT_FIELD_NUMBER: _ClassVar[int]
    PRICE_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    FILLED_FIELD_NUMBER: _ClassVar[int]
    REMAINING_FIELD_NUMBER: _ClassVar[int]
    COST_FIELD_NUMBER: _ClassVar[int]
    AVERAGE_FIELD_NUMBER: _ClassVar[int]
    CLIENT_ORDER_ID_FIELD_NUMBER: _ClassVar[int]
    TIMESTAMP_FIELD_NUMBER: _ClassVar[int]
    id: str
    symbol: str
    side: str
    type: str
    amount: float
    price: float
    status: str
    filled: float
    remaining: float
    cost: float
    average: float
    client_order_id: str
    timestamp: int
    def __init__(self, id: _Optional[str] = ..., symbol: _Optional[str] = ..., side: _Optional[str] = ..., type: _Optional[str] = ..., amount: _Optional[float] = ..., price: _Optional[float] = ..., status: _Optional[str] = ..., filled: _Optional[float] = ..., remaining: _Optional[float] = ..., cost: _Optional[float] = ..., average: _Optional[float] = ..., client_order_id: _Optional[str] = ..., timestamp: _Optional[int] = ...) -> None: ...

class CancelOrderRequest(_message.Message):
    __slots__ = ("exchange", "id", "symbol")
    EXCHANGE_FIELD_NUMBER: _ClassVar[int]
    ID_FIELD_NUMBER: _ClassVar[int]
    SYMBOL_FIELD_NUMBER: _ClassVar[int]
    exchange: str
    id: str
    symbol: str
    def __init__(self, exchange: _Optional[str] = ..., id: _Optional[str] = ..., symbol: _Optional[str] = ...) -> None: ...

class CancelOrderResponse(_message.Message):
    __slots__ = ("id", "status")
    ID_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    id: str
    status: str
    def __init__(self, id: _Optional[str] = ..., status: _Optional[str] = ...) -> None: ...

class GetOrderRequest(_message.Message):
    __slots__ = ("exchange", "id", "symbol")
    EXCHANGE_FIELD_NUMBER: _ClassVar[int]
    ID_FIELD_NUMBER: _ClassVar[int]
    SYMBOL_FIELD_NUMBER: _ClassVar[int]
    exchange: str
    id: str
    symbol: str
    def __init__(self, exchange: _Optional[str] = ..., id: _Optional[str] = ..., symbol: _Optional[str] = ...) -> None: ...

class GetOpenOrdersRequest(_message.Message):
    __slots__ = ("exchange", "symbol", "limit")
    EXCHANGE_FIELD_NUMBER: _ClassVar[int]
    SYMBOL_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    exchange: str
    symbol: str
    limit: int
    def __init__(self, exchange: _Optional[str] = ..., symbol: _Optional[str] = ..., limit: _Optional[int] = ...) -> None: ...

class GetRecentTradesRequest(_message.Message):
    __slots__ = ("exchange", "symbol", "since", "limit")
    EXCHANGE_FIELD_NUMBER: _ClassVar[int]
    SYMBOL_FIELD_NUMBER: _ClassVar[int]
    SINCE_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    exchange: str
    symbol: str
    since: int
    limit: int
    def __init__(self, exchange: _Optional[str] = ..., symbol: _Optional[str] = ..., since: _Optional[int] = ..., limit: _Optional[int] = ...) -> None: ...

class OrdersResponse(_message.Message):
    __slots__ = ("orders",)
    ORDERS_FIELD_NUMBER: _ClassVar[int]
    orders: _containers.RepeatedCompositeFieldContainer[OrderResponse]
    def __init__(self, orders: _Optional[_Iterable[_Union[OrderResponse, _Mapping]]] = ...) -> None: ...

class ResetStateRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ResetStateResponse(_message.Message):
    __slots__ = ("status",)
    STATUS_FIELD_NUMBER: _ClassVar[int]
    status: str
    def __init__(self, status: _Optional[str] = ...) -> None: ...
