package execution

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "trading/robot/go-bot/gen/go/v1"
	"trading/robot/go-bot/internal/config"
)

// GatewayClient defines the interface for communicating with the Python exchange gateway.
type GatewayClient interface {
	Ping(ctx context.Context) (string, error)
	GetTicker(ctx context.Context, exchange, symbol string) (*pb.TickerResponse, error)
	GetBalance(ctx context.Context, exchange, currency string) (*pb.BalanceResponse, error)
	CreateOrder(ctx context.Context, req *pb.CreateOrderRequest) (*pb.OrderResponse, error)
	CreateStopOrder(ctx context.Context, req *pb.CreateStopOrderRequest) (*pb.OrderResponse, error)
	CancelOrder(ctx context.Context, exchange, symbol, id string) (*pb.CancelOrderResponse, error)
	GetOrder(ctx context.Context, exchange, symbol, id string) (*pb.OrderResponse, error)
	GetOpenOrders(ctx context.Context, exchange, symbol string, limit int) (*pb.OrdersResponse, error)
	GetRecentTrades(
		ctx context.Context, exchange, symbol string, since int64, limit int,
	) (*pb.OrdersResponse, error)
	ResetState(ctx context.Context) (*pb.ResetStateResponse, error)
	Close() error
}

type gatewayClient struct {
	conn   *grpc.ClientConn
	client pb.ExchangeServiceClient
}

// NewGatewayClient creates and connects a new gRPC client to the Python gateway.
func NewGatewayClient(cfg *config.GRPCConfig) (GatewayClient, error) {
	// For this project, we use insecure credentials for local communication.
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	conn, err := grpc.NewClient(cfg.PythonGatewayAddress, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC client: %w", err)
	}

	gwClient := &gatewayClient{
		conn:   conn,
		client: pb.NewExchangeServiceClient(conn),
	}

	// Perform an initial Ping to ensure the gateway is responsive on startup.
	// This makes the application fail fast if the connection cannot be established.
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectionTimeout)
	defer cancel()

	_, err = gwClient.Ping(ctx)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("initial health check to python-gateway failed: %w", err)
	}

	return gwClient, nil
}

// Ping sends a Ping request to the gateway to check for liveness.
func (c *gatewayClient) Ping(ctx context.Context) (string, error) {
	resp, err := c.client.Ping(ctx, &pb.PingRequest{})
	if err != nil {
		return "", fmt.Errorf("ping request to gateway failed: %w", err)
	}
	return resp.GetMessage(), nil
}

// GetTicker fetches the current price for a given symbol.
func (c *gatewayClient) GetTicker(
	ctx context.Context, exchange, symbol string,
) (*pb.TickerResponse, error) {
	resp, err := c.client.GetTicker(ctx, &pb.GetTickerRequest{Exchange: exchange, Symbol: symbol})
	if err != nil {
		return nil, fmt.Errorf("failed to get ticker for %s on %s: %w", symbol, exchange, err)
	}
	return resp, nil
}

// GetBalance fetches the account balance.
func (c *gatewayClient) GetBalance(
	ctx context.Context, exchange, currency string,
) (*pb.BalanceResponse, error) {
	req := &pb.GetBalanceRequest{Exchange: exchange}
	if currency != "" {
		req.Currency = &currency
	}
	resp, err := c.client.GetBalance(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance for %s on %s: %w", currency, exchange, err)
	}
	return resp, nil
}

// CreateOrder places a new order on the exchange.
func (c *gatewayClient) CreateOrder(
	ctx context.Context, req *pb.CreateOrderRequest,
) (*pb.OrderResponse, error) {
	resp, err := c.client.CreateOrder(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create order for %s on %s: %w", req.Symbol, req.Exchange, err)
	}
	return resp, nil
}

// CreateStopOrder places a stop-loss or stop-limit order on the exchange.
func (c *gatewayClient) CreateStopOrder(
	ctx context.Context, req *pb.CreateStopOrderRequest,
) (*pb.OrderResponse, error) {
	resp, err := c.client.CreateStopOrder(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create stop order for %s on %s: %w", req.Symbol, req.Exchange, err)
	}
	return resp, nil
}

// CancelOrder cancels an existing order.
func (c *gatewayClient) CancelOrder(
	ctx context.Context, exchange, symbol, id string,
) (*pb.CancelOrderResponse, error) {
	resp, err := c.client.CancelOrder(
		ctx,
		&pb.CancelOrderRequest{Exchange: exchange, Id: id, Symbol: symbol},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to cancel order %s on %s: %w", id, exchange, err)
	}
	return resp, nil
}

// GetOrder fetches details of a specific order.
func (c *gatewayClient) GetOrder(
	ctx context.Context, exchange, symbol, id string,
) (*pb.OrderResponse, error) {
	resp, err := c.client.GetOrder(ctx, &pb.GetOrderRequest{Exchange: exchange, Id: id, Symbol: symbol})
	if err != nil {
		return nil, fmt.Errorf("failed to get order %s on %s: %w", id, exchange, err)
	}
	return resp, nil
}

// GetOpenOrders fetches all open orders for a symbol.
func (c *gatewayClient) GetOpenOrders(
	ctx context.Context, exchange, symbol string, limit int,
) (*pb.OrdersResponse, error) {
	req := &pb.GetOpenOrdersRequest{Exchange: exchange}
	if symbol != "" {
		req.Symbol = &symbol
	}
	if limit > 0 {
		l := int32(limit)
		req.Limit = &l
	}
	resp, err := c.client.GetOpenOrders(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get open orders for %s on %s: %w", symbol, exchange, err)
	}
	return resp, nil
}

// GetRecentTrades fetches historical executions.
func (c *gatewayClient) GetRecentTrades(
	ctx context.Context, exchange, symbol string, since int64, limit int,
) (*pb.OrdersResponse, error) {
	req := &pb.GetRecentTradesRequest{Exchange: exchange}
	if symbol != "" {
		req.Symbol = &symbol
	}
	if since > 0 {
		req.Since = &since
	}
	if limit > 0 {
		l := int32(limit)
		req.Limit = &l
	}
	resp, err := c.client.GetRecentTrades(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get trade history for %s on %s: %w", symbol, exchange, err)
	}
	return resp, nil
}

// ResetState resets the state of the exchange (used for testing).
func (c *gatewayClient) ResetState(ctx context.Context) (*pb.ResetStateResponse, error) {
	resp, err := c.client.ResetState(ctx, &pb.ResetStateRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to reset state: %w", err)
	}
	return resp, nil
}

// Close gracefully closes the connection to the gateway.
func (c *gatewayClient) Close() error {
	return c.conn.Close()
}
