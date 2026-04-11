package signal_generator

import (
	"fmt"
	"log/slog"

	"trading/robot/go-bot/internal/components/risk"
	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/strategy"
)

// SignalGenerator manages the data feed and strategy updates for a specific symbol.
type SignalGenerator struct {
	logger   *slog.Logger
	symbol   string
	exchange string
	risk     risk.PairRisk
	strategy *strategy.Strategy
}

// NewSignalGenerator creates a new SignalGenerator instance.
func NewSignalGenerator(logger *slog.Logger, riskData repository.RiskPair, strategyData repository.StrategyPair) (*SignalGenerator, error) {
	// Map application config to strategy config
	stratCfg := strategy.StrategyConfig{}

	logger.Info("Initializing strategy", "type", strategyData.Type, "symbol", strategyData.InstrumentSymbol, "exchange", strategyData.ExchangeName)

	switch strategyData.Type {
	case repository.StrategyMomentumTrailing, repository.StrategyMomentumProfit:
		stratCfg.WindowSeconds = strategyData.Momentum.WindowSeconds
		stratCfg.MomentumWindows = make([]strategy.MomentumWindow, len(strategyData.Momentum.Windows))
		for i, w := range strategyData.Momentum.Windows {
			stratCfg.MomentumWindows[i] = strategy.MomentumWindow{
				LookbackSeconds: w.LookbackSeconds,
				Threshold:       w.Threshold,
			}
		}
		stratCfg.MomentumRequireAll = strategyData.Momentum.RequireAll
		stratCfg.StopLossPct = strategyData.Momentum.StopLossPct

		windowDump := make([]string, len(stratCfg.MomentumWindows))
		for i, w := range stratCfg.MomentumWindows {
			windowDump[i] = fmt.Sprintf("{lookback: %ds, threshold: %.2f%%}", w.LookbackSeconds, w.Threshold*100)
		}

		if strategyData.Type == repository.StrategyMomentumTrailing {
			stratCfg.Type = strategy.StrategyMomentumTrailing
			stratCfg.ActivationPct = strategyData.Momentum.ActivationPct
			stratCfg.TrailingStopPct = strategyData.Momentum.TrailingStopPct

			logger.Info("Configured MomentumTrailing strategy",
				"window_seconds", stratCfg.WindowSeconds,
				"windows", windowDump,
				"require_all", stratCfg.MomentumRequireAll,
				"stop_loss_pct", stratCfg.StopLossPct,
				"activation_pct", stratCfg.ActivationPct,
				"trailing_stop_pct", stratCfg.TrailingStopPct,
			)
		} else {
			stratCfg.Type = strategy.StrategyMomentumProfit
			stratCfg.ProfitTargetPct = strategyData.Momentum.ProfitTargetPct

			logger.Info("Configured MomentumProfit strategy",
				"window_seconds", stratCfg.WindowSeconds,
				"windows", windowDump,
				"require_all", stratCfg.MomentumRequireAll,
				"stop_loss_pct", stratCfg.StopLossPct,
				"profit_target_pct", stratCfg.ProfitTargetPct,
			)
		}

	case repository.StrategyDummy:
		stratCfg.Type = strategy.StrategyDummy
		logger.Info("Configured Dummy strategy")

	default:
		return nil, fmt.Errorf("unsupported strategy type: %s", strategyData.Type)
	}

	strat, err := strategy.NewStrategy(stratCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize strategy: %w", err)
	}

	pairRisk := risk.PairRisk{
		RiskPerTrade:    riskData.RiskPerTrade,
		MaxPositionSize: riskData.MaxPositionSize.Float64,
	}

	return &SignalGenerator{
		logger:   logger.With("component", "signal_generator", "exchange", strategyData.ExchangeName, "symbol", strategyData.InstrumentSymbol),
		symbol:   strategyData.InstrumentSymbol,
		exchange: strategyData.ExchangeName,
		risk:     pairRisk,
		strategy: strat,
	}, nil
}

// Name returns the unique identifier for this generator.
func (s *SignalGenerator) Name() string {
	return "SignalGenerator-" + s.exchange + "-" + s.symbol
}

// Symbol returns the symbol managed by this generator.
func (s *SignalGenerator) Symbol() string {
	return s.symbol
}

// Exchange returns the exchange name for this generator.
func (s *SignalGenerator) Exchange() string {
	return s.exchange
}

// Risk returns the operational risk rules for this generator.
func (s *SignalGenerator) Risk() risk.PairRisk {
	return s.risk
}

// State returns the current internal state of the strategy engine.
func (s *SignalGenerator) State() strategy.StrategyState {
	return s.strategy.GetState()
}

// Warmup primes the strategy engine with historical market data.
func (s *SignalGenerator) Warmup(history []repository.MarketDataTick) error {
	if len(history) == 0 {
		return nil
	}

	points := make([]strategy.PricePoint, len(history))
	for i, tick := range history {
		points[i] = strategy.PricePoint{
			Timestamp: tick.TickUnixAt,
			Price:     tick.Price,
		}
	}

	cfg := s.strategy.GetConfig()
	var initErr error
	switch cfg.Type {
	case strategy.StrategyMomentumProfit:
		initErr = s.strategy.InitProfit(points, strategy.StateIdle, 0)
	case strategy.StrategyMomentumTrailing:
		initErr = s.strategy.InitTrailing(points, strategy.StateIdle, 0, 0)
	default:
		s.logger.Debug("Skipping warm-up: strategy type does not support history initialization")
		return nil
	}

	return initErr
}

// Sync performs a non-destructive initialization to restore strategy metadata.
// This is used during reconciliation or restarts to sync state without wiping history.
func (s *SignalGenerator) SyncState(state strategy.StrategyState, entryPrice, highestPrice float64) error {
	cfg := s.strategy.GetConfig()
	switch cfg.Type {
	case strategy.StrategyMomentumProfit:
		return s.strategy.InitProfit(nil, state, entryPrice)
	case strategy.StrategyMomentumTrailing:
		return s.strategy.InitTrailing(nil, state, entryPrice, highestPrice)
	default:
		// For Dummy or other types, we just ensure the state is consistent.
		return nil
	}
}

// Update updates the strategy state with a new price and returns the generated signal.
func (s *SignalGenerator) UpdatePrice(price float64, timestamp int64) (strategy.Signal, error) {
	// Update strategy with the new price
	if err := s.strategy.UpdatePrice(price, timestamp); err != nil {
		return 0, err
	}

	// Return the signal
	return s.strategy.GetSignal(), nil
}

// Confirm notifies the strategy that a pending signal (Buy or Sell) was successfully filled.
func (s *SignalGenerator) Confirm(sig strategy.Signal, price float64) {
	s.strategy.ConfirmSignal(sig, price)
}

// Cancel reverts a pending signal to its previous state if the execution was rejected or failed.
func (s *SignalGenerator) Cancel(sig strategy.Signal) {
	s.strategy.CancelSignal(sig)
}

// Reset forces the strategy engine back to an IDLE searching state.
func (s *SignalGenerator) Reset() {
	s.strategy.ResetSignal()
}

// Close releases resources held by the strategy.
func (s *SignalGenerator) Close() error {
	if s.strategy != nil {
		s.strategy.Close()
	}
	return nil
}
