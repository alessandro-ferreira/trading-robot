package signal_generator

import (
	"database/sql"
	"log/slog"
	"os"
	"testing"

	"trading/robot/go-bot/internal/database/repository"
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
		strategyCfg repository.StrategyPair
		wantErr     bool
		errSubstr   string
	}{
		{
			name:        "invalid strategy type returns error",
			strategyCfg: repository.StrategyPair{Type: "unknown_strategy"},
			wantErr:     true,
			errSubstr:   "unsupported strategy type",
		},
		{
			name: "dummy strategy initialization",
			strategyCfg: repository.StrategyPair{
				ExchangeName:     exchange,
				InstrumentSymbol: symbol,
				Type:             repository.StrategyDummy,
			},
			wantErr: false,
		},
		{
			name: "momentum profit strategy initialization",
			strategyCfg: repository.StrategyPair{
				ExchangeName:     exchange,
				InstrumentSymbol: symbol,
				Type:             repository.StrategyMomentumProfit,
				Momentum: repository.StrategyMomentum{
					WindowSeconds: 60,
					Windows: []repository.MomentumWindow{
						{LookbackSeconds: 30, Threshold: 0.01},
					},
					StopLossPct:     0.02,
					ProfitTargetPct: 0.05,
				},
			},
			wantErr: false,
		},
		{
			name: "momentum trailing strategy initialization",
			strategyCfg: repository.StrategyPair{
				ExchangeName:     exchange,
				InstrumentSymbol: symbol,
				Type:             repository.StrategyMomentumTrailing,
				Momentum: repository.StrategyMomentum{
					WindowSeconds: 60,
					Windows: []repository.MomentumWindow{
						{LookbackSeconds: 30, Threshold: 0.01},
					},
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
			riskData := repository.RiskPair{RiskPerTrade: 100.0}
			sg, err := NewSignalGenerator(logger, riskData, tt.strategyCfg)
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
	strategyCfg := repository.StrategyPair{
		ExchangeName:     exchange,
		InstrumentSymbol: symbol,
		Type:             repository.StrategyDummy,
	}

	sg, err := NewSignalGenerator(logger, repository.RiskPair{RiskPerTrade: 100.0}, strategyCfg)
	require.NoError(t, err)
	defer sg.Close()

	assert.Equal(t, symbol, sg.Symbol())
	assert.Equal(t, exchange, sg.Exchange())
	assert.Equal(t, "SignalGenerator-kraken-ETH/USDT", sg.Name())
}

func TestSignalGenerator_Warmup(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	symbol := "BTC/USD"
	exchange := "binance"

	stratCfg := repository.StrategyPair{
		ExchangeName:     exchange,
		InstrumentSymbol: symbol,
		Type:             repository.StrategyMomentumTrailing,
		Momentum: repository.StrategyMomentum{
			WindowSeconds: 60,
			Windows: []repository.MomentumWindow{
				{LookbackSeconds: 30, Threshold: 0.001},
			},
			StopLossPct:     0.02,
			ActivationPct:   0.05,
			TrailingStopPct: 0.01,
		},
	}

	riskData := repository.RiskPair{RiskPerTrade: 100.0}
	sg, err := NewSignalGenerator(logger, riskData, stratCfg)
	require.NoError(t, err)
	defer sg.Close()

	history := []repository.MarketDataTick{
		{TickUnixAt: 1000, Price: 50000.0},
		{TickUnixAt: 1010, Price: 50100.0},
		{TickUnixAt: 1020, Price: 50200.0},
	}

	err = sg.Warmup(history)
	assert.NoError(t, err)
	// Verify it can process next tick after warmup
	_, err = sg.UpdatePrice(50300.0, 1030)
	assert.NoError(t, err)
}

func TestSignalGenerator_Sync(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	symbol := "BTC/USD"
	exchange := "binance"
	riskData := repository.RiskPair{RiskPerTrade: 100.0}

	stratCfg := repository.StrategyPair{
		ExchangeName:     exchange,
		InstrumentSymbol: symbol,
		Type:             repository.StrategyMomentumTrailing,
		Momentum: repository.StrategyMomentum{
			WindowSeconds: 10,
			Windows: []repository.MomentumWindow{
				{LookbackSeconds: 5, Threshold: 0.01},
			},
			StopLossPct:     0.01,
			ActivationPct:   0.05,
			TrailingStopPct: 0.02,
		},
	}

	sg, err := NewSignalGenerator(logger, riskData, stratCfg)
	require.NoError(t, err)
	defer sg.Close()

	// Test SyncState: move strategy to ACTIVE state with specific entry and peak prices
	err = sg.SyncState(strategy.StateActive, 50000.0, 51000.0)
	assert.NoError(t, err)
	assert.Equal(t, strategy.StateActive, sg.State())
}

func TestSignalGenerator_Update(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	symbol := "BTC/USD"
	exchange := "binance"

	strategyCfg := repository.StrategyPair{
		ExchangeName:     exchange,
		InstrumentSymbol: symbol,
		Type:             repository.StrategyMomentumTrailing,
		Momentum: repository.StrategyMomentum{
			WindowSeconds: 100,
			Windows: []repository.MomentumWindow{
				{LookbackSeconds: 50, Threshold: 0.01},
			},
			StopLossPct:     0.1,
			ActivationPct:   0.05,
			TrailingStopPct: 0.02,
		},
	}

	riskData := repository.RiskPair{RiskPerTrade: 100.0}

	sg, err := NewSignalGenerator(logger, riskData, strategyCfg)
	require.NoError(t, err)
	defer sg.Close()

	t.Run("hold signal on small price move", func(t *testing.T) {
		// Threshold is 1%, so 50000 -> 50100 is < 1%
		_, err := sg.UpdatePrice(50000.0, 1000)
		require.NoError(t, err)

		sig, err := sg.UpdatePrice(50100.0, 1001)
		assert.NoError(t, err)
		assert.Equal(t, strategy.SignalSearchingEntry, sig)
	})
}

func TestSignalGenerator_State(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	symbol := "BTC/USD"
	exchange := "binance"
	strategyCfg := repository.StrategyPair{
		ExchangeName:     exchange,
		InstrumentSymbol: symbol,
		Type:             repository.StrategyDummy,
	}

	riskData := repository.RiskPair{RiskPerTrade: 100.0}
	sg, err := NewSignalGenerator(logger, riskData, strategyCfg)
	require.NoError(t, err)
	defer sg.Close()

	assert.Equal(t, strategy.StateIdle, sg.State())

	// Trigger a buy in dummy strategy to change state
	_, err = sg.UpdatePrice(100.0, 1000)
	require.NoError(t, err)

	assert.Equal(t, strategy.StatePendingBuy, sg.State())
}

func TestSignalGenerator_Lifecycle(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	symbol := "BTC/USD"
	exchange := "binance"
	strategyCfg := repository.StrategyPair{
		ExchangeName:     exchange,
		InstrumentSymbol: symbol,
		Type:             repository.StrategyDummy,
	}

	riskData := repository.RiskPair{
		RiskPerTrade:    100.0,
		MaxPositionSize: sql.NullFloat64{Float64: 10.0, Valid: true},
	}
	sg, err := NewSignalGenerator(logger, riskData, strategyCfg)
	require.NoError(t, err)
	defer sg.Close()

	assert.NotPanics(t, func() {
		// Trigger a buy in dummy strategy
		sig, _ := sg.UpdatePrice(100.0, 1000)
		assert.Equal(t, strategy.SignalBuy, sig)

		// Verify lifecycle methods
		sg.Confirm(strategy.SignalBuy, 100.0)
		sg.Cancel(strategy.SignalSell) // No-op if not pending, shouldn't panic
		sg.Reset()
	})
}
