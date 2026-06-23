//go:build integration

package strategy

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testStep struct {
	timestamp      int64
	price          float64
	expectedSignal StrategySignal
	description    string
}

func loadTestSteps(t *testing.T, filename string) []testStep {
	path := filepath.Join("testdata", filename)
	file, err := os.Open(path)
	require.NoError(t, err, "failed to open CSV file")
	defer file.Close()

	reader := csv.NewReader(file)
	// Read all records
	records, err := reader.ReadAll()
	require.NoError(t, err, "failed to read CSV file")

	var steps []testStep
	// Skip header row (index 0)
	for i := 1; i < len(records); i++ {
		record := records[i]
		timestamp, err := strconv.ParseInt(record[0], 10, 64)
		require.NoError(t, err, "invalid timestamp at row %d", i+1)

		price, err := strconv.ParseFloat(record[1], 64)
		require.NoError(t, err, "invalid price at row %d", i+1)

		expectedSignalInt, err := strconv.Atoi(record[2])
		require.NoError(t, err, "invalid expected signal at row %d", i+1)

		steps = append(steps, testStep{
			timestamp:      timestamp,
			price:          price,
			expectedSignal: StrategySignal(expectedSignalInt),
			description:    record[3],
		})
	}
	return steps
}

type initConfig struct {
	inPosition   bool
	entryPrice   float64
	highestPrice float64
}

func TestStrategy_Integration_DataDriven(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		config   StrategyConfig
		init     *initConfig
	}{
		{
			name:     "Trailing Stop Bull Run",
			filename: "trailing_stop_bull_run.csv",
			config: StrategyConfig{
				Type:          StrategyMomentumTrailing,
				WindowSeconds: 5,
				MomentumWindows: []MomentumWindow{
					{LookbackSeconds: 3, Threshold: 0.05},
				},
				StopLossPct:     0.10,
				ActivationPct:   0.08,
				TrailingStopPct: 0.05,
			},
		},
		{
			name:     "Entry with Single Momentum Window",
			filename: "entry_single_window.csv",
			config: StrategyConfig{
				Type:          StrategyMomentumProfit, // Using Profit strategy for this entry test
				WindowSeconds: 120,
				MomentumWindows: []MomentumWindow{
					{LookbackSeconds: 60, Threshold: 0.02}, // 2% over 60s
				},
				MomentumRequireAll: true,
				// Dummy values for exit rules, not relevant for this entry test
				StopLossPct:     0.10,
				ProfitTargetPct: 0.10,
			},
		},
		{
			name:     "Entry with Multiple Windows (AND)",
			filename: "entry_multi_window_and.csv",
			config: StrategyConfig{
				Type:          StrategyMomentumProfit,
				WindowSeconds: 200,
				MomentumWindows: []MomentumWindow{
					{LookbackSeconds: 60, Threshold: 0.01},
					{LookbackSeconds: 120, Threshold: 0.02},
					{LookbackSeconds: 180, Threshold: 0.03},
				},
				MomentumRequireAll: true,
				StopLossPct:        0.10,
				ProfitTargetPct:    0.10,
			},
		},
		{
			name:     "Entry with Multiple Windows (OR)",
			filename: "entry_multi_window_or.csv",
			config: StrategyConfig{
				Type:          StrategyMomentumProfit,
				WindowSeconds: 200,
				MomentumWindows: []MomentumWindow{
					{LookbackSeconds: 60, Threshold: 0.01},
					{LookbackSeconds: 120, Threshold: 0.05},
				},
				MomentumRequireAll: false,
				StopLossPct:        0.10,
				ProfitTargetPct:    0.10,
			},
		},
		{
			name:     "Exit on Stop Loss (Profit Strategy)",
			filename: "exit_profit_stop_loss.csv",
			config: StrategyConfig{
				Type:          StrategyMomentumProfit,
				WindowSeconds: 65, // Window must be long enough for the lookback
				MomentumWindows: []MomentumWindow{
					{LookbackSeconds: 60, Threshold: 0.025},
				},
				StopLossPct:     0.05,
				ProfitTargetPct: 0.10,
			},
			init: &initConfig{inPosition: true, entryPrice: 100.0},
		},
		{
			name:     "Exit on Take Profit (Profit Strategy)",
			filename: "exit_profit_take_profit.csv",
			config: StrategyConfig{
				Type:            StrategyMomentumProfit,
				WindowSeconds:   65,
				MomentumWindows: []MomentumWindow{{LookbackSeconds: 60, Threshold: 0.025}},
				StopLossPct:     0.10,
				ProfitTargetPct: 0.05,
			},
			init: &initConfig{inPosition: true, entryPrice: 100.0},
		},
		{
			name:     "Exit on Initial Stop Loss (Trailing Strategy)",
			filename: "exit_trailing_stop_loss.csv",
			config: StrategyConfig{
				Type:            StrategyMomentumTrailing,
				WindowSeconds:   65,
				MomentumWindows: []MomentumWindow{{LookbackSeconds: 60, Threshold: 0.025}},
				StopLossPct:     0.10,
				ActivationPct:   0.05,
				TrailingStopPct: 0.02,
			},
			init: &initConfig{inPosition: true, entryPrice: 100.0, highestPrice: 100.0},
		},
		{
			name:     "Exit on Trailing Activation (Trailing Strategy)",
			filename: "exit_trailing_activation.csv",
			config: StrategyConfig{
				Type:            StrategyMomentumTrailing,
				WindowSeconds:   65,
				MomentumWindows: []MomentumWindow{{LookbackSeconds: 60, Threshold: 0.025}},
				StopLossPct:     0.10,
				ActivationPct:   0.05,
				TrailingStopPct: 0.02,
			},
			init: &initConfig{inPosition: true, entryPrice: 100.0, highestPrice: 100.0},
		},
		{
			name:     "Full Cycle (Profit Strategy)",
			filename: "full_cycle_profit.csv",
			config: StrategyConfig{
				Type:          StrategyMomentumProfit,
				WindowSeconds: 120,
				MomentumWindows: []MomentumWindow{
					{LookbackSeconds: 60, Threshold: 0.025},
				},
				StopLossPct:     0.05,
				ProfitTargetPct: 0.05,
			},
		},
		{
			name:     "Full Cycle (Trailing Strategy)",
			filename: "full_cycle_trailing.csv",
			config: StrategyConfig{
				Type:          StrategyMomentumTrailing,
				WindowSeconds: 120,
				MomentumWindows: []MomentumWindow{
					{LookbackSeconds: 60, Threshold: 0.025},
				},
				StopLossPct:     0.05,
				ActivationPct:   0.05,
				TrailingStopPct: 0.03,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps := loadTestSteps(t, tt.filename)
			s, err := NewStrategy(tt.config)
			require.NoError(t, err)
			require.NotNil(t, s)
			defer s.Close()

			// Apply initialization if provided
			if tt.init != nil {
				// Create a dummy history point to satisfy the API, using the entry price
				// This simulates that we entered the position "now" or recently.
				history := []PricePoint{{Timestamp: 0, Price: tt.init.entryPrice}}
				if tt.config.Type == StrategyMomentumTrailing {
					require.NoError(t, s.InitTrailing(history, tt.init.inPosition, tt.init.entryPrice, tt.init.highestPrice))
				} else {
					require.NoError(t, s.InitProfit(history, tt.init.inPosition, tt.init.entryPrice))
				}
			}

			for i, step := range steps {
				err := s.UpdatePrice(step.price, step.timestamp)
				assert.NoError(t, err, "Row %d: UpdatePrice failed", i+2)

				signal := s.GetSignal()

				assert.Equal(t, step.expectedSignal, signal,
					"Row %d: %s (Price: %.2f)", i+2, step.description, step.price)

				// Simulate an immediate fill to progress the state machine.
				if signal == SignalBuy {
					s.SetInPosition(true, step.price, step.price)
				} else if signal == SignalSell {
					s.SetInPosition(false, 0, 0)
				}
			}
		})
	}
}
