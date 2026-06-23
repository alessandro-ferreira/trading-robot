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
		ConnectionTimeout:    time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewGatewayClient(&cfg)
	require.NoError(t, err, "Failed to connect to python-gateway")
	defer client.Close()

	// Test Ping
	pong, err := client.Ping(ctx)
	require.NoError(t, err, "Ping should not error")
	require.NotEmpty(t, pong)

	// Reset state before running tests to ensure isolation
	_, err = client.ResetState(ctx)
	require.NoError(t, err, "Failed to reset gateway state")

	// Test GetTicker
	symbol := "BTC/USDT"
	tickerResp, err := client.GetTicker(ctx, "dummy", symbol)
	require.NoError(t, err, "GetTicker should not error")
	require.NotNil(t, tickerResp, "GetTicker response should not be nil")
	require.Equal(t, symbol, tickerResp.Symbol)
	require.Greater(t, tickerResp.Price, 0.0)

	// Test GetBalance (Initial setup from dummy.py: USDT 10000)
	balanceResp, err := client.GetBalance(ctx, "dummy", "USDT")
	require.NoError(t, err, "GetBalance should not error")
	require.NotNil(t, balanceResp, "GetBalance response should not be nil")
	require.NotEmpty(t, balanceResp.Balances)
	require.Equal(t, "USDT", balanceResp.Balances[0].Asset)
	require.Equal(t, 10000.0, balanceResp.Balances[0].Free)

	// Test CreateOrder (Market Buy) - Executes immediately in DummyExchange
	marketOrderReq := &pb.CreateOrderRequest{
		Exchange: "dummy",
		Symbol:   symbol,
		Side:     repository.OrderSideBuy,
		Type:     repository.OrderTypeMarket,
		Amount:   0.1,
	}
	marketResp, err := client.CreateOrder(ctx, marketOrderReq)
	require.NoError(t, err)
	require.Equal(t, repository.OrderStatusClosed, marketResp.Status)
	require.Equal(t, 0.1, marketResp.Filled)

	// Test GetRecentTrades - Verifying the trade from the market order
	recentTrades, err := client.GetRecentTrades(ctx, "dummy", symbol, 0, 10)
	require.NoError(t, err)
	require.NotEmpty(t, recentTrades.Orders)
	require.Equal(t, 0.1, recentTrades.Orders[0].Amount)

	// Test CreateOrder (Limit Buy) - Stays open in DummyExchange
	price := 30000.0
	createOrderReq := &pb.CreateOrderRequest{
		Exchange: "dummy",
		Symbol:   symbol,
		Side:     repository.OrderSideBuy,
		Type:     repository.OrderTypeLimit,
		Amount:   0.01,
		Price:    &price,
	}
	orderResp, err := client.CreateOrder(ctx, createOrderReq)
	require.NoError(t, err, "CreateOrder should not error")
	require.NotNil(t, orderResp, "CreateOrder response should not be nil")
	require.Equal(t, repository.OrderStatusOpen, orderResp.Status)

	// Test GetOrder
	getOrderResp, err := client.GetOrder(ctx, "dummy", symbol, orderResp.Id)
	require.NoError(t, err, "GetOrder should not error")
	require.Equal(t, orderResp.Id, getOrderResp.Id)
	require.Equal(t, repository.OrderStatusOpen, getOrderResp.Status)

	// Test GetOpenOrders
	openOrdersResp, err := client.GetOpenOrders(ctx, "dummy", symbol, 10)
	require.NoError(t, err, "GetOpenOrders should not error")
	require.Len(t, openOrdersResp.Orders, 1)
	require.Equal(t, orderResp.Id, openOrdersResp.Orders[0].Id)

	// Test CancelOrder
	cancelResp, err := client.CancelOrder(ctx, "dummy", symbol, orderResp.Id)
	require.NoError(t, err, "CancelOrder should not error")
	require.Equal(t, orderResp.Id, cancelResp.Id)
	require.Equal(t, repository.OrderStatusCanceled, cancelResp.Status)

	// Test CreateStopOrder
	stopPrice := 25000.0
	stopOrderReq := &pb.CreateStopOrderRequest{
		Exchange:  "dummy",
		Symbol:    symbol,
		Side:      repository.OrderSideSell,
		Amount:    0.05,
		StopPrice: stopPrice,
	}
	stopResp, err := client.CreateStopOrder(ctx, stopOrderReq)
	require.NoError(t, err)
	require.Equal(t, repository.OrderStatusOpen, stopResp.Status)
	require.Equal(t, stopPrice, stopResp.Price)

	// Test Exchange not configured error
	_, err = client.GetTicker(ctx, "nonexistent_exchange", symbol)
	require.Error(t, err, "Expected error for nonexistent exchange")
}
