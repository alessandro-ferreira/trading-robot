package portfolio

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"trading/robot/go-bot/internal/database"
	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/strategy"
)

// CashBalance represents the available liquidity for a specific asset on an exchange.
type CashBalance struct {
	Exchange string
	Asset    string
	Free     float64
	Used     float64
	Total    float64
}

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

	// cashBalances tracks liquid assets per exchange and per currency using key "exchange|asset".
	cashBalances map[string]*CashBalance

	// positions maps "exchange|symbol" to the current Position.
	positions map[string]*Position
}

// NewPortfolio creates a new Portfolio instance.
func NewPortfolio(logger *slog.Logger, db *database.DB, repo *repository.Container) *Portfolio {
	return &Portfolio{
		logger:       logger,
		db:           db,
		repo:         repo,
		cashBalances: make(map[string]*CashBalance),
		positions:    make(map[string]*Position),
	}
}

// LoadState hydrates the portfolio state from the persistent storage.
// This should be called on application startup or when a full refresh is needed.
func (p *Portfolio) LoadState(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Hydrate liquid cash balances
	balances, err := p.repo.Balances.GetAllBalances(ctx, p.db)
	if err != nil {
		return fmt.Errorf("load state: failed to fetch balances: %w", err)
	}

	p.cashBalances = make(map[string]*CashBalance)
	for _, b := range balances {
		key := makeKey(b.ExchangeName, b.AssetSymbol)
		p.cashBalances[key] = &CashBalance{
			Exchange: b.ExchangeName,
			Asset:    b.AssetSymbol,
			Free:     b.Free,
			Used:     b.Used,
			Total:    b.Total,
		}
	}

	// Hydrate open positions
	positions, err := p.repo.Positions.GetOpenPositions(ctx, p.db, "", "")
	if err != nil {
		return fmt.Errorf("load state: failed to fetch positions: %w", err)
	}

	p.positions = make(map[string]*Position)
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

// splitSymbol extracts base and quote assets from a symbol (e.g., "BTC/USDT" -> "BTC", "USDT").
func splitSymbol(symbol string) (string, string) {
	parts := strings.Split(symbol, "/")
	if len(parts) != 2 {
		return symbol, ""
	}
	return parts[0], parts[1]
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
