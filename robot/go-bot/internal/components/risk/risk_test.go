//go:build unit

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
	defaultPairRisk = PairRisk{AllocatedBudget: 100.0}
)

func TestEvaluateEntry(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name             string
		config           config.RiskConfig
		currentPositions int
		currentDailyLoss float64
		price            float64
		availableBudget  float64
		risk             PairRisk
		wantAllowed      bool
		wantReason       string
		wantUnits        float64
	}{
		{
			name:             "Allow trade: within limits",
			config:           defaultRiskConfig,
			currentPositions: 3,
			currentDailyLoss: 50.0,
			price:            50.0,
			availableBudget:  1000.0,
			risk:             defaultPairRisk,
			wantAllowed:      true,
			wantUnits:        1.98, // (100 / 50) * 0.99
		},
		{
			name:             "Reject trade: max positions reached",
			config:           defaultRiskConfig,
			currentPositions: 5,
			currentDailyLoss: 50.0,
			price:            50.0,
			availableBudget:  1000.0,
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
			availableBudget:  1000.0,
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
			availableBudget:  1000.0,
			risk:             defaultPairRisk,
			wantAllowed:      true,
			wantUnits:        1.98,
		},
		{
			name:             "Allow trade: cap by MaxAssetUnits",
			config:           defaultRiskConfig,
			currentPositions: 3,
			currentDailyLoss: 50.0,
			price:            10.0,
			availableBudget:  1000.0,
			risk: PairRisk{
				AllocatedBudget: 100.0,
				MaxAssetUnits:   5.0,
			},
			wantAllowed: true,
			wantUnits:   4.95, // 5.0 * 0.99
		},
		{
			name:             "Allow trade: budget capped by available balance",
			config:           defaultRiskConfig,
			currentPositions: 0,
			currentDailyLoss: 0,
			price:            10.0,
			availableBudget:  50.0, // Budget is 100, but only 50 available
			risk:             defaultPairRisk,
			wantAllowed:      true,
			wantUnits:        4.95, // (50 / 10) * 0.99
		},
		{
			name:             "Reject trade: invalid allocated budget (< Min)",
			config:           defaultRiskConfig,
			currentPositions: 0,
			currentDailyLoss: 0,
			price:            50.0,
			availableBudget:  1000.0,
			risk: PairRisk{
				AllocatedBudget: 5.0,
			},
			wantAllowed: false,
			wantReason:  "invalid allocated budget configuration",
		},
		{
			name:             "Reject trade: invalid price (<= 0)",
			config:           defaultRiskConfig,
			currentPositions: 0,
			currentDailyLoss: 0,
			price:            0.0,
			availableBudget:  1000.0,
			risk: PairRisk{
				AllocatedBudget: 100.0,
			},
			wantAllowed: false,
			wantReason:  "invalid price",
		},
		{
			name:             "Reject trade: available budget < Min",
			config:           defaultRiskConfig,
			currentPositions: 0,
			currentDailyLoss: 0,
			price:            50.0,
			availableBudget:  5.0,
			risk:             defaultPairRisk,
			wantAllowed:      false,
			wantReason:       "insufficient exchange balance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(logger, tt.config)
			eval := m.EvaluateEntry(
				tt.currentPositions, tt.currentDailyLoss, tt.price, tt.availableBudget, tt.risk,
			)

			assert.Equal(t, tt.wantAllowed, eval.Allowed)
			if !tt.wantAllowed {
				assert.Contains(t, eval.Reason, tt.wantReason)
			} else {
				assert.InDelta(t, tt.wantUnits, eval.ApprovedUnits, 0.0001)
			}
		})
	}
}
