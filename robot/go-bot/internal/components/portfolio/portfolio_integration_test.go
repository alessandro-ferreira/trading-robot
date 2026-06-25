//go:build integration

package portfolio

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

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repoContainer := repository.New()
	p := NewPortfolio(logger, db, repoContainer)

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

	// Initial State
	t.Run("InitialState", func(t *testing.T) {
		openPositions, err := repo.Positions.GetActivePositions(ctx, db, "", "")
		require.NoError(t, err)
		assert.Empty(t, openPositions)
		assert.Equal(t, 0, p.GetActivePositionsCount())
	})

	// Create Position (Unknown Origin - Adopting a ghost balance)
	t.Run("CreateUnknownOrigin", func(t *testing.T) {
		// No price, no orderID -> results in UnknownOrigin = true
		err := p.CreatePosition(ctx, exchange, symbol, 0.5, 0, 0)
		require.NoError(t, err)
		assert.Equal(t, 1, p.GetActivePositionsCount())

		pos, err := repo.Positions.GetPosition(ctx, db, exchange, symbol)
		require.NoError(t, err)
		assert.True(t, pos.UnknownOrigin)
		assert.Equal(t, 0.5, pos.Quantity)
		assert.Equal(t, 0.0, pos.EntryPrice)
	})

	// Promote Position (Providing OrderID and Price from trade history)
	t.Run("PromotePosition", func(t *testing.T) {
		// Create a dummy order first to satisfy the foreign key constraint fk_positions_order
		orderID, err := repo.Orders.CreateOrder(ctx, db, repository.OrderData{
			ExchangeName:     exchange,
			InstrumentSymbol: symbol,
			ExchangeOrderID:  "dummy-order-link-1001",
			Side:             repository.OrderSideBuy,
			OrderType:        repository.OrderTypeLimit,
			Status:           repository.OrderStatusClosed,
		})
		require.NoError(t, err)

		err = p.CreatePosition(ctx, exchange, symbol, 0.5, 42000.0, orderID)
		require.NoError(t, err)

		pos, err := repo.Positions.GetPosition(ctx, db, exchange, symbol)
		require.NoError(t, err)
		assert.False(t, pos.UnknownOrigin, "Position should be promoted to known origin")
		assert.Equal(t, 42000.0, pos.EntryPrice)
		assert.Equal(t, orderID, pos.OrderID.Int64)
	})

	// Update Position (Standard update loop)
	t.Run("UpdatePosition", func(t *testing.T) {
		updates := repository.PositionData{
			Quantity:     0.6,
			HighestPrice: 45000.0,
		}
		err := p.UpdatePosition(ctx, exchange, symbol, updates)
		require.NoError(t, err)

		pos, err := repo.Positions.GetPosition(ctx, db, exchange, symbol)
		require.NoError(t, err)
		assert.Equal(t, 0.6, pos.Quantity)
		assert.Equal(t, 45000.0, pos.HighestPrice)
		assert.True(t, pos.OrderID.Valid)
	})

	// Delete Position (Closing trade)
	t.Run("DeletePosition", func(t *testing.T) {
		err := p.DeletePosition(ctx, exchange, symbol)
		require.NoError(t, err)
		assert.Equal(t, 0, p.GetActivePositionsCount())

		// Verify soft delete
		openPositions, _ := repo.Positions.GetActivePositions(ctx, db, exchange, symbol)
		assert.Empty(t, openPositions)

		_, err = repo.Positions.GetPosition(ctx, db, exchange, symbol)
		require.Error(t, err, "GetPosition should fail for inactive rows")
	})
}

func TestPortfolio_Integration_TotalValue(t *testing.T) {
	p, repo, db, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Seed multiple balances across exchanges
	balances := []repository.BalanceData{
		{ExchangeName: "dummy", AssetSymbol: "BTC", Total: 1.5, Free: 1.0, Used: 0.5},
		{ExchangeName: "dummy", AssetSymbol: "USDT", Total: 5000.0, Free: 5000.0},
		{ExchangeName: "binance", AssetSymbol: "BTC", Total: 0.5, Free: 0.5},
	}

	for _, b := range balances {
		_, err := repo.Balances.UpsertBalance(ctx, db, b)
		require.NoError(t, err)
	}

	totals, err := p.GetTotalValue(ctx)
	require.NoError(t, err)

	// BTC should be 1.5 + 0.5 = 2.0
	assert.Equal(t, 2.0, totals["BTC"])
	assert.Equal(t, 5000.0, totals["USDT"])
}

func TestPortfolio_Integration_StateManagement(t *testing.T) {
	p, repo, db, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchange := "dummy"
	symbol1 := "BTC/USDT"
	symbol2 := "ETH/USDT"

	// Manually insert positions into database to simulate existing state.
	pos1 := repository.PositionData{
		ExchangeName: exchange, InstrumentSymbol: symbol1,
		Side: repository.PositionSideLong, Quantity: 1.0, EntryPrice: 30000, Active: true, UnknownOrigin: true,
	}
	pos2 := repository.PositionData{
		ExchangeName: exchange, InstrumentSymbol: symbol2,
		Side: repository.PositionSideLong, Quantity: 10.0, EntryPrice: 2000, Active: true, UnknownOrigin: true,
	}

	require.NoError(t, repo.Positions.UpsertPosition(ctx, db, pos1))
	require.NoError(t, repo.Positions.UpsertPosition(ctx, db, pos2))

	// In-memory count is currently 0
	assert.Equal(t, 0, p.GetActivePositionsCount())

	// LoadState - Hydrate the in-memory map from DB
	err := p.LoadState(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, p.GetActivePositionsCount())

	// RefreshState (Simulate an external change: position closed in DB)
	require.NoError(t, repo.Positions.DeletePosition(ctx, db, exchange, symbol1))

	err = p.RefreshState(ctx, exchange, symbol1)
	require.NoError(t, err)
	assert.Equal(t, 1, p.GetActivePositionsCount(), "Map should have updated after refresh")

	// RefreshState (Simulate an external change: new position created in DB)
	require.NoError(t, repo.Positions.UpsertPosition(ctx, db, pos1))

	err = p.RefreshState(ctx, exchange, symbol1)
	require.NoError(t, err)
	assert.Equal(t, 2, p.GetActivePositionsCount())
}
