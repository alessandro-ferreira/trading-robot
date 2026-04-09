package portfolio

import (
	"context"

	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/strategy"
)

// UpdatePrice updates the current price and unrealized P&L for a specific symbol.
func (p *Portfolio) UpdatePrice(ctx context.Context, exchange, symbol string, price float64) {
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

	// Update high-water mark for trailing stops
	if price > pos.HighestPrice {
		pos.HighestPrice = price

		// Persist to DB if the high-water mark changed to ensure crash-resilience
		dto := repository.PositionData{
			ExchangeName:     pos.Exchange,
			InstrumentSymbol: pos.Symbol,
			Side:             repository.PositionSideLong,
			Quantity:         pos.Quantity,
			EntryPrice:       pos.EntryPrice,
			HighestPrice:     pos.HighestPrice,
			StrategyState:    pos.StrategyState.String(),
			Active:           true,
		}
		if err := p.repo.Positions.UpsertPosition(ctx, p.db, dto); err != nil {
			p.logger.Error("Failed to persist high-water mark", "symbol", symbol, "error", err)
		}
	}
}

// SyncMetadata updates the strategy state for persistence.
func (p *Portfolio) SyncMetadata(ctx context.Context, exchange, symbol string, state strategy.StrategyState) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := makeKey(exchange, symbol)
	if pos, exists := p.positions[key]; exists {
		pos.StrategyState = state

		dto := repository.PositionData{
			ExchangeName:     pos.Exchange,
			InstrumentSymbol: pos.Symbol,
			Side:             repository.PositionSideLong,
			Quantity:         pos.Quantity,
			EntryPrice:       pos.EntryPrice,
			HighestPrice:     pos.HighestPrice,
			StrategyState:    pos.StrategyState.String(),
			Active:           true,
		}
		if err := p.repo.Positions.UpsertPosition(ctx, p.db, dto); err != nil {
			p.logger.Error("Failed to persist strategy state", "symbol", symbol, "error", err)
		}
	}
}
