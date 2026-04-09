package signal_generator

import (
	"log/slog"
	"os"
	"testing"

	"trading/robot/go-bot/internal/config"
	"trading/robot/go-bot/internal/strategy"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSignalGenerator(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	symbol := "BTC/USD"
	exchange := "binance"

	tests := []struct {
		name        string
		strategyCfg config.StrategyConfig
		wantErr     bool
		errSubstr   string
	}{
		{
			name:        "invalid strategy type returns error",
			strategyCfg: config.StrategyConfig{Type: "unknown_strategy"},
			wantErr:     true,
			errSubstr:   "unsupported strategy type",
		},
		{
			name:        "dummy strategy initialization",
			strategyCfg: config.StrategyConfig{Type: config.StrategyDummy},
			wantErr:     false,
		},
		{
			name: "momentum profit strategy initialization",
			strategyCfg: config.StrategyConfig{
				Type: config.StrategyMomentumProfit,
				Momentum: config.MomentumConfig{
					WindowSeconds:   60,
					LookbackSeconds: 30,
					Threshold:       0.01,
					StopLossPct:     0.02,
					ProfitTargetPct: 0.05,
				},
			},
			wantErr: false,
		},
		{
			name: "momentum trailing strategy initialization",
			strategyCfg: config.StrategyConfig{
				Type: config.StrategyMomentumTrailing,
				Momentum: config.MomentumConfig{
					WindowSeconds:   60,
					LookbackSeconds: 30,
					Threshold:       0.01,
					StopLossPct:     0.02,
					ActivationPct:   0.03,
					TrailingStopPct: 0.01,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sg, err := NewSignalGenerator(logger, symbol, exchange, config.PairRiskConfig{RiskPerTrade: 100.0}, tt.strategyCfg)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, sg)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, sg)
				if sg != nil {
					sg.Close()
				}
			}
		})
	}
}

func TestSignalGenerator_Metadata(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	symbol := "ETH/USDT"
	exchange := "kraken"
	strategyCfg := config.StrategyConfig{Type: config.StrategyDummy}

	sg, err := NewSignalGenerator(logger, symbol, exchange, config.PairRiskConfig{RiskPerTrade: 100.0}, strategyCfg)
	require.NoError(t, err)
	defer sg.Close()

	assert.Equal(t, symbol, sg.Symbol())
	assert.Equal(t, exchange, sg.Exchange())
	assert.Equal(t, "SignalGenerator-kraken-ETH/USDT", sg.Name())
}

func TestSignalGenerator_Sync(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	symbol := "BTC/USD"
	exchange := "binance"
	riskCfg := config.PairRiskConfig{RiskPerTrade: 100.0}

	stratCfg := config.StrategyConfig{
		Type: config.StrategyMomentumTrailing,
		Momentum: config.MomentumConfig{
			WindowSeconds:   10,
			LookbackSeconds: 5,
			Threshold:       0.01,
			StopLossPct:     0.01,
			ActivationPct:   0.05,
			TrailingStopPct: 0.02,
		},
	}

	sg, err := NewSignalGenerator(logger, symbol, exchange, riskCfg, stratCfg)
	require.NoError(t, err)
	defer sg.Close()

	// Test Sync: move strategy to ACTIVE state with specific entry and peak prices
	err = sg.Sync(strategy.StateActive, 50000.0, 51000.0)
	assert.NoError(t, err)
	assert.Equal(t, strategy.StateActive, sg.GetState())
}

func TestSignalGenerator_UpdateAndGetSignal(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	symbol := "BTC/USD"
	exchange := "binance"

	// Setup config for momentum strategy
	strategyCfg := config.StrategyConfig{
		Type: config.StrategyMomentumTrailing, // Use momentum to trigger the specific config mapping path
		Momentum: config.MomentumConfig{
			WindowSeconds:   100,
			LookbackSeconds: 50,
			Threshold:       0.01,
			StopLossPct:     0.1,
			ActivationPct:   0.05,
			TrailingStopPct: 0.02,
		},
	}

	sg, err := NewSignalGenerator(logger, symbol, exchange, config.PairRiskConfig{RiskPerTrade: 100.0}, strategyCfg)
	require.NoError(t, err)
	defer sg.Close()

	t.Run("hold signal on small price move", func(t *testing.T) {
		// Threshold is 1%, so 50000 -> 50100 is < 1%
		_, err := sg.UpdateAndGetSignal(50000.0, 1000)
		require.NoError(t, err)

		sig, err := sg.UpdateAndGetSignal(50100.0, 1001)
		assert.NoError(t, err)
		assert.Equal(t, strategy.SignalSearchingEntry, sig)
	})
}

func TestSignalGenerator_GetState(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	sg, err := NewSignalGenerator(logger, "BTC/USD", "binance", config.PairRiskConfig{RiskPerTrade: 100.0}, config.StrategyConfig{Type: config.StrategyDummy})
	require.NoError(t, err)
	defer sg.Close()

	assert.Equal(t, strategy.StateIdle, sg.GetState())

	// Trigger a buy in dummy strategy to change state
	_, err = sg.UpdateAndGetSignal(100.0, 1000)
	require.NoError(t, err)

	assert.Equal(t, strategy.StatePendingBuy, sg.GetState())
}

func TestSignalGenerator_Lifecycle(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	sg, err := NewSignalGenerator(logger, "BTC/USD", "binance", config.PairRiskConfig{RiskPerTrade: 100.0}, config.StrategyConfig{Type: config.StrategyDummy})
	require.NoError(t, err)
	defer sg.Close()

	assert.NotPanics(t, func() {
		// Trigger a buy in dummy strategy
		sig, _ := sg.UpdateAndGetSignal(100.0, 1000)
		assert.Equal(t, strategy.SignalBuy, sig)

		// Verify lifecycle methods
		sg.Confirm(strategy.SignalBuy, 100.0)
		sg.Cancel(strategy.SignalSell) // No-op if not pending, shouldn't panic
		sg.Reset()
	})
}
