package signal_generator

import (
	"database/sql"
	"io"
	"log/slog"
	"testing"

	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/strategy"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testSymbol   = "BTC/USD"
	testExchange = "binance"
	testRisk     = repository.RiskPair{RiskPerTrade: 100.0}
)

func TestNewSignalGenerator(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

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
				ExchangeName:     testExchange,
				InstrumentSymbol: testSymbol,
				Type:             repository.StrategyDummy,
			},
			wantErr: false,
		},
		{
			name: "momentum profit strategy initialization",
			strategyCfg: repository.StrategyPair{
				ExchangeName:     testExchange,
				InstrumentSymbol: testSymbol,
				Type:             repository.StrategyMomentumProfit,
				Momentum: repository.StrategyMomentum{
					WindowSeconds: 60,
					Windows: []repository.MomentumWindow{
						{LookbackSeconds: 30, Threshold: 0.01},
					},
					StopLossPct:     0.02,
					ProfitTargetPct: sql.NullFloat64{Float64: 0.05, Valid: true},
				},
			},
			wantErr: false,
		},
		{
			name: "momentum trailing strategy initialization",
			strategyCfg: repository.StrategyPair{
				ExchangeName:     testExchange,
				InstrumentSymbol: testSymbol,
				Type:             repository.StrategyMomentumTrailing,
				Momentum: repository.StrategyMomentum{
					WindowSeconds: 60,
					Windows: []repository.MomentumWindow{
						{LookbackSeconds: 30, Threshold: 0.01},
					},
					StopLossPct:     0.02,
					ActivationPct:   sql.NullFloat64{Float64: 0.03, Valid: true},
					TrailingStopPct: sql.NullFloat64{Float64: 0.01, Valid: true},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sg, err := NewSignalGenerator(logger, testRisk, tt.strategyCfg, "test")
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
	strategyCfg := repository.StrategyPair{
		ExchangeName:     testExchange,
		InstrumentSymbol: testSymbol,
		Type:             repository.StrategyDummy,
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	name := "SignalGenerator-binance-BTC/USD"
	sg, err := NewSignalGenerator(logger, testRisk, strategyCfg, name)
	require.NoError(t, err)
	defer sg.Close()

	assert.Equal(t, testSymbol, sg.InstrumentSymbol())
	assert.Equal(t, testExchange, sg.Exchange())
	assert.Equal(t, name, sg.Name())
	assert.Equal(t, testRisk.RiskPerTrade, sg.Risk().RiskPerTrade)
	assert.Equal(t, testRisk.MaxPositionSize.Float64, sg.Risk().MaxPositionSize)
	assert.Equal(t, strategy.StrategyDummy, sg.StrategyConfig().Type)
}

func TestSignalGenerator_UpdateConfigFromPair(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	strategyCfg := repository.StrategyPair{
		ExchangeName:     testExchange,
		InstrumentSymbol: testSymbol,
		Type:             repository.StrategyDummy,
		Status:           repository.StrategyEnabled,
	}

	sg, err := NewSignalGenerator(logger, testRisk, strategyCfg, "test")
	require.NoError(t, err)
	defer sg.Close()

	assert.False(t, sg.IsPendingTerminate())

	// Update to PendingDisabled
	strategyCfg.Status = repository.StrategyPendingDisabled
	err = sg.UpdateConfigFromPair(strategyCfg)
	assert.NoError(t, err)
	assert.True(t, sg.IsPendingTerminate())

	// Update back to Enabled
	strategyCfg.Status = repository.StrategyEnabled
	err = sg.UpdateConfigFromPair(strategyCfg)
	assert.NoError(t, err)
	assert.False(t, sg.IsPendingTerminate())

	sg.SetPendingTerminate(true)
	assert.True(t, sg.IsPendingTerminate())
}

func TestSignalGenerator_Warmup(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("success with momentum trailing", func(t *testing.T) {
		stratCfg := repository.StrategyPair{
			ExchangeName:     testExchange,
			InstrumentSymbol: testSymbol,
			Type:             repository.StrategyMomentumTrailing,
			Momentum: repository.StrategyMomentum{
				WindowSeconds: 60,
				Windows: []repository.MomentumWindow{
					{LookbackSeconds: 30, Threshold: 0.001},
				},
				StopLossPct:     0.02,
				ActivationPct:   sql.NullFloat64{Float64: 0.05, Valid: true},
				TrailingStopPct: sql.NullFloat64{Float64: 0.01, Valid: true},
			},
		}
		sg, err := NewSignalGenerator(logger, testRisk, stratCfg, "test")
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
		_, err = sg.GetSignal(50300.0, 1030)
		assert.NoError(t, err)
	})

	t.Run("success with momentum profit", func(t *testing.T) {
		stratCfg := repository.StrategyPair{
			Type: repository.StrategyMomentumProfit,
			Momentum: repository.StrategyMomentum{
				WindowSeconds:   60,
				Windows:         []repository.MomentumWindow{{LookbackSeconds: 30, Threshold: 0.001}},
				StopLossPct:     0.02,
				ProfitTargetPct: sql.NullFloat64{Float64: 0.05, Valid: true},
			},
		}
		sg, err := NewSignalGenerator(logger, testRisk, stratCfg, "test")
		require.NoError(t, err)
		defer sg.Close()

		history := []repository.MarketDataTick{
			{TickUnixAt: 1000, Price: 50000.0},
			{TickUnixAt: 1010, Price: 50100.0},
		}
		assert.NoError(t, sg.Warmup(history))
	})

	t.Run("skip warmup for dummy strategy", func(t *testing.T) {
		stratCfg := repository.StrategyPair{Type: repository.StrategyDummy}
		sg, err := NewSignalGenerator(logger, testRisk, stratCfg, "test")
		require.NoError(t, err)
		defer sg.Close()

		history := []repository.MarketDataTick{{TickUnixAt: 1000, Price: 50000.0}}
		assert.NoError(t, sg.Warmup(history))
	})

	t.Run("empty history", func(t *testing.T) {
		stratCfg := repository.StrategyPair{Type: repository.StrategyDummy}
		sg, err := NewSignalGenerator(logger, testRisk, stratCfg, "test")
		require.NoError(t, err)
		defer sg.Close()

		assert.NoError(t, sg.Warmup(nil))
	})

	t.Run("error with unsorted history", func(t *testing.T) {
		stratCfg := repository.StrategyPair{
			Type: repository.StrategyMomentumProfit,
			Momentum: repository.StrategyMomentum{
				WindowSeconds:   60,
				Windows:         []repository.MomentumWindow{{LookbackSeconds: 30, Threshold: 0.01}},
				StopLossPct:     0.02,
				ProfitTargetPct: sql.NullFloat64{Float64: 0.05, Valid: true},
			},
		}
		sg, err := NewSignalGenerator(logger, testRisk, stratCfg, "test")
		require.NoError(t, err)
		defer sg.Close()

		history := []repository.MarketDataTick{{TickUnixAt: 200, Price: 10.0}, {TickUnixAt: 100, Price: 11.0}}
		assert.Error(t, sg.Warmup(history))
	})
}

func TestSignalGenerator_SetInPosition(t *testing.T) {
	stratCfg := repository.StrategyPair{
		ExchangeName:     testExchange,
		InstrumentSymbol: testSymbol,
		Type:             repository.StrategyMomentumTrailing,
		Momentum: repository.StrategyMomentum{
			WindowSeconds: 10,
			Windows: []repository.MomentumWindow{
				{LookbackSeconds: 5, Threshold: 0.01},
			},
			StopLossPct:     0.01,
			ActivationPct:   sql.NullFloat64{Float64: 0.05, Valid: true},
			TrailingStopPct: sql.NullFloat64{Float64: 0.02, Valid: true},
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sg, err := NewSignalGenerator(logger, testRisk, stratCfg, "test")
	require.NoError(t, err)
	defer sg.Close()

	// Test SetInPosition: move strategy to in-position state with specific entry and peak prices
	sg.SetInPosition(true, 50000.0, 51000.0)
	// Verify the state transition by checking that the signal is now tracking exit
	assert.Equal(t, strategy.SignalTrackingSellExit, sg.strategy.GetSignal())
}

func TestSignalGenerator_GetSignal(t *testing.T) {
	strategyCfg := repository.StrategyPair{
		ExchangeName:     testExchange,
		InstrumentSymbol: testSymbol,
		Type:             repository.StrategyMomentumTrailing,
		Momentum: repository.StrategyMomentum{
			WindowSeconds: 100,
			Windows: []repository.MomentumWindow{
				{LookbackSeconds: 50, Threshold: 0.01},
			},
			StopLossPct:     0.1,
			ActivationPct:   sql.NullFloat64{Float64: 0.05, Valid: true},
			TrailingStopPct: sql.NullFloat64{Float64: 0.02, Valid: true},
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sg, err := NewSignalGenerator(logger, testRisk, strategyCfg, "test")
	require.NoError(t, err)
	defer sg.Close()

	t.Run("hold signal on small price move", func(t *testing.T) {
		// Threshold is 1%, so 50000 -> 50100 is < 1%
		_, err := sg.GetSignal(50000.0, 1000)
		require.NoError(t, err)

		sig, err := sg.GetSignal(50100.0, 1001)
		assert.NoError(t, err)
		assert.Equal(t, strategy.SignalSearchingBuyEntry, sig)
	})
}

func TestSignalGenerator_Lifecycle(t *testing.T) {
	strategyCfg := repository.StrategyPair{
		ExchangeName:     testExchange,
		InstrumentSymbol: testSymbol,
		Type:             repository.StrategyMomentumTrailing,
		Momentum: repository.StrategyMomentum{
			WindowSeconds: 10,
			Windows: []repository.MomentumWindow{
				{LookbackSeconds: 5, Threshold: 0.01},
			},
			StopLossPct:     0.1,
			ActivationPct:   sql.NullFloat64{Float64: 0.05, Valid: true},
			TrailingStopPct: sql.NullFloat64{Float64: 0.02, Valid: true},
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sg, err := NewSignalGenerator(logger, testRisk, strategyCfg, "test")
	require.NoError(t, err)
	defer sg.Close()

	assert.NotPanics(t, func() {
		// Trigger a buy (2% gain)
		_, _ = sg.GetSignal(100.0, 0)
		_, _ = sg.GetSignal(100.0, 5)
		sig, _ := sg.GetSignal(102.0, 10)
		assert.Equal(t, strategy.SignalBuy, sig)

		// Verify lifecycle methods
		sg.SetInPosition(true, 102.0, 102.0)
		_ = sg.RetrySignal(strategy.SignalSell) // No-op if not pending, shouldn't panic
		_ = sg.RetrySignal(strategy.SignalBuy)
	})
}
