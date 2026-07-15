//go:build unit

package strategy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrategy_SinalString(t *testing.T) {
	assert.Equal(t, "invalid", SignalInvalid.String())
	assert.Equal(t, "buy", SignalBuy.String())
	assert.Equal(t, "sell", SignalSell.String())
	assert.Equal(t, "searching_buy_entry", SignalSearchingBuyEntry.String())
	assert.Equal(t, "tracking_sell_exit", SignalTrackingSellExit.String())
	assert.Equal(t, "waiting_buy_fill", SignalWaitingBuyFill.String())
	assert.Equal(t, "waiting_sell_fill", SignalWaitingSellFill.String())
	assert.Equal(t, "invalid", StrategySignal(99).String()) // Default case
}

func TestStrategy_Creation(t *testing.T) {
	t.Run("valid configuration", func(t *testing.T) {
		cfg := StrategyConfig{
			Type:          StrategyMomentumTrailing,
			WindowSeconds: 100,
			MomentumWindows: []MomentumWindow{
				{LookbackSeconds: 50, Threshold: 0.01},
			},
			StopLossPct:     0.1,
			ActivationPct:   0.05,
			TrailingStopPct: 0.02,
		}
		s, err := NewStrategy(cfg)
		assert.NoError(t, err)
		assert.NotNil(t, s)
		defer s.Close()
		assert.Equal(t, cfg, s.GetConfig())
	})

	t.Run("invalid configuration", func(t *testing.T) {
		// stop_loss_pct must be positive according to C++ validation
		invalidCfg := StrategyConfig{
			Type:        StrategyMomentumTrailing,
			StopLossPct: -0.1,
		}
		s, err := NewStrategy(invalidCfg)
		assert.Error(t, err)
		assert.Nil(t, s)
		assert.Contains(t, err.Error(), "invalid/unrecognized config type or invalid parameters")
	})

	t.Run("too many momentum windows truncated", func(t *testing.T) {
		windows := make([]MomentumWindow, 15)
		for i := 0; i < 15; i++ {
			windows[i] = MomentumWindow{LookbackSeconds: 10 + i, Threshold: 0.01}
		}

		cfg := StrategyConfig{
			Type:            StrategyMomentumProfit,
			WindowSeconds:   1000,
			MomentumWindows: windows,
			StopLossPct:     0.1,
			ProfitTargetPct: 0.1,
		}

		s, err := NewStrategy(cfg)
		if err == nil {
			s.Close()
		}
	})
}

func TestStrategy_Dummy(t *testing.T) {
	cfg := StrategyConfig{Type: StrategyDummy}
	s, err := NewStrategy(cfg)
	assert.NoError(t, err)
	defer s.Close()

	// With the new safety guard in strategy.cpp, empty rules mean it stays in searching mode.
	err = s.UpdatePrice(100.0, 1000)
	assert.NoError(t, err)
	assert.Equal(t, SignalSearchingBuyEntry, s.GetSignal(), "Dummy strategy with no rules should stay in searching mode")
}

func TestStrategy_Initialization(t *testing.T) {
	cfg := StrategyConfig{
		Type:          StrategyMomentumProfit,
		WindowSeconds: 100,
		MomentumWindows: []MomentumWindow{
			{LookbackSeconds: 50, Threshold: 0.01},
		},
		StopLossPct:     0.1,
		ProfitTargetPct: 0.05,
	}

	t.Run("init profit with unsorted history", func(t *testing.T) {
		s, _ := NewStrategy(cfg)
		defer s.Close()

		unsortedTicks := []PricePoint{
			{Timestamp: 200, Price: 101.0},
			{Timestamp: 100, Price: 100.0},
		}
		err := s.InitProfit(unsortedTicks, false, 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "the history is not in chronological order")
	})

	t.Run("init profit with sorted history", func(t *testing.T) {
		s, _ := NewStrategy(cfg)
		defer s.Close()

		sortedTicks := []PricePoint{
			{Timestamp: 100, Price: 100.0},
			{Timestamp: 200, Price: 101.0},
		}
		err := s.InitProfit(sortedTicks, false, 0)
		assert.NoError(t, err)
	})

	t.Run("init trailing with metadata rehydration", func(t *testing.T) {
		cfgTrailing := cfg
		cfgTrailing.Type = StrategyMomentumTrailing
		cfgTrailing.ActivationPct = 0.05
		cfgTrailing.TrailingStopPct = 0.02

		s, _ := NewStrategy(cfgTrailing)
		defer s.Close()

		// Verify providing nil history preserves existing state/metadata logic
		err := s.InitTrailing(nil, true, 100.0, 105.0)
		assert.NoError(t, err)

		// Feed a live price to ensure rules have valid data to evaluate.
		_ = s.UpdatePrice(105.0, 100)

		// Triggers tracking since we are in position
		assert.Equal(t, SignalTrackingSellExit, s.GetSignal())
	})
}

func TestStrategy_UpdateConfig(t *testing.T) {
	cfg := StrategyConfig{
		Type:          StrategyMomentumTrailing,
		WindowSeconds: 100,
		MomentumWindows: []MomentumWindow{
			{LookbackSeconds: 50, Threshold: 0.01},
		},
		StopLossPct:     0.1,
		ActivationPct:   0.05,
		TrailingStopPct: 0.02,
	}
	s, err := NewStrategy(cfg)
	require.NoError(t, err)
	defer s.Close()

	// Update to a valid new configuration (tightening the stop loss)
	newCfg := cfg
	newCfg.StopLossPct = 0.05
	err = s.UpdateConfig(newCfg)
	assert.NoError(t, err)
	assert.Equal(t, 0.05, s.GetConfig().StopLossPct)

	t.Run("update with invalid parameters rejected", func(t *testing.T) {
		invalidCfg := cfg
		invalidCfg.StopLossPct = -1.0
		err = s.UpdateConfig(invalidCfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update strategy config")
	})

	t.Run("update to dummy strategy", func(t *testing.T) {
		dummyCfg := StrategyConfig{Type: StrategyDummy}
		err := s.UpdateConfig(dummyCfg)
		assert.NoError(t, err)
	})
}

func TestStrategy_EqualsConfig(t *testing.T) {
	cfg1 := StrategyConfig{
		Type:          StrategyMomentumProfit,
		WindowSeconds: 100,
		MomentumWindows: []MomentumWindow{
			{LookbackSeconds: 50, Threshold: 0.01},
		},
		StopLossPct:     0.1,
		ProfitTargetPct: 0.05,
	}
	cfg2 := cfg1
	cfg3 := cfg1
	cfg3.StopLossPct = 0.2
	cfg4 := cfg1
	cfg4.MomentumWindows = []MomentumWindow{
		{LookbackSeconds: 50, Threshold: 0.02},
	}

	s, err := NewStrategy(cfg1)
	require.NoError(t, err)
	defer s.Close()

	assert.True(t, s.EqualsConfig(cfg1))
	assert.True(t, s.EqualsConfig(cfg2))
	assert.False(t, s.EqualsConfig(cfg3))
	assert.False(t, s.EqualsConfig(cfg4))
}

func TestStrategy_BuyFlow(t *testing.T) {
	cfg := StrategyConfig{
		Type:               StrategyMomentumProfit,
		WindowSeconds:      100,
		MomentumWindows:    []MomentumWindow{{LookbackSeconds: 50, Threshold: 0.01}},
		StopLossPct:        0.1,
		ProfitTargetPct:    0.1,
		MomentumRequireAll: true,
	}
	s, err := NewStrategy(cfg)
	require.NoError(t, err)
	defer s.Close()

	// Warm up and trigger Buy (2% gain)
	_ = s.UpdatePrice(100.0, 0)
	_ = s.UpdatePrice(100.0, 50)
	err = s.UpdatePrice(102.0, 100)
	assert.NoError(t, err)

	assert.Equal(t, SignalBuy, s.GetSignal(), "Signal should be BUY after momentum trigger")
	assert.Equal(t, SignalWaitingBuyFill, s.GetSignal(), "Signal should lock until confirmed")

	// Confirm fill: Moves to ACTIVE state
	s.SetInPosition(true, 102.0, 102.0)
	assert.Equal(t, SignalTrackingSellExit, s.GetSignal(), "Subsequent signal should be TRACKING")
}

func TestStrategy_SellFlow(t *testing.T) {
	cfg := StrategyConfig{
		Type:               StrategyMomentumProfit,
		WindowSeconds:      100,
		MomentumWindows:    []MomentumWindow{{LookbackSeconds: 50, Threshold: 0.01}},
		StopLossPct:        0.01,
		ProfitTargetPct:    0.1,
		MomentumRequireAll: true,
	}
	s, err := NewStrategy(cfg)
	require.NoError(t, err)
	defer s.Close()

	// Force Active state via Init
	err = s.InitProfit(nil, true, 100.0)
	require.NoError(t, err)

	// Trigger Sell (2% drop exceeds 1% stop loss)
	_ = s.UpdatePrice(98.0, 101)
	assert.Equal(t, SignalSell, s.GetSignal(), "Signal should be SELL after SL trigger")
	assert.Equal(t, SignalWaitingSellFill, s.GetSignal(), "Signal should lock until confirmed")

	// Confirm Sell -> Should return to searching
	s.SetInPosition(false, 0, 0)
	assert.Equal(t, SignalSearchingBuyEntry, s.GetSignal(), "Should return to searching after confirming fill")
}

func TestStrategy_RetrySignal(t *testing.T) {
	cfg := StrategyConfig{
		Type:               StrategyMomentumProfit,
		WindowSeconds:      100,
		MomentumWindows:    []MomentumWindow{{LookbackSeconds: 50, Threshold: 0.01}},
		StopLossPct:        0.01,
		ProfitTargetPct:    0.1,
		MomentumRequireAll: true,
	}
	s, err := NewStrategy(cfg)
	require.NoError(t, err)
	defer s.Close()

	t.Run("retry buy signal returns to idle", func(t *testing.T) {
		_ = s.UpdatePrice(100.0, 0)
		_ = s.UpdatePrice(100.0, 50)
		_ = s.UpdatePrice(102.0, 100) // Trigger buy
		assert.Equal(t, SignalBuy, s.GetSignal())

		require.NoError(t, s.RetrySignal(SignalBuy))
		// Feed a neutral price so momentum doesn't immediately re-trigger
		_ = s.UpdatePrice(100.0, 101)
		assert.Equal(t, SignalSearchingBuyEntry, s.GetSignal())
	})

	t.Run("retry sell signal returns to active", func(t *testing.T) {
		_ = s.InitProfit(nil, true, 100.0)
		_ = s.UpdatePrice(98.0, 200) // Trigger SL
		assert.Equal(t, SignalSell, s.GetSignal())

		require.NoError(t, s.RetrySignal(SignalSell))
		// Price recovery prevents immediate re-trigger
		_ = s.UpdatePrice(100.0, 201)
		assert.Equal(t, SignalTrackingSellExit, s.GetSignal())
	})
}

func TestStrategy_UpdatePrice_Validation(t *testing.T) {
	cfg := StrategyConfig{
		Type:            StrategyDummy,
		WindowSeconds:   100,
		StopLossPct:     0.1,
		ProfitTargetPct: 0.1,
	}
	s, err := NewStrategy(cfg)
	require.NoError(t, err)
	defer s.Close()

	t.Run("stale timestamp rejected", func(t *testing.T) {
		_ = s.UpdatePrice(100.0, 1000)
		err := s.UpdatePrice(101.0, 999)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tick rejected")
	})

	t.Run("unrealistic price jump rejected", func(t *testing.T) {
		_ = s.UpdatePrice(100.0, 1100)
		err := s.UpdatePrice(200.0, 1101) // 100% jump
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tick rejected")
	})

	t.Run("non-positive price rejected", func(t *testing.T) {
		err := s.UpdatePrice(-1.0, 1200)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tick rejected")
	})
}
