//go:build integration

package execution

import (
	"context"
	"os"
	"testing"
	"time"

	pb "trading/robot/go-bot/gen/go/v1"
	"trading/robot/go-bot/internal/config"
	"trading/robot/go-bot/internal/database/repository"

	"github.com/stretchr/testify/require"
)

// getEnv returns the value of an environment variable or a default value.
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func TestGatewayClient_Integration(t *testing.T) {
	// These should match your docker-compose setup
	addr := getEnv("PYTHON_GATEWAY_ADDR", "localhost:15051")

	cfg := config.GRPCConfig{
		PythonGatewayAddress: addr,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewGatewayClient(&cfg)
	require.NoError(t, err, "Failed to connect to python-gateway")
	defer client.Close()

	// Reset state before running tests to ensure isolation
	_, err = client.ResetState(ctx)
	require.NoError(t, err, "Failed to reset gateway state")

	// Test GetTicker
	tickerResp, err := client.GetTicker(ctx, "dummy", "BTC/USDT")
	require.NoError(t, err, "GetTicker should not error")
	require.NotNil(t, tickerResp, "GetTicker response should not be nil")
	t.Logf("Ticker: %s", tickerResp.String())

	// Test GetBalance
	balanceResp, err := client.GetBalance(ctx, "dummy", "USDT")
	require.NoError(t, err, "GetBalance should not error")
	require.NotNil(t, balanceResp, "GetBalance response should not be nil")
	t.Logf("Balance: %s", balanceResp.String())

	// Test CreateOrder
	price := 20000.0
	createOrderReq := &pb.CreateOrderRequest{
		Exchange: "dummy",
		Symbol:   "BTC/USDT",
		Side:     repository.OrderSideBuy,
		Type:     repository.OrderTypeLimit,
		Amount:   0.01,
		Price:    &price,
	}
	orderResp, err := client.CreateOrder(ctx, createOrderReq)
	require.NoError(t, err, "CreateOrder should not error")
	require.NotNil(t, orderResp, "CreateOrder response should not be nil")
	t.Logf("Order: %s", orderResp.String())

	// Test CancelOrder
	cancelResp, err := client.CancelOrder(ctx, "dummy", "BTC/USDT", "order-id-123")
	require.NoError(t, err, "CancelOrder should not error")
	require.NotNil(t, cancelResp, "CancelOrder response should not be nil")
	t.Logf("Cancel: %s", cancelResp.String())

	// Test GetOrder
	getOrderResp, err := client.GetOrder(ctx, "dummy", "BTC/USDT", "order-id-123")
	require.NoError(t, err, "GetOrder should not error")
	require.NotNil(t, getOrderResp, "GetOrder response should not be nil")
	t.Logf("Get: %s", getOrderResp.String())

	// Test GetOpenOrders
	openOrdersResp, err := client.GetOpenOrders(ctx, "dummy", "BTC/USDT", 10)
	require.NoError(t, err, "GetOpenOrders should not error")
	require.NotNil(t, openOrdersResp, "GetOpenOrders response should not be nil")
	t.Logf("OpenOrders: %s", openOrdersResp.String())

	// Test Exchange not configured error
	_, err = client.GetTicker(ctx, "nonexistent_exchange", "BTC/USDT")
	require.Error(t, err, "Expected error for nonexistent exchange")
	t.Logf("Error: %s", err.Error())
}
