package execution

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "trading/robot/go-bot/gen/go/v1"
	"trading/robot/go-bot/internal/config"
)

// GatewayClient is a client for the Python exchange gateway.
type GatewayClient struct {
	conn   *grpc.ClientConn
	client pb.ExchangeServiceClient
}

// NewGatewayClient creates and connects a new gRPC client to the Python gateway.
func NewGatewayClient(cfg *config.GRPCConfig) (*GatewayClient, error) {
	// For this project, we use insecure credentials for local communication.
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	conn, err := grpc.NewClient(cfg.PythonGatewayAddress, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC client: %w", err)
	}

	gwClient := &GatewayClient{
		conn:   conn,
		client: pb.NewExchangeServiceClient(conn),
	}

	// Perform an initial Ping to ensure the gateway is responsive on startup.
	// This makes the application fail fast if the connection cannot be established.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := gwClient.Ping(ctx); err != nil {
		conn.Close()
		return nil, fmt.Errorf("initial health check to python-gateway failed: %w", err)
	}

	return gwClient, nil
}

// Ping sends a Ping request to the gateway to check for liveness.
func (c *GatewayClient) Ping(ctx context.Context) (string, error) {
	resp, err := c.client.Ping(ctx, &pb.PingRequest{})
	if err != nil {
		return "", fmt.Errorf("ping request to gateway failed: %w", err)
	}
	return resp.GetMessage(), nil
}

// GetTicker fetches the current price for a given symbol.
func (c *GatewayClient) GetTicker(ctx context.Context, symbol string) (*pb.TickerResponse, error) {
	resp, err := c.client.GetTicker(ctx, &pb.GetTickerRequest{Symbol: symbol})
	if err != nil {
		return nil, fmt.Errorf("failed to get ticker for %s: %w", symbol, err)
	}
	return resp, nil
}

// GetBalance fetches the account balance.
func (c *GatewayClient) GetBalance(ctx context.Context, currency string) (*pb.BalanceResponse, error) {
	resp, err := c.client.GetBalance(ctx, &pb.GetBalanceRequest{Currency: currency})
	if err != nil {
		return nil, fmt.Errorf("failed to get balance for %s: %w", currency, err)
	}
	return resp, nil
}

// CreateOrder places a new order on the exchange.
func (c *GatewayClient) CreateOrder(ctx context.Context, req *pb.CreateOrderRequest) (*pb.OrderResponse, error) {
	resp, err := c.client.CreateOrder(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create order for %s: %w", req.Symbol, err)
	}
	return resp, nil
}

// CancelOrder cancels an existing order.
func (c *GatewayClient) CancelOrder(ctx context.Context, id, symbol string) (*pb.CancelOrderResponse, error) {
	resp, err := c.client.CancelOrder(ctx, &pb.CancelOrderRequest{Id: id, Symbol: symbol})
	if err != nil {
		return nil, fmt.Errorf("failed to cancel order %s: %w", id, err)
	}
	return resp, nil
}

// GetOrder fetches details of a specific order.
func (c *GatewayClient) GetOrder(ctx context.Context, id, symbol string) (*pb.OrderResponse, error) {
	resp, err := c.client.GetOrder(ctx, &pb.GetOrderRequest{Id: id, Symbol: symbol})
	if err != nil {
		return nil, fmt.Errorf("failed to get order %s: %w", id, err)
	}
	return resp, nil
}

// GetOpenOrders fetches all open orders for a symbol.
func (c *GatewayClient) GetOpenOrders(ctx context.Context, symbol string) (*pb.OpenOrdersResponse, error) {
	resp, err := c.client.GetOpenOrders(ctx, &pb.GetOpenOrdersRequest{Symbol: symbol})
	if err != nil {
		return nil, fmt.Errorf("failed to get open orders for %s: %w", symbol, err)
	}
	return resp, nil
}

// Close gracefully closes the connection to the gateway.
func (c *GatewayClient) Close() error {
	return c.conn.Close()
}
