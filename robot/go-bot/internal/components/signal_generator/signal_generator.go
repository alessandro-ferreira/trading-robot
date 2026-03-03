package signal_generator

import (
	"context"
	"log/slog"
	"time"

	pb "trading/robot/go-bot/gen/go/v1"
	"trading/robot/go-bot/internal/config"
	"trading/robot/go-bot/internal/strategy"
)

// MarketDataProvider defines the interface for fetching market data.
// This allows us to mock the GatewayClient for testing.
type MarketDataProvider interface {
	GetTicker(ctx context.Context, symbol, exchange string) (*pb.TickerResponse, error)
}

// SignalGenerator manages the data feed and strategy updates for a specific symbol.
type SignalGenerator struct {
	logger   *slog.Logger
	client   MarketDataProvider
	strategy *strategy.Strategy
	symbol   string
	exchange string
	interval time.Duration
}

// NewSignalGenerator creates a new SignalGenerator instance.
func NewSignalGenerator(logger *slog.Logger, client MarketDataProvider, symbol, exchange string, appCfg config.StrategyConfig) *SignalGenerator {
	// Map application config to strategy config
	stratCfg := strategy.StrategyConfig{}

	if appCfg.Type == config.StrategyMomentumTrailing {
		stratCfg.Type = strategy.StrategyMomentumTrailing
		stratCfg.WindowSize = appCfg.Momentum.WindowSize
		stratCfg.MomentumWindows = []strategy.MomentumWindow{
			{
				Lookback:  appCfg.Momentum.Lookback,
				Threshold: appCfg.Momentum.Threshold,
			},
		}
		stratCfg.StopLossPct = appCfg.Momentum.StopLossPct
		stratCfg.ActivationPct = appCfg.Momentum.ActivationPct
		stratCfg.TrailingStopPct = appCfg.Momentum.TrailingStopPct
	} else {
		stratCfg.Type = strategy.StrategyDummy
	}

	return &SignalGenerator{
		logger:   logger.With("component", "signal_generator", "symbol", symbol),
		client:   client,
		strategy: strategy.NewStrategy(stratCfg),
		symbol:   symbol,
		exchange: exchange,
		interval: 5 * time.Second,
	}
}

// Name returns the name of the background task.
func (s *SignalGenerator) Name() string {
	return "SignalGenerator-" + s.symbol
}

// Run starts the signal generation loop.
func (s *SignalGenerator) Run(ctx context.Context) {
	s.logger.Info("Starting signal generation loop")
	defer s.strategy.Close()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Stopping signal generation loop")
			return
		case <-ticker.C:
			if err := s.process(ctx); err != nil {
				s.logger.Error("Error in signal generation loop", "error", err)
			}
		}
	}
}

func (s *SignalGenerator) process(ctx context.Context) error {
	// 1. Fetch latest price from the gateway
	tickerData, err := s.client.GetTicker(ctx, s.symbol, s.exchange)
	if err != nil {
		return err
	}

	// 2. Update the C++ strategy
	s.strategy.UpdatePrice(tickerData.Price)

	// 3. Get the signal
	signal := s.strategy.GetSignal()

	// 4. Log the result (In Phase 6, this will send the signal to the Execution component)
	s.logger.Info("Strategy Update", "price", tickerData.Price, "signal", signal)

	return nil
}
