package signal_generator

import (
	"fmt"
	"log/slog"
	"sync"

	"trading/robot/go-bot/internal/components/risk"
	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/strategy"
)

// SignalGenerator manages the data feed and strategy updates for a specific symbol.
type SignalGenerator struct {
	mu               sync.Mutex
	logger           *slog.Logger
	name             string
	exchange         string
	instrumentSymbol string
	risk             risk.PairRisk
	strategy         *strategy.Strategy
	pendingTerminate bool
}

// NewSignalGenerator creates a new SignalGenerator instance.
func NewSignalGenerator(
	logger *slog.Logger, riskData repository.RiskPair, strategyData repository.StrategyPair, name string,
) (*SignalGenerator, error) {
	stratCfg, err := mapConfig(logger, strategyData)
	if err != nil {
		return nil, err
	}

	strat, err := strategy.NewStrategy(stratCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize strategy: %w", err)
	}

	pairRisk := risk.PairRisk{
		InstrumentSymbol: riskData.InstrumentSymbol,
		AllocatedBudget:  riskData.AllocatedBudget,
		MaxAssetUnits:    riskData.MaxAssetUnits.Float64,
	}

	return &SignalGenerator{
		mu: sync.Mutex{},
		logger: logger.With(
			"component", "signal_generator",
			"exchange", strategyData.ExchangeName, "symbol", strategyData.InstrumentSymbol,
		),
		name:             name,
		exchange:         strategyData.ExchangeName,
		instrumentSymbol: strategyData.InstrumentSymbol,
		risk:             pairRisk,
		strategy:         strat,
	}, nil
}

// UpdateConfigFromPair updates the internal strategy parameters from a strategy pair record.
func (s *SignalGenerator) UpdateConfigFromPair(strategyData repository.StrategyPair) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	stratCfg, err := mapConfig(s.logger, strategyData)
	if err != nil {
		return err
	}

	// Update the C++ strategy engine hyperparameters without clearing history.
	if err := s.strategy.UpdateConfig(stratCfg); err != nil {
		return fmt.Errorf("failed to update strategy engine: %w", err)
	}

	// Update the termination flag based on the latest database status.
	s.pendingTerminate = (strategyData.Status == repository.StrategyPendingDisabled)

	return nil
}

// Name returns the unique identifier for this generator.
func (s *SignalGenerator) Name() string {
	return s.name
}

// Exchange returns the exchange name for this generator.
func (s *SignalGenerator) Exchange() string {
	return s.exchange
}

// Symbol returns the instrument symbol managed by this generator.
func (s *SignalGenerator) InstrumentSymbol() string {
	return s.instrumentSymbol
}

// Risk returns the operational risk rules for this generator.
func (s *SignalGenerator) Risk() risk.PairRisk {
	return s.risk
}

// StrategyConfig returns the current strategy configuration.
func (s *SignalGenerator) StrategyConfig() strategy.StrategyConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.strategy.GetConfig()
}

// IsPendingTerminate returns true if the strategy is scheduled to be disabled.
func (s *SignalGenerator) IsPendingTerminate() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pendingTerminate
}

// Warmup primes the strategy engine with historical market data.
func (s *SignalGenerator) Warmup(history []repository.MarketDataTick) error {
	s.mu.Lock()
	defer s.mu.Unlock()

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
		initErr = s.strategy.InitProfit(points, false, 0)
	case strategy.StrategyMomentumTrailing:
		initErr = s.strategy.InitTrailing(points, false, 0, 0)
	default:
		s.logger.Info("Skipping warm-up: strategy type does not support history initialization")
		return nil
	}

	return initErr
}

// SetPendingTerminate updates the termination flag.
func (s *SignalGenerator) SetPendingTerminate(flag bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingTerminate = flag
}

// SetInPosition updates strategy to in-position state with the given entry price and highest price since entry.
func (s *SignalGenerator) SetInPosition(inPosition bool, entryPrice, highestPrice float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.strategy.SetInPosition(inPosition, entryPrice, highestPrice)
}

// Update updates the strategy state with a new price and returns the generated signal.
func (s *SignalGenerator) GetSignal(price float64, timestamp int64) (strategy.StrategySignal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update strategy with the new price
	if err := s.strategy.UpdatePrice(price, timestamp); err != nil {
		return 0, err
	}

	// Return the signal
	return s.strategy.GetSignal(), nil
}

// RetrySignal should be used in case of error when placing an order.
func (s *SignalGenerator) RetrySignal(signal strategy.StrategySignal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.strategy.RetrySignal(signal)
}

// Close releases resources held by the strategy.
func (s *SignalGenerator) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.strategy != nil {
		s.strategy.Close()
	}
	return nil
}

// mapConfig translates database strategy metadata into the engine's StrategyConfig.
func mapConfig(
	logger *slog.Logger,
	strategyData repository.StrategyPair,
) (strategy.StrategyConfig, error) {
	// Map application config to strategy config
	stratCfg := strategy.StrategyConfig{}

	logger.Info(
		"Mapping strategy configuration", "type", strategyData.Type, "symbol", strategyData.InstrumentSymbol,
	)

	switch strategyData.Type {
	case repository.StrategyMomentumTrailing, repository.StrategyMomentumProfit:
		stratCfg.WindowSeconds = strategyData.Momentum.WindowSeconds

		windowCount := len(strategyData.Momentum.Windows)
		if windowCount > strategy.MaxMomentumWindows {
			windowCount = strategy.MaxMomentumWindows
		}

		stratCfg.MomentumWindows = make([]strategy.MomentumWindow, windowCount)
		for i, w := range strategyData.Momentum.Windows {
			if i >= windowCount {
				break
			}
			stratCfg.MomentumWindows[i] = strategy.MomentumWindow{
				LookbackSeconds: w.LookbackSeconds,
				Threshold:       w.Threshold,
			}
		}
		stratCfg.MomentumRequireAll = strategyData.Momentum.RequireAll
		stratCfg.StopLossPct = strategyData.Momentum.StopLossPct

		windowDump := make([]string, len(stratCfg.MomentumWindows))
		for i, w := range stratCfg.MomentumWindows {
			windowDump[i] = fmt.Sprintf(
				"{lookback: %ds, threshold: %.2f%%}", w.LookbackSeconds, w.Threshold*100,
			)
		}

		if strategyData.Type == repository.StrategyMomentumTrailing {
			stratCfg.Type = strategy.StrategyMomentumTrailing
			stratCfg.ActivationPct = strategyData.Momentum.ActivationPct.Float64
			stratCfg.TrailingStopPct = strategyData.Momentum.TrailingStopPct.Float64

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
			stratCfg.ProfitTargetPct = strategyData.Momentum.ProfitTargetPct.Float64

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
		return strategy.StrategyConfig{}, fmt.Errorf("unsupported strategy type: %s", strategyData.Type)
	}

	return stratCfg, nil
}
