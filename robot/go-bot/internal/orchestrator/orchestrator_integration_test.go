//go:build integration

package orchestrator

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"trading/robot/go-bot/internal/components/execution"
	"trading/robot/go-bot/internal/components/portfolio"
	"trading/robot/go-bot/internal/config"
	"trading/robot/go-bot/internal/database"
	"trading/robot/go-bot/internal/database/repository"
)

// setupOrchestratorIntegrationTest initializes all dependencies for an Orchestrator integration test.
// It returns the initialized Orchestrator, database connection, execution client, and a cleanup function to release resources after the test.
func setupOrchestratorIntegrationTest(t *testing.T, maxOpenPositions int) (*Orchestrator, *database.DB, *execution.GatewayClient, func()) {
	t.Helper()

	getEnv := func(key, defaultValue string) string {
		if value, exists := os.LookupEnv(key); exists {
			return value
		}
		return defaultValue
	}

	dbConfig := config.DatabaseConfig{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     5433,
		User:     getEnv("DB_USER", "testuser"),
		Password: getEnv("DB_PASSWORD", "testpassword"),
		DBName:   getEnv("DB_NAME", "testdb"),
		SSLMode:  getEnv("DB_SSLMODE", "disable"),
	}

	grpcConfig := config.GRPCConfig{
		PythonGatewayAddress: getEnv("PYTHON_GATEWAY_ADDR", "localhost:15051"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)

	// Initialize Infrastructure
	db, err := database.NewDBPool(ctx, dbConfig)
	require.NoError(t, err, "Failed to connect to database")
	require.NoError(t, db.Ping(ctx), "Failed to ping database")

	client, err := execution.NewGatewayClient(&grpcConfig)
	require.NoError(t, err, "Failed to connect to gateway")

	_, err = client.ResetState(ctx)
	require.NoError(t, err, "Failed to reset gateway state")

	// Initialize Components
	repoContainer := repository.New()
	execSvc := execution.NewService(slog.Default(), db, client, repoContainer)

	pf := portfolio.NewPortfolio(slog.Default(), db, repoContainer, 10000.0)
	require.NoError(t, pf.LoadState(ctx), "Failed to load portfolio state")

	// Define a test-specific configuration
	cfg := &config.Config{
		Risk: config.RiskConfig{
			MaxOpenPositions: maxOpenPositions,
			MaxDailyLoss:     1000.0,
		},
	}

	orch, err := New(slog.Default(), db, repoContainer, cfg, pf, execSvc, 500*time.Millisecond)
	require.NoError(t, err, "Failed to create Orchestrator")

	cleanup := func() {
		cancel()
		orch.Close()
		client.Close()
		db.Close()
	}

	return orch, db, client, cleanup
}

// TestOrchestrator_ExecutionOk verifies that the orchestrator boots correctly and processes a proactive strategy.
func TestOrchestrator_ExecutionOk(t *testing.T) {
	// Set MaxOpenPositions to allow trading
	orch, db, _, cleanup := setupOrchestratorIntegrationTest(t, 5)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- orch.Start(ctx)
	}()

	t.Log("Orchestrator started, waiting for signals to initialize...")

	// Wait a moment for StrategyWarmup and background workers to start
	time.Sleep(1 * time.Second)

	// Verify that the signals map was populated from database migrations
	// Migrations 10 and 15 insert BTC/USDT, ETH/USDT, and LTC/USDT
	assert.GreaterOrEqual(t, len(orch.signals), 3, "Orchestrator should have initialized strategy pairs from DB")

	repo := repository.New()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var orderPlaced bool
Loop:
	for {
		select {
		case <-ctx.Done():
			break Loop
		case err := <-errCh:
			require.NoError(t, err)
			break Loop
		case <-ticker.C:
			// Check for LTC/USDT order. The 'dummy' strategy triggers a buy immediately.
			orders, _ := repo.Orders.GetOrders(ctx, db, "dummy", "LTC/USDT", 1)
			if len(orders) > 0 {
				orderPlaced = true
				t.Log("Execution Ok: Order placed for dummy strategy on LTC/USDT")
				break Loop
			}
		}
	}

	assert.True(t, orderPlaced, "Orchestrator should have triggered at least one order from the dummy strategy")

	cancel()
	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Orchestrator failed to shut down gracefully within timeout")
	}
}
