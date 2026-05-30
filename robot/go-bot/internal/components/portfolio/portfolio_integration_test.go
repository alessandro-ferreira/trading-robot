//go:build integration

package portfolio

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

// setupIntegrationTest initializes dependencies for the portfolio integration test.
func setupIntegrationTest(t *testing.T) (Portfolio, *repository.Container, *database.DB, func()) {
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

	repoContainer := repository.New()
	p := NewPortfolio(slog.Default(), db, repoContainer)

	cleanup := func() {
		cancel()
		db.Close()
	}

	return p, repoContainer, db, cleanup
}

func TestPortfolio_Integration_Lifecycle(t *testing.T) {
	p, repo, db, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchange := "dummy"
	symbol := "BTC/USDT"

	// Initial State: Verify no open positions
	t.Log("Checking initial state (expecting empty)")
	openPositions, err := repo.Positions.GetActivePositions(ctx, db, "", "")
	require.NoError(t, err)
	assert.Empty(t, openPositions, "Should have no open positions initially")

	// Open a new position (Buy 1 BTC @ 20000)
	t.Log("Inserting/Buying new position (1 BTC @ 20000)")
	err = p.CreatePosition(ctx, exchange, symbol, 1.0, 20000.0, 0)
	require.NoError(t, err)

	// Verify persistence via GetPosition
	t.Log("Verifying persistence via GetPosition")
	posData, err := repo.Positions.GetPosition(ctx, db, exchange, symbol)
	require.NoError(t, err)
	assert.Equal(t, exchange, posData.ExchangeName)
	assert.Equal(t, symbol, posData.InstrumentSymbol)
	assert.InDelta(t, 1.0, posData.Quantity, 0.000001)
	assert.InDelta(t, 20000.0, posData.EntryPrice, 0.000001)
	assert.Equal(t, 20000.0, posData.HighestPrice)
	assert.True(t, posData.Active)

	// Increase position (Buy 1 BTC @ 22000)
	// New Avg Price = (1*20000 + 1*22000) / 2 = 21000
	t.Log("Updating position (Buy 1 more BTC @ 22000)")
	err = p.UpdatePosition(ctx, exchange, symbol, repository.PositionData{
		Quantity:      2.0,
		EntryPrice:    21000.0,
		HighestPrice:  22000.0,
		UnknownOrigin: true,
	})
	require.NoError(t, err)

	// Verify persistence via GetPosition
	t.Log("Verifying update persistence")
	posData, err = repo.Positions.GetPosition(ctx, db, exchange, symbol)
	require.NoError(t, err)
	assert.InDelta(t, 2.0, posData.Quantity, 0.000001)
	assert.InDelta(t, 21000.0, posData.EntryPrice, 0.000001)
	assert.Equal(t, 22000.0, posData.HighestPrice)

	// Get Open Positions: Should return the single aggregated position
	t.Log("Verifying GetOpenPositions")
	openPositions, err = repo.Positions.GetActivePositions(ctx, db, "", "")
	require.NoError(t, err)
	require.Len(t, openPositions, 1)
	assert.Equal(t, symbol, openPositions[0].InstrumentSymbol)
	assert.InDelta(t, 2.0, openPositions[0].Quantity, 0.000001)

	// Delete: Close position (Sell 2 BTC)
	t.Log("Deleting position (Selling all)")
	err = p.DeletePosition(ctx, exchange, symbol)
	require.NoError(t, err)

	// Verify deletion via GetOpenPositions (should be empty)
	t.Log("Verifying deletion via GetOpenPositions")
	openPositions, err = repo.Positions.GetActivePositions(ctx, db, "", "")
	require.NoError(t, err)
	assert.Empty(t, openPositions, "Should have no open positions after selling all")

	// Verify GetPosition returns error or empty for inactive
	// The default implementation filters by active=TRUE, so it should fail to find it.
	_, err = repo.Positions.GetPosition(ctx, db, exchange, symbol)
	require.Error(t, err, "GetPosition should fail for closed/inactive position")
}
