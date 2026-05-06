package strategy

/*
#cgo CFLAGS: -I${SRCDIR}/../../../strategy-core/include
#cgo LDFLAGS: -L${SRCDIR}/../../../strategy-core/build -lstrategy -lstdc++ -lm
#include "trading/api.h"
*/
import "C"
import (
	"errors"
	"unsafe"
)

// StrategyType defines the type of strategy to use.
type StrategyType int

const (
	StrategyDummy            StrategyType = C.STRATEGY_DUMMY
	StrategyMomentumProfit   StrategyType = C.STRATEGY_MOMENTUM_PROFIT
	StrategyMomentumTrailing StrategyType = C.STRATEGY_MOMENTUM_TRAILING

	MaxMomentumWindows = C.MAX_MOMENTUM_WINDOWS
)

// Signal defines the trading signal returned by the strategy.
type StrategySignal int

const (
	SignalInvalid           StrategySignal = C.SIGNAL_INVALID
	SignalBuy               StrategySignal = C.SIGNAL_BUY
	SignalSell              StrategySignal = C.SIGNAL_SELL
	SignalSearchingBuyEntry StrategySignal = C.SIGNAL_SEARCHING_BUY_ENTRY
	SignalTrackingSellExit  StrategySignal = C.SIGNAL_TRACKING_SELL_EXIT
	SignalWaitingBuyFill    StrategySignal = C.SIGNAL_WAITING_BUY_FILL
	SignalWaitingSellFill   StrategySignal = C.SIGNAL_WAITING_SELL_FILL
)

func (s StrategySignal) String() string {
	switch s {
	case SignalInvalid:
		return "invalid"
	case SignalBuy:
		return "buy"
	case SignalSell:
		return "sell"
	case SignalSearchingBuyEntry:
		return "searching_buy_entry"
	case SignalTrackingSellExit:
		return "tracking_sell_exit"
	case SignalWaitingBuyFill:
		return "waiting_buy_fill"
	case SignalWaitingSellFill:
		return "waiting_sell_fill"
	default:
		return "invalid"
	}
}

// MomentumWindow defines parameters for a single momentum condition.
type MomentumWindow struct {
	LookbackSeconds int
	Threshold       float64
}

// PricePoint mirrors the C struct for passing historical price data.
type PricePoint struct {
	Timestamp int64
	Price     float64
}

// StrategyConfig mirrors the C struct for passing parameters.
type StrategyConfig struct {
	Type               StrategyType
	WindowSeconds      int
	MomentumWindows    []MomentumWindow
	MomentumRequireAll bool // true = AND, false = OR
	StopLossPct        float64
	ProfitTargetPct    float64
	ActivationPct      float64
	TrailingStopPct    float64
}

// Strategy wraps a C++ strategy handle and its configuration.
type Strategy struct {
	handle C.StrategyHandle
	cfg    StrategyConfig
}

// NewStrategy creates a new Strategy instance based on the provided configuration.
func NewStrategy(cfg StrategyConfig) (*Strategy, error) {
	cCfg := toCConfig(cfg)
	handle := C.Strategy_Create(cCfg)
	if handle == nil {
		return nil, errors.New(
			"failed to create strategy: invalid/unrecognized config type or invalid parameters",
		)
	}
	return &Strategy{
		handle: handle,
		cfg:    cfg,
	}, nil
}

// Close releases the underlying C++ strategy handle and associated resources.
func (s *Strategy) Close() {
	if s.handle != nil {
		C.Strategy_Destroy(s.handle)
		s.handle = nil
	}
}

// GetConfig returns the configuration used to create this strategy.
func (s *Strategy) GetConfig() StrategyConfig {
	return s.cfg
}

// UpdateConfig updates the internal strategy parameters without wiping history.
func (s *Strategy) UpdateConfig(cfg StrategyConfig) error {
	cCfg := toCConfig(cfg)
	res := C.Strategy_UpdateConfig(s.handle, cCfg)
	if res == C.STRATEGY_FAILURE {
		return errors.New("failed to update strategy config: invalid parameters for current engine")
	}
	s.cfg = cfg
	return nil
}

// InitProfit initializes the strategy state for profit-target based logic with historical data.
func (s *Strategy) InitProfit(ticks []PricePoint, inPosition bool, entryPrice float64) error {
	var cTicks *C.PricePoint
	count := len(ticks)
	if count > 0 {
		cTicks = (*C.PricePoint)(unsafe.Pointer(&ticks[0]))
	}

	var inPos C.int = 0
	if inPosition {
		inPos = 1
	}
	res := C.Strategy_Init_Profit(
		s.handle, (*C.PricePoint)(cTicks), C.int(count), inPos, C.double(entryPrice),
	)
	if res == C.STRATEGY_FAILURE {
		return errors.New(
			"failed to initialize profit strategy: the history is not in chronological order",
		)
	}
	return nil
}

// InitTrailing initializes the strategy state for trailing-stop based logic with historical data.
func (s *Strategy) InitTrailing(
	ticks []PricePoint,
	inPosition bool,
	entryPrice, highestPrice float64,
) error {
	var cTicks *C.PricePoint
	count := len(ticks)
	if count > 0 {
		cTicks = (*C.PricePoint)(unsafe.Pointer(&ticks[0]))
	}

	var inPos C.int = 0
	if inPosition {
		inPos = 1
	}
	res := C.Strategy_Init_Trailing(
		s.handle, (*C.PricePoint)(cTicks), C.int(count), inPos, C.double(entryPrice), C.double(highestPrice),
	)
	if res == C.STRATEGY_FAILURE {
		return errors.New(
			"failed to initialize trailing strategy: the history is not in chronological order",
		)
	}
	return nil
}

// SetInPosition sets strategy in-position state with the given entry price and highest price since entry.
func (s *Strategy) SetInPosition(inPosition bool, entryPrice, highestPrice float64) {
	var inPos C.int = 0
	if inPosition {
		inPos = 1
	}
	C.Strategy_SetInPosition(s.handle, inPos, C.double(entryPrice), C.double(highestPrice))
}

// UpdatePrice feeds a new price tick into the strategy engine.
func (s *Strategy) UpdatePrice(price float64, timestamp int64) error {
	res := C.Strategy_UpdatePrice(s.handle, C.double(price), C.longlong(timestamp))
	if res == C.STRATEGY_FAILURE {
		return errors.New(
			"failed to update price: tick rejected (invalid timestamp, non-positive price, or unrealistic jump)",
		)
	}
	return nil
}

// GetSignal queries the strategy for the current trading signal.
func (s *Strategy) GetSignal() StrategySignal {
	return StrategySignal(C.Strategy_GetSignal(s.handle))
}

// RetrySignal should be used in case of error when placing an order.
func (s *Strategy) RetrySignal(signal StrategySignal) error {
	if signal != SignalBuy && signal != SignalSell {
		return errors.New("invalid signal for retry: only BUY or SELL can be retried")
	}
	C.Strategy_RetrySignal(s.handle, C.StrategySignal(signal))
	return nil
}

// toCConfig maps the Go StrategyConfig to the C struct.
func toCConfig(cfg StrategyConfig) C.StrategyConfig {
	cCfg := C.StrategyConfig{
		_type:             C.StrategyType(cfg.Type),
		window_seconds:    C.longlong(cfg.WindowSeconds),
		stop_loss_pct:     C.double(cfg.StopLossPct),
		profit_target_pct: C.double(cfg.ProfitTargetPct),
		activation_pct:    C.double(cfg.ActivationPct),
		trailing_stop_pct: C.double(cfg.TrailingStopPct),
	}

	if cfg.MomentumRequireAll {
		cCfg.momentum_require_all = 1
	} else {
		cCfg.momentum_require_all = 0
	}

	count := len(cfg.MomentumWindows)
	if count > MaxMomentumWindows {
		count = MaxMomentumWindows
	}
	cCfg.num_momentum_windows = C.int(count)

	for i := 0; i < count; i++ {
		cCfg.momentum_windows[i].lookback_seconds = C.longlong(cfg.MomentumWindows[i].LookbackSeconds)
		cCfg.momentum_windows[i].threshold = C.double(cfg.MomentumWindows[i].Threshold)
	}
	return cCfg
}
