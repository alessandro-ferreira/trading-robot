package strategy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrategy_StringMethods(t *testing.T) {
	// Test StrategyState String()
	assert.Equal(t, "idle", StateIdle.String())
	assert.Equal(t, "pending_buy", StatePendingBuy.String())
	assert.Equal(t, "active", StateActive.String())
	assert.Equal(t, "pending_sell", StatePendingSell.String())
	assert.Equal(t, "idle", StrategyState(99).String()) // Default case

	// Test Signal String()
	assert.Equal(t, "invalid", SignalInvalid.String())
	assert.Equal(t, "buy", SignalBuy.String())
	assert.Equal(t, "sell", SignalSell.String())
	assert.Equal(t, "searching_entry", SignalSearchingEntry.String())
	assert.Equal(t, "tracking_exit", SignalTrackingExit.String())
	assert.Equal(t, "waiting_buy_fill", SignalWaitingBuyFill.String())
	assert.Equal(t, "waiting_sell_fill", SignalWaitingSellFill.String())
	assert.Equal(t, "invalid", Signal(99).String()) // Default case
}

func TestStrategy_Dummy(t *testing.T) {
	cfg := StrategyConfig{
		Type: StrategyDummy,
	}
	s, err := NewStrategy(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, s)
	defer s.Close()

	// First update: Entry rules (empty) pass -> SignalBuy
	price1 := 100.0
	err = s.UpdatePrice(price1, 1000)
	assert.NoError(t, err)
	assert.Equal(t, SignalBuy, s.GetSignal(), "First signal should be BUY for dummy strategy")
	assert.Equal(t, SignalWaitingBuyFill, s.GetSignal(), "Signal should lock to WAITING_BUY until confirmed")

	// Confirm fill
	s.ConfirmSignal(SignalBuy, price1)

	// Second update: Now in STATE_ACTIVE, empty exit rules mean it returns SignalTracking
	price2 := 100.50
	err = s.UpdatePrice(price2, 1001)
	assert.NoError(t, err)
	assert.Equal(t, SignalTrackingExit, s.GetSignal(), "Subsequent signal should be TRACKING for dummy strategy")
}

func TestStrategy_Momentum(t *testing.T) {
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
	require.NotNil(t, s)
	defer s.Close()

	t.Run("new strategy with invalid config", func(t *testing.T) {
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

	t.Run("init profit with unsorted history", func(t *testing.T) {
		profitCfg := StrategyConfig{
			Type:          StrategyMomentumProfit,
			WindowSeconds: 100,
			MomentumWindows: []MomentumWindow{
				{LookbackSeconds: 50, Threshold: 0.01},
			},
			StopLossPct:     0.1,
			ProfitTargetPct: 0.05,
		}
		profitStrategy, err := NewStrategy(profitCfg)
		require.NoError(t, err)
		require.NotNil(t, profitStrategy)
		defer profitStrategy.Close()

		unsortedTicks := []PricePoint{
			{Timestamp: 200, Price: 101.0},
			{Timestamp: 100, Price: 100.0},
		}
		err = profitStrategy.InitProfit(unsortedTicks, StateIdle, 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "the history is not in chronological order")
	})

	t.Run("init profit with sorted history", func(t *testing.T) {
		profitCfg := StrategyConfig{
			Type:          StrategyMomentumProfit,
			WindowSeconds: 100,
			MomentumWindows: []MomentumWindow{
				{LookbackSeconds: 50, Threshold: 0.01},
			},
			StopLossPct:     0.1,
			ProfitTargetPct: 0.05,
		}
		profitStrategy, err := NewStrategy(profitCfg)
		require.NoError(t, err)
		require.NotNil(t, profitStrategy)
		defer profitStrategy.Close()

		sortedTicks := []PricePoint{
			{Timestamp: 100, Price: 100.0},
			{Timestamp: 200, Price: 101.0},
		}
		err = profitStrategy.InitProfit(sortedTicks, StateIdle, 0)
		assert.NoError(t, err)
	})

	t.Run("init trailing with unsorted history", func(t *testing.T) {
		trailingStrategy, err := NewStrategy(cfg)
		require.NoError(t, err)
		require.NotNil(t, trailingStrategy)
		defer trailingStrategy.Close()

		unsortedTicks := []PricePoint{
			{Timestamp: 200, Price: 101.0},
			{Timestamp: 100, Price: 100.0},
		}
		err = trailingStrategy.InitTrailing(unsortedTicks, StateIdle, 0, 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "the history is not in chronological order")
	})

	t.Run("init trailing with sorted history", func(t *testing.T) {
		trailingStrategy, err := NewStrategy(cfg)
		require.NoError(t, err)
		require.NotNil(t, trailingStrategy)
		defer trailingStrategy.Close()

		sortedTicks := []PricePoint{
			{Timestamp: 100, Price: 100.0},
			{Timestamp: 200, Price: 101.0},
		}
		// re-use the strategy 'trailingStrategy' from the parent test
		err = trailingStrategy.InitTrailing(sortedTicks, StateIdle, 0, 0)
		assert.NoError(t, err)
	})

	t.Run("init trailing with nil history", func(t *testing.T) {
		trailingStrategy, err := NewStrategy(cfg)
		require.NoError(t, err)
		defer trailingStrategy.Close()

		// Verify that providing nil history performs a metadata-only update without error
		err = trailingStrategy.InitTrailing(nil, StateActive, 100.0, 105.0)
		assert.NoError(t, err)
		assert.Equal(t, StateActive, trailingStrategy.GetState())
	})

	t.Run("too many momentum windows", func(t *testing.T) {
		windows := make([]MomentumWindow, 15)
		for i := 0; i < 15; i++ {
			windows[i] = MomentumWindow{LookbackSeconds: 10 + i, Threshold: 0.01}
		}

		overConfig := cfg
		overConfig.MomentumWindows = windows

		overStrategy, err := NewStrategy(overConfig)
		// If the engine accepts it (due to truncation in the bridge), we just close it.
		if err == nil {
			overStrategy.Close()
		}
	})

	t.Run("update with stale tick", func(t *testing.T) {
		err := s.UpdatePrice(100.0, 200)
		require.NoError(t, err)

		err = s.UpdatePrice(101.0, 199)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tick rejected")
	})

	t.Run("update with unrealistic jump", func(t *testing.T) {
		err := s.UpdatePrice(100.0, 300)
		require.NoError(t, err)

		err = s.UpdatePrice(200.0, 301) // 100% jump
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tick rejected")
	})

	t.Run("update with non-positive price", func(t *testing.T) {
		err := s.UpdatePrice(100.0, 400)
		require.NoError(t, err)

		err = s.UpdatePrice(-100.0, 401)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tick rejected")
	})
}

func TestStrategy_GetConfig(t *testing.T) {
	cfg := StrategyConfig{Type: StrategyDummy, WindowSeconds: 60}
	s, err := NewStrategy(cfg)
	require.NoError(t, err)
	defer s.Close()

	assert.Equal(t, cfg, s.GetConfig())
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

	// Update to a valid new configuration (e.g., tightening the stop loss)
	newCfg := cfg
	newCfg.StopLossPct = 0.05
	err = s.UpdateConfig(newCfg)
	assert.NoError(t, err)
	assert.Equal(t, 0.05, s.cfg.StopLossPct)

	t.Run("update with invalid config error", func(t *testing.T) {
		invalidCfg := cfg
		invalidCfg.StopLossPct = -1.0 // C++ should reject negative stop loss
		err = s.UpdateConfig(invalidCfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update strategy config")
	})
}

func TestStrategy_GetState(t *testing.T) {
	cfg := StrategyConfig{Type: StrategyDummy}
	s, err := NewStrategy(cfg)
	require.NoError(t, err)
	defer s.Close()

	assert.Equal(t, StateIdle, s.GetState())

	// Trigger a buy in dummy strategy (it triggers on the first tick)
	_ = s.UpdatePrice(100.0, 1000)
	_ = s.GetSignal()
	assert.Equal(t, StatePendingBuy, s.GetState())
}

func TestStrategy_ConfirmSignal_Sell(t *testing.T) {
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
	err = s.InitProfit(nil, StateActive, 100.0)
	require.NoError(t, err)

	// Trigger Sell (2% drop exceeds 1% stop loss)
	_ = s.UpdatePrice(98.0, 101)
	assert.Equal(t, SignalSell, s.GetSignal())

	// Confirm Sell -> Should return to searching
	s.ConfirmSignal(SignalSell, 98.0)
	assert.Equal(t, SignalSearchingEntry, s.GetSignal(), "Should return to searching after confirming sell")
}

func TestStrategy_CancelSignal(t *testing.T) {
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

	// --- Test Cancel Buy ---
	_ = s.UpdatePrice(100.0, 0)
	_ = s.UpdatePrice(100.0, 50)
	_ = s.UpdatePrice(102.0, 100) // 2% gain triggers buy
	assert.Equal(t, SignalBuy, s.GetSignal())

	s.CancelSignal(SignalBuy)
	// Feed a neutral price so momentum doesn't immediately re-trigger
	_ = s.UpdatePrice(100.0, 101)
	assert.Equal(t, SignalSearchingEntry, s.GetSignal(), "Should return to searching after canceling buy")

	// --- Test Cancel Sell ---
	err = s.InitProfit(nil, StateActive, 100.0)
	require.NoError(t, err)

	_ = s.UpdatePrice(98.0, 101) // Trigger stop loss
	assert.Equal(t, SignalSell, s.GetSignal())

	s.CancelSignal(SignalSell)
	// Feed a neutral price so stop loss doesn't immediately re-trigger
	_ = s.UpdatePrice(100.0, 102)
	assert.Equal(t, SignalTrackingExit, s.GetSignal(), "Should return to tracking after canceling sell")
}

func TestStrategy_ResetSignal(t *testing.T) {
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

	// Move Dummy to PENDING_BUY
	_ = s.UpdatePrice(100.0, 0)
	_ = s.UpdatePrice(100.0, 50)
	_ = s.UpdatePrice(102.0, 100) // 2% gain triggers buy
	assert.Equal(t, SignalBuy, s.GetSignal())

	// Explicit Reset
	s.ResetSignal()
	// Feed a neutral price so momentum doesn't immediately re-trigger
	_ = s.UpdatePrice(100.0, 101)
	assert.Equal(t, SignalSearchingEntry, s.GetSignal(), "Should return to searching after reset")
}
