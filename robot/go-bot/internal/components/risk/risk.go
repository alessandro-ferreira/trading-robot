package risk

import (
	"fmt"
	"log/slog"

	"trading/robot/go-bot/internal/config"
)

// Config defines the global risk parameters for the manager.
type RiskConfig struct {
	MaxOpenPositions int
	MaxDailyLoss     float64
}

// PairRisk defines the operational risk rules for a specific trading pair.
type PairRisk struct {
	RiskPerTrade float64
}

// Manager handles risk checks and policy enforcement.
type Manager struct {
	logger *slog.Logger
	config RiskConfig
}

// Evaluation contains the result of a risk check.
type Evaluation struct {
	// Allowed indicates if the trade is permitted.
	Allowed bool
	// Reason provides a human-readable explanation if the trade is rejected.
	Reason string
	// ApprovedSize is the quantity of the asset to buy/sell.
	ApprovedSize float64
}

// NewManager creates a new risk manager with the provided configuration.
func NewManager(logger *slog.Logger, cfg config.RiskConfig) *Manager {
	logger.Info("Initializing Risk Manager",
		"max_open_positions", cfg.MaxOpenPositions,
		"max_daily_loss", cfg.MaxDailyLoss,
	)

	return &Manager{
		logger: logger,
		config: RiskConfig{
			MaxOpenPositions: cfg.MaxOpenPositions,
			MaxDailyLoss:     cfg.MaxDailyLoss,
		},
	}
}

// EvaluateEntry checks if a new trade can be opened and calculates the position size.
func (m *Manager) EvaluateEntry(currentPositions int, currentDailyLoss float64, price float64, risk PairRisk) Evaluation {
	// Check Max Open Positions
	if m.config.MaxOpenPositions > 0 && currentPositions >= m.config.MaxOpenPositions {
		m.logger.Warn("Risk check failed: Max open positions reached",
			"current", currentPositions,
			"limit", m.config.MaxOpenPositions)
		return Evaluation{
			Allowed: false,
			Reason:  fmt.Sprintf("max open positions reached (%d >= %d)", currentPositions, m.config.MaxOpenPositions),
		}
	}

	// Check Daily Loss Limit
	if m.config.MaxDailyLoss > 0 && currentDailyLoss >= m.config.MaxDailyLoss {
		m.logger.Warn("Risk check failed: Max daily loss reached",
			"current_loss", currentDailyLoss,
			"limit", m.config.MaxDailyLoss)
		return Evaluation{
			Allowed: false,
			Reason:  fmt.Sprintf("max daily loss reached (%.2f >= %.2f)", currentDailyLoss, m.config.MaxDailyLoss),
		}
	}

	// Calculate Position Size
	// For now, we use a simple Fixed Amount model from config.
	// In the future, this could be dynamic (e.g., % of equity).
	if risk.RiskPerTrade <= 0 {
		m.logger.Warn("Risk check failed: Invalid risk per trade configuration", "value", risk.RiskPerTrade)
		return Evaluation{
			Allowed: false,
			Reason:  "invalid risk per trade configuration (must be > 0)",
		}
	}

	if price <= 0 {
		return Evaluation{
			Allowed: false,
			Reason:  "invalid price (must be > 0)",
		}
	}

	// Size = Fixed Amount / Price
	size := risk.RiskPerTrade / price

	return Evaluation{
		Allowed:      true,
		ApprovedSize: size,
	}
}
