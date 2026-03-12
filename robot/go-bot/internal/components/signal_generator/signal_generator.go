package signal_generator

import (
	"context"
	"fmt"
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
}

// NewSignalGenerator creates a new SignalGenerator instance.
func NewSignalGenerator(logger *slog.Logger, client MarketDataProvider, symbol, exchange string, appCfg config.StrategyConfig) (*SignalGenerator, error) {
	// Map application config to strategy config
	stratCfg := strategy.StrategyConfig{}
	logger.Info("Initializing strategy", "type", appCfg.Type, "symbol", symbol, "exchange", exchange)

	switch appCfg.Type {
	case config.StrategyMomentumTrailing:
		stratCfg.Type = strategy.StrategyMomentumTrailing
		stratCfg.WindowSeconds = appCfg.Momentum.WindowSeconds
		// In the future, this could support multiple momentum windows from config
		stratCfg.MomentumWindows = []strategy.MomentumWindow{{
			LookbackSeconds: appCfg.Momentum.LookbackSeconds,
			Threshold:       appCfg.Momentum.Threshold,
		}}
		stratCfg.MomentumRequireAll = appCfg.Momentum.RequireAll
		stratCfg.StopLossPct = appCfg.Momentum.StopLossPct
		stratCfg.ActivationPct = appCfg.Momentum.ActivationPct
		stratCfg.TrailingStopPct = appCfg.Momentum.TrailingStopPct
	case config.StrategyMomentumProfit:
		stratCfg.Type = strategy.StrategyMomentumProfit
		stratCfg.WindowSeconds = appCfg.Momentum.WindowSeconds
		stratCfg.MomentumWindows = []strategy.MomentumWindow{{
			LookbackSeconds: appCfg.Momentum.LookbackSeconds,
			Threshold:       appCfg.Momentum.Threshold,
		}}
		stratCfg.MomentumRequireAll = appCfg.Momentum.RequireAll
		stratCfg.StopLossPct = appCfg.Momentum.StopLossPct
		stratCfg.ProfitTargetPct = appCfg.Momentum.ProfitTargetPct
	case config.StrategyDummy:
		stratCfg.Type = strategy.StrategyDummy
	default:
		return nil, fmt.Errorf("unsupported strategy type: %s", appCfg.Type)
	}

	strat, err := strategy.NewStrategy(stratCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize strategy: %w", err)
	}

	return &SignalGenerator{
		logger:   logger.With("component", "signal_generator", "exchange", exchange, "symbol", symbol),
		client:   client,
		strategy: strat,
		symbol:   symbol,
		exchange: exchange,
	}, nil
}

// Name returns the name of the background task.
func (s *SignalGenerator) Name() string {
	return "SignalGenerator-" + s.exchange + "-" + s.symbol
}

// Process fetches the latest price and updates the strategy.
func (s *SignalGenerator) Process(ctx context.Context) error {
	// Fetch latest price from the gateway
	tickerData, err := s.client.GetTicker(ctx, s.symbol, s.exchange)
	if err != nil {
		return err
	}

	// Update strategy with the new price
	if err := s.strategy.UpdatePrice(tickerData.Price, time.Now().Unix()); err != nil {
		s.logger.Warn("Failed to update strategy price", "price", tickerData.Price, "error", err)
		return err
	}

	// Get the signal
	signal := s.strategy.GetSignal()

	// Log the result (In the future, this will send the signal to the Execution component)
	s.logger.Info("Strategy Update", "price", tickerData.Price, "signal", signal)

	return nil
}

// Close releases resources held by the strategy.
func (s *SignalGenerator) Close() error {
	if s.strategy != nil {
		s.strategy.Close()
	}
	return nil
}
