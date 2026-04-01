package portfolio

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"trading/robot/go-bot/internal/database"
	"trading/robot/go-bot/internal/database/repository"
)

// Position represents the current holding of a specific asset.
type Position struct {
	Exchange           string
	Symbol             string
	Quantity           float64
	EntryPrice         float64
	CurrentPrice       float64
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

// makeKey creates a unique key for the positions map.
func makeKey(exchange, symbol string) string {
	return fmt.Sprintf("%s|%s", exchange, symbol)
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
			CurrentPrice:  posData.CurrentPrice,
			UnrealizedPnL: posData.UnrealizedPnL,
		}
	}
	return nil
}

// GetTotalValue calculates the total equity of the portfolio (Cash + Market Value of Positions).
func (p *Portfolio) GetTotalValue() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	total := p.cashBalance
	for _, pos := range p.positions {
		total += pos.CurrentPrice * pos.Quantity
	}
	return total
}

// GetOpenPositionsCount returns the number of currently active positions.
// This is used by the Risk Manager to enforce MaxOpenPositions limits.
func (p *Portfolio) GetOpenPositionsCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.positions)
}

// GetPosition returns a copy of the position for a given symbol.
func (p *Portfolio) GetPosition(exchange, symbol string) (*Position, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	key := makeKey(exchange, symbol)
	pos, exists := p.positions[key]
	if !exists {
		return nil, false
	}
	// Return a copy to avoid race conditions if the caller modifies it
	posCopy := *pos
	return &posCopy, true
}

// GetCashBalance returns the current available cash balance.
func (p *Portfolio) GetCashBalance() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cashBalance
}

// UpdatePrice updates the current price and unrealized P&L for a specific symbol.
func (p *Portfolio) UpdatePrice(exchange, symbol string, price float64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := makeKey(exchange, symbol)
	pos, exists := p.positions[key]
	if !exists {
		return
	}

	pos.CurrentPrice = price
	// Unrealized P&L = (Current Price - Entry Price) * Quantity
	pos.UnrealizedPnL = (price - pos.EntryPrice) * pos.Quantity
}

// UpdatePosition adds or updates a position based on a trade execution.
// quantity > 0 for Buy, quantity < 0 for Sell.
func (p *Portfolio) UpdatePosition(ctx context.Context, exchange, symbol string, quantity float64, price float64) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := makeKey(exchange, symbol)
	pos, exists := p.positions[key]

	if quantity > 0 { // BUY
		cost := quantity * price
		if p.cashBalance < cost {
			return fmt.Errorf("insufficient funds: required %.2f, available %.2f", cost, p.cashBalance)
		}

		p.cashBalance -= cost

		if !exists {
			pos = &Position{
				Exchange:     exchange,
				Symbol:       symbol,
				Quantity:     quantity,
				EntryPrice:   price,
				CurrentPrice: price,
			}
		} else {
			// Calculate new weighted average entry price
			totalCost := (pos.Quantity * pos.EntryPrice) + cost
			newQuantity := pos.Quantity + quantity
			pos.EntryPrice = totalCost / newQuantity
			// Just in case quantity was tiny/negative before, though shouldn't happen here
			pos.Quantity = newQuantity
			pos.CurrentPrice = price
		}
		p.positions[key] = pos
	} else { // SELL
		sellQuantity := -quantity
		if !exists || pos.Quantity < sellQuantity {
			return fmt.Errorf("insufficient position: holding %.4f, trying to sell %.4f",
				func() float64 {
					if pos == nil {
						return 0
					}
					return pos.Quantity
				}(), sellQuantity)
		}

		revenue := sellQuantity * price
		p.cashBalance += revenue
		pos.Quantity -= sellQuantity
		pos.CurrentPrice = price

		// Clean up empty positions
		const epsilon = 1e-9
		if pos.Quantity <= epsilon { // Epsilon for float comparison
			delete(p.positions, key)
			pos = nil // Mark as nil for persistence check
		}
	}

	// Persist changes
	var err error
	if pos != nil {
		dto := repository.PositionData{
			ExchangeName:     pos.Exchange,
			InstrumentSymbol: pos.Symbol,
			Side:             repository.PositionSideLong, // Defaulting to Long for Spot
			Quantity:         pos.Quantity,
			EntryPrice:       pos.EntryPrice,
			CurrentPrice:     pos.CurrentPrice,
			UnrealizedPnL:    pos.UnrealizedPnL,
			Active:           true,
		}
		err = p.repo.Positions.UpsertPosition(ctx, p.db, dto)
	} else {
		err = p.repo.Positions.DeletePosition(ctx, p.db, exchange, symbol)
	}

	if err != nil {
		p.logger.Error("Failed to persist portfolio state", "error", err)
		// We log the error but do not fail the function, as in-memory state is valid
	}

	p.logger.Info("Portfolio updated", "exchange", exchange, "symbol", symbol, "quantity", quantity, "price", price, "cash", p.cashBalance)
	return nil
}
