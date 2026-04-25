package portfolio

import (
	"context"

	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/strategy"
)

// RefreshState reloads specific balance and position data from the database.
func (p *Portfolio) RefreshState(ctx context.Context, exchange, symbol string, syncBalance, syncPosition bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if syncBalance {
		if balances, err := p.repo.Balances.GetAllBalances(ctx, p.db); err == nil {
			for _, b := range balances {
				if b.ExchangeName == exchange {
					key := makeKey(exchange, b.AssetSymbol)
					p.cashBalances[key] = &CashBalance{
						Exchange: b.ExchangeName,
						Asset:    b.AssetSymbol,
						Free:     b.Free,
						Used:     b.Used,
						Total:    b.Total,
					}
				}
			}
		}
	}

	if syncPosition {
		posData, err := p.repo.Positions.GetPosition(ctx, p.db, exchange, symbol)
		key := makeKey(exchange, symbol)
		if err != nil {
			delete(p.positions, key)
		} else {
			p.positions[key] = &Position{
				Exchange:      posData.ExchangeName,
				Symbol:        posData.InstrumentSymbol,
				Quantity:      posData.Quantity,
				EntryPrice:    posData.EntryPrice,
				HighestPrice:  posData.HighestPrice,
				StrategyState: toState(posData.StrategyState),
			}
		}
	}

	return nil
}

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
			p.logger.Warn("Failed to persist high-water mark", "symbol", symbol, "error", err)
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
			p.logger.Warn("Failed to persist strategy state", "symbol", symbol, "error", err)
		}
	}
}
