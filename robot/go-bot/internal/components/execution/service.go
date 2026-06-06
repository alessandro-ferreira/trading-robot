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
	GetOpenOrders(
		ctx context.Context, exchange, instrumentSymbol string, limit int,
	) ([]repository.OrderData, error)
	GetRecentTrades(
		ctx context.Context, exchange, instrumentSymbol string, since int64, limit int,
	) ([]repository.OrderData, error)
}

type service struct {
	logger *slog.Logger
	db     *database.DB
	client GatewayClient
	repo   *repository.Container
}

// NewService creates a new execution Service.
func NewService(
	logger *slog.Logger,
	db *database.DB,
	client GatewayClient,
	repo *repository.Container,
) Service {
	return &service{
		logger: logger,
		db:     db,
		client: client,
		repo:   repo,
	}
}

// GetTicker fetches the current ticker for a given symbol on a specific exchange.
func (s *service) GetTicker(
	ctx context.Context, exchange, instrumentSymbol string,
) (repository.MarketDataTick, error) {
	log := s.logger.With("exchange", exchange, "symbol", instrumentSymbol)

	log.Info("Fetching ticker from exchange")

	// Fetch from Exchange via gRPC
	resp, err := s.client.GetTicker(ctx, exchange, instrumentSymbol)
	if err != nil {
		return repository.MarketDataTick{}, fmt.Errorf("failed to fetch ticker from gateway: %w", err)
	}

	log.Info("Ticker received", "price", resp.Price)

	// Persist the tick to the database for historical analysis and strategy warm-up
	tick := repository.MarketDataTick{
		ExchangeName: exchange,
		Symbol:       instrumentSymbol,
		Price:        resp.Price,
		TickUnixAt:   time.Now().Unix(),
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
	resp, err := s.client.GetBalance(ctx, exchange, assetSymbol)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch balances from gateway: %w", err)
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
		collected = append(collected, balance)
	}

	return collected, nil
}

// CreateOrder places a new order on the exchange and persists it to the database.
func (s *service) CreateOrder(
	ctx context.Context, exchange, instrumentSymbol, side, orderType string, amount, price float64,
) (repository.OrderData, error) {
	log := s.logger.With("exchange", exchange, "symbol", instrumentSymbol)

	log.Info("Creating order", "side", side, "type", orderType, "amount", amount, "price", price)

	// Create order on exchange
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
		return repository.OrderData{}, fmt.Errorf("failed to create order on gateway: %w", err)
	}

	log.Info("Order created on exchange", "exchange_order_id", resp.Id, "status", resp.Status)

	// Persist to Database
	orderData := repository.OrderData{
		ExchangeName:     exchange,
		InstrumentSymbol: instrumentSymbol,
		ExchangeOrderID:  resp.Id,
		Side:             side,
		OrderType:        orderType,
		Amount:           amount,
		Filled:           resp.Filled,
		Remaining:        resp.Remaining,
		Cost:             resp.Cost,
		Status:           resp.Status,
		Price:            sql.NullFloat64{Float64: price, Valid: price > 0},
		AveragePrice:     sql.NullFloat64{Float64: resp.Average, Valid: resp.Average > 0},
		Fee:              sql.NullFloat64{Float64: resp.Fee, Valid: resp.Fee > 0},
		FeeAssetSymbol: sql.NullString{
			String: resp.FeeCurrency,
			Valid:  resp.FeeCurrency != "",
		},
		ExchangeTimestamp: sql.NullTime{Time: time.UnixMilli(resp.Timestamp), Valid: resp.Timestamp > 0},
	}

	id, err := s.repo.Orders.CreateOrder(ctx, s.db, orderData)
	if err != nil {
		return orderData, fmt.Errorf("order created but failed to persist: %w", err)
	}

	log.Info("Order persisted successfully", "internal_id", id, "exchange_order_id", resp.Id)
	orderData.ID = id

	return orderData, nil
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
		ExchangeOrderID:  resp.Id,
		Side:             side,
		OrderType:        orderType,
		Amount:           amount,
		Filled:           resp.Filled,
		Remaining:        resp.Remaining,
		Cost:             resp.Cost,
		Status:           resp.Status,
		Price:            sql.NullFloat64{Float64: stopPrice, Valid: stopPrice > 0},
		AveragePrice:     sql.NullFloat64{Float64: resp.Average, Valid: resp.Average > 0},
		Fee:              sql.NullFloat64{Float64: resp.Fee, Valid: resp.Fee > 0},
		FeeAssetSymbol: sql.NullString{
			String: resp.FeeCurrency,
			Valid:  resp.FeeCurrency != "",
		},
		ExchangeTimestamp: sql.NullTime{Time: time.UnixMilli(resp.Timestamp), Valid: resp.Timestamp > 0},
	}

	id, err := s.repo.Orders.CreateOrder(ctx, s.db, orderData)
	if err != nil {
		return orderData, fmt.Errorf("stop order created but failed to persist: %w", err)
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
		return fmt.Errorf("failed to fetch order details after cancellation: %w", err)
	}

	log.Info(
		"Order canceled and fetched",
		"exchange_order_id", orderResp.Id, "status", orderResp.Status, "filled", orderResp.Filled,
	)

	// Update Database
	orderData := repository.OrderData{
		ExchangeName:     exchange,
		InstrumentSymbol: orderResp.Symbol,
		ExchangeOrderID:  orderResp.Id,
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
		Fee:          sql.NullFloat64{Float64: orderResp.Fee, Valid: orderResp.Fee > 0},
		FeeAssetSymbol: sql.NullString{
			String: orderResp.FeeCurrency,
			Valid:  orderResp.FeeCurrency != "",
		},
		ExchangeTimestamp: sql.NullTime{
			Time:  time.UnixMilli(orderResp.Timestamp),
			Valid: orderResp.Timestamp > 0,
		},
	}

	id, err := s.repo.Orders.UpdateOrder(ctx, s.db, orderData)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Warn(
				"Canceled order not found in database, creating new record",
				"exchange_order_id", orderResp.Id,
			)
			id, err = s.repo.Orders.CreateOrder(ctx, s.db, orderData)
		}
		if err != nil {
			return fmt.Errorf("order canceled but failed to update db: %w", err)
		}
	}

	log.Info(
		"Canceled order updated in database",
		"internal_id", id, "exchange_order_id", orderResp.Id, "status", orderResp.Status,
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

	// Update Database with the latest state
	orderData := repository.OrderData{
		ExchangeName:     exchange,
		InstrumentSymbol: orderResp.Symbol,
		ExchangeOrderID:  orderResp.Id,
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
		ExchangeTimestamp: sql.NullTime{
			Time:  time.UnixMilli(orderResp.Timestamp),
			Valid: orderResp.Timestamp > 0,
		},
		Fee: sql.NullFloat64{Float64: orderResp.Fee, Valid: orderResp.Fee > 0},
		FeeAssetSymbol: sql.NullString{
			String: orderResp.FeeCurrency,
			Valid:  orderResp.FeeCurrency != "",
		},
	}

	id, err := s.repo.Orders.UpdateOrder(ctx, s.db, orderData)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Warn(
				"Order not found in database, creating new record",
				"exchange_order_id", orderResp.Id,
			)
			id, err = s.repo.Orders.CreateOrder(ctx, s.db, orderData)
		}
		if err != nil {
			return orderData, fmt.Errorf("order fetched but failed to update db: %w", err)
		}
	}

	log.Info(
		"Fetched order updated in database",
		"internal_id", id, "exchange_order_id", orderResp.Id, "status", orderResp.Status,
	)
	orderData.ID = id

	return orderData, nil
}

// GetOpenOrders fetches all open orders for a symbol from the exchange and updates the database.
func (s *service) GetOpenOrders(
	ctx context.Context, exchange, instrumentSymbol string, limit int,
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
		orderData := repository.OrderData{
			ExchangeName:     exchange,
			InstrumentSymbol: orderResp.Symbol,
			ExchangeOrderID:  orderResp.Id,
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
			Fee:          sql.NullFloat64{Float64: orderResp.Fee, Valid: orderResp.Fee > 0},
			FeeAssetSymbol: sql.NullString{
				String: orderResp.FeeCurrency,
				Valid:  orderResp.FeeCurrency != "",
			},
			ExchangeTimestamp: sql.NullTime{
				Time:  time.UnixMilli(orderResp.Timestamp),
				Valid: orderResp.Timestamp > 0,
			},
		}

		id, err := s.repo.Orders.UpdateOrder(ctx, s.db, orderData)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				id, err = s.repo.Orders.CreateOrder(ctx, s.db, orderData)
			}
			if err != nil {
				log.Warn(
					"Failed to sync open order state",
					"error", err, "exchange_order_id", orderResp.Id,
				)
				// Continue to the next order even if one fails to update.
				continue
			}
		}

		log.Info(
			"Open order processed",
			"internal_id", id, "exchange_order_id", orderResp.Id, "status", orderResp.Status,
		)
		orderData.ID = id
		collected = append(collected, orderData)
	}

	log.Info("Open orders processed and database updated")

	return collected, nil
}

// GetRecentTrades fetches recent executions from the exchange and ensures they are persisted.
func (s *service) GetRecentTrades(
	ctx context.Context, exchange, instrumentSymbol string, since int64, limit int,
) ([]repository.OrderData, error) {
	log := s.logger.With("exchange", exchange, "symbol", instrumentSymbol)

	log.Info("Fetching trade history from exchange", "since", since, "limit", limit)

	resp, err := s.client.GetRecentTrades(ctx, exchange, instrumentSymbol, since, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch trades from gateway: %w", err)
	}

	collected := make([]repository.OrderData, 0, len(resp.Orders))
	for _, o := range resp.Orders {
		trade := repository.OrderData{
			ExchangeName:     exchange,
			InstrumentSymbol: o.Symbol,
			ExchangeOrderID:  o.Id,
			ClientOrderID:    sql.NullString{String: o.ClientOrderId, Valid: o.ClientOrderId != ""},
			Side:             o.Side,
			OrderType:        o.Type,
			Price:            sql.NullFloat64{Float64: o.Price, Valid: o.Price > 0},
			Amount:           o.Amount,
			Filled:           o.Filled,
			Remaining:        o.Remaining,
			AveragePrice:     sql.NullFloat64{Float64: o.Average, Valid: o.Average > 0},
			Cost:             o.Cost,
			Status:           o.Status,
			Fee:              sql.NullFloat64{Float64: o.Fee, Valid: o.Fee > 0},
			FeeAssetSymbol: sql.NullString{
				String: o.FeeCurrency,
				Valid:  o.FeeCurrency != "",
			},
			ExchangeTimestamp: sql.NullTime{Time: time.UnixMilli(o.Timestamp), Valid: o.Timestamp > 0},
		}

		// Sync historical execution to DB
		id, err := s.repo.Orders.UpdateOrder(ctx, s.db, trade)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				log.Warn(
					"Historical trade not found in database, creating new record",
					"exchange_order_id", o.Id,
				)
				id, err = s.repo.Orders.CreateOrder(ctx, s.db, trade)
			}
			if err != nil {
				log.Warn("Failed to sync historical trade", "error", err, "exchange_order_id", o.Id)
				// Continue to the next trade even if one fails to update.
				continue
			}
		}
		trade.ID = id
		collected = append(collected, trade)
	}

	log.Info("Trade history processed", "count", len(resp.Orders))

	return collected, nil
}
