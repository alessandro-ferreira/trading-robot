package risk

import (
	"log/slog"
	"os"
	"testing"

	"trading/robot/go-bot/internal/config"

	"github.com/stretchr/testify/assert"
)

func TestEvaluateEntry(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tests := []struct {
		name             string
		config           config.RiskConfig
		currentPositions int
		currentDailyLoss float64
		price            float64
		wantAllowed      bool
		wantReason       string
		wantSize         float64
	}{
		{
			name: "Allow trade: within limits",
			config: config.RiskConfig{
				MaxOpenPositions: 5,
				MaxDailyLoss:     100.0,
				RiskPerTrade:     100.0,
			},
			currentPositions: 3,
			currentDailyLoss: 50.0,
			price:            50.0,
			wantAllowed:      true,
			wantSize:         2.0, // 100 / 50 = 2
		},
		{
			name: "Reject trade: max positions reached",
			config: config.RiskConfig{
				MaxOpenPositions: 5,
				MaxDailyLoss:     100.0,
				RiskPerTrade:     100.0,
			},
			currentPositions: 5,
			currentDailyLoss: 50.0,
			price:            50.0,
			wantAllowed:      false,
			wantReason:       "max open positions reached",
		},
		{
			name: "Reject trade: max daily loss reached",
			config: config.RiskConfig{
				MaxOpenPositions: 5,
				MaxDailyLoss:     100.0,
				RiskPerTrade:     100.0,
			},
			currentPositions: 3,
			currentDailyLoss: 100.0,
			price:            50.0,
			wantAllowed:      false,
			wantReason:       "max daily loss reached",
		},
		{
			name: "Allow trade: unlimited positions (0 config)",
			config: config.RiskConfig{
				MaxOpenPositions: 0,
				MaxDailyLoss:     100.0,
				RiskPerTrade:     100.0,
			},
			currentPositions: 100,
			currentDailyLoss: 50.0,
			price:            50.0,
			wantAllowed:      true,
			wantSize:         2.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(logger, tt.config)
			eval := m.EvaluateEntry(tt.currentPositions, tt.currentDailyLoss, tt.price)

			assert.Equal(t, tt.wantAllowed, eval.Allowed)
			if !tt.wantAllowed {
				assert.Contains(t, eval.Reason, tt.wantReason)
			} else {
				assert.InDelta(t, tt.wantSize, eval.ApprovedSize, 0.0001)
			}
		})
	}
}
