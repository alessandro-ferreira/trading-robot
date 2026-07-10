package risk

import (
	"fmt"
	"log/slog"
	"strings"

	"trading/robot/go-bot/internal/config"
)

// MinExchangeBudget defines the minimum budget required to place a trade,
// accounting for exchange-enforced minimum order values. While some venues
// allow less, 20.0 is a safer industry standard for USDT/USD/BRL/EUR pairs.
const MinExchangeBudget = 20.0

// slippageBuffer is a safety margin applied to the calculated asset units
// to prevent "Insufficient Funds" errors caused by price increases
// between the risk check and the actual order execution.
const SlippageBuffer = 0.99 // 1% safety buffer

// Config defines the global risk parameters for the manager.
type RiskConfig struct {
	MaxOpenPositions  int
	MaxBudgetPerTrade map[string]float64
}

// PairRisk defines the operational risk rules for a specific trading pair.
type PairRisk struct {
	InstrumentSymbol string
	AllocatedBudget  float64
	MaxAssetUnits    float64
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
	// ApprovedUnits is the quantity of the asset to buy/sell.
	ApprovedUnits float64
}

// NewManager creates a new risk manager with the provided configuration.
func NewManager(logger *slog.Logger, cfg config.RiskConfig) *Manager {
	logger.Info("Initializing Risk Manager",
		"max_open_positions", cfg.MaxOpenPositions,
		"max_budget_per_trade", cfg.MaxBudgetPerTrade,
	)

	return &Manager{
		logger: logger,
		config: RiskConfig{
			MaxOpenPositions:  cfg.MaxOpenPositions,
			MaxBudgetPerTrade: cfg.MaxBudgetPerTrade,
		},
	}
}

// EvaluateEntry checks if a new trade can be opened and calculates the position size.
// It now considers the available quote balance (USDT/BRL) on the exchange.
func (m *Manager) EvaluateEntry(
	currentPositions int, price float64, availableBudget float64, pr PairRisk,
) Evaluation {
	// --- Global Validation Checks ---
	if m.config.MaxOpenPositions > 0 && currentPositions >= m.config.MaxOpenPositions {
		m.logger.Warn("Risk check failed: Max open positions reached",
			"current", currentPositions,
			"limit", m.config.MaxOpenPositions)
		return Evaluation{
			Allowed: false,
			Reason: fmt.Sprintf(
				"max open positions reached (%d >= %d)", currentPositions, m.config.MaxOpenPositions,
			),
		}
	}

	if availableBudget < MinExchangeBudget {
		m.logger.Warn("Risk check failed: Insufficient exchange balance", "available", availableBudget)
		return Evaluation{
			Allowed: false,
			Reason:  fmt.Sprintf("insufficient exchange balance (must be >= %.2f)", MinExchangeBudget),
		}
	}

	if price <= 0 {
		return Evaluation{
			Allowed: false,
			Reason:  "invalid price (must be > 0)",
		}
	}

	// --- Pair Validation Checks ---
	asset, budgetAsset := "", ""
	if parts := strings.Split(pr.InstrumentSymbol, "/"); len(parts) == 2 {
		asset = parts[0]
		budgetAsset = parts[1]
	}

	// Cap allocated budget by global budget limit and available exchange balance
	targetBudget := pr.AllocatedBudget

	if limit, ok := m.config.MaxBudgetPerTrade[budgetAsset]; ok && limit > 0 {
		if targetBudget > limit {
			m.logger.Warn("Risk check: Budget capped by global budget limit",
				"asset", budgetAsset, "requested", pr.AllocatedBudget, "limit", limit)
			targetBudget = limit
		}
	}

	if targetBudget > availableBudget {
		m.logger.Warn("Budget exceeds available exchange balance, adjusting target value",
			"asset", budgetAsset, "budget", targetBudget, "available", availableBudget)
		targetBudget = availableBudget
	}

	if targetBudget < MinExchangeBudget {
		m.logger.Warn("Risk check failed: Invalid budget configuration", "value", targetBudget)
		return Evaluation{
			Allowed: false,
			Reason: fmt.Sprintf(
				"invalid (capped) allocated budget configuration (must be >= %.2f)", MinExchangeBudget,
			),
		}
	}

	// Final cap by Max Asset Units (Quantity cap)
	assetUnits := targetBudget / price

	if pr.MaxAssetUnits > 0 && assetUnits > pr.MaxAssetUnits {
		m.logger.Warn("Position quantity capped by asset unit limit",
			"asset", asset, "requested", assetUnits, "limit", pr.MaxAssetUnits)
		assetUnits = pr.MaxAssetUnits
	}

	// Apply safety buffer to handle potential price slippage before execution.
	assetUnits *= SlippageBuffer

	return Evaluation{
		Allowed:       true,
		ApprovedUnits: assetUnits,
	}
}
