package risk

import (
	"io"
	"log/slog"
	"testing"

	"trading/robot/go-bot/internal/config"

	"github.com/stretchr/testify/assert"
)

var (
	defaultRiskConfig = config.RiskConfig{
		MaxOpenPositions: 5,
		MaxDailyLoss:     100.0,
	}
	defaultPairRisk = PairRisk{RiskPerTrade: 100.0}
)

func TestEvaluateEntry(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name             string
		config           config.RiskConfig
		currentPositions int
		currentDailyLoss float64
		price            float64
		risk             PairRisk
		wantAllowed      bool
		wantReason       string
		wantSize         float64
	}{
		{
			name:             "Allow trade: within limits",
			config:           defaultRiskConfig,
			currentPositions: 3,
			currentDailyLoss: 50.0,
			price:            50.0,
			risk:             defaultPairRisk,
			wantAllowed:      true,
			wantSize:         2.0, // 100 / 50 = 2
		},
		{
			name:             "Reject trade: max positions reached",
			config:           defaultRiskConfig,
			currentPositions: 5,
			currentDailyLoss: 50.0,
			price:            50.0,
			risk:             defaultPairRisk,
			wantAllowed:      false,
			wantReason:       "max open positions reached",
		},
		{
			name:             "Reject trade: max daily loss reached",
			config:           defaultRiskConfig,
			currentPositions: 3,
			currentDailyLoss: 100.0,
			price:            50.0,
			risk:             defaultPairRisk,
			wantAllowed:      false,
			wantReason:       "max daily loss reached",
		},
		{
			name:             "Allow trade: unlimited positions (0 config)",
			config:           config.RiskConfig{MaxOpenPositions: 0, MaxDailyLoss: 100.0},
			currentPositions: 100,
			currentDailyLoss: 50.0,
			price:            50.0,
			risk:             defaultPairRisk,
			wantAllowed:      true,
			wantSize:         2.0,
		},
		{
			name:             "Allow trade: cap by MaxPositionSize",
			config:           defaultRiskConfig,
			currentPositions: 3,
			currentDailyLoss: 50.0,
			price:            10.0,
			risk: PairRisk{
				RiskPerTrade:    100.0,
				MaxPositionSize: 5.0,
			},
			wantAllowed: true,
			wantSize:    5.0,
		},
		{
			name:             "Reject trade: invalid risk per trade (<= 0)",
			config:           defaultRiskConfig,
			currentPositions: 0,
			currentDailyLoss: 0,
			price:            50.0,
			risk: PairRisk{
				RiskPerTrade: 0.0,
			},
			wantAllowed: false,
			wantReason:  "invalid risk per trade configuration",
		},
		{
			name:             "Reject trade: invalid price (<= 0)",
			config:           defaultRiskConfig,
			currentPositions: 0,
			currentDailyLoss: 0,
			price:            0.0,
			risk: PairRisk{
				RiskPerTrade: 100.0,
			},
			wantAllowed: false,
			wantReason:  "invalid price",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(logger, tt.config)
			eval := m.EvaluateEntry(tt.currentPositions, tt.currentDailyLoss, tt.price, tt.risk)

			assert.Equal(t, tt.wantAllowed, eval.Allowed)
			if !tt.wantAllowed {
				assert.Contains(t, eval.Reason, tt.wantReason)
			} else {
				assert.InDelta(t, tt.wantSize, eval.ApprovedSize, 0.0001)
			}
		})
	}
}
