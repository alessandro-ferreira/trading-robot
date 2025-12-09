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

// Close gracefully closes the connection to the gateway.
func (c *GatewayClient) Close() error {
	return c.conn.Close()
}
