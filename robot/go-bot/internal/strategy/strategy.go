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
)

// Signal defines the trading signal returned by the strategy.
type Signal int

const (
	SignalBuy  Signal = C.SIGNAL_BUY
	SignalSell Signal = C.SIGNAL_SELL
	SignalHold Signal = C.SIGNAL_HOLD
)

// MomentumWindow defines parameters for a single momentum condition.
type MomentumWindow struct {
	LookbackSeconds int64
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
	WindowSeconds      int64
	MomentumWindows    []MomentumWindow
	MomentumRequireAll bool // true = AND, false = OR
	StopLossPct        float64
	ProfitTargetPct    float64
	ActivationPct      float64
	TrailingStopPct    float64
}

type Strategy struct {
	handle C.StrategyHandle
}

func NewStrategy(cfg StrategyConfig) (*Strategy, error) {
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

	// Handle Momentum Windows array
	count := len(cfg.MomentumWindows)
	if count > C.MAX_MOMENTUM_WINDOWS {
		count = C.MAX_MOMENTUM_WINDOWS
	}
	cCfg.num_momentum_windows = C.int(count)

	for i := 0; i < count; i++ {
		cCfg.momentum_windows[i].lookback_seconds = C.longlong(cfg.MomentumWindows[i].LookbackSeconds)
		cCfg.momentum_windows[i].threshold = C.double(cfg.MomentumWindows[i].Threshold)
	}

	handle := C.Strategy_Create(cCfg)
	if handle == nil {
		return nil, errors.New("failed to create strategy: invalid/unrecognized config type or invalid parameters")
	}
	return &Strategy{
		handle: handle,
	}, nil
}

func (s *Strategy) Close() {
	if s.handle != nil {
		C.Strategy_Destroy(s.handle)
		s.handle = nil
	}
}

func (s *Strategy) InitProfit(ticks []PricePoint, inPosition bool, entryPrice float64) error {
	var cTicks *C.PricePoint
	count := len(ticks)
	if count > 0 {
		cTicks = (*C.PricePoint)(unsafe.Pointer(&ticks[0]))
	}

	var cInPosition C.int
	if inPosition {
		cInPosition = 1
	}

	res := C.Strategy_Init_Profit(s.handle, (*C.PricePoint)(cTicks), C.int(count), cInPosition, C.double(entryPrice))
	if res == C.STRATEGY_FAILURE {
		return errors.New("failed to initialize profit strategy: the history is not in chronological order")
	}
	return nil
}

func (s *Strategy) InitTrailing(ticks []PricePoint, inPosition bool, entryPrice, highestPrice float64) error {
	var cTicks *C.PricePoint
	count := len(ticks)
	if count > 0 {
		cTicks = (*C.PricePoint)(unsafe.Pointer(&ticks[0]))
	}

	var cInPosition C.int
	if inPosition {
		cInPosition = 1
	}

	res := C.Strategy_Init_Trailing(s.handle, (*C.PricePoint)(cTicks), C.int(count), cInPosition, C.double(entryPrice), C.double(highestPrice))
	if res == C.STRATEGY_FAILURE {
		return errors.New("failed to initialize trailing strategy: the history is not in chronological order")
	}
	return nil
}

func (s *Strategy) UpdatePrice(price float64, timestamp int64) error {
	res := C.Strategy_UpdatePrice(s.handle, C.double(price), C.longlong(timestamp))
	if res == C.STRATEGY_FAILURE {
		return errors.New("failed to update price: tick rejected (invalid timestamp, non-positive price, or unrealistic jump)")
	}
	return nil
}

func (s *Strategy) GetSignal() Signal {
	signal := Signal(C.Strategy_GetSignal(s.handle))
	switch signal {
	case SignalBuy, SignalSell, SignalHold:
		return signal
	default:
		// This case should not be reached if the C++ core is correct.
		// Safely default to HOLD.
		return SignalHold
	}
}
