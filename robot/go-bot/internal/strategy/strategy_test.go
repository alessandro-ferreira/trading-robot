package strategy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	// Second update: In position, Exit rules (empty) pass (return false) -> SignalHold
	price2 := 100.50
	err = s.UpdatePrice(price2, 1001)
	assert.NoError(t, err)
	assert.Equal(t, SignalHold, s.GetSignal(), "Subsequent signal should be HOLD for dummy strategy")
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
		err = profitStrategy.InitProfit(unsortedTicks, false, 0)
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
		err = profitStrategy.InitProfit(sortedTicks, false, 0)
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
		err = trailingStrategy.InitTrailing(unsortedTicks, false, 0, 0)
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
		err = trailingStrategy.InitTrailing(sortedTicks, false, 0, 0)
		assert.NoError(t, err)
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
