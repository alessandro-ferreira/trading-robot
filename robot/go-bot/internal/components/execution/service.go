package execution

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	pb "trading/robot/go-bot/gen/go/v1"
	"trading/robot/go-bot/internal/database"
	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/utils"

	"github.com/jackc/pgx/v5"
)

// Service defines the interface for trade execution and order management.
type Service interface {
	GetTicker(
		ctx context.Context, exchange, instrumentSymbol string,
	) (repository.MarketDataTick, error)
	GetBalance(
		ctx context.Context, exchange, assetSymbol string,
	) ([]repository.BalanceData, error)
	CreateOrder(
		ctx context.Context, exchange, instrumentSymbol, side, orderType string, amount, price float64,
	) (repository.OrderData, error)
	CreateStopOrder(
		ctx context.Context, exchange, instrumentSymbol, side string, amount, stopPrice, limitPrice float64,
	) (repository.OrderData, error)
	CancelOrder(
		ctx context.Context, exchange, instrumentSymbol, exchangeOrderID string,
	) error
	GetOrder(
		ctx context.Context, exchange, instrumentSymbol, exchangeOrderID string,
	) (repository.OrderData, error)
	GetOrders(
		ctx context.Context, exchange, instrumentSymbol string, limit int, updatedb bool,
	) ([]repository.OrderData, error)
	GetOpenOrders(
		ctx context.Context, exchange, instrumentSymbol string, limit int, updatedb bool,
	) ([]repository.OrderData, error)
}

type service struct {
	logger *slog.Logger
	db     *database.DB
	client Client
	repo   *repository.Container
	clock  utils.Clock
}

// NewService creates a new execution Service.
func NewService(
	logger *slog.Logger,
	db *database.DB,
	client Client,
	repo *repository.Container,
	clock utils.Clock,
) Service {
	return &service{
		logger: logger,
		db:     db,
		client: client,
		repo:   repo,
		clock:  clock,
	}
}

// GetTicker fetches the current ticker for a given symbol on a specific exchange.
func (s *service) GetTicker(
	ctx context.Context, exchange, instrumentSymbol string,
) (repository.MarketDataTick, error) {
	log := s.logger.With("exchange", exchange, "symbol", instrumentSymbol)

	log.Debug("Fetching ticker from exchange")

	// Fetch from Exchange via gRPC
	resp, err := s.client.GetTicker(ctx, exchange, instrumentSymbol)
	if err != nil {
		return repository.MarketDataTick{}, fmt.Errorf("failed to fetch ticker from gateway: %w", err)
	}

	log.Debug("Ticker received", "price", resp.Price)

	// Persist the tick to the database for historical analysis and strategy warm-up
	tick := repository.MarketDataTick{
		ExchangeName: exchange,
		Symbol:       instrumentSymbol,
		Price:        resp.Price,
		TickUnixAt:   s.clock.Now().Unix(),
	}

	if err := s.repo.MarketData.InsertTick(ctx, s.db, tick); err != nil {
		return tick, fmt.Errorf("ticker received but failed to persist tick: %w", err)
	}

	return tick, nil
}

// GetBalance retrieves the balance for a specific asset on a specific exchange.
func (s *service) GetBalance(
	ctx context.Context, exchange, assetSymbol string,
) ([]repository.BalanceData, error) {
	log := s.logger.With("exchange", exchange)
	if assetSymbol != "" {
		log = log.With("symbol", assetSymbol)
	}

	log.Info("Fetching balances from exchange")

	// Fetch from Exchange via gRPC.
	resp, err := s.client.GetBalance(ctx, exchange)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch balances from gateway: %w", err)
	}

	log.Info("Balances received", "count", len(resp.Balances))
	for _, b := range resp.Balances {
		log.Debug("Balance received", "asset", b.Asset, "total", b.Total)
	}

	collected := make([]repository.BalanceData, 0, len(resp.Balances))
	// Iterate through all assets returned by the exchange to update the database
	for _, b := range resp.Balances {
		symbol := b.Asset
		free := b.Free
		used := b.Used
		total := b.Total

		// Validate that the numbers add up, accounting for float precision.
		const epsilon = 1e-9
		if math.Abs(total-(free+used)) > epsilon {
			log.Warn(
				"Balance inconsistency detected from exchange",
				"asset", symbol, "free", free, "used", used, "total", total, "discrepancy", total-(free+used),
			)
		}

		balance := repository.BalanceData{
			ExchangeName: exchange,
			AssetSymbol:  symbol,
			Free:         free,
			Used:         used,
			Total:        total,
		}

		id, err := s.repo.Balances.UpsertBalance(ctx, s.db, balance)
		if err != nil {
			// if assetSymbol is specified, we treat failure to persist as an error.
			if assetSymbol != "" {
				return nil, fmt.Errorf("failed to persist balance: %w", err)
			}
			log.Warn("Failed to persist balance", "error", err)
			continue
		}
		balance.ID = id

		// Ensure we only return the specific asset requested to the caller.
		if assetSymbol == "" || symbol == assetSymbol {
			collected = append(collected, balance)
		}
	}

	return collected, nil
}

// CreateOrder places a new order on the exchange and persists it to the database.
func (s *service) CreateOrder(
	ctx context.Context, exchange, instrumentSymbol, side, orderType string, amount, price float64,
) (repository.OrderData, error) {
	log := s.logger.With("exchange", exchange, "symbol", instrumentSymbol)

	log.Info("Creating order", "side", side, "type", orderType, "amount", amount, "price", price)

	// Persist to database first with status='new' and no exchange_order_id.
	// If the exchange call fails, the orphaned order will be reconciled later.
	orderData := repository.OrderData{
		ExchangeName:     exchange,
		InstrumentSymbol: instrumentSymbol,
		Side:             side,
		OrderType:        orderType,
		Amount:           amount,
		Status:           repository.OrderStatusNew,
		Price:            sql.NullFloat64{Float64: price, Valid: price > 0},
	}

	id, err := s.repo.Orders.CreateOrder(ctx, s.db, orderData)
	if err != nil {
		return repository.OrderData{}, fmt.Errorf("failed to persist order: %w", err)
	}
	orderData.ID = id

	log.Info("Order persisted locally", "internal_id", id, "amount", amount)

	// Send to exchange
	req := &pb.CreateOrderRequest{
		Exchange: exchange,
		Symbol:   instrumentSymbol,
		Side:     side,
		Type:     orderType,
		Amount:   amount,
	}
	if price > 0 {
		req.Price = &price
	}

	resp, err := s.client.CreateOrder(ctx, req)
	if err != nil {
		log.Warn(
			"Failed to create order on exchange, orphaned order persisted", "internal_id", id, "error", err,
		)
		return repository.OrderData{}, fmt.Errorf("failed to create order on gateway: %w", err)
	}

	log.Info("Order created on exchange", "exchange_order_id", resp.Id, "status", resp.Status)

	updateData := s.mapOrderResponse(exchange, resp)
	updateData.ID = id
	updateData, err = s.updateOrderResponse(ctx, updateData)
	if err != nil {
		return updateData, fmt.Errorf(
			"order %d (%s) created on exchange but failed to update db: %w", updateData.ID, resp.Id, err,
		)
	}

	log.Info("Order updated with exchange confirmation", "internal_id", id, "exchange_order_id", resp.Id)

	return updateData, nil
}

// CreateStopOrder places a stop-loss or take-profit order (market or limit trigger) and persists it.
func (s *service) CreateStopOrder(
	ctx context.Context, exchange, instrumentSymbol, side string, amount, stopPrice, limitPrice float64,
) (repository.OrderData, error) {
	log := s.logger.With("exchange", exchange, "symbol", instrumentSymbol)

	orderType := repository.OrderTypeStopMarket
	if limitPrice > 0 {
		orderType = repository.OrderTypeStopLimit
	}

	log.Info(
		"Creating stop order", "side", side, "type", orderType, "amount", amount, "stop_price", stopPrice,
	)

	req := &pb.CreateStopOrderRequest{
		Exchange:  exchange,
		Symbol:    instrumentSymbol,
		Side:      side,
		Amount:    amount,
		StopPrice: stopPrice,
	}
	if limitPrice > 0 {
		req.LimitPrice = &limitPrice
	}

	resp, err := s.client.CreateStopOrder(ctx, req)
	if err != nil {
		return repository.OrderData{}, fmt.Errorf("failed to create stop order on gateway: %w", err)
	}

	log.Info("Stop order created on exchange", "exchange_order_id", resp.Id, "status", resp.Status)

	// Persist to Database
	orderData := repository.OrderData{
		ExchangeName:     exchange,
		InstrumentSymbol: instrumentSymbol,
		ExchangeOrderID:  sql.NullString{String: resp.Id, Valid: resp.Id != ""},
		Side:             side,
		OrderType:        orderType,
		Amount:           amount,
		Filled:           resp.Filled,
		Remaining:        resp.Remaining,
		Cost:             resp.Cost,
		Status:           resp.Status,
		Price:            sql.NullFloat64{Float64: resp.Price, Valid: resp.Price > 0},
		AveragePrice:     sql.NullFloat64{Float64: resp.Average, Valid: resp.Average > 0},
		Fee:              sql.NullFloat64{Float64: resp.Fee, Valid: resp.FeeCurrency != ""},
		FeeAssetSymbol: sql.NullString{
			String: resp.FeeCurrency,
			Valid:  resp.FeeCurrency != "",
		},
		ExchangeTimestamp: sql.NullTime{Time: time.UnixMilli(resp.Timestamp), Valid: resp.Timestamp > 0},
	}

	id, err := s.repo.Orders.CreateOrder(ctx, s.db, orderData)
	if err != nil {
		return orderData, fmt.Errorf(
			"stop order %d (%s) created but failed to persist: %w", id, resp.Id, err,
		)
	}

	log.Info("Stop order persisted successfully", "internal_id", id, "exchange_order_id", resp.Id)
	orderData.ID = id

	return orderData, nil
}

// CancelOrder cancels an existing order on the exchange and updates the database.
func (s *service) CancelOrder(
	ctx context.Context, exchange, instrumentSymbol, exchangeOrderID string,
) error {
	log := s.logger.With("exchange", exchange, "symbol", instrumentSymbol)

	log.Info("Canceling order", "exchange_order_id", exchangeOrderID)

	// Cancel on Exchange
	_, err := s.client.CancelOrder(ctx, exchange, instrumentSymbol, exchangeOrderID)
	if err != nil {
		return fmt.Errorf("failed to cancel order on gateway: %w", err)
	}

	// Fetch latest order state from Exchange to ensure we have correct fill amounts
	//    Cancellation might result in a final fill or partial fill state.
	orderResp, err := s.client.GetOrder(ctx, exchange, instrumentSymbol, exchangeOrderID)
	if err != nil {
		return fmt.Errorf("failed to fetch order %s details after cancellation: %w", exchangeOrderID, err)
	}

	log.Info(
		"Order canceled and fetched",
		"exchange_order_id", orderResp.Id, "status", orderResp.Status, "filled", orderResp.Filled,
	)

	orderData, err := s.updateOrderResponse(ctx, s.mapOrderResponse(exchange, orderResp))
	if err != nil {
		return fmt.Errorf(
			"order %d (%s) canceled on exchange but failed to update db: %w",
			orderData.ID, orderResp.Id, err,
		)
	}

	log.Info(
		"Canceled order updated in database",
		"internal_id", orderData.ID, "exchange_order_id", orderResp.Id, "status", orderResp.Status,
	)

	return nil
}

// GetOrder fetches the latest order details from the exchange and updates the database.
func (s *service) GetOrder(
	ctx context.Context, exchange, instrumentSymbol, exchangeOrderID string,
) (repository.OrderData, error) {
	log := s.logger.With("exchange", exchange, "symbol", instrumentSymbol)

	log.Info("Fetching order from exchange", "exchange_order_id", exchangeOrderID)

	// Fetch latest order state from Exchange
	orderResp, err := s.client.GetOrder(ctx, exchange, instrumentSymbol, exchangeOrderID)
	if err != nil {
		return repository.OrderData{}, fmt.Errorf("failed to fetch order from gateway: %w", err)
	}

	log.Info(
		"Order fetched from exchange",
		"exchange_order_id", orderResp.Id, "status", orderResp.Status, "filled", orderResp.Filled,
	)

	orderData, err := s.updateOrderResponse(ctx, s.mapOrderResponse(exchange, orderResp))
	if err != nil {
		return orderData, fmt.Errorf(
			"order %d (%s) fetched but failed to update db: %w", orderData.ID, orderResp.Id, err,
		)
	}

	log.Info(
		"Fetched order updated in database",
		"internal_id", orderData.ID, "exchange_order_id", orderResp.Id, "status", orderResp.Status,
	)

	return orderData, nil
}

// GetOrders fetches a list of orders for a symbol from the exchange and updates the database.
func (s *service) GetOrders(
	ctx context.Context, exchange, instrumentSymbol string, limit int, updatedb bool,
) ([]repository.OrderData, error) {
	log := s.logger.With("exchange", exchange, "symbol", instrumentSymbol)

	log.Info("Fetching orders from exchange", "limit", limit)

	resp, err := s.client.GetOrders(ctx, exchange, instrumentSymbol, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch orders from gateway: %w", err)
	}

	log.Info("Orders fetched from exchange", "count", len(resp.Orders))

	collected := make([]repository.OrderData, 0, len(resp.Orders))
	for _, orderResp := range resp.Orders {
		orderData := s.mapOrderResponse(exchange, orderResp)
		if updatedb {
			orderData, err = s.updateOrderResponse(ctx, orderData)
			if err != nil {
				log.Warn(
					"Failed to update order state",
					"error", err, "exchange_order_id", orderResp.Id,
				)
				// Continue to the next order even if one fails to update.
				continue
			}
		}

		log.Info(
			"Order processed",
			"internal_id", orderData.ID, "exchange_order_id", orderResp.Id, "status", orderResp.Status,
		)
		collected = append(collected, orderData)
	}

	log.Info("Orders processed", "updatedb", updatedb)

	return collected, nil
}

// GetOpenOrders fetches all open orders for a symbol from the exchange and updates the database.
func (s *service) GetOpenOrders(
	ctx context.Context, exchange, instrumentSymbol string, limit int, updatedb bool,
) ([]repository.OrderData, error) {
	log := s.logger.With("exchange", exchange, "symbol", instrumentSymbol)

	log.Info("Fetching open orders from exchange", "limit", limit)

	resp, err := s.client.GetOpenOrders(ctx, exchange, instrumentSymbol, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch open orders from gateway: %w", err)
	}

	log.Info("Open orders fetched", "count", len(resp.Orders))

	collected := make([]repository.OrderData, 0, len(resp.Orders))
	for _, orderResp := range resp.Orders {
		orderData := s.mapOrderResponse(exchange, orderResp)
		if updatedb {
			orderData, err = s.updateOrderResponse(ctx, orderData)
			if err != nil {
				log.Warn(
					"Failed to update open order state",
					"error", err, "exchange_order_id", orderResp.Id,
				)
				// Continue to the next order even if one fails to update.
				continue
			}
		}

		log.Info(
			"Open order processed",
			"internal_id", orderData.ID, "exchange_order_id", orderResp.Id, "status", orderResp.Status,
		)
		collected = append(collected, orderData)
	}

	log.Info("Open orders processed", "updatedb", updatedb)

	return collected, nil
}

// mapOrderResponse maps a gRPC OrderResponse to the internal OrderData structure.
func (s *service) mapOrderResponse(
	exchange string, orderResp *pb.OrderResponse,
) repository.OrderData {
	return repository.OrderData{
		ExchangeName:     exchange,
		InstrumentSymbol: orderResp.Symbol,
		ExchangeOrderID: sql.NullString{
			String: orderResp.Id,
			Valid:  orderResp.Id != "",
		},
		ClientOrderID: sql.NullString{
			String: orderResp.ClientOrderId,
			Valid:  orderResp.ClientOrderId != "",
		},
		Side:         orderResp.Side,
		OrderType:    orderResp.Type,
		Price:        sql.NullFloat64{Float64: orderResp.Price, Valid: orderResp.Price > 0},
		Amount:       orderResp.Amount,
		Filled:       orderResp.Filled,
		Remaining:    orderResp.Remaining,
		AveragePrice: sql.NullFloat64{Float64: orderResp.Average, Valid: orderResp.Average > 0},
		Cost:         orderResp.Cost,
		Status:       orderResp.Status,
		Fee:          sql.NullFloat64{Float64: orderResp.Fee, Valid: orderResp.FeeCurrency != ""},
		FeeAssetSymbol: sql.NullString{
			String: orderResp.FeeCurrency,
			Valid:  orderResp.FeeCurrency != "",
		},
		ExchangeTimestamp: sql.NullTime{
			Time:  time.UnixMilli(orderResp.Timestamp),
			Valid: orderResp.Timestamp > 0,
		},
	}
}

// updateOrderResponse updates the order record in the database based on the latest exchange response.
func (s *service) updateOrderResponse(
	ctx context.Context, orderData repository.OrderData,
) (repository.OrderData, error) {
	id, err := s.repo.Orders.UpdateOrder(ctx, s.db, orderData)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.logger.Warn(
				"Order not found in database, creating new record",
				"exchange_order_id", orderData.ExchangeOrderID,
			)
			id, err = s.repo.Orders.CreateOrder(ctx, s.db, orderData)
		}
		if err != nil {
			return orderData, err
		}
	}

	orderData.ID = id
	return orderData, nil
}
