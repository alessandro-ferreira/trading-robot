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
	"trading/robot/go-bot/internal/utils"

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

	client, err := execution.NewClient(&grpcConfig)
	require.NoError(t, err, "failed to connect to gateway")

	_, err = client.ResetState(ctx)
	require.NoError(t, err, "failed to reset gateway state")

	// Initialize Components
	logger := slog.New(slog.NewTextHandler(io.Discard, nil)) // slog.Default()
	repoContainer := repository.New()
	pf := portfolio.NewPortfolio(logger, db, repoContainer)
	clock := utils.NewSystemClock()
	execSvc := execution.NewService(logger, db, client, repoContainer, clock)

	// Ensure a clean state for positions to avoid interference from previous tests.
	activePos, _ := repoContainer.Positions.GetPositions(ctx, db, "", "")
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

	openOrder, err := repo.Orders.GetOrder(ctx, db, exchange, order.ExchangeOrderID.String)
	require.NoError(t, err)
	assert.Equal(t, repository.OrderStatusOpen, openOrder.Status)

	// Run the reconciler
	err = recon.SyncOrders(ctx, exchange, symbol)
	require.NoError(t, err)

	// Validate that the order status is updated to closed in the database after reconciliation.
	updatedOrder, err := repo.Orders.GetOrder(ctx, db, exchange, order.ExchangeOrderID.String)
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
	err = execSvc.CancelOrder(ctx, exchange, symbol, order.ExchangeOrderID.String)
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

// TestReconciler_Integration_SyncExpiredStopOrder verifies that when a stop order is expired/canceled
// on the exchange, the reconciler updates the local order status and adjusts the position accordingly.
func TestReconciler_Integration_SyncExpiredStopOrder(t *testing.T) {
	recon, execSvc, db, repo, cleanup := setupReconcilerIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchange := "dummy"
	symbol := "BTC/USDT"

	// Create a filled buy order on the exchange to provide funds for the stop order
	buyOrder, err := execSvc.CreateOrder(ctx, exchange, symbol, repository.OrderSideBuy, repository.OrderTypeMarket, 0.001, 0)
	require.NoError(t, err)
	require.Equal(t, repository.OrderStatusClosed, buyOrder.Status)

	order, err := execSvc.CreateStopOrder(ctx, exchange, symbol, repository.OrderSideSell, buyOrder.Filled, 48000.0, 0)
	require.NoError(t, err)

	// Create a position with StopLossActive = true linked to the buy order
	pos := repository.PositionData{
		ExchangeName:     exchange,
		InstrumentSymbol: symbol,
		OrderID:          sql.NullInt64{Int64: buyOrder.ID, Valid: true},
		Side:             repository.PositionSideLong,
		Quantity:         buyOrder.Filled,
		EntryPrice:       50000.0,
		HighestPrice:     50000.0,
		StopLossActive:   true,
		UnknownOrigin:    false,
		Active:           true,
	}
	require.NoError(t, repo.Positions.UpsertPosition(ctx, db, pos))

	// Cancel the stop order on the exchange (simulating expiration / external cancel)
	err = execSvc.CancelOrder(ctx, exchange, symbol, order.ExchangeOrderID.String)
	require.NoError(t, err)

	// Update local DB balance to simulate remaining partial-fill quantity (e.g. 0.0008)
	remainingQty := buyOrder.Filled * 0.8
	_, err = repo.Balances.UpsertBalance(ctx, db, repository.BalanceData{
		ExchangeName: exchange,
		AssetSymbol:  "BTC",
		Free:         remainingQty,
		Used:         0,
		Total:        remainingQty,
	})
	require.NoError(t, err)

	// Run the reconciler
	err = recon.SyncStopOrders(ctx, exchange, symbol)
	require.NoError(t, err)

	// Assert order status was synced to canceled
	updatedOrder, err := repo.Orders.GetOrder(ctx, db, exchange, order.ExchangeOrderID.String)
	require.NoError(t, err)
	assert.Equal(t, repository.OrderStatusCanceled, updatedOrder.Status)

	// Assert position has StopLossActive = false and Quantity updated
	updatedPos, err := repo.Positions.GetPosition(ctx, db, exchange, symbol)
	require.NoError(t, err)
	assert.False(t, updatedPos.StopLossActive)
	assert.Equal(t, remainingQty, updatedPos.Quantity)
}

// TestReconciler_Integration_SyncExecutedStopOrder verifies that when a stop order is executed
// on the exchange, the reconciler updates the local order status and position accordingly.
func TestReconciler_Integration_SyncExecutedStopOrder(t *testing.T) {
	recon, execSvc, db, repo, cleanup := setupReconcilerIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchange := "dummy"
	symbol := "BTC/USDT"

	// Create a filled buy order on the exchange to provide funds for the stop order
	buyOrder, err := execSvc.CreateOrder(ctx, exchange, symbol, repository.OrderSideBuy, repository.OrderTypeMarket, 0.001, 0)
	require.NoError(t, err)
	require.Equal(t, repository.OrderStatusClosed, buyOrder.Status)

	order, err := execSvc.CreateStopOrder(ctx, exchange, symbol, repository.OrderSideSell, buyOrder.Filled, 48000.0, 0)
	require.NoError(t, err)

	// Create a position with StopLossActive = true linked to the buy order
	pos := repository.PositionData{
		ExchangeName:     exchange,
		InstrumentSymbol: symbol,
		OrderID:          sql.NullInt64{Int64: buyOrder.ID, Valid: true},
		Side:             repository.PositionSideLong,
		Quantity:         buyOrder.Filled,
		EntryPrice:       50000.0,
		HighestPrice:     50000.0,
		StopLossActive:   true,
		UnknownOrigin:    false,
		Active:           true,
	}
	require.NoError(t, repo.Positions.UpsertPosition(ctx, db, pos))

	// Fetch the executed stop order from the exchange until it is filled (simulate execution)
	require.Eventually(t, func() bool {
		_, err := execSvc.GetTicker(ctx, exchange, symbol)
		require.NoError(t, err)

		exchOrder, err := execSvc.GetOrder(ctx, exchange, symbol, order.ExchangeOrderID.String)
		require.NoError(t, err)
		return exchOrder.Status == repository.OrderStatusClosed
	}, 250*time.Millisecond, 5*time.Millisecond, "Stop order should be executed on the exchange")

	// Set DB balance to zero (simulating full execution and balance liquidation)
	_, err = repo.Balances.UpsertBalance(ctx, db, repository.BalanceData{
		ExchangeName: exchange,
		AssetSymbol:  "BTC",
		Free:         0,
		Used:         0,
		Total:        0,
	})
	require.NoError(t, err)

	// Make DB think the stop order is still open
	order.Status = repository.OrderStatusOpen
	_, err = repo.Orders.UpdateOrder(ctx, db, order)
	require.NoError(t, err)

	// Run the reconciler
	err = recon.SyncStopOrders(ctx, exchange, symbol)
	require.NoError(t, err)

	// Assert order status was synced to closed
	updatedOrder, err := repo.Orders.GetOrder(ctx, db, exchange, order.ExchangeOrderID.String)
	require.NoError(t, err)
	assert.Equal(t, repository.OrderStatusClosed, updatedOrder.Status)

	// Position should still have original quantity and StopLossActive = true (SyncPositions will clean it)
	currentPos, err := repo.Positions.GetPosition(ctx, db, exchange, symbol)
	require.NoError(t, err)
	assert.True(t, currentPos.StopLossActive)
	assert.Equal(t, buyOrder.Filled, currentPos.Quantity)
}

// TestReconciler_Integration_CancelStaleStopOrder verifies that a stop order owned by an older order is canceled.
func TestReconciler_Integration_CancelStaleStopOrder(t *testing.T) {
	recon, execSvc, db, repo, cleanup := setupReconcilerIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchange := "dummy"
	symbol := "BTC/USDT"

	// Create a filled buy order, stop order, and active position.
	firstBuy, err := execSvc.CreateOrder(ctx, exchange, symbol, repository.OrderSideBuy, repository.OrderTypeMarket, 0.001, 0)
	require.NoError(t, err)
	require.Equal(t, repository.OrderStatusClosed, firstBuy.Status)

	stopOrder, err := execSvc.CreateStopOrder(ctx, exchange, symbol, repository.OrderSideSell, firstBuy.Filled, 1.0, 0)
	require.NoError(t, err)

	position := repository.PositionData{
		ExchangeName:     exchange,
		InstrumentSymbol: symbol,
		OrderID:          sql.NullInt64{Int64: firstBuy.ID, Valid: true},
		Side:             repository.PositionSideLong,
		Quantity:         firstBuy.Filled,
		EntryPrice:       50000,
		HighestPrice:     50000,
		StopLossActive:   true,
		Active:           true,
	}
	require.NoError(t, repo.Positions.UpsertPosition(ctx, db, position))

	// Create a newer buy order so the existing stop belongs to the previous position lifecycle.
	secondBuy, err := execSvc.CreateOrder(ctx, exchange, symbol, repository.OrderSideBuy, repository.OrderTypeMarket, 0.001, 0)
	require.NoError(t, err)
	require.Equal(t, repository.OrderStatusClosed, secondBuy.Status)

	// Set the wallet balance and reconcile stop orders.
	_, err = repo.Balances.UpsertBalance(ctx, db, repository.BalanceData{
		ExchangeName: exchange,
		AssetSymbol:  "BTC",
		Free:         firstBuy.Filled + secondBuy.Filled,
		Total:        firstBuy.Filled + secondBuy.Filled,
	})
	require.NoError(t, err)

	require.NoError(t, recon.SyncStopOrders(ctx, exchange, symbol))

	// Verify the stale stop was canceled and position protection was reset.
	updatedStop, err := repo.Orders.GetOrder(ctx, db, exchange, stopOrder.ExchangeOrderID.String)
	require.NoError(t, err)
	assert.Equal(t, repository.OrderStatusCanceled, updatedStop.Status)
	updatedPosition, err := repo.Positions.GetPosition(ctx, db, exchange, symbol)
	require.NoError(t, err)
	assert.False(t, updatedPosition.StopLossActive)
}

// TestReconciler_Integration_SyncMissingStopOrder verifies that when a stop order is missing on the exchange
// but the position is active, SyncStopOrders should deactivate the stop loss and update the position quantity.
func TestReconciler_Integration_SyncMissingStopOrder(t *testing.T) {
	recon, execSvc, db, repo, cleanup := setupReconcilerIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchange := "dummy"
	symbol := "BTC/USDT"

	// Create a filled buy order on the exchange
	buyOrder, err := execSvc.CreateOrder(ctx, exchange, symbol, repository.OrderSideBuy, repository.OrderTypeMarket, 0.001, 0)
	require.NoError(t, err)
	require.Equal(t, repository.OrderStatusClosed, buyOrder.Status)

	// Create an active position with StopLossActive = true linked to the buy order
	pos := repository.PositionData{
		ExchangeName:     exchange,
		InstrumentSymbol: symbol,
		OrderID:          sql.NullInt64{Int64: buyOrder.ID, Valid: true},
		Side:             repository.PositionSideLong,
		Quantity:         buyOrder.Filled,
		EntryPrice:       50000.0,
		HighestPrice:     50000.0,
		StopLossActive:   true,
		UnknownOrigin:    false,
		Active:           true,
	}
	require.NoError(t, repo.Positions.UpsertPosition(ctx, db, pos))

	// Set wallet balance to non-zero (e.g. 0.00095 due to fees or dust)
	remainingQty := buyOrder.Filled * 0.95
	_, err = repo.Balances.UpsertBalance(ctx, db, repository.BalanceData{
		ExchangeName: exchange,
		AssetSymbol:  "BTC",
		Free:         remainingQty,
		Used:         0,
		Total:        remainingQty,
	})
	require.NoError(t, err)

	// No stop orders exist in DB or gateway. Run SyncStopOrders.
	err = recon.SyncStopOrders(ctx, exchange, symbol)
	require.NoError(t, err)

	// Position should have StopLossActive = false and Quantity updated to wallet balance
	updatedPos, err := repo.Positions.GetPosition(ctx, db, exchange, symbol)
	require.NoError(t, err)
	assert.False(t, updatedPos.StopLossActive)
	assert.Equal(t, remainingQty, updatedPos.Quantity)
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
	recon, execSvc, db, repo, cleanup := setupReconcilerIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchange := "dummy"
	symbol := "LTC/USDT"

	// Create a single position slightly out of sync with wallet (fee/dust drift).
	ticker, err := execSvc.GetTicker(ctx, exchange, symbol)
	require.NoError(t, err)
	quantity := (10 * portfolio.PositionDeletionThreshold) / ticker.Price
	pos := repository.PositionData{
		ExchangeName:     exchange,
		InstrumentSymbol: symbol,
		Side:             repository.PositionSideLong,
		Quantity:         quantity,
		EntryPrice:       ticker.Price,
		HighestPrice:     ticker.Price,
		UnknownOrigin:    true,
	}
	require.NoError(t, repo.Positions.UpsertPosition(ctx, db, pos))

	// Verify the position was created matching the price and quantity
	pos, err = repo.Positions.GetPosition(ctx, db, exchange, symbol)
	require.NoError(t, err)
	assert.Equal(t, quantity, pos.Quantity)
	assert.Equal(t, ticker.Price, pos.EntryPrice)
	assert.Equal(t, ticker.Price, pos.HighestPrice)
	assert.True(t, pos.UnknownOrigin)

	// Set the wallet balance to simulate a small drift due to fees/dust
	walletQuantity := quantity * 0.95
	_, err = repo.Balances.UpsertBalance(ctx, db, repository.BalanceData{
		ExchangeName: exchange,
		AssetSymbol:  "LTC",
		Free:         walletQuantity,
		Used:         0,
		Total:        walletQuantity,
	})
	require.NoError(t, err)

	// Reconcile positions which should correct small drifts
	err = recon.SyncPositions(ctx, exchange, "")
	require.NoError(t, err)

	// Verify the position quantity is adjusted to the wallet truth
	updated, err := repo.Positions.GetPosition(ctx, db, exchange, symbol)
	require.NoError(t, err)
	assert.Equal(t, walletQuantity, updated.Quantity)
}

// TestReconciler_Integration_DeleteDustPosition verifies that a position below the retention value is removed.
func TestReconciler_Integration_DeleteDustPosition(t *testing.T) {
	recon, execSvc, db, repo, cleanup := setupReconcilerIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchange := "dummy"
	symbol := "LTC/USDT"

	// Create a known-origin position linked to a filled buy order.
	ticker, err := execSvc.GetTicker(ctx, exchange, symbol)
	require.NoError(t, err)
	buyOrder, err := execSvc.CreateOrder(ctx, exchange, symbol, repository.OrderSideBuy, repository.OrderTypeMarket, 0.001, 0)
	require.NoError(t, err)
	require.Equal(t, repository.OrderStatusClosed, buyOrder.Status)
	position := repository.PositionData{
		ExchangeName:     exchange,
		InstrumentSymbol: symbol,
		OrderID:          sql.NullInt64{Int64: buyOrder.ID, Valid: true},
		Side:             repository.PositionSideLong,
		Quantity:         buyOrder.Filled,
		EntryPrice:       ticker.Price,
		HighestPrice:     ticker.Price,
		UnknownOrigin:    false,
		Active:           true,
	}
	require.NoError(t, repo.Positions.UpsertPosition(ctx, db, position))

	require.NoError(t, recon.SyncPositions(ctx, exchange, ""))
	// Verify the over-retention position was not removed.
	_, err = repo.Positions.GetPosition(ctx, db, exchange, symbol)
	require.NoError(t, err)

	// Set the wallet value below the retention threshold.
	walletQuantity := (portfolio.PositionDeletionThreshold / ticker.Price) * 0.5
	_, err = repo.Balances.UpsertBalance(ctx, db, repository.BalanceData{
		ExchangeName: exchange,
		AssetSymbol:  "LTC",
		Free:         walletQuantity,
		Total:        walletQuantity,
	})
	require.NoError(t, err)

	require.NoError(t, recon.SyncPositions(ctx, exchange, ""))
	// Verify the below-retention position was removed.
	_, err = repo.Positions.GetPosition(ctx, db, exchange, symbol)
	assert.Error(t, err)
}

// TestReconciler_Integration_SkipGhostBelowActivation verifies that a ghost balance below activation is ignored.
func TestReconciler_Integration_SkipGhostBelowActivation(t *testing.T) {
	recon, execSvc, db, repo, cleanup := setupReconcilerIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	exchange := "dummy"
	symbol := "LTC/USDT"

	// Enable a strategy for the ghost balance's instrument.
	err := repo.Strategies.UpsertEnabledStrategy(ctx, db, exchange, symbol, repository.StrategyMomentumProfit, "integration-test", repository.StrategyMomentum{
		WindowSeconds:   5,
		Windows:         []repository.MomentumWindow{{LookbackSeconds: 1, Threshold: 0.01 * 0.01}},
		RequireAll:      true,
		StopLossPct:     1 * 0.01,
		ProfitTargetPct: sql.NullFloat64{Float64: 0.5 * 0.01, Valid: true},
	})
	require.NoError(t, err)

	// Set a wallet balance below the activation threshold and reconcile positions.
	ticker, err := execSvc.GetTicker(ctx, exchange, symbol)
	require.NoError(t, err)
	walletQuantity := (portfolio.PositionCreationThreshold - 0.01) / ticker.Price
	_, err = repo.Balances.UpsertBalance(ctx, db, repository.BalanceData{
		ExchangeName: exchange,
		AssetSymbol:  "LTC",
		Free:         walletQuantity,
		Total:        walletQuantity,
	})
	require.NoError(t, err)

	require.NoError(t, recon.SyncPositions(ctx, exchange, ""))

	// Verify the below-activation ghost balance was ignored.
	_, err = repo.Positions.GetPosition(ctx, db, exchange, symbol)
	assert.Error(t, err)
}

// TestReconciler_Integration_AdoptGhostBalance verifies a matched enabled strategy and wallet balance are adopted as a ghost position.
func TestReconciler_Integration_AdoptGhostBalance(t *testing.T) {
	recon, execSvc, db, repo, cleanup := setupReconcilerIntegrationTest(t)
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

	// Set a wallet balance above the activation threshold and reconcile positions.
	ticker, err := execSvc.GetTicker(ctx, exchange, symbol)
	require.NoError(t, err)
	walletQuantity := (portfolio.PositionCreationThreshold + 0.01) / ticker.Price
	_, err = repo.Balances.UpsertBalance(ctx, db, repository.BalanceData{
		ExchangeName: exchange,
		AssetSymbol:  "LTC",
		Free:         walletQuantity,
		Used:         0,
		Total:        walletQuantity,
	})
	require.NoError(t, err)

	// Run SyncPositions which should adopt ghost balance as a position
	err = recon.SyncPositions(ctx, exchange, "")
	require.NoError(t, err)

	// Verify the ghost position created
	pos, err := repo.Positions.GetPosition(ctx, db, exchange, symbol)
	require.NoError(t, err)
	assert.True(t, pos.UnknownOrigin)
	assert.Equal(t, walletQuantity, pos.Quantity)
}
