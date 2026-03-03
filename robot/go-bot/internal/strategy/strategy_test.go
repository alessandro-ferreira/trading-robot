package strategy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStrategyIntegration(t *testing.T) {
	cfg := StrategyConfig{
		Type: StrategyDummy,
	}
	s := NewStrategy(cfg)
	assert.NotNil(t, s)
	defer s.Close()

	// Test the dummy strategy logic
	// First update: Entry rules (empty) pass -> Buy Signal (1.0)
	price1 := 100.0
	s.UpdatePrice(price1)
	assert.Equal(t, 1.0, s.GetSignal(), "First signal should be BUY (1.0) for dummy strategy")

	// Second update: In position, Exit rules (empty) pass (return false) -> Hold Signal (0.0)
	price2 := 200.50
	s.UpdatePrice(price2)
	assert.Equal(t, 0.0, s.GetSignal(), "Subsequent signal should be HOLD (0.0) for dummy strategy")
}
