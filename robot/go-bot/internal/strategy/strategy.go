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

// StrategyState defines the internal lifecycle state of the strategy.
type StrategyState int

const (
	StateIdle        StrategyState = C.STATE_IDLE
	StatePendingBuy  StrategyState = C.STATE_PENDING_BUY
	StateActive      StrategyState = C.STATE_ACTIVE
	StatePendingSell StrategyState = C.STATE_PENDING_SELL
)

func (s StrategyState) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StatePendingBuy:
		return "pending_buy"
	case StateActive:
		return "active"
	case StatePendingSell:
		return "pending_sell"
	default:
		return "idle"
	}
}

// Signal defines the trading signal returned by the strategy.
type Signal int

const (
	SignalInvalid         Signal = C.SIGNAL_INVALID
	SignalBuy             Signal = C.SIGNAL_BUY
	SignalSell            Signal = C.SIGNAL_SELL
	SignalSearchingEntry  Signal = C.SIGNAL_SEARCHING_ENTRY
	SignalTrackingExit    Signal = C.SIGNAL_TRACKING_EXIT
	SignalWaitingBuyFill  Signal = C.SIGNAL_WAITING_BUY_FILL
	SignalWaitingSellFill Signal = C.SIGNAL_WAITING_SELL_FILL
)

func (s Signal) String() string {
	switch s {
	case SignalInvalid:
		return "invalid"
	case SignalBuy:
		return "buy"
	case SignalSell:
		return "sell"
	case SignalSearchingEntry:
		return "searching_entry"
	case SignalTrackingExit:
		return "tracking_exit"
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
		return nil, errors.New("failed to create strategy: invalid/unrecognized config type or invalid parameters")
	}
	return &Strategy{
		handle: handle,
		cfg:    cfg,
	}, nil
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

// GetConfig returns the configuration used to create this strategy.
func (s *Strategy) GetConfig() StrategyConfig {
	return s.cfg
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
	if count > C.MAX_MOMENTUM_WINDOWS {
		count = C.MAX_MOMENTUM_WINDOWS
	}
	cCfg.num_momentum_windows = C.int(count)

	for i := 0; i < count; i++ {
		cCfg.momentum_windows[i].lookback_seconds = C.longlong(cfg.MomentumWindows[i].LookbackSeconds)
		cCfg.momentum_windows[i].threshold = C.double(cfg.MomentumWindows[i].Threshold)
	}
	return cCfg
}

// Close releases the underlying C++ strategy handle and associated resources.
func (s *Strategy) Close() {
	if s.handle != nil {
		C.Strategy_Destroy(s.handle)
		s.handle = nil
	}
}

// InitProfit initializes the strategy state for profit-target based logic with historical data.
func (s *Strategy) InitProfit(ticks []PricePoint, state StrategyState, entryPrice float64) error {
	var cTicks *C.PricePoint
	count := len(ticks)
	if count > 0 {
		cTicks = (*C.PricePoint)(unsafe.Pointer(&ticks[0]))
	}

	res := C.Strategy_Init_Profit(s.handle, (*C.PricePoint)(cTicks), C.int(count), C.StrategyState(state), C.double(entryPrice))
	if res == C.STRATEGY_FAILURE {
		return errors.New("failed to initialize profit strategy: the history is not in chronological order")
	}
	return nil
}

// InitTrailing initializes the strategy state for trailing-stop based logic with historical data.
func (s *Strategy) InitTrailing(ticks []PricePoint, state StrategyState, entryPrice, highestPrice float64) error {
	var cTicks *C.PricePoint
	count := len(ticks)
	if count > 0 {
		cTicks = (*C.PricePoint)(unsafe.Pointer(&ticks[0]))
	}

	res := C.Strategy_Init_Trailing(s.handle, (*C.PricePoint)(cTicks), C.int(count), C.StrategyState(state), C.double(entryPrice), C.double(highestPrice))
	if res == C.STRATEGY_FAILURE {
		return errors.New("failed to initialize trailing strategy: the history is not in chronological order")
	}
	return nil
}

// UpdatePrice feeds a new price tick into the strategy engine.
func (s *Strategy) UpdatePrice(price float64, timestamp int64) error {
	res := C.Strategy_UpdatePrice(s.handle, C.double(price), C.longlong(timestamp))
	if res == C.STRATEGY_FAILURE {
		return errors.New("failed to update price: tick rejected (invalid timestamp, non-positive price, or unrealistic jump)")
	}
	return nil
}

// GetState returns the current internal state of the strategy machine.
func (s *Strategy) GetState() StrategyState {
	return StrategyState(C.Strategy_GetState(s.handle))
}

// GetSignal queries the strategy for the current trading signal (Buy, Sell, or Hold).
func (s *Strategy) GetSignal() Signal {
	return Signal(C.Strategy_GetSignal(s.handle))
}

// ConfirmSignal confirms that a pending signal has been filled.
func (s *Strategy) ConfirmSignal(signal Signal, fillPrice float64) {
	C.Strategy_ConfirmSignal(s.handle, C.Signal(signal), C.double(fillPrice))
}

// CancelSignal cancels a pending signal, returning the strategy to its previous state.
func (s *Strategy) CancelSignal(signal Signal) {
	C.Strategy_CancelSignal(s.handle, C.Signal(signal))
}

// ResetSignal explicitly returns the strategy to the IDLE state.
func (s *Strategy) ResetSignal() {
	C.Strategy_ResetSignal(s.handle)
}
