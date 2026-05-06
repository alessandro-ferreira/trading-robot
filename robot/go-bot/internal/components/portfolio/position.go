package portfolio

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"trading/robot/go-bot/internal/database/repository"

	"github.com/jackc/pgx/v5"
)

// GetPosition
func (p *portfolio) GetPosition(
	ctx context.Context, exchange, symbol string,
) (repository.PositionData, error) {
	return p.repo.Positions.GetPosition(ctx, p.db, exchange, symbol)
}

// CreatePosition
func (p *portfolio) CreatePosition(
	ctx context.Context, exchange, instrumentSymbol string, quantity, price float64, orderID int64,
) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := makeKey(exchange, instrumentSymbol)

	// Check for existing active position
	existing, err := p.repo.Positions.GetPosition(ctx, p.db, exchange, instrumentSymbol)
	if err == nil {
		if existing.UnknownOrigin {
			if orderID <= 0 || price <= 0 || quantity <= 0 {
				return fmt.Errorf(
					"failed to update existing position: orderId, price or quantity invalid",
					"quantity", quantity,
					"orderID", orderID,
					"price", price,
				)
			}
			existing.OrderID = sql.NullInt64{Int64: orderID, Valid: true}
			existing.EntryPrice = price
			existing.Quantity = quantity
			existing.HighestPrice = math.Max(existing.HighestPrice, price)
			existing.UnknownOrigin = false
			if err := p.repo.Positions.UpsertPosition(ctx, p.db, existing); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("failed to update existing position: position already exists with known origin")
		}
	} else if errors.Is(err, pgx.ErrNoRows) {
		dto := repository.PositionData{
			ExchangeName:     exchange,
			InstrumentSymbol: instrumentSymbol,
			OrderID:          sql.NullInt64{Int64: orderID, Valid: orderID > 0},
			Side:             repository.PositionSideLong,
			Quantity:         quantity,
			EntryPrice:       price,
			HighestPrice:     price,
			UnknownOrigin:    orderID == 0,
			Active:           true,
		}

		if err := p.repo.Positions.UpsertPosition(ctx, p.db, dto); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("failed to check existing position: %w", err)
	}

	p.positions[key] = struct{}{}
	return nil
}

// UpdatePosition
func (p *portfolio) UpdatePosition(
	ctx context.Context, exchange, instrumentSymbol string, updates repository.PositionData,
) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	pos, err := p.repo.Positions.GetPosition(ctx, p.db, exchange, instrumentSymbol)
	if err != nil {
		return err
	}

	// Apply selective updates
	if updates.OrderID.Valid {
		pos.OrderID = updates.OrderID
	}
	if updates.Quantity > 0 {
		pos.Quantity = updates.Quantity
	}
	if updates.EntryPrice > 0 {
		pos.EntryPrice = updates.EntryPrice
	}
	if updates.HighestPrice > 0 {
		pos.HighestPrice = math.Max(pos.HighestPrice, updates.HighestPrice)
	}
	pos.UnknownOrigin = updates.UnknownOrigin
	pos.StopLossBlock = updates.StopLossBlock

	return p.repo.Positions.UpsertPosition(ctx, p.db, pos)
}

// DeletePosition
func (p *portfolio) DeletePosition(ctx context.Context, exchange, instrumentSymbol string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.repo.Positions.DeletePosition(ctx, p.db, exchange, instrumentSymbol); err != nil {
		return err
	}

	key := makeKey(exchange, instrumentSymbol)
	delete(p.positions, key)
	return nil
}
