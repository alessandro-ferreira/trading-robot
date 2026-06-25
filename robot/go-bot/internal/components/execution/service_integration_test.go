//go:build integration

package execution

import (
	"context"
	"io"
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
		ConnectionTimeout:    time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)

	// Initialize Infrastructure
	db, err := database.NewDBPool(ctx, dbConfig)
	require.NoError(t, err, "failed to connect to database")
	require.NoError(t, db.Ping(ctx), "failed to ping database")

	client, err := NewGatewayClient(&grpcConfig)
	require.NoError(t, err, "failed to connect to gateway")

	_, err = client.ResetState(ctx)
	require.NoError(t, err, "failed to reset gateway state")

	// Initialize Components
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repoContainer := repository.New()
	svc := NewService(logger, db, client, repoContainer)

	// Teardown function
	cleanup := func() {
		cancel()
		client.Close()
		db.Close()
	}

	return svc, client, db, repoContainer, cleanup
}

func TestService_Integration_GetTicker(t *testing.T) {
	svc, _, db, repo, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchange := "dummy"
	symbol := "BTC/USDT"

	// Execution
	tick, err := svc.GetTicker(ctx, exchange, symbol)
	require.NoError(t, err)
	assert.Greater(t, tick.Price, 0.0)

	// Verification in Database (market_data_ticks is a hypertable)
	ticks, err := repo.MarketData.GetMarketDataTicks(ctx, db, exchange, symbol, 1)
	require.NoError(t, err)
	require.NotEmpty(t, ticks)
	assert.Equal(t, tick.Price, ticks[0].Price)
}

func TestService_Integration_GetBalance(t *testing.T) {
	svc, client, db, repo, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchangeName := "dummy"

	// Verify Initial Seed Balance (USDT 10000)
	resp, err := svc.GetBalance(ctx, exchangeName, "USDT")
	require.NoError(t, err)
	require.Len(t, resp, 1)
	assert.Equal(t, 10000.0, resp[0].Total)

	// Verify persistence
	storedBalances, _ := repo.Balances.GetAllBalances(ctx, db, "")
	var foundUSDT bool
	for _, b := range storedBalances {
		if b.AssetSymbol == "USDT" {
			assert.Equal(t, 10000.0, b.Total)
			foundUSDT = true
		}
	}
	assert.True(t, foundUSDT)

	// Perform a Market Buy to trigger balance swap in DummyExchange
	amount := 0.001
	_, err = svc.CreateOrder(ctx, exchangeName, "BTC/USDT", repository.OrderSideBuy, repository.OrderTypeMarket, amount, 0)
	require.NoError(t, err)

	// Fetch BTC balance (should now be 0.001)
	resp, err = svc.GetBalance(ctx, exchangeName, "BTC")
	require.NoError(t, err)
	require.Len(t, resp, 1)
	assert.Equal(t, amount, resp[0].Total)

	// Verify persistence of the new BTC balance
	storedBalances, _ = repo.Balances.GetAllBalances(ctx, db, exchangeName)
	var storedBalance repository.BalanceData
	var found bool
	for _, b := range storedBalances {
		if b.AssetSymbol == "BTC" {
			storedBalance = b
			found = true
		}
	}
	require.True(t, found)
	assert.Equal(t, amount, storedBalance.Total)

	// Verify specific asset baseline expectation directly from Gateway
	expectedResp, err := client.GetBalance(ctx, exchangeName, "BTC")
	require.NoError(t, err)
	var expectedTotal float64
	for _, b := range expectedResp.Balances {
		if b.Asset == "BTC" {
			expectedTotal = b.Total
			break
		}
	}
	assert.Equal(t, expectedTotal, storedBalance.Total)
}

// TestService_Integration_OrderLifecycle covers the full create-get-cancel-list flow.
func TestService_Integration_OrderLifecycle(t *testing.T) {
	svc, _, db, repo, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchangeName := "dummy"
	symbol := "BTC/USDT"

	// Seed the account with BTC first so we can test a SELL order later
	_, err := svc.CreateOrder(ctx, exchangeName, symbol, repository.OrderSideBuy, repository.OrderTypeMarket, 0.01, 0)
	require.NoError(t, err)

	// List open orders (should be empty)
	t.Log("Listing open orders (expecting empty)")
	openOrders, err := svc.GetOpenOrders(ctx, exchangeName, symbol, 10)
	require.NoError(t, err)
	assert.Empty(t, openOrders, "Initially, there should be no open orders")
	t.Logf("Initial open orders response: %+v", openOrders)

	// Create the first order
	t.Log("Creating first order")
	price1 := 40000.0
	amount1 := 0.01
	order1, err := svc.CreateOrder(ctx, exchangeName, symbol, repository.OrderSideBuy, repository.OrderTypeLimit, amount1, price1)
	require.NoError(t, err)
	require.NotNil(t, order1)
	assert.Equal(t, repository.OrderStatusOpen, order1.Status)
	assert.Equal(t, symbol, order1.InstrumentSymbol)
	t.Logf("Created order 1 response: %+v", order1)

	// Get the order from the service
	t.Log("Getting the first order from the service")
	fetchedOrder1, err := svc.GetOrder(ctx, exchangeName, symbol, order1.ExchangeOrderID)
	require.NoError(t, err)
	assert.Equal(t, order1.ExchangeOrderID, fetchedOrder1.ExchangeOrderID)
	assert.Equal(t, repository.OrderStatusOpen, fetchedOrder1.Status)
	t.Logf("Fetched order 1 response: %+v", fetchedOrder1)

	// Get the order directly from the database
	t.Log("Getting the first order from the database")
	dbOrder1, err := repo.Orders.GetOrder(ctx, db, exchangeName, order1.ExchangeOrderID)
	require.NoError(t, err)
	assert.Equal(t, order1.ExchangeOrderID, dbOrder1.ExchangeOrderID)
	assert.Equal(t, repository.OrderStatusOpen, dbOrder1.Status)
	assert.Equal(t, amount1, dbOrder1.Amount)
	assert.Equal(t, price1, dbOrder1.Price.Float64)
	t.Logf("DB order 1: %+v", dbOrder1)

	// Create a second order
	t.Log("Creating second order")
	price2 := 41000.0
	amount2 := 0.01
	order2, err := svc.CreateOrder(ctx, exchangeName, symbol, repository.OrderSideSell, repository.OrderTypeLimit, amount2, price2)
	require.NoError(t, err)
	require.NotNil(t, order2)
	assert.Equal(t, repository.OrderStatusOpen, order2.Status)
	t.Logf("Created order 2 response: %+v", order2)

	// List open orders (should have two)
	t.Log("Listing open orders (expecting two)")
	openOrders, err = svc.GetOpenOrders(ctx, exchangeName, symbol, 10)
	require.NoError(t, err)
	assert.Len(t, openOrders, 2, "Should be two open orders")
	t.Logf("Open orders response (with 2 orders): %+v", openOrders)

	// Cancel the second order
	t.Log("Canceling the second order")
	err = svc.CancelOrder(ctx, exchangeName, symbol, order2.ExchangeOrderID)
	require.NoError(t, err)

	// Get the second order from the database to check status
	t.Log("Getting the second order from the database (expecting canceled)")
	dbOrder2, err := repo.Orders.GetOrder(ctx, db, exchangeName, order2.ExchangeOrderID)
	require.NoError(t, err)
	assert.Equal(t, order2.ExchangeOrderID, dbOrder2.ExchangeOrderID)
	assert.Equal(t, repository.OrderStatusCanceled, dbOrder2.Status, "Order status in DB should be canceled")
	t.Logf("DB order 2 after cancel: %+v", dbOrder2)

	// List open orders again (should have one)
	t.Log("Listing open orders again (expecting one)")
	openOrders, err = svc.GetOpenOrders(ctx, exchangeName, symbol, 10)
	require.NoError(t, err)
	require.Len(t, openOrders, 1, "Should be one open order left")
	assert.Equal(t, order1.ExchangeOrderID, openOrders[0].ExchangeOrderID, "The remaining open order should be the first one")
}

func TestService_Integration_StopOrder(t *testing.T) {
	svc, _, db, repo, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchange := "dummy"
	symbol := "BTC/USDT"

	// Execution
	stopPrice := 35000.0
	order, err := svc.CreateStopOrder(ctx, exchange, symbol, repository.OrderSideSell, 0.01, stopPrice, 0)
	require.NoError(t, err)
	assert.Equal(t, repository.OrderStatusOpen, order.Status)
	assert.Equal(t, repository.OrderTypeStopMarket, order.OrderType)

	// Verify persistence
	dbOrder, err := repo.Orders.GetOrder(ctx, db, exchange, order.ExchangeOrderID)
	require.NoError(t, err)
	assert.Equal(t, stopPrice, dbOrder.Price.Float64)
	assert.Equal(t, repository.OrderTypeStopMarket, dbOrder.OrderType)
}

func TestService_Integration_RecentTrades(t *testing.T) {
	svc, _, db, repo, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchange := "dummy"
	symbol := "ETH/USDT"

	// Capture 'since' before creating the order to ensure it's included
	since := time.Now().Add(-1 * time.Minute).UnixMilli()

	// Create a Market Buy (immediately generates a trade in DummyExchange)
	amount := 0.01
	order, err := svc.CreateOrder(ctx, exchange, symbol, repository.OrderSideBuy, repository.OrderTypeMarket, amount, 0)
	require.NoError(t, err)
	require.Equal(t, repository.OrderStatusClosed, order.Status)

	// Fetch Recent Trades
	trades, err := svc.GetRecentTrades(ctx, exchange, symbol, since, 10)
	require.NoError(t, err)

	// We expect at least the trade from our market order
	var found bool
	for _, tr := range trades {
		if tr.ExchangeOrderID == order.ExchangeOrderID || tr.ExchangeOrderID == "trade-"+order.ExchangeOrderID {
			assert.Equal(t, amount, tr.Filled)
			assert.Equal(t, repository.OrderStatusClosed, tr.Status)
			found = true
		}
	}
	assert.True(t, found, "The trade from the market order should be returned by GetRecentTrades")

	// Verify DB sync: The order status in DB should be Closed (synced by GetRecentTrades logic)
	dbOrder, err := repo.Orders.GetOrder(ctx, db, exchange, order.ExchangeOrderID)
	require.NoError(t, err)
	assert.Equal(t, repository.OrderStatusClosed, dbOrder.Status)
}
