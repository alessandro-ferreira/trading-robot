//go:build integration

package reconcil

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"trading/robot/go-bot/internal/components/execution"
	"trading/robot/go-bot/internal/components/portfolio"
	"trading/robot/go-bot/internal/config"
	"trading/robot/go-bot/internal/database"
	"trading/robot/go-bot/internal/database/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupReconcilerIntegrationTest initializes dependencies for reconciliation integration tests.
// It returns a configured Reconciler, execution service, database handle, repository container,
// and a cleanup function to release resources after the test.
func setupReconcilerIntegrationTest(
	t *testing.T,
) (Reconciler, execution.Service, *database.DB, *repository.Container, func()) {
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
		ConnectionTimeout:    time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)

	// Initialize Infrastructure
	db, err := database.NewDBPool(ctx, dbConfig)
	require.NoError(t, err, "failed to connect to database")
	require.NoError(t, db.Ping(ctx), "failed to ping database")

	client, err := execution.NewGatewayClient(&grpcConfig)
	require.NoError(t, err, "failed to connect to gateway")

	_, err = client.ResetState(ctx)
	require.NoError(t, err, "failed to reset gateway state")

	// Initialize Components
	logger := slog.New(slog.NewTextHandler(io.Discard, nil)) // slog.Default()
	repoContainer := repository.New()
	pf := portfolio.NewPortfolio(logger, db, repoContainer)
	execSvc := execution.NewService(logger, db, client, repoContainer)

	// Ensure a clean state for positions to avoid interference from previous tests.
	activePos, _ := repoContainer.Positions.GetActivePositions(ctx, db, "", "")
	for _, p := range activePos {
		_ = repoContainer.Positions.DeletePosition(ctx, db, p.ExchangeName, p.InstrumentSymbol)
	}

	recon := NewReconciler(logger, db, repoContainer, execSvc, pf)

	cleanup := func() {
		cancel()
		client.Close()
		db.Close()
	}

	return recon, execSvc, db, repoContainer, cleanup
}

// TestReconciler_Integration_SyncBuyOrder verifies a buy filled exchange order is converted into a local position.
func TestReconciler_Integration_SyncBuyOrder(t *testing.T) {
	recon, execSvc, db, repo, cleanup := setupReconcilerIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchange := "dummy"
	symbol := "BTC/USDT"

	// Create a filled order on the dummy gateway to simulate DB stale state
	order, err := execSvc.CreateOrder(ctx, exchange, symbol, repository.OrderSideBuy, repository.OrderTypeMarket, 0.001, 0)
	require.NoError(t, err)
	require.Equal(t, repository.OrderStatusClosed, order.Status)

	// Make DB think the order is still open to exercise promotion logic
	order.Status = repository.OrderStatusOpen
	_, err = repo.Orders.UpdateOrder(ctx, db, order)
	require.NoError(t, err)

	// Run the reconciler
	err = recon.SyncOrders(ctx, exchange, symbol)
	require.NoError(t, err)

	// Verify the position was created matching the filled quantity
	pos, err := repo.Positions.GetPosition(ctx, db, exchange, symbol)
	require.NoError(t, err)
	assert.False(t, pos.UnknownOrigin)
	assert.Equal(t, order.Filled, pos.Quantity)
	assert.NotZero(t, pos.EntryPrice)
}

// TestReconciler_Integration_SyncSellOrder verifies a sell filled exchange order and update it locally.
func TestReconciler_Integration_SyncSellOrder(t *testing.T) {
	recon, execSvc, db, repo, cleanup := setupReconcilerIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchange := "dummy"
	symbol := "BTC/USDT"

	// Create a buy order to have sufficient funds and then create a filled sell order to simulate DB stale state
	_, err := execSvc.CreateOrder(ctx, exchange, symbol, repository.OrderSideBuy, repository.OrderTypeMarket, 0.001, 0)
	require.NoError(t, err)

	order, err := execSvc.CreateOrder(ctx, exchange, symbol, repository.OrderSideSell, repository.OrderTypeMarket, 0.001, 0)
	require.NoError(t, err)
	require.Equal(t, repository.OrderStatusClosed, order.Status)

	// Set the balance to zero to simulate a liquidation scenario and check if the reconciler handles it correctly.
	_, err = repo.Balances.UpsertBalance(ctx, db, repository.BalanceData{
		ExchangeName: exchange,
		AssetSymbol:  "BTC",
		Free:         0,
		Used:         0,
		Total:        0,
	})
	require.NoError(t, err)

	// Make DB think the order is still open to check its status
	order.Status = repository.OrderStatusOpen
	_, err = repo.Orders.UpdateOrder(ctx, db, order)
	require.NoError(t, err)

	openOrder, err := repo.Orders.GetOrder(ctx, db, exchange, order.ExchangeOrderID)
	require.NoError(t, err)
	assert.Equal(t, repository.OrderStatusOpen, openOrder.Status)

	// Run the reconciler
	err = recon.SyncOrders(ctx, exchange, symbol)
	require.NoError(t, err)

	// Validate that the order status is updated to closed in the database after reconciliation.
	updatedOrder, err := repo.Orders.GetOrder(ctx, db, exchange, order.ExchangeOrderID)
	require.NoError(t, err)
	assert.Equal(t, repository.OrderStatusClosed, updatedOrder.Status)
}

// TestReconciler_Integration_NoSyncCanceledOrder verifies that when the exchange reports a canceled order
// for a DB-open order, no position is created.
func TestReconciler_Integration_NoSyncCanceledOrder(t *testing.T) {
	recon, execSvc, db, repo, cleanup := setupReconcilerIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchange := "dummy"
	symbol := "BTC/USDT"

	// Create an exchange limit order that remains open in the dummy gateway.
	order, err := execSvc.CreateOrder(ctx, exchange, symbol, repository.OrderSideBuy, repository.OrderTypeLimit, 0.001, 30000.0)
	require.NoError(t, err)
	require.Equal(t, repository.OrderStatusOpen, order.Status)

	// Cancel the open exchange order so the gateway reports 'canceled'.
	err = execSvc.CancelOrder(ctx, exchange, symbol, order.ExchangeOrderID)
	require.NoError(t, err)

	// Now deliberately set DB to think the order is still open (stale DB state).
	order.Status = repository.OrderStatusOpen
	_, err = repo.Orders.UpdateOrder(ctx, db, order)
	require.NoError(t, err)

	// Run SyncOrders. The exchange reports canceled, so no CreatePosition should occur.
	err = recon.SyncOrders(ctx, exchange, symbol)
	require.NoError(t, err)

	// Verify no active position linked to this symbol.
	_, err = repo.Positions.GetPosition(ctx, db, exchange, symbol)
	assert.Error(t, err)
}

// TestReconciler_Integration_SyncExternalLiquidation verifies that a zero wallet balance closes an active position.
func TestReconciler_Integration_SyncExternalLiquidation(t *testing.T) {
	recon, _, db, repo, cleanup := setupReconcilerIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchange := "dummy"
	symbol := "ETH/USDT"

	// Create an active position and set wallet balance to zero
	pos := repository.PositionData{
		ExchangeName:     exchange,
		InstrumentSymbol: symbol,
		Side:             repository.PositionSideLong,
		Quantity:         0.01,
		EntryPrice:       1000.0,
		HighestPrice:     1000.0,
		UnknownOrigin:    true,
	}
	require.NoError(t, repo.Positions.UpsertPosition(ctx, db, pos))

	// Verify the position was created
	pos, err := repo.Positions.GetPosition(ctx, db, exchange, symbol)
	require.NoError(t, err)

	_, err = repo.Balances.UpsertBalance(ctx, db, repository.BalanceData{
		ExchangeName: exchange,
		AssetSymbol:  "ETH",
		Free:         0,
		Used:         0,
		Total:        0,
	})
	require.NoError(t, err)

	// Run positions reconciliation
	err = recon.SyncPositions(ctx, exchange, "")
	require.NoError(t, err)

	// Verify the position removed due to zero balance
	_, err = repo.Positions.GetPosition(ctx, db, exchange, symbol)
	assert.Error(t, err)
}

// TestReconciler_Integration_FixFeeDustDrift verifies a single active position is snapped to wallet truth.
func TestReconciler_Integration_FixFeeDustDrift(t *testing.T) {
	recon, _, db, repo, cleanup := setupReconcilerIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchange := "dummy"
	symbol := "LTC/USDT"

	quantity := 0.02
	price := 80.0
	// Create a single position slightly out of sync with wallet (fee/dust drift)
	pos := repository.PositionData{
		ExchangeName:     exchange,
		InstrumentSymbol: symbol,
		Side:             repository.PositionSideLong,
		Quantity:         quantity,
		EntryPrice:       price,
		HighestPrice:     price,
		UnknownOrigin:    true,
	}
	require.NoError(t, repo.Positions.UpsertPosition(ctx, db, pos))

	// Verify the position was created matching the price and quantity
	pos, err := repo.Positions.GetPosition(ctx, db, exchange, symbol)
	require.NoError(t, err)
	assert.Equal(t, quantity, pos.Quantity)
	assert.Equal(t, price, pos.EntryPrice)
	assert.Equal(t, price, pos.HighestPrice)
	assert.True(t, pos.UnknownOrigin)

	_, err = repo.Balances.UpsertBalance(ctx, db, repository.BalanceData{
		ExchangeName: exchange,
		AssetSymbol:  "LTC",
		Free:         0.01,
		Used:         0,
		Total:        0.01,
	})
	require.NoError(t, err)

	// Reconcile positions which should correct small drifts
	err = recon.SyncPositions(ctx, exchange, "")

	// Verify the position quantity is adjusted to the wallet truth
	updated, err := repo.Positions.GetPosition(ctx, db, exchange, symbol)
	require.NoError(t, err)
	assert.Equal(t, 0.01, updated.Quantity)
}

// TestReconciler_Integration_AdoptGhostBalance verifies a matched enabled strategy and wallet balance are adopted as a ghost position.
func TestReconciler_Integration_AdoptGhostBalance(t *testing.T) {
	recon, _, db, repo, cleanup := setupReconcilerIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchange := "dummy"
	symbol := "LTC/USDT"

	// Enable a strategy and create a matching wallet balance
	err := repo.Strategies.UpsertEnabledStrategy(ctx, db, exchange, symbol, repository.StrategyMomentumProfit, "integration-test", repository.StrategyMomentum{
		WindowSeconds:   5,
		Windows:         []repository.MomentumWindow{{LookbackSeconds: 1, Threshold: 0.01 * 0.01}},
		RequireAll:      true,
		StopLossPct:     1 * 0.01,
		ProfitTargetPct: sql.NullFloat64{Float64: 0.5 * 0.01, Valid: true},
	})
	require.NoError(t, err)

	_, err = repo.Balances.UpsertBalance(ctx, db, repository.BalanceData{
		ExchangeName: exchange,
		AssetSymbol:  "LTC",
		Free:         2.0,
		Used:         0,
		Total:        2.0,
	})
	require.NoError(t, err)

	// Run SyncPositions which should adopt ghost balance as a position
	err = recon.SyncPositions(ctx, exchange, "")
	require.NoError(t, err)

	// Verify the ghost position created
	pos, err := repo.Positions.GetPosition(ctx, db, exchange, symbol)
	require.NoError(t, err)
	assert.True(t, pos.UnknownOrigin)
	assert.Equal(t, 2.0, pos.Quantity)
}

// TestReconciler_Integration_NoPromoteQuantityMismatch verifies that a trade whose filled quantity
// differs beyond the allowed fee margin does not promote an unknown-origin position.
func TestReconciler_Integration_NoPromoteQuantityMismatch(t *testing.T) {
	recon, execSvc, db, repo, cleanup := setupReconcilerIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchange := "dummy"
	symbol := "LTC/USDT"

	// Create an exchange order with a larger filled quantity than the DB position.
	order, err := execSvc.CreateOrder(ctx, exchange, symbol, repository.OrderSideBuy, repository.OrderTypeMarket, 1.10, 0)
	require.NoError(t, err)
	require.Equal(t, repository.OrderStatusClosed, order.Status)

	// Insert an unknown-origin position with smaller quantity (1.0) into DB
	pos := repository.PositionData{
		ExchangeName:     exchange,
		InstrumentSymbol: symbol,
		Side:             repository.PositionSideLong,
		Quantity:         1.0,
		EntryPrice:       0,
		HighestPrice:     0,
		UnknownOrigin:    true,
	}
	require.NoError(t, repo.Positions.UpsertPosition(ctx, db, pos))

	// Sync trade history — the mismatch beyond maxFeeMargin should prevent promotion.
	err = recon.SyncTradeHistory(ctx, exchange, symbol, 1*time.Minute)
	require.NoError(t, err)

	// Verify the position remains unknown-origin and not promoted
	updated, err := repo.Positions.GetPosition(ctx, db, exchange, symbol)
	require.NoError(t, err)
	assert.True(t, updated.UnknownOrigin)
	assert.Equal(t, 0.0, updated.EntryPrice)
}

// TestReconciler_Integration_PromoteUnknowOrigin verifies that an unknown-origin position is promoted by trade history when matching.
func TestReconciler_Integration_PromoteUnknowOrigin(t *testing.T) {
	recon, execSvc, db, repo, cleanup := setupReconcilerIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchange := "dummy"
	symbol := "SOL/USDT"

	// Create a order in the dummy gateway
	order, err := execSvc.CreateOrder(ctx, exchange, symbol, repository.OrderSideBuy, repository.OrderTypeMarket, 0.01, 0)
	require.NoError(t, err)
	require.Equal(t, repository.OrderStatusClosed, order.Status)

	// Insert an unknown-origin position with the order filled as quantity
	pos := repository.PositionData{
		ExchangeName:     exchange,
		InstrumentSymbol: symbol,
		Side:             repository.PositionSideLong,
		Quantity:         order.Filled,
		EntryPrice:       0,
		HighestPrice:     0,
		UnknownOrigin:    true,
	}
	require.NoError(t, repo.Positions.UpsertPosition(ctx, db, pos))

	// Sync trade history to find matching execution and promote the position
	err = recon.SyncTradeHistory(ctx, exchange, symbol, 1*time.Minute)
	require.NoError(t, err)

	// Position is promoted and entry price set
	updated, err := repo.Positions.GetPosition(ctx, db, exchange, symbol)
	require.NoError(t, err)
	assert.False(t, updated.UnknownOrigin)
	assert.NotZero(t, updated.EntryPrice)
}
