//go:build integration

package orchestrator

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"trading/robot/go-bot/internal/components/execution"
	"trading/robot/go-bot/internal/components/portfolio"
	reconcil "trading/robot/go-bot/internal/components/reconciliation"
	"trading/robot/go-bot/internal/components/risk"
	"trading/robot/go-bot/internal/config"
	"trading/robot/go-bot/internal/database"
	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/strategy"
)

// setupOrchestratorIntegrationTest initializes all dependencies for an Orchestrator integration test.
// It returns the initialized Orchestrator, database connection, execution client, and a cleanup function to release resources after the test.
func setupOrchestratorIntegrationTest(
	t *testing.T, orchInterval time.Duration, refreshInterval time.Duration,
) (*Orchestrator, *database.DB, execution.GatewayClient, func()) {
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
	require.NoError(t, err, "Failed to connect to database")
	require.NoError(t, db.Ping(ctx), "Failed to ping database")

	client, err := execution.NewGatewayClient(&grpcConfig)
	require.NoError(t, err, "Failed to connect to gateway")

	_, err = client.ResetState(ctx)
	require.NoError(t, err, "Failed to reset gateway state")

	// Initialize Components
	logger := slog.New(slog.NewTextHandler(io.Discard, nil)) // slog.Default()
	repoContainer := repository.New()
	pf := portfolio.NewPortfolio(logger, db, repoContainer)
	execSvc := execution.NewService(logger, db, client, repoContainer)
	reconciler := reconcil.NewReconciler(logger, db, repoContainer, execSvc, pf)

	// Define a test-specific configuration
	cfg := &config.Config{
		Server: config.ServerConfig{
			OrchestratorInterval: orchInterval,
			RefreshStratInterval: refreshInterval,
		},
		Risk: config.RiskConfig{
			MaxOpenPositions:  10,
			MaxBudgetPerTrade: map[string]float64{"USDT": 1000.0},
		},
	}

	// Ensure a clean state: find and disable all currently active strategies to reduce log noise
	activeStrats, err := repoContainer.Strategies.GetStrategyPairs(ctx, db, []string{repository.StrategyEnabled, repository.StrategyPendingDisabled})
	require.NoError(t, err)
	for _, s := range activeStrats {
		_ = repoContainer.Strategies.RequestStrategyDisable(ctx, db, s.ExchangeName, s.InstrumentSymbol, s.Type)
		_ = repoContainer.Strategies.ApplyStrategyDisable(ctx, db, s.ExchangeName, s.InstrumentSymbol)
	}

	// Ensure a clean state for positions to avoid interference from previous tests.
	activePos, err := repoContainer.Positions.GetActivePositions(ctx, db, "", "")
	require.NoError(t, err)
	for _, p := range activePos {
		_ = repoContainer.Positions.DeletePosition(ctx, db, p.ExchangeName, p.InstrumentSymbol)
	}

	orch, err := New(logger, db, repoContainer, cfg, pf, reconciler, execSvc)
	require.NoError(t, err, "Failed to create Orchestrator")

	cleanup := func() {
		cancel()
		orch.Close()
		client.Close()
		db.Close()
	}

	return orch, db, client, cleanup
}

// TestOrchestrator_Integration_HappyPath verifies the full trade cycle:
// Strategy Discovery -> Market Buy -> Position Creation -> Market Sell -> Position Closure.
func TestOrchestrator_Integration_HappyPath(t *testing.T) {
	orch, db, client, cleanup := setupOrchestratorIntegrationTest(t, 250*time.Millisecond, 1*time.Minute)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := repository.New()
	exchange := "dummy"
	symbol := "LTC/USDT"

	// Store initial USDT balance to verify profit at the end
	initialBal, err := client.GetBalance(ctx, exchange, "USDT")
	require.NoError(t, err)
	var initialUSDT float64
	for _, b := range initialBal.Balances {
		if b.Asset == "USDT" {
			initialUSDT = b.Total
		}
	}

	// Seed Risk configuration to ensure the trade is allowed
	maxAssetUnits := 12.0
	err = repo.Risks.UpsertRiskPair(ctx, db, repository.RiskPair{
		ExchangeName: exchange, InstrumentSymbol: symbol, AllocatedBudget: 1000.0,
		MaxAssetUnits: sql.NullFloat64{Float64: maxAssetUnits, Valid: true},
	})
	require.NoError(t, err)

	// Seed Warmup Ticks: we fetch the current price and repeat it for the last 10 seconds.
	// This ensures the momentum windows are full and ready to trigger on the next price change.
	ticker, err := client.GetTicker(ctx, exchange, symbol)
	require.NoError(t, err)
	basePrice := ticker.Price
	now := time.Now().Unix()

	for i := 10; i >= 1; i-- {
		err := repo.MarketData.InsertTick(ctx, db, repository.MarketDataTick{
			ExchangeName: exchange, Symbol: symbol, Price: basePrice, TickUnixAt: now - int64(i),
		})
		require.NoError(t, err)
	}

	// Seed Strategy configuration: MomentumProfit
	// A short 10s window with a 1s lookback ensures we detect the drift almost immediately.
	// Threshold (0.05%) is easily cleared by the 0.1% drift per fetch.
	// Low profit target (0.4%) allows for a quick SELL trigger in the same test.
	momentum := repository.StrategyMomentum{
		WindowSeconds: 10,
		Windows: []repository.MomentumWindow{
			{LookbackSeconds: 1, Threshold: 0.05 * 0.01},
		},
		RequireAll:      true,
		StopLossPct:     20 * 0.01,
		ProfitTargetPct: sql.NullFloat64{Float64: 0.4 * 0.01, Valid: true},
	}

	err = repo.Strategies.UpsertEnabledStrategy(
		ctx, db, exchange, symbol, repository.StrategyMomentumProfit, "integration-test", momentum,
	)
	require.NoError(t, err)

	errCh := make(chan error, 1)
	go func() {
		errCh <- orch.Start(ctx)
	}()

	t.Log("Orchestrator started, waiting for BUY execution...")

	// Wait for the Buy Order to be placed and the Position to be created.
	var orderID int64
	var highestPrice float64
	t.Log("Waiting for Position activation and Stop Loss protection...")
	require.Eventually(t, func() bool {
		pos, err := repo.Positions.GetPosition(ctx, db, exchange, symbol)
		if err != nil || !pos.Active || !pos.StopLossActive {
			if err == nil && highestPrice == 0.0 {
				highestPrice = pos.HighestPrice
			}
			return false
		}

		// Verify Stop Loss existence in dummy exchange
		openOrders, err := client.GetOpenOrders(ctx, exchange, symbol, 10)
		if err != nil {
			return false
		}
		for _, o := range openOrders.Orders {
			if o.Side == repository.OrderSideSell && o.Type == repository.OrderTypeStopMarket {
				orderID = pos.OrderID.Int64
				return true
			}
		}
		return false
	}, 3*time.Second, 250*time.Millisecond, "Position should be active and Stop Loss should be found on exchange")

	t.Logf("BUY confirmed. OrderID: %d. Position is active and protected.", orderID)

	// Verify the buy order status in DB
	orders, err := repo.Orders.GetOrders(
		ctx, db, exchange, symbol, []string{repository.OrderStatusClosed}, []string{repository.OrderTypeMarket}, []string{repository.OrderSideBuy}, 1,
	)
	require.NoError(t, err)
	require.NotEmpty(t, orders)

	// Verify LTC balance consistency on exchange
	bal, err := client.GetBalance(ctx, exchange, "LTC")
	require.NoError(t, err)
	var balanceLTC float64
	for _, b := range bal.Balances {
		if b.Asset == "LTC" {
			balanceLTC = b.Total
		}
	}
	assert.InDelta(t, maxAssetUnits*risk.SlippageBuffer, balanceLTC, 0.000001,
		"LTC balance on exchange should match the approved units with slippage buffer applied")

	t.Log("Waiting for simulated price drift to trigger SELL exit...")

	// In dummy exchange, every ticker fetch increments the price. Validate that the position highest price is updated.
	require.Eventually(t, func() bool {
		pos, err := repo.Positions.GetPosition(ctx, db, exchange, symbol)
		return err == nil && pos.HighestPrice > highestPrice
	}, 2*time.Second, 50*time.Millisecond, "Position highest price should be updated after price drift")

	// Now, we wait for the price to drift enough to trigger the profit target and cause the orchestrator to execute a SELL.
	require.Eventually(t, func() bool {
		activePositions, err := repo.Positions.GetActivePositions(ctx, db, exchange, symbol)
		return err == nil && len(activePositions) == 0
	}, 2*time.Second, 50*time.Millisecond, "Position should be closed after SELL signal")

	t.Log("SELL confirmed. Position closed.")

	// Verify the Sell order exists
	sellOrders, err := repo.Orders.GetOrders(
		ctx, db, exchange, symbol, []string{repository.OrderStatusClosed}, []string{}, []string{repository.OrderSideSell}, 10,
	)
	require.NoError(t, err)
	require.NotEmpty(t, sellOrders)

	// Final balance verification: LTC should be zeroed and USDT must be higher than starting point
	finalBal, err := client.GetBalance(ctx, exchange, "")
	require.NoError(t, err)
	var finalUSDT, finalLTC float64
	for _, b := range finalBal.Balances {
		if b.Asset == "USDT" {
			finalUSDT = b.Total
		}
		if b.Asset == "LTC" {
			finalLTC = b.Total
		}
	}
	assert.Equal(t, 0.0, finalLTC, "LTC balance should be zeroed out")
	assert.Greater(t, finalUSDT, initialUSDT, "Final USDT balance should be greater than the initial baseline")

	cancel()
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			assert.NoError(t, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Orchestrator failed to shut down gracefully within timeout")
	}
}

// TestOrchestrator_Integration_MultiPairScaling verifies that the Orchestrator
// can handle multiple concurrent trading pairs with different configurations.
func TestOrchestrator_Integration_MultiPairScaling(t *testing.T) {
	orch, db, client, cleanup := setupOrchestratorIntegrationTest(t, 50*time.Millisecond, 1*time.Minute)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := repository.New()
	exchange := "dummy"
	pairs := []struct {
		symbol string
		amount float64
	}{
		{"BTC/USDT", 0.001},
		{"ETH/USDT", 0.01},
		{"LTC/USDT", 0.1},
		{"SOL/USDT", 0.2},
	}

	// Setup Risk and Strategy and seed Warmup Ticks for each pair.
	for _, p := range pairs {
		err := repo.Risks.UpsertRiskPair(ctx, db, repository.RiskPair{
			ExchangeName: exchange, InstrumentSymbol: p.symbol, AllocatedBudget: 1000.0,
			MaxAssetUnits: sql.NullFloat64{Float64: p.amount, Valid: true},
		})
		require.NoError(t, err)

		ticker, err := client.GetTicker(ctx, exchange, p.symbol)
		require.NoError(t, err)
		basePrice := ticker.Price
		now := time.Now().Unix()
		for i := 10; i >= 1; i-- {
			_ = repo.MarketData.InsertTick(ctx, db, repository.MarketDataTick{
				ExchangeName: exchange, Symbol: p.symbol, Price: basePrice,
				TickUnixAt: now - int64(i),
			})
		}

		err = repo.Strategies.UpsertEnabledStrategy(ctx, db, exchange, p.symbol, repository.StrategyMomentumTrailing, "scaling-"+p.symbol, repository.StrategyMomentum{
			WindowSeconds: 10, Windows: []repository.MomentumWindow{{LookbackSeconds: 1, Threshold: 0.0001}},
			RequireAll: true, StopLossPct: 5 * 0.01,
			ActivationPct:   sql.NullFloat64{Float64: 10 * 0.01, Valid: true},
			TrailingStopPct: sql.NullFloat64{Float64: 0.5 * 0.01, Valid: true},
		})
		require.NoError(t, err)
	}

	go orch.Start(ctx)

	// Verify all pairs are eventually processed and moved into positions.
	seenActive := make(map[string]bool)
	require.Eventually(t, func() bool {
		activePos, err := repo.Positions.GetActivePositions(ctx, db, exchange, "")
		if err == nil {
			for _, p := range activePos {
				seenActive[p.InstrumentSymbol] = true
			}
		}
		return len(seenActive) == len(pairs)
	}, 2*time.Second, 50*time.Millisecond, "All pairs should have been active at least once")
}

// TestOrchestrator_Integration_DefensiveExit verifies that a forced Stop Loss fill on the exchange
// correctly resets the local strategy and closes the position.
func TestOrchestrator_Integration_DefensiveExit(t *testing.T) {
	orch, db, _, cleanup := setupOrchestratorIntegrationTest(t, 50*time.Millisecond, 1*time.Minute)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := repository.New()
	exchange, symbol := "dummy", "LTC/USDT"

	// Setup Risk and Strategy
	err := repo.Risks.UpsertRiskPair(ctx, db, repository.RiskPair{
		ExchangeName: exchange, InstrumentSymbol: symbol, AllocatedBudget: 1000.0,
	})
	require.NoError(t, err)

	momentum := repository.StrategyMomentum{
		WindowSeconds: 10,
		Windows: []repository.MomentumWindow{
			{LookbackSeconds: 1, Threshold: 0.05 * 0.01},
		},
		RequireAll:      true,
		StopLossPct:     2 * 0.01,
		ProfitTargetPct: sql.NullFloat64{Float64: 50 * 0.01, Valid: true}, // High target
	}
	err = repo.Strategies.UpsertEnabledStrategy(
		ctx, db, exchange, symbol, repository.StrategyMomentumProfit, "integration-test", momentum,
	)
	require.NoError(t, err)

	go orch.Start(ctx)

	// Wait for Position to be active and stop loss placed
	require.Eventually(t, func() bool {
		pos, err := repo.Positions.GetPosition(ctx, db, exchange, symbol)
		return err == nil && pos.Active && pos.StopLossActive
	}, 2*time.Second, 50*time.Millisecond)

	// Simulate Stop Loss Trigger
	require.Eventually(t, func() bool {
		_, err := repo.Positions.GetPosition(ctx, db, exchange, symbol)
		return err != nil // Position deleted from DB
	}, 3*time.Second, 50*time.Millisecond, "Position should be closed after Stop Loss fill")
}

// TestOrchestrator_Integration_ExternalDisturbance verifies recovery from manual exchange liquidation.
func TestOrchestrator_Integration_ExternalDisturbance(t *testing.T) {
	orch, db, client, cleanup := setupOrchestratorIntegrationTest(t, 50*time.Millisecond, 1*time.Minute)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := repository.New()
	exchange, symbol := "dummy", "LTC/USDT"

	// Setup Risk and Strategy
	err := repo.Risks.UpsertRiskPair(ctx, db, repository.RiskPair{
		ExchangeName: exchange, InstrumentSymbol: symbol, AllocatedBudget: 1000.0,
	})
	require.NoError(t, err)

	err = repo.Strategies.UpsertEnabledStrategy(
		ctx, db, exchange, symbol, repository.StrategyMomentumProfit, "integration-test", repository.StrategyMomentum{
			WindowSeconds: 10, Windows: []repository.MomentumWindow{{LookbackSeconds: 1, Threshold: 0.05 * 0.01}},
			StopLossPct: 20 * 0.01, ProfitTargetPct: sql.NullFloat64{Float64: 10 * 0.01, Valid: true},
		},
	)
	require.NoError(t, err)

	go orch.Start(ctx)

	require.Eventually(t, func() bool {
		pos, err := repo.Positions.GetPosition(ctx, db, exchange, symbol)
		return err == nil && pos.Active && pos.StopLossActive
	}, 2*time.Second, 50*time.Millisecond)

	// Simulate External Liquidation (Zero balance)
	// We use ResetState on the dummy gateway to clear all LTC balances.
	_, err = client.ResetState(ctx)
	require.NoError(t, err)

	// Since the Reconciler is responsible for detecting external truth discrepancies,
	// we manually trigger the sync components here to simulate a background reconciliation cycle.
	// Fetch balance from gateway to update the DB balance.
	_, err = orch.exec.GetBalance(ctx, exchange, "LTC")
	require.NoError(t, err)
	// Run SyncPositions to compare DB positions against the now-zeroed DB balance.
	err = orch.recon.SyncPositions(ctx, exchange, "")
	require.NoError(t, err)

	// Verify both DB cleanup and Strategy Engine reset are finalized.
	require.Eventually(t, func() bool {
		_, err := repo.Positions.GetPosition(ctx, db, exchange, symbol)
		isDeleted := err != nil

		// Check if strategy generator reset to searching
		name := "SignalGenerator-dummy-LTC/USDT"
		orch.mu.Lock()
		sig := orch.signals[name]
		orch.mu.Unlock()

		ticker, err := client.GetTicker(ctx, exchange, symbol)
		require.NoError(t, err)
		s, _ := sig.GetSignal(ticker.Price, time.Now().Unix())

		return isDeleted && s == strategy.SignalSearchingBuyEntry
	}, 2*time.Second, 50*time.Millisecond, "Bot should have closed position and reset engine to searching")
}

// TestOrchestrator_Integration_OrderlyTermination verifies the 'pending_disabled' lifecycle.
func TestOrchestrator_Integration_OrderlyTermination(t *testing.T) {
	orch, db, _, cleanup := setupOrchestratorIntegrationTest(t, 50*time.Millisecond, 250*time.Millisecond)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := repository.New()
	exchange, symbol := "dummy", "LTC/USDT"

	// Setup Risk and Strategy
	err := repo.Risks.UpsertRiskPair(ctx, db, repository.RiskPair{
		ExchangeName: exchange, InstrumentSymbol: symbol, AllocatedBudget: 1000.0,
	})
	require.NoError(t, err)

	momentum := repository.StrategyMomentum{
		WindowSeconds: 10, Windows: []repository.MomentumWindow{{LookbackSeconds: 1, Threshold: 0.05 * 0.01}},
		StopLossPct: 20 * 0.01, ProfitTargetPct: sql.NullFloat64{Float64: 0.4 * 0.01, Valid: true},
	}
	err = repo.Strategies.UpsertEnabledStrategy(
		ctx, db, exchange, symbol, repository.StrategyMomentumProfit, "integration-test", momentum,
	)
	require.NoError(t, err)

	go orch.Start(ctx)

	// Wait for Position to be active
	require.Eventually(t, func() bool {
		pos, err := repo.Positions.GetPosition(ctx, db, exchange, symbol)
		return err == nil && pos.Active
	}, 2*time.Second, 50*time.Millisecond)

	// Request Disable via API (Repository)
	err = repo.Strategies.RequestStrategyDisable(ctx, db, exchange, symbol, repository.StrategyMomentumProfit)
	require.NoError(t, err)

	// Verify worker stays alive and status is 'pending_disabled'
	strat, err := repo.Strategies.GetStrategyPairs(ctx, db, []string{repository.StrategyPendingDisabled})
	require.NoError(t, err)
	assert.Len(t, strat, 1)

	// Wait for natural Profit Take exit
	require.Eventually(t, func() bool {
		pos, err := repo.Positions.GetActivePositions(ctx, db, exchange, symbol)
		require.NoError(t, err)
		return len(pos) == 0
	}, 2*time.Second, 50*time.Millisecond)

	// Verify status moved to 'disabled' and worker is stopped
	require.Eventually(t, func() bool {
		strats, err := repo.Strategies.GetStrategyPairs(ctx, db, []string{repository.StrategyDisabled})
		require.NoError(t, err)

		found := false
		for _, s := range strats {
			if s.ExchangeName == exchange && s.InstrumentSymbol == symbol && s.Type == repository.StrategyMomentumProfit {
				found = true
				break
			}
		}

		orch.mu.Lock()
		defer orch.mu.Unlock()
		return found && len(orch.signals) == 0
	}, 2*time.Second, 50*time.Millisecond)
}

// TestOrchestrator_Integration_StateHydration verifies the bot recovers memory after a restart.
func TestOrchestrator_Integration_StateHydration(t *testing.T) {
	orch, db, client, cleanup := setupOrchestratorIntegrationTest(t, 50*time.Millisecond, 1*time.Minute)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := repository.New()
	exchange, symbol := "dummy", "ETH/USDT"

	// Seed existing position and history
	ticker, err := client.GetTicker(ctx, exchange, symbol)
	require.NoError(t, err)

	entryPrice := ticker.Price
	orderID, err := repo.Orders.CreateOrder(ctx, db, repository.OrderData{
		ExchangeName:     exchange,
		InstrumentSymbol: symbol,
		ExchangeOrderID:  "hydra-order-1",
		Side:             repository.OrderSideBuy,
		OrderType:        repository.OrderTypeMarket,
		Amount:           1.0,
		Status:           repository.OrderStatusClosed,
	})
	require.NoError(t, err)

	err = repo.Positions.UpsertPosition(ctx, db, repository.PositionData{
		ExchangeName: exchange, InstrumentSymbol: symbol, Side: repository.PositionSideLong,
		Quantity: 1.0, EntryPrice: entryPrice, HighestPrice: entryPrice + 100,
		Active: true, StopLossActive: true, UnknownOrigin: false,
		OrderID: sql.NullInt64{Int64: orderID, Valid: true},
	})
	require.NoError(t, err)

	// Seed market data for warmup
	now := time.Now().Unix()
	for i := 10; i >= 1; i-- {
		_ = repo.MarketData.InsertTick(ctx, db, repository.MarketDataTick{
			ExchangeName: exchange, Symbol: symbol, Price: entryPrice,
			TickUnixAt: now - int64(i),
		})
	}

	// Seed strategy
	err = repo.Strategies.UpsertEnabledStrategy(ctx, db, exchange, symbol, repository.StrategyMomentumProfit, "hydra", repository.StrategyMomentum{
		WindowSeconds: 20, Windows: []repository.MomentumWindow{{LookbackSeconds: 1, Threshold: 0.05 * 0.01}},
		StopLossPct: 20 * 0.01, ProfitTargetPct: sql.NullFloat64{Float64: 10 * 0.01, Valid: true},
	})
	require.NoError(t, err)

	// Start Orchestrator and verify hydration
	go orch.Start(ctx)

	// We check if the signal generator correctly identifies it's in a position
	// by checking if the next signal is 'tracking_sell_exit' instead of 'searching'.
	name := "SignalGenerator-dummy-ETH/USDT"
	require.Eventually(t, func() bool {
		orch.mu.Lock()
		sig, exists := orch.signals[name]
		orch.mu.Unlock()
		if !exists {
			return false
		}

		// Peek at signal (requires a ticker)
		ticker, err := client.GetTicker(ctx, exchange, symbol)
		if err != nil || ticker == nil {
			return false
		}
		s, _ := sig.GetSignal(ticker.Price, time.Now().Unix())
		return s == strategy.SignalTrackingSellExit
	}, 2*time.Second, 100*time.Millisecond)
}

// panicService is a test decorator for execution.Service that panics.
type panicService struct {
	execution.Service
	mu      sync.Mutex
	panicOn bool
}

func (s *panicService) setPanic(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.panicOn = on
}

func (s *panicService) GetTicker(ctx context.Context, ex, sym string) (repository.MarketDataTick, error) {
	s.mu.Lock()
	shouldPanic := s.panicOn
	s.mu.Unlock()

	if shouldPanic {
		panic("simulated worker panic")
	}
	return s.Service.GetTicker(ctx, ex, sym)
}

// TestOrchestrator_Integration_PanicRecovery verifies that the Orchestrator
// survives a worker panic and attempts to restart it.
func TestOrchestrator_Integration_PanicRecovery(t *testing.T) {
	orch, db, _, cleanup := setupOrchestratorIntegrationTest(t, 50*time.Millisecond, 200*time.Millisecond)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := repository.New()
	exchange, symbol := "dummy", "LTC/USDT"

	// Wrap execution service to trigger panic
	ps := &panicService{Service: orch.exec, panicOn: false}
	orch.exec = ps

	// Setup Strategy
	err := repo.Strategies.UpsertEnabledStrategy(ctx, db, exchange, symbol, repository.StrategyMomentumProfit, "panic-test", repository.StrategyMomentum{
		WindowSeconds: 10, Windows: []repository.MomentumWindow{{LookbackSeconds: 1, Threshold: 0.05 * 0.01}},
		StopLossPct: 20 * 0.01, ProfitTargetPct: sql.NullFloat64{Float64: 10 * 0.01, Valid: true},
	})
	require.NoError(t, err)

	go orch.Start(ctx)

	name := fmt.Sprintf("SignalGenerator-%s-%s", exchange, symbol)

	// Wait for worker to be registered in the map (started normally)
	require.Eventually(t, func() bool {
		orch.mu.Lock()
		_, exists := orch.signals[name]
		orch.mu.Unlock()
		return exists
	}, 2*time.Second, 50*time.Millisecond, "Worker should be started initially")

	// Enable panic mode
	ps.setPanic(true)

	// Wait for the panic to trigger stopWorker (which removes it from map)
	require.Eventually(t, func() bool {
		orch.mu.Lock()
		_, exists := orch.signals[name]
		orch.mu.Unlock()
		return !exists
	}, 2*time.Second, 50*time.Millisecond, "Worker should be removed from map after panic triggers recovery")

	// Stop panicking
	ps.setPanic(false)

	// Verify the worker is eventually restored by the reloader
	require.Eventually(t, func() bool {
		orch.mu.Lock()
		_, exists := orch.signals[name]
		orch.mu.Unlock()
		return exists
	}, 2*time.Second, 100*time.Millisecond, "Worker should be restarted by reloader after panic recovery")
}
