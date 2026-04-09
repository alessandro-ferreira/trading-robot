package portfolio

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"trading/robot/go-bot/internal/database"
	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/strategy"
)

// Position represents the current holding of a specific asset.
type Position struct {
	Exchange           string
	Symbol             string
	Quantity           float64
	EntryPrice         float64
	CurrentPrice       float64
	HighestPrice       float64 // High-water mark for trailing stops
	StrategyState      strategy.StrategyState
	UnrealizedPnL      float64
	AssociatedOrderIDs []string // Tracks open orders (e.g., Stop Loss) related to this position
}

// Portfolio manages the state of assets and positions.
type Portfolio struct {
	mu     sync.RWMutex
	logger *slog.Logger
	db     *database.DB
	repo   *repository.Container

	// cashBalance tracks the quote currency (e.g., USDT) available for trading.
	cashBalance float64

	// positions maps symbol (e.g., "BTC/USD") to the current Position.
	positions map[string]*Position
}

// NewPortfolio creates a new Portfolio instance.
func NewPortfolio(logger *slog.Logger, db *database.DB, repo *repository.Container, initialCash float64) *Portfolio {
	return &Portfolio{
		logger:      logger,
		db:          db,
		repo:        repo,
		cashBalance: initialCash,
		positions:   make(map[string]*Position),
	}
}

// LoadState hydrates the portfolio state from the persistent storage.
// This should be called on application startup.
func (p *Portfolio) LoadState(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	positions, err := p.repo.Positions.GetOpenPositions(ctx, p.db)
	if err != nil {
		return fmt.Errorf("failed to load positions from repository: %w", err)
	}

	for _, posData := range positions {
		key := makeKey(posData.ExchangeName, posData.InstrumentSymbol)
		p.positions[key] = &Position{
			Exchange:      posData.ExchangeName,
			Symbol:        posData.InstrumentSymbol,
			Quantity:      posData.Quantity,
			EntryPrice:    posData.EntryPrice,
			HighestPrice:  posData.HighestPrice,
			StrategyState: toState(posData.StrategyState),
		}
	}
	return nil
}

// makeKey creates a unique key for the positions map.
func makeKey(exchange, symbol string) string {
	return fmt.Sprintf("%s|%s", exchange, symbol)
}

func toState(s string) strategy.StrategyState {
	switch s {
	case "idle":
		return strategy.StateIdle
	case "pending_buy":
		return strategy.StatePendingBuy
	case "active":
		return strategy.StateActive
	case "pending_sell":
		return strategy.StatePendingSell
	default:
		return strategy.StateIdle
	}
}
