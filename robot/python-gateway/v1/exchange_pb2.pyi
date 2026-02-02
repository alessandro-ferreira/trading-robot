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
    __slots__ = ("symbol",)
    SYMBOL_FIELD_NUMBER: _ClassVar[int]
    symbol: str
    def __init__(self, symbol: _Optional[str] = ...) -> None: ...

class TickerResponse(_message.Message):
    __slots__ = ("symbol", "price")
    SYMBOL_FIELD_NUMBER: _ClassVar[int]
    PRICE_FIELD_NUMBER: _ClassVar[int]
    symbol: str
    price: float
    def __init__(self, symbol: _Optional[str] = ..., price: _Optional[float] = ...) -> None: ...

class GetBalanceRequest(_message.Message):
    __slots__ = ("currency",)
    CURRENCY_FIELD_NUMBER: _ClassVar[int]
    currency: str
    def __init__(self, currency: _Optional[str] = ...) -> None: ...

class BalanceResponse(_message.Message):
    __slots__ = ("free", "used", "total")
    class FreeEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: float
        def __init__(self, key: _Optional[str] = ..., value: _Optional[float] = ...) -> None: ...
    class UsedEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: float
        def __init__(self, key: _Optional[str] = ..., value: _Optional[float] = ...) -> None: ...
    class TotalEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: float
        def __init__(self, key: _Optional[str] = ..., value: _Optional[float] = ...) -> None: ...
    FREE_FIELD_NUMBER: _ClassVar[int]
    USED_FIELD_NUMBER: _ClassVar[int]
    TOTAL_FIELD_NUMBER: _ClassVar[int]
    free: _containers.ScalarMap[str, float]
    used: _containers.ScalarMap[str, float]
    total: _containers.ScalarMap[str, float]
    def __init__(self, free: _Optional[_Mapping[str, float]] = ..., used: _Optional[_Mapping[str, float]] = ..., total: _Optional[_Mapping[str, float]] = ...) -> None: ...

class CreateOrderRequest(_message.Message):
    __slots__ = ("symbol", "side", "type", "amount", "price")
    SYMBOL_FIELD_NUMBER: _ClassVar[int]
    SIDE_FIELD_NUMBER: _ClassVar[int]
    TYPE_FIELD_NUMBER: _ClassVar[int]
    AMOUNT_FIELD_NUMBER: _ClassVar[int]
    PRICE_FIELD_NUMBER: _ClassVar[int]
    symbol: str
    side: str
    type: str
    amount: float
    price: float
    def __init__(self, symbol: _Optional[str] = ..., side: _Optional[str] = ..., type: _Optional[str] = ..., amount: _Optional[float] = ..., price: _Optional[float] = ...) -> None: ...

class OrderResponse(_message.Message):
    __slots__ = ("id", "symbol", "side", "type", "amount", "price", "status", "filled", "remaining", "cost", "average")
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
    def __init__(self, id: _Optional[str] = ..., symbol: _Optional[str] = ..., side: _Optional[str] = ..., type: _Optional[str] = ..., amount: _Optional[float] = ..., price: _Optional[float] = ..., status: _Optional[str] = ..., filled: _Optional[float] = ..., remaining: _Optional[float] = ..., cost: _Optional[float] = ..., average: _Optional[float] = ...) -> None: ...

class CancelOrderRequest(_message.Message):
    __slots__ = ("id", "symbol")
    ID_FIELD_NUMBER: _ClassVar[int]
    SYMBOL_FIELD_NUMBER: _ClassVar[int]
    id: str
    symbol: str
    def __init__(self, id: _Optional[str] = ..., symbol: _Optional[str] = ...) -> None: ...

class CancelOrderResponse(_message.Message):
    __slots__ = ("id", "status")
    ID_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    id: str
    status: str
    def __init__(self, id: _Optional[str] = ..., status: _Optional[str] = ...) -> None: ...

class GetOrderRequest(_message.Message):
    __slots__ = ("id", "symbol")
    ID_FIELD_NUMBER: _ClassVar[int]
    SYMBOL_FIELD_NUMBER: _ClassVar[int]
    id: str
    symbol: str
    def __init__(self, id: _Optional[str] = ..., symbol: _Optional[str] = ...) -> None: ...

class GetOpenOrdersRequest(_message.Message):
    __slots__ = ("symbol",)
    SYMBOL_FIELD_NUMBER: _ClassVar[int]
    symbol: str
    def __init__(self, symbol: _Optional[str] = ...) -> None: ...

class OpenOrdersResponse(_message.Message):
    __slots__ = ("orders",)
    ORDERS_FIELD_NUMBER: _ClassVar[int]
    orders: _containers.RepeatedCompositeFieldContainer[OrderResponse]
    def __init__(self, orders: _Optional[_Iterable[_Union[OrderResponse, _Mapping]]] = ...) -> None: ...
