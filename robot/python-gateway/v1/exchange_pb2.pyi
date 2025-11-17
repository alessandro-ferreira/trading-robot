from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class OrderType(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    ORDER_TYPE_UNSPECIFIED: _ClassVar[OrderType]
    MARKET: _ClassVar[OrderType]
    LIMIT: _ClassVar[OrderType]

class OrderSide(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    ORDER_SIDE_UNSPECIFIED: _ClassVar[OrderSide]
    BUY: _ClassVar[OrderSide]
    SELL: _ClassVar[OrderSide]
ORDER_TYPE_UNSPECIFIED: OrderType
MARKET: OrderType
LIMIT: OrderType
ORDER_SIDE_UNSPECIFIED: OrderSide
BUY: OrderSide
SELL: OrderSide

class CreateOrderRequest(_message.Message):
    __slots__ = ("symbol", "type", "side", "amount", "price")
    SYMBOL_FIELD_NUMBER: _ClassVar[int]
    TYPE_FIELD_NUMBER: _ClassVar[int]
    SIDE_FIELD_NUMBER: _ClassVar[int]
    AMOUNT_FIELD_NUMBER: _ClassVar[int]
    PRICE_FIELD_NUMBER: _ClassVar[int]
    symbol: str
    type: OrderType
    side: OrderSide
    amount: float
    price: float
    def __init__(self, symbol: _Optional[str] = ..., type: _Optional[_Union[OrderType, str]] = ..., side: _Optional[_Union[OrderSide, str]] = ..., amount: _Optional[float] = ..., price: _Optional[float] = ...) -> None: ...

class CreateOrderResponse(_message.Message):
    __slots__ = ("order_id", "status", "timestamp")
    ORDER_ID_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    TIMESTAMP_FIELD_NUMBER: _ClassVar[int]
    order_id: str
    status: str
    timestamp: int
    def __init__(self, order_id: _Optional[str] = ..., status: _Optional[str] = ..., timestamp: _Optional[int] = ...) -> None: ...

class StreamTickerRequest(_message.Message):
    __slots__ = ("symbol",)
    SYMBOL_FIELD_NUMBER: _ClassVar[int]
    symbol: str
    def __init__(self, symbol: _Optional[str] = ...) -> None: ...

class Ticker(_message.Message):
    __slots__ = ("symbol", "timestamp", "last", "bid", "ask")
    SYMBOL_FIELD_NUMBER: _ClassVar[int]
    TIMESTAMP_FIELD_NUMBER: _ClassVar[int]
    LAST_FIELD_NUMBER: _ClassVar[int]
    BID_FIELD_NUMBER: _ClassVar[int]
    ASK_FIELD_NUMBER: _ClassVar[int]
    symbol: str
    timestamp: int
    last: float
    bid: float
    ask: float
    def __init__(self, symbol: _Optional[str] = ..., timestamp: _Optional[int] = ..., last: _Optional[float] = ..., bid: _Optional[float] = ..., ask: _Optional[float] = ...) -> None: ...

class FetchOHLCVRequest(_message.Message):
    __slots__ = ("symbol", "timeframe", "since", "limit")
    SYMBOL_FIELD_NUMBER: _ClassVar[int]
    TIMEFRAME_FIELD_NUMBER: _ClassVar[int]
    SINCE_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    symbol: str
    timeframe: str
    since: int
    limit: int
    def __init__(self, symbol: _Optional[str] = ..., timeframe: _Optional[str] = ..., since: _Optional[int] = ..., limit: _Optional[int] = ...) -> None: ...

class FetchOHLCVResponse(_message.Message):
    __slots__ = ("ohlcv",)
    OHLCV_FIELD_NUMBER: _ClassVar[int]
    ohlcv: _containers.RepeatedScalarFieldContainer[float]
    def __init__(self, ohlcv: _Optional[_Iterable[float]] = ...) -> None: ...
