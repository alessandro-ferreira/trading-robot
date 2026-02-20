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

func TestService_Integration_GetBalance(t *testing.T) {
	// Setup Configuration
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Initialize Dependencies
	// Database
	db, err := database.NewDBPool(ctx, dbConfig)
	require.NoError(t, err, "Failed to connect to database")
	defer db.Close()

	// Ensure DB connection is alive
	err = db.Ping(ctx)
	require.NoError(t, err, "Failed to ping database")

	// Gateway Client
	client, err := NewGatewayClient(&grpcConfig)
	require.NoError(t, err, "Failed to connect to gateway")
	defer client.Close()

	// Repository Container
	repoContainer := repository.New()

	// Service
	svc := NewService(slog.Default(), db, client, repoContainer)

	// Execution
	exchangeName := "dummy"
	assetSymbol := "BTC"

	// Fetch directly from Gateway to establish baseline expectation
	expectedResp, err := client.GetBalance(ctx, assetSymbol, exchangeName)
	require.NoError(t, err, "Failed to fetch expected balance from gateway")
	t.Logf("Expected Balance (from Gateway): %s", expectedResp.String())

	// Call Service method (which fetches and persists)
	returnedBalance, err := svc.GetBalance(ctx, exchangeName, assetSymbol)
	require.NoError(t, err, "Service.GetBalance failed")
	require.NotNil(t, returnedBalance, "GetBalance should return the persisted balance")

	// Verification
	// Fetch all balances from DB to verify persistence
	storedBalances, err := repoContainer.Balances.GetAllBalances(ctx, db)
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

	// Assert that the ID returned by the service matches the one in the DB
	assert.Equal(t, storedBalance.ID, returnedBalance.ID, "ID from service should match stored ID")

	// Assert values match
	// The gateway returns maps, so we access the specific symbol
	expectedFree := expectedResp.Free[assetSymbol]
	expectedUsed := expectedResp.Used[assetSymbol]
	expectedTotal := expectedResp.Total[assetSymbol]

	assert.InDelta(t, expectedFree, storedBalance.Free, 1e-9, "Free amount mismatch")
	assert.InDelta(t, expectedUsed, storedBalance.Used, 1e-9, "Used amount mismatch")
	assert.InDelta(t, expectedTotal, storedBalance.Total, 1e-9, "Total amount mismatch")
}
