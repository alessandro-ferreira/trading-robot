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
		Pairs: []config.PairConfig{
			{
				Symbol:   "BTC/USDT",
				Exchange: "dummy",
				Risk: config.PairRiskConfig{
					RiskPerTrade: 100.0,
				},
				Strategy: config.StrategyConfig{
					Type: config.StrategyMomentumTrailing,
					Momentum: config.MomentumConfig{
						WindowSeconds:   2,
						LookbackSeconds: 1,
						Threshold:       0.0001,
						StopLossPct:     0.1,
						ActivationPct:   0.05,
						TrailingStopPct: 0.02,
					},
				},
			},
			{
				Symbol:   "ETH/USDT",
				Exchange: "dummy",
				Risk: config.PairRiskConfig{
					RiskPerTrade: 50.0,
				},
				Strategy: config.StrategyConfig{
					Type: config.StrategyMomentumTrailing,
					Momentum: config.MomentumConfig{
						WindowSeconds:   2,
						LookbackSeconds: 1,
						Threshold:       0.0001,
						StopLossPct:     0.1,
						ActivationPct:   0.05,
						TrailingStopPct: 0.02,
					},
				},
			},
		},
	}

	orch, err := New(slog.Default(), cfg, pf, execSvc, 500*time.Millisecond)
	require.NoError(t, err, "Failed to create Orchestrator")

	cleanup := func() {
		cancel()
		orch.Close()
		client.Close()
		db.Close()
	}

	return orch, db, client, cleanup
}

// TestOrchestrator_Integration_Concurrency verifies that multiple pair loops run independently.
func TestOrchestrator_Integration_Concurrency(t *testing.T) {
	// Set MaxOpenPositions high enough to allow both pairs to trade
	orch, db, _, cleanup := setupOrchestratorIntegrationTest(t, 10)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- orch.Start(ctx)
	}()

	t.Log("Monitoring orchestrator for order placement...")

	repo := repository.New()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var btcPlaced, ethPlaced bool
Loop:
	for {
		select {
		case <-ctx.Done():
			break Loop
		case err := <-errCh:
			require.NoError(t, err)
			break Loop
		case <-ticker.C:
			// Verify both symbols eventually get orders (Concurrency)
			if !btcPlaced {
				orders, _ := repo.Orders.GetOrders(ctx, db, "dummy", "BTC/USDT", 1)
				if len(orders) > 0 {
					btcPlaced = true
					t.Log("BTC order detected")
				}
			}
			if !ethPlaced {
				orders, _ := repo.Orders.GetOrders(ctx, db, "dummy", "ETH/USDT", 1)
				if len(orders) > 0 {
					ethPlaced = true
					t.Log("ETH order detected")
				}
			}

			if btcPlaced && ethPlaced {
				t.Log("Concurrency verified: Both pairs placed orders")
				break Loop
			}
		}
	}

	assert.True(t, btcPlaced && ethPlaced, "Orchestrator should have triggered orders for both pairs")

	cancel()
	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Orchestrator failed to shut down gracefully within timeout")
	}
}
