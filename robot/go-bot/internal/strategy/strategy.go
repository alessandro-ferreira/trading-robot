package strategy

/*
#cgo CFLAGS: -I${SRCDIR}/../../../strategy-core/include
#cgo LDFLAGS: -L${SRCDIR}/../../../strategy-core/build -lstrategy -lstdc++ -lm
#include "trading/api.h"
*/
import "C"

// StrategyType defines the type of strategy to use.
type StrategyType int

const (
	StrategyDummy            StrategyType = C.STRATEGY_DUMMY
	StrategyMomentumProfit   StrategyType = C.STRATEGY_MOMENTUM_PROFIT
	StrategyMomentumTrailing StrategyType = C.STRATEGY_MOMENTUM_TRAILING
)

// MomentumWindow defines parameters for a single momentum condition.
type MomentumWindow struct {
	Lookback  int
	Threshold float64
}

// StrategyConfig mirrors the C struct for passing parameters.
type StrategyConfig struct {
	Type               StrategyType
	WindowSize         int
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

func NewStrategy(cfg StrategyConfig) *Strategy {
	cCfg := C.StrategyConfig{
		_type:             C.StrategyType(cfg.Type),
		window_size:       C.int(cfg.WindowSize),
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
		cCfg.momentum_windows[i].lookback = C.int(cfg.MomentumWindows[i].Lookback)
		cCfg.momentum_windows[i].threshold = C.double(cfg.MomentumWindows[i].Threshold)
	}

	return &Strategy{
		handle: C.Strategy_Create(cCfg),
	}
}

func (s *Strategy) Close() {
	if s.handle != nil {
		C.Strategy_Destroy(s.handle)
		s.handle = nil
	}
}

func (s *Strategy) UpdatePrice(price float64) {
	C.Strategy_UpdatePrice(s.handle, C.double(price))
}

func (s *Strategy) GetSignal() float64 {
	return float64(C.Strategy_GetSignal(s.handle))
}
