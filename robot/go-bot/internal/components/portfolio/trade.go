package portfolio

import (
	"context"
	"fmt"

	pb "trading/robot/go-bot/gen/go/v1"
	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/strategy"
)

// ApplyExecution updates the portfolio state based on a finalized order execution.
// It handles the conversion from gRPC OrderResponse to internal position updates.
func (p *Portfolio) ApplyExecution(ctx context.Context, exchange string, order *pb.OrderResponse) error {
	if order == nil || order.Status != "closed" {
		return nil
	}

	quantity := order.Filled
	if order.Side == "sell" {
		quantity = -quantity
	}

	// Use average execution price if available, otherwise fall back to limit price
	price := order.Average
	if price <= 0 {
		price = order.Price
	}

	p.logger.Info("Applying execution to portfolio",
		"symbol", order.Symbol,
		"side", order.Side,
		"filled", order.Filled,
		"avg_price", price)

	return p.UpdatePosition(ctx, exchange, order.Symbol, quantity, price)
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
				Exchange:      exchange,
				Symbol:        symbol,
				Quantity:      quantity,
				EntryPrice:    price,
				CurrentPrice:  price,
				HighestPrice:  price,
				StrategyState: strategy.StateActive,
			}
		} else {
			// Calculate new weighted average entry price
			totalCost := (pos.Quantity * pos.EntryPrice) + cost
			newQuantity := pos.Quantity + quantity
			pos.EntryPrice = totalCost / newQuantity
			pos.Quantity = newQuantity
			pos.CurrentPrice = price
			if price > pos.HighestPrice {
				pos.HighestPrice = price
			}
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

		// Clean up empty positions using epsilon for float comparison
		const epsilon = 1e-9
		if pos.Quantity <= epsilon {
			delete(p.positions, key)
			pos = nil
		}
	}

	// Persist changes to DB
	var err error
	if pos != nil {
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
		err = p.repo.Positions.UpsertPosition(ctx, p.db, dto)
	} else {
		err = p.repo.Positions.DeletePosition(ctx, p.db, exchange, symbol)
	}

	if err != nil {
		p.logger.Error("Failed to persist portfolio state", "error", err)
	}

	p.logger.Info("Portfolio updated", "exchange", exchange, "symbol", symbol, "quantity", quantity, "price", price, "cash", p.cashBalance)
	return nil
}
