package signal_generator

import (
	"fmt"
	"log/slog"

	"trading/robot/go-bot/internal/components/risk"
	"trading/robot/go-bot/internal/config"
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
func NewSignalGenerator(logger *slog.Logger, symbol, exchange string, riskCfg config.PairRiskConfig, strategyCfg config.StrategyConfig) (*SignalGenerator, error) {
	// Map application config to strategy config
	stratCfg := strategy.StrategyConfig{}

	logger.Info("Initializing strategy", "type", strategyCfg.Type, "symbol", symbol, "exchange", exchange)

	switch strategyCfg.Type {
	case config.StrategyMomentumTrailing:
		stratCfg.Type = strategy.StrategyMomentumTrailing
		stratCfg.WindowSeconds = strategyCfg.Momentum.WindowSeconds
		// In the future, this could support multiple momentum windows from config
		stratCfg.MomentumWindows = []strategy.MomentumWindow{{
			LookbackSeconds: strategyCfg.Momentum.LookbackSeconds,
			Threshold:       strategyCfg.Momentum.Threshold,
		}}
		stratCfg.MomentumRequireAll = strategyCfg.Momentum.RequireAll
		stratCfg.StopLossPct = strategyCfg.Momentum.StopLossPct
		stratCfg.ActivationPct = strategyCfg.Momentum.ActivationPct
		stratCfg.TrailingStopPct = strategyCfg.Momentum.TrailingStopPct

		logger.Info("Configured MomentumTrailing strategy",
			"window_seconds", stratCfg.WindowSeconds,
			"lookback_seconds", stratCfg.MomentumWindows[0].LookbackSeconds,
			"threshold", stratCfg.MomentumWindows[0].Threshold,
			"require_all", stratCfg.MomentumRequireAll,
			"stop_loss_pct", stratCfg.StopLossPct,
			"activation_pct", stratCfg.ActivationPct,
			"trailing_stop_pct", stratCfg.TrailingStopPct,
		)
	case config.StrategyMomentumProfit:
		stratCfg.Type = strategy.StrategyMomentumProfit
		stratCfg.WindowSeconds = strategyCfg.Momentum.WindowSeconds
		stratCfg.MomentumWindows = []strategy.MomentumWindow{{
			LookbackSeconds: strategyCfg.Momentum.LookbackSeconds,
			Threshold:       strategyCfg.Momentum.Threshold,
		}}
		stratCfg.MomentumRequireAll = strategyCfg.Momentum.RequireAll
		stratCfg.StopLossPct = strategyCfg.Momentum.StopLossPct
		stratCfg.ProfitTargetPct = strategyCfg.Momentum.ProfitTargetPct

		logger.Info("Configured MomentumProfit strategy",
			"window_seconds", stratCfg.WindowSeconds,
			"lookback_seconds", stratCfg.MomentumWindows[0].LookbackSeconds,
			"threshold", stratCfg.MomentumWindows[0].Threshold,
			"require_all", stratCfg.MomentumRequireAll,
			"stop_loss_pct", stratCfg.StopLossPct,
			"profit_target_pct", stratCfg.ProfitTargetPct,
		)
	case config.StrategyDummy:
		stratCfg.Type = strategy.StrategyDummy

		logger.Info("Configured Dummy strategy")
	default:
		return nil, fmt.Errorf("unsupported strategy type: %s", strategyCfg.Type)
	}

	strat, err := strategy.NewStrategy(stratCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize strategy: %w", err)
	}

	pairRisk := risk.PairRisk{
		RiskPerTrade: riskCfg.RiskPerTrade,
	}

	return &SignalGenerator{
		logger:   logger.With("component", "signal_generator", "exchange", exchange, "symbol", symbol),
		symbol:   symbol,
		exchange: exchange,
		risk:     pairRisk,
		strategy: strat,
	}, nil
}

// Name returns the name of the background task.
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

// UpdateAndGetSignal updates the strategy state with a new price and returns the signal.
func (s *SignalGenerator) UpdateAndGetSignal(price float64, timestamp int64) (strategy.Signal, error) {
	// Update strategy with the new price
	if err := s.strategy.UpdatePrice(price, timestamp); err != nil {
		return 0, err
	}

	// Return the signal
	return s.strategy.GetSignal(), nil
}

// Close releases resources held by the strategy.
func (s *SignalGenerator) Close() error {
	if s.strategy != nil {
		s.strategy.Close()
	}
	return nil
}
