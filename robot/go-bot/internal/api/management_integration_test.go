//go:build integration

package api

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "trading/robot/go-bot/gen/go/v1"
	"trading/robot/go-bot/internal/config"
	"trading/robot/go-bot/internal/database"
	"trading/robot/go-bot/internal/database/repository"
)

// setupIntegrationTest initializes dependencies for the management integration test.
func setupIntegrationTest(t *testing.T) (*ManagementServer, *repository.Container, *database.DB, func()) {
	t.Helper()

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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	db, err := database.NewDBPool(ctx, dbConfig)
	require.NoError(t, err, "Failed to connect to database")

	err = db.Ping(ctx)
	require.NoError(t, err, "Failed to ping database")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repoContainer := repository.New()
	server := NewManagementServer(logger, db, repoContainer)

	cleanup := func() {
		cancel()
		db.Close()
	}

	return server, repoContainer, db, cleanup
}

func TestManagementServer_Integration_StrategyLifecycle(t *testing.T) {
	server, repos, db, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchange := "dummy"
	symbol := "BTC/USDT"

	// Strategy Lifecycle: Enable Dummy
	t.Run("EnableDummyStrategy", func(t *testing.T) {
		req := &pb.UpdateStrategyRequest{
			Exchange:     exchange,
			Symbol:       symbol,
			StrategyType: repository.StrategyDummy,
			Enabled:      true,
		}

		resp, err := server.UpdateStrategy(ctx, req)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		// Verify in database
		pairs, err := repos.Strategies.GetStrategyPairs(ctx, db, []string{repository.StrategyEnabled})
		require.NoError(t, err)

		var found bool
		for _, p := range pairs {
			if p.ExchangeName == exchange && p.InstrumentSymbol == symbol {
				assert.Equal(t, repository.StrategyDummy, p.Type)
				assert.Equal(t, repository.StrategyEnabled, p.Status)
				found = true
			}
		}
		assert.True(t, found, "Strategy should be enabled in database")
	})

	// Strategy Lifecycle: Update to Momentum Profit
	t.Run("UpdateToMomentumProfit", func(t *testing.T) {
		req := &pb.UpdateStrategyRequest{
			Exchange:     exchange,
			Symbol:       symbol,
			StrategyType: repository.StrategyMomentumProfit,
			Enabled:      true,
			MomentumParams: &pb.MomentumParams{
				Label:           "integration-test",
				WindowSeconds:   60,
				StopLossPct:     0.2 * 0.01,
				ProfitTargetPct: floatPointer(0.5 * 0.01),
				Windows: []*pb.MomentumWindow{
					{LookbackSeconds: 30, Threshold: 0.1 * 0.01},
				},
			},
		}

		resp, err := server.UpdateStrategy(ctx, req)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		pairs, err := repos.Strategies.GetStrategyPairs(ctx, db, []string{repository.StrategyEnabled})
		require.NoError(t, err)

		var found bool
		for _, p := range pairs {
			if p.ExchangeName == exchange && p.InstrumentSymbol == symbol {
				assert.Equal(t, repository.StrategyMomentumProfit, p.Type)
				assert.Equal(t, 60, p.Momentum.WindowSeconds)
				assert.Equal(t, 0.005, p.Momentum.ProfitTargetPct.Float64)
				found = true
			}
		}
		assert.True(t, found, "Strategy should be updated to momentum_profit")
	})

	// Strategy Lifecycle: Request Disable
	t.Run("RequestDisable", func(t *testing.T) {
		req := &pb.UpdateStrategyRequest{
			Exchange:     exchange,
			Symbol:       symbol,
			StrategyType: repository.StrategyMomentumProfit,
			Enabled:      false,
		}

		resp, err := server.UpdateStrategy(ctx, req)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		// Verify status is pending_disabled in database
		pairs, err := repos.Strategies.GetStrategyPairs(ctx, db, []string{repository.StrategyPendingDisabled})
		require.NoError(t, err)

		var found bool
		for _, p := range pairs {
			if p.ExchangeName == exchange && p.InstrumentSymbol == symbol {
				assert.Equal(t, repository.StrategyPendingDisabled, p.Status)
				found = true
			}
		}
		assert.True(t, found, "Strategy should be in pending_disabled status")
	})

	// Strategy Lifecycle: Apply Disable (Simulating Orchestrator completion)
	t.Run("ApplyDisable", func(t *testing.T) {
		// This method is called by the Orchestrator after verifying no active positions.
		// We call it here to validate the repository logic.
		err := repos.Strategies.ApplyStrategyDisable(ctx, db, exchange, symbol)
		require.NoError(t, err)

		// Verify status is disabled in database
		pairs, err := repos.Strategies.GetStrategyPairs(ctx, db, []string{repository.StrategyDisabled})
		require.NoError(t, err)

		var found bool
		for _, p := range pairs {
			if p.ExchangeName == exchange && p.InstrumentSymbol == symbol {
				assert.Equal(t, repository.StrategyDisabled, p.Status)
				found = true
			}
		}
		assert.True(t, found, "Strategy should be in disabled status")

		// Verify it is no longer returned as enabled or pending
		activePairs, err := repos.Strategies.GetStrategyPairs(ctx, db, []string{repository.StrategyEnabled, repository.StrategyPendingDisabled})
		require.NoError(t, err)

		// We check that the specific pair we disabled is no longer in the active list.
		// We don't use assert.Empty because the DB might contain other seeded strategies.
		for _, p := range activePairs {
			if p.ExchangeName == exchange && p.InstrumentSymbol == symbol {
				assert.Fail(t, "Strategy %s/%s should no longer be in the active set", exchange, symbol)
			}
		}
	})
}

func TestManagementServer_Integration_RiskLifecycle(t *testing.T) {
	server, repos, db, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchange := "dummy"
	symbol := "BTC/USDT"

	// Risk Lifecycle: Create
	t.Run("CreateRisk", func(t *testing.T) {
		req := &pb.UpdateRiskRequest{
			Exchange:        exchange,
			Symbol:          symbol,
			AllocatedBudget: 500.0,
			MaxAssetUnits:   2.0,
		}

		resp, err := server.UpdateRisk(ctx, req)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		risk, err := repos.Risks.GetRiskPair(ctx, db, exchange, symbol)
		require.NoError(t, err)
		assert.Equal(t, 500.0, risk.AllocatedBudget)
		assert.Equal(t, 2.0, risk.MaxAssetUnits.Float64)
	})

	// Risk Lifecycle: Update
	t.Run("UpdateRisk", func(t *testing.T) {
		req := &pb.UpdateRiskRequest{
			Exchange:        exchange,
			Symbol:          symbol,
			AllocatedBudget: 1000.0,
			MaxAssetUnits:   5.0,
		}

		resp, err := server.UpdateRisk(ctx, req)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		risk, err := repos.Risks.GetRiskPair(ctx, db, exchange, symbol)
		require.NoError(t, err)
		assert.Equal(t, 1000.0, risk.AllocatedBudget)
		assert.Equal(t, 5.0, risk.MaxAssetUnits.Float64)
	})
}

func floatPointer(f float64) *float64 {
	return &f
}
