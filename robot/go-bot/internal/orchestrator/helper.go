package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"trading/robot/go-bot/internal/components/signal_generator"
	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/utils"

	"github.com/jackc/pgx/v5"
)

// ----------------------------------------------------------------------------
// Helper Methods
// ----------------------------------------------------------------------------

// checkPosition searches for an active position in the portfolio and handles unknown origin positions.
func (o *Orchestrator) checkPosition(
	ctx context.Context,
	log *slog.Logger,
	exchange, instrumentSymbol string,
	sig *signal_generator.SignalGenerator,
) (repository.PositionData, error) {
	pos, err := o.portfolio.GetPosition(ctx, exchange, instrumentSymbol)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return repository.PositionData{}, nil
		}
		log.Error("failed to query position", "err", err)
		return repository.PositionData{}, fmt.Errorf("failed to query position: %w", err)
	}

	if pos.UnknownOrigin {
		log.Warn("position stuck in invalid state (no order link), resetting strategy state")
		sig.SetInPosition(false, 0, 0)
		return pos, nil
	}

	return pos, nil
}

// checkPendingOrder checks if there is a pending order in the database and verifies its existence on the exchange.
func (o *Orchestrator) checkPendingOrder(
	ctx context.Context,
	log *slog.Logger,
	exchange, instrumentSymbol string,
) error {
	// Check if we have a pending order (status 'new' and no exchange order ID) in the database.
	status := []string{repository.OrderStatusNew}
	dbOrders, err := o.repo.Orders.GetOrders(
		ctx, o.db, exchange, instrumentSymbol, status, []string{}, []string{}, 1,
	)
	if err != nil {
		log.Error("failed to fetch pending orders from database", "err", err)
		return fmt.Errorf("failed to fetch pending orders from database: %w", err)
	}

	if len(dbOrders) < 1 || dbOrders[0].ExchangeOrderID.Valid {
		return nil
	}
	dbOrder := dbOrders[0]

	// If we have a pending order, check whether it exists on the exchange before rejecting it.
	for _, retryDelay := range o.cfg.Server.CheckPendingPolicy {
		if retryDelay > 0 {
			select {
			case <-time.After(retryDelay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// Fetch the latest orders from the exchange to check if the pending order exists.
		exchangeOrders, err := o.exec.GetOrders(ctx, exchange, instrumentSymbol, 10, false)
		if err != nil {
			log.Error("failed to fetch orders from exchange", "err", err)
			return fmt.Errorf("failed to fetch orders from exchange: %w", err)
		}

		for _, order := range exchangeOrders {
			if dbOrder.Side == order.Side &&
				utils.IsEqualEps(dbOrder.Amount, order.Amount) {
				// If the order matches the pending order, attempt to link it to the exchange order.
				dbOrder.ExchangeOrderID = order.ExchangeOrderID
				_, err = o.repo.Orders.UpdateOrder(ctx, o.db, dbOrder)
				if err != nil {
					// If the exchange order ID is already associated with another active order,
					// the unique index idx_orders_exchange_order_id_active prevents the update.
					log.Error(
						"failed to update pending order with exchange order ID", "err", err,
						"db_order_id", dbOrder.ID, "exchange_order_id", dbOrder.ExchangeOrderID,
					)
					dbOrder.ExchangeOrderID = sql.NullString{String: "", Valid: false}
					continue
				}

				log.Info(
					"pending order found on exchange, updated on database",
					"order_id", dbOrder.ID, "exchange_order_id", dbOrder.ExchangeOrderID,
				)

				return nil
			}
		}
	}

	// If we still have a pending order after retries, we set its status to 'rejected'.
	dbOrder.Status = repository.OrderStatusRejected
	_, err = o.repo.Orders.UpdateOrder(ctx, o.db, dbOrder)
	if err != nil {
		log.Error("failed to update pending order to rejected", "db_order_id", dbOrder.ID, "err", err)
		return fmt.Errorf("failed to update pending order to rejected: %w", err)
	}
	log.Warn("pending order not found on exchange after retries, marked as rejected", "order_id", dbOrder.ID)

	return nil
}

// getDbOpenOrders fetches open orders from the database and handles errors.
func (o *Orchestrator) getDbOpenOrders(
	ctx context.Context,
	log *slog.Logger,
	exchange, instrumentSymbol string,
	types, sides []string,
) ([]repository.OrderData, error) {
	statuses := []string{repository.OrderStatusNew, repository.OrderStatusOpen}
	dbOrders, err := o.repo.Orders.GetOrders(
		ctx, o.db, exchange, instrumentSymbol, statuses, types, sides, 1,
	)
	if err != nil {
		log.Error("failed to fetch open orders from database", "err", err)
		return nil, fmt.Errorf("failed to fetch open orders from database: %w", err)
	}

	return dbOrders, nil
}

// getBalance fetches the balance for a given asset from the exchange and handles errors.
func (o *Orchestrator) getBalance(
	ctx context.Context,
	log *slog.Logger,
	exchange, instrumentSymbol string,
) (repository.BalanceData, error) {
	asset := strings.Split(instrumentSymbol, "/")[0]
	balances, err := o.exec.GetBalance(ctx, exchange, asset)
	if err != nil || len(balances) == 0 {
		if err != nil {
			log.Error("failed to verify balance on exchange", "err", err)
		} else {
			log.Warn("balance no found on exchange", "asset", asset)
		}
		return repository.BalanceData{}, fmt.Errorf("failed to get balance: %w", err)
	}

	return balances[0], nil
}

// getStopOrder fetches the stop loss order for a given instrument from the exchange and handles errors.
func (o *Orchestrator) getStopLossOrder(
	ctx context.Context,
	log *slog.Logger,
	exchange, instrumentSymbol string,
) (repository.OrderData, error) {
	openOrders, err := o.exec.GetOpenOrders(ctx, exchange, instrumentSymbol, 10, true)
	if err != nil {
		log.Error("failed to fetch open orders for stop loss check", "err", err)
		return repository.OrderData{}, fmt.Errorf("failed to fetch open orders for stop loss check: %w", err)
	}

	for _, ord := range openOrders {
		if ord.Side == repository.OrderSideSell && ord.OrderType == repository.OrderTypeStopMarket {
			return ord, nil
		}
	}

	return repository.OrderData{}, nil
}
