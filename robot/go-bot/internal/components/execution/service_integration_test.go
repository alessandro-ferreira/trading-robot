//go:build integration

package execution

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"trading/robot/go-bot/internal/config"
	"trading/robot/go-bot/internal/database"
	"trading/robot/go-bot/internal/database/repository"
)

// setupIntegrationTest initializes all dependencies for an integration test.
// It returns the initialized Service and a cleanup function to release resources after the test.
func setupIntegrationTest(t *testing.T) (Service, GatewayClient, *database.DB, *repository.Container, func()) {
	// Use t.Helper() to indicate this is a test helper function.
	t.Helper()

	// getEnv returns the value of an environment variable or a default value.
	getEnv := func(key, defaultValue string) string {
		if value, exists := os.LookupEnv(key); exists {
			return value
		}
		return defaultValue
	}

	// DB Config - matches docker-compose.yml test-db
	dbConfig := config.DatabaseConfig{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     5433,
		User:     getEnv("DB_USER", "testuser"),
		Password: getEnv("DB_PASSWORD", "testpassword"),
		DBName:   getEnv("DB_NAME", "testdb"),
		SSLMode:  getEnv("DB_SSLMODE", "disable"),
	}

	// gRPC Config - matches docker-compose.yml python-gateway
	grpcConfig := config.GRPCConfig{
		PythonGatewayAddress: getEnv("PYTHON_GATEWAY_ADDR", "localhost:15051"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)

	// Initialize Dependencies
	db, err := database.NewDBPool(ctx, dbConfig)
	require.NoError(t, err, "Failed to connect to database")

	err = db.Ping(ctx)
	require.NoError(t, err, "Failed to ping database")

	client, err := NewGatewayClient(&grpcConfig)
	require.NoError(t, err, "Failed to connect to gateway")

	// Reset the gateway state to ensure a clean slate for the test
	_, err = client.ResetState(ctx)
	require.NoError(t, err, "Failed to reset gateway state")

	repoContainer := repository.New()
	svc := NewService(slog.Default(), db, client, repoContainer)

	// Teardown function
	cleanup := func() {
		cancel()
		client.Close()
		db.Close()
	}

	return svc, client, db, repoContainer, cleanup
}

func TestService_Integration_GetBalance(t *testing.T) {
	svc, client, db, repo, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Execution
	exchangeName := "dummy"
	assetSymbol := "BTC"

	// Fetch directly from Gateway to establish baseline expectation
	expectedResp, err := client.GetBalance(ctx, exchangeName, assetSymbol)
	require.NoError(t, err, "Failed to fetch expected balance from gateway")
	t.Logf("Expected Balance (from Gateway): %s", expectedResp.String())

	// Call Service method (which fetches and persists)
	resp, err := svc.GetBalance(ctx, exchangeName, assetSymbol)
	require.NoError(t, err, "Service.GetBalance failed")
	require.NotNil(t, resp, "GetBalance should return the gRPC response")

	// Verification
	// Fetch all balances from DB to verify persistence
	storedBalances, err := repo.Balances.GetAllBalances(ctx, db)
	require.NoError(t, err, "Failed to fetch balances from database")

	// Find our specific record
	var storedBalance repository.BalanceData
	var found bool
	for _, b := range storedBalances {
		if b.ExchangeName == exchangeName && b.AssetSymbol == assetSymbol {
			storedBalance = b
			found = true
			break
		}
	}

	require.True(t, found, "Balance record for %s/%s not found in database", exchangeName, assetSymbol)
	t.Logf("Stored Balance (from DB): %+v", storedBalance)

	// Assert values match
	// The gateway returns a list, find the matching asset
	var expectedFree, expectedUsed, expectedTotal float64
	for _, b := range expectedResp.Balances {
		if b.Asset == assetSymbol {
			expectedFree, expectedUsed, expectedTotal = b.Free, b.Used, b.Total
			break
		}
	}

	assert.InDelta(t, expectedFree, storedBalance.Free, 1e-9, "Free amount mismatch")
	assert.InDelta(t, expectedUsed, storedBalance.Used, 1e-9, "Used amount mismatch")
	assert.InDelta(t, expectedTotal, storedBalance.Total, 1e-9, "Total amount mismatch")
}

// TestService_Integration_OrderLifecycle covers the full create-get-cancel-list flow.
func TestService_Integration_OrderLifecycle(t *testing.T) {
	svc, _, db, repo, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchangeName := "dummy"
	symbol := "BTC/USDT"

	// List open orders (should be empty)
	t.Log("Listing open orders (expecting empty)")
	openOrders, err := svc.GetOpenOrders(ctx, exchangeName, symbol, 10)
	require.NoError(t, err)
	assert.Empty(t, openOrders.Orders, "Initially, there should be no open orders")
	t.Logf("Initial open orders response: %s", openOrders.String())

	// Create the first order
	t.Log("Creating first order")
	price1 := 40000.0
	amount1 := 0.1
	order1, err := svc.CreateOrder(ctx, exchangeName, symbol, repository.OrderSideBuy, repository.OrderTypeLimit, amount1, price1)
	require.NoError(t, err)
	require.NotNil(t, order1)
	assert.Equal(t, repository.OrderStatusOpen, order1.Status)
	assert.Equal(t, symbol, order1.Symbol)
	t.Logf("Created order 1 response: %s", order1.String())

	// Get the order from the service
	t.Log("Getting the first order from the service")
	fetchedOrder1, err := svc.GetOrder(ctx, exchangeName, symbol, order1.Id)
	require.NoError(t, err)
	assert.Equal(t, order1.Id, fetchedOrder1.Id)
	assert.Equal(t, repository.OrderStatusOpen, fetchedOrder1.Status)
	t.Logf("Fetched order 1 response: %s", fetchedOrder1.String())

	// Get the order directly from the database
	t.Log("Getting the first order from the database")
	dbOrder1, err := repo.Orders.GetOrder(ctx, db, exchangeName, order1.Id)
	require.NoError(t, err)
	assert.Equal(t, order1.Id, dbOrder1.ExchangeOrderID)
	assert.Equal(t, repository.OrderStatusOpen, dbOrder1.Status)
	assert.Equal(t, amount1, dbOrder1.Amount)
	assert.Equal(t, price1, dbOrder1.Price.Float64)
	t.Logf("DB order 1: %+v", dbOrder1)

	// Create a second order
	t.Log("Creating second order")
	price2 := 41000.0
	amount2 := 0.2
	order2, err := svc.CreateOrder(ctx, exchangeName, symbol, repository.OrderSideSell, repository.OrderTypeLimit, amount2, price2)
	require.NoError(t, err)
	require.NotNil(t, order2)
	assert.Equal(t, repository.OrderStatusOpen, order2.Status)
	t.Logf("Created order 2 response: %s", order2.String())

	// List open orders (should have two)
	t.Log("Listing open orders (expecting two)")
	openOrders, err = svc.GetOpenOrders(ctx, exchangeName, symbol, 10)
	require.NoError(t, err)
	assert.Len(t, openOrders.Orders, 2, "Should be two open orders")
	t.Logf("Open orders response (with 2 orders): %s", openOrders.String())

	// Cancel the second order
	t.Log("Canceling the second order")
	err = svc.CancelOrder(ctx, exchangeName, symbol, order2.Id)
	require.NoError(t, err)

	// Get the second order from the database to check status
	t.Log("Getting the second order from the database (expecting canceled)")
	dbOrder2, err := repo.Orders.GetOrder(ctx, db, exchangeName, order2.Id)
	require.NoError(t, err)
	assert.Equal(t, order2.Id, dbOrder2.ExchangeOrderID)
	assert.Equal(t, repository.OrderStatusCanceled, dbOrder2.Status, "Order status in DB should be canceled")
	t.Logf("DB order 2 after cancel: %+v", dbOrder2)

	// List open orders again (should have one)
	t.Log("Listing open orders again (expecting one)")
	openOrders, err = svc.GetOpenOrders(ctx, exchangeName, symbol, 10)
	require.NoError(t, err)
	require.Len(t, openOrders.Orders, 1, "Should be one open order left")
	assert.Equal(t, order1.Id, openOrders.Orders[0].Id, "The remaining open order should be the first one")
	t.Logf("Final open orders response (with 1 order): %s", openOrders.String())
}
