package execution

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"time"

	pb "trading/robot/go-bot/gen/go/v1"
	"trading/robot/go-bot/internal/database"
	"trading/robot/go-bot/internal/database/repository"
)

// TODO: Implement comprehensive failure handling for the full communication chain
// with recovery mechanisms (e.g., reconciliation, retries, state synchronization) to handle
// partial failures (e.g., order created on exchange but failed to persist in DB).

// Service provides methods for trade execution and order management.
type Service struct {
	logger *slog.Logger
	db     *database.DB
	client *GatewayClient
	repo   *repository.Container
}

// NewService creates a new execution Service.
func NewService(logger *slog.Logger, db *database.DB, client *GatewayClient, repo *repository.Container) *Service {
	return &Service{
		logger: logger,
		db:     db,
		client: client,
		repo:   repo,
	}
}

// GetTicker fetches the current ticker for a given symbol on a specific exchange.
func (s *Service) GetTicker(ctx context.Context, symbol, exchangeName string) (*pb.TickerResponse, error) {
	s.logger.Info("Fetching ticker from exchange", "exchange", exchangeName, "symbol", symbol)

	// Fetch from Exchange via gRPC
	resp, err := s.client.GetTicker(ctx, symbol, exchangeName)
	if err != nil {
		s.logger.Error("Failed to fetch ticker from gateway", "error", err, "exchange", exchangeName, "symbol", symbol)
		return nil, fmt.Errorf("failed to fetch ticker from gateway: %w", err)
	}

	s.logger.Info("Ticker received", "symbol", resp.Symbol, "price", resp.Price)

	// Persist the tick to the database for historical analysis and strategy warm-up
	tick := repository.MarketDataTick{
		ExchangeName: exchangeName,
		Symbol:       symbol,
		Price:        resp.Price,
		TickUnixAt:   time.Now().Unix(),
	}

	if err := s.repo.MarketData.InsertTick(ctx, s.db, tick); err != nil {
		s.logger.Warn("Failed to persist market data tick", "error", err, "symbol", symbol)
	}

	return resp, nil
}

// GetBalance retrieves the balance for a specific asset on a specific exchange.
func (s *Service) GetBalance(ctx context.Context, exchangeName, assetSymbol string) (*repository.BalanceData, error) {
	s.logger.Info("Fetching balance from exchange", "exchange", exchangeName, "asset", assetSymbol)

	// Fetch from Exchange via gRPC
	resp, err := s.client.GetBalance(ctx, assetSymbol, exchangeName)
	if err != nil {
		s.logger.Error("Failed to fetch balance from gateway", "error", err, "exchange", exchangeName, "asset", assetSymbol)
		return nil, fmt.Errorf("failed to fetch balance from gateway: %w", err)
	}

	// Extract values (default to 0 if missing)
	free := resp.Free[assetSymbol]
	used := resp.Used[assetSymbol]
	total := resp.Total[assetSymbol]

	// Validate that the numbers add up, accounting for float precision.
	const epsilon = 1e-9
	if math.Abs(total-(free+used)) > epsilon {
		s.logger.Warn(
			"Balance inconsistency detected from exchange",
			"asset", assetSymbol,
			"free", free,
			"used", used,
			"total", total,
			"discrepancy", total-(free+used),
		)
	}

	s.logger.Info("Balance received", "asset", assetSymbol, "free", free, "used", used, "total", total)

	// Persist to Database
	balance := repository.BalanceData{
		ExchangeName: exchangeName,
		AssetSymbol:  assetSymbol,
		Free:         free,
		Used:         used,
		Total:        total,
	}
	id, err := s.repo.Balances.UpsertBalance(ctx, s.db, balance)
	if err != nil {
		s.logger.Error("Failed to persist balance", "error", err, "exchange", exchangeName, "asset", assetSymbol)
		return nil, fmt.Errorf("failed to persist balance: %w", err)
	}
	balance.ID = id

	s.logger.Info("Balance persisted successfully", "internal_id", id, "exchange", exchangeName, "asset", assetSymbol, "total", total)

	return &balance, nil
}

// CreateOrder places a new order on the exchange and persists it to the database.
func (s *Service) CreateOrder(ctx context.Context, symbol, side, orderType string, amount, price float64, exchangeName string) (*pb.OrderResponse, error) {
	s.logger.Info("Creating order", "exchange", exchangeName, "symbol", symbol, "side", side, "type", orderType, "amount", amount, "price", price)

	// Create order on exchange
	req := &pb.CreateOrderRequest{
		Symbol:   symbol,
		Side:     side,
		Type:     orderType,
		Amount:   amount,
		Price:    price,
		Exchange: exchangeName,
	}

	resp, err := s.client.CreateOrder(ctx, req)
	if err != nil {
		s.logger.Error("Failed to create order on exchange", "error", err, "exchange", exchangeName, "symbol", symbol)
		return nil, fmt.Errorf("failed to create order on gateway: %w", err)
	}

	s.logger.Info("Order created on exchange", "exchange_order_id", resp.Id, "status", resp.Status)

	// Persist to Database
	orderData := repository.OrderData{
		ExchangeName:      exchangeName,
		InstrumentSymbol:  symbol,
		ExchangeOrderID:   resp.Id,
		Side:              side,
		OrderType:         orderType,
		Amount:            amount,
		Filled:            resp.Filled,
		Remaining:         resp.Remaining,
		Cost:              resp.Cost,
		Status:            resp.Status,
		Price:             sql.NullFloat64{Float64: price, Valid: price > 0},
		AveragePrice:      sql.NullFloat64{Float64: resp.Average, Valid: resp.Average > 0},
		ExchangeTimestamp: sql.NullTime{Time: time.UnixMilli(resp.Timestamp), Valid: resp.Timestamp > 0},
	}

	id, err := s.repo.Orders.CreateOrder(ctx, s.db, orderData)
	if err != nil {
		s.logger.Error("Failed to persist created order", "error", err, "exchange", exchangeName, "symbol", symbol, "exchange_order_id", resp.Id)
		return resp, fmt.Errorf("order created but failed to persist: %w", err)
	}

	s.logger.Info("Order persisted successfully", "internal_id", id, "exchange_order_id", resp.Id)

	return resp, nil
}

// CancelOrder cancels an existing order on the exchange and updates the database.
func (s *Service) CancelOrder(ctx context.Context, exchangeOrderID, symbol, exchangeName string) error {
	s.logger.Info("Canceling order", "exchange", exchangeName, "symbol", symbol, "exchange_order_id", exchangeOrderID)

	// Cancel on Exchange
	_, err := s.client.CancelOrder(ctx, exchangeOrderID, symbol, exchangeName)
	if err != nil {
		s.logger.Error("Failed to cancel order on gateway", "error", err, "exchange", exchangeName, "symbol", symbol, "exchange_order_id", exchangeOrderID)
		return fmt.Errorf("failed to cancel order on gateway: %w", err)
	}

	// Fetch latest order state from Exchange to ensure we have correct fill amounts
	//    Cancellation might result in a final fill or partial fill state.
	orderResp, err := s.client.GetOrder(ctx, exchangeOrderID, symbol, exchangeName)
	if err != nil {
		s.logger.Error("Failed to fetch order details after cancellation", "error", err, "exchange", exchangeName, "symbol", symbol, "exchange_order_id", exchangeOrderID)
		return fmt.Errorf("failed to fetch order details after cancellation: %w", err)
	}

	s.logger.Info("Order canceled and fetched", "exchange_order_id", orderResp.Id, "status", orderResp.Status, "filled", orderResp.Filled)

	// Update Database
	orderData := repository.OrderData{
		ExchangeName:      exchangeName,
		ExchangeOrderID:   orderResp.Id,
		Filled:            orderResp.Filled,
		Remaining:         orderResp.Remaining,
		Cost:              orderResp.Cost,
		Status:            orderResp.Status,
		AveragePrice:      sql.NullFloat64{Float64: orderResp.Average, Valid: orderResp.Average > 0},
		ExchangeTimestamp: sql.NullTime{Time: time.UnixMilli(orderResp.Timestamp), Valid: orderResp.Timestamp > 0},
	}

	id, err := s.repo.Orders.UpdateOrder(ctx, s.db, orderData)
	if err != nil {
		s.logger.Error("Failed to update canceled order in database", "error", err, "exchange", exchangeName, "symbol", symbol, "exchange_order_id", exchangeOrderID)
		return fmt.Errorf("order canceled but failed to update db: %w", err)
	}

	s.logger.Info("Canceled order updated in database", "internal_id", id, "exchange_order_id", orderResp.Id, "status", orderResp.Status)
	return nil
}

// GetOrder fetches the latest order details from the exchange and updates the database.
func (s *Service) GetOrder(ctx context.Context, exchangeOrderID, symbol, exchangeName string) (*pb.OrderResponse, error) {
	s.logger.Info("Fetching order from exchange", "exchange", exchangeName, "symbol", symbol, "exchange_order_id", exchangeOrderID)

	// Fetch latest order state from Exchange
	orderResp, err := s.client.GetOrder(ctx, exchangeOrderID, symbol, exchangeName)
	if err != nil {
		s.logger.Error("Failed to fetch order from gateway", "error", err, "exchange", exchangeName, "symbol", symbol, "exchange_order_id", exchangeOrderID)
		return nil, fmt.Errorf("failed to fetch order from gateway: %w", err)
	}

	s.logger.Info("Order fetched from exchange", "exchange_order_id", orderResp.Id, "status", orderResp.Status, "filled", orderResp.Filled)

	// Update Database with the latest state
	orderData := repository.OrderData{
		ExchangeName:      exchangeName,
		ExchangeOrderID:   orderResp.Id,
		Filled:            orderResp.Filled,
		Remaining:         orderResp.Remaining,
		Cost:              orderResp.Cost,
		Status:            orderResp.Status,
		AveragePrice:      sql.NullFloat64{Float64: orderResp.Average, Valid: orderResp.Average > 0},
		ExchangeTimestamp: sql.NullTime{Time: time.UnixMilli(orderResp.Timestamp), Valid: orderResp.Timestamp > 0},
	}

	id, err := s.repo.Orders.UpdateOrder(ctx, s.db, orderData)
	if err != nil {
		s.logger.Error("Failed to update fetched order in database", "error", err, "exchange", exchangeName, "symbol", symbol, "exchange_order_id", exchangeOrderID)
		return orderResp, fmt.Errorf("order fetched but failed to update db: %w", err)
	}

	s.logger.Info("Fetched order updated in database", "internal_id", id, "exchange_order_id", orderResp.Id, "status", orderResp.Status)
	return orderResp, nil
}

// GetOpenOrders fetches all open orders for a symbol from the exchange and updates the database.
func (s *Service) GetOpenOrders(ctx context.Context, symbol, exchangeName string) (*pb.OpenOrdersResponse, error) {
	s.logger.Info("Fetching open orders from exchange", "exchange", exchangeName, "symbol", symbol)

	resp, err := s.client.GetOpenOrders(ctx, symbol, exchangeName)
	if err != nil {
		s.logger.Error("Failed to fetch open orders from gateway", "error", err, "exchange", exchangeName, "symbol", symbol)
		return nil, fmt.Errorf("failed to fetch open orders from gateway: %w", err)
	}

	s.logger.Info("Open orders fetched", "count", len(resp.Orders))

	for _, orderResp := range resp.Orders {
		orderData := repository.OrderData{
			ExchangeName:      exchangeName,
			InstrumentSymbol:  orderResp.Symbol,
			ExchangeOrderID:   orderResp.Id,
			ClientOrderID:     sql.NullString{String: orderResp.ClientOrderId, Valid: orderResp.ClientOrderId != ""},
			Side:              orderResp.Side,
			OrderType:         orderResp.Type,
			Price:             sql.NullFloat64{Float64: orderResp.Price, Valid: orderResp.Price > 0},
			Amount:            orderResp.Amount,
			Filled:            orderResp.Filled,
			Remaining:         orderResp.Remaining,
			AveragePrice:      sql.NullFloat64{Float64: orderResp.Average, Valid: orderResp.Average > 0},
			Cost:              orderResp.Cost,
			Status:            orderResp.Status,
			ExchangeTimestamp: sql.NullTime{Time: time.UnixMilli(orderResp.Timestamp), Valid: orderResp.Timestamp > 0},
		}

		id, err := s.repo.Orders.UpdateOrder(ctx, s.db, orderData)
		if err != nil {
			s.logger.Warn("Failed to update open order in database", "error", err, "exchange_order_id", orderResp.Id)
			// Continue to the next order even if one fails to update.
			continue
		}

		s.logger.Info("Open order processed", "internal_id", id, "exchange_order_id", orderResp.Id, "status", orderResp.Status)
	}

	s.logger.Info("Open orders processed and database updated", "exchange", exchangeName, "symbol", symbol)

	return resp, nil
}
