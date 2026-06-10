package orchestrator

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"trading/robot/go-bot/internal/components/signal_generator"
	"trading/robot/go-bot/internal/config"
	"trading/robot/go-bot/internal/database/repository"
)

// --- Mocks ---

type MockStrategiesRepo struct{ mock.Mock }

func (m *MockStrategiesRepo) GetStrategyPairs(ctx context.Context, db repository.DBExecutor, st []string) ([]repository.StrategyPair, error) {
	args := m.Called(ctx, db, st)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]repository.StrategyPair), args.Error(1)
}
func (m *MockStrategiesRepo) UpsertEnabledStrategy(ctx context.Context, db repository.DBExecutor, ex, sym, typ, lbl string, mom repository.StrategyMomentum) error {
	return m.Called(ctx, db, ex, sym, typ, lbl, mom).Error(0)
}
func (m *MockStrategiesRepo) RequestStrategyDisable(ctx context.Context, db repository.DBExecutor, ex, sym, typ string) error {
	return m.Called(ctx, db, ex, sym, typ).Error(0)
}
func (m *MockStrategiesRepo) ApplyStrategyDisable(ctx context.Context, db repository.DBExecutor, ex, sym string) error {
	return m.Called(ctx, db, ex, sym).Error(0)
}

type MockMarketDataRepo struct{ mock.Mock }

func (m *MockMarketDataRepo) GetMarketDataTicks(ctx context.Context, db repository.DBExecutor, ex, sym string, lim int) ([]repository.MarketDataTick, error) {
	args := m.Called(ctx, db, ex, sym, lim)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]repository.MarketDataTick), args.Error(1)
}
func (m *MockMarketDataRepo) InsertTick(ctx context.Context, db repository.DBExecutor, t repository.MarketDataTick) error {
	return m.Called(ctx, db, t).Error(0)
}

type MockRiskRepo struct{ mock.Mock }

func (m *MockRiskRepo) GetRiskPair(ctx context.Context, db repository.DBExecutor, ex, sym string) (repository.RiskPair, error) {
	args := m.Called(ctx, db, ex, sym)
	return args.Get(0).(repository.RiskPair), args.Error(1)
}
func (m *MockRiskRepo) UpsertRiskPair(ctx context.Context, db repository.DBExecutor, r repository.RiskPair) error {
	return m.Called(ctx, db, r).Error(0)
}

type MockPortfolio struct{ mock.Mock }

func (m *MockPortfolio) LoadState(ctx context.Context) error { return m.Called(ctx).Error(0) }
func (m *MockPortfolio) RefreshState(ctx context.Context, ex, sym string) error {
	return m.Called(ctx, ex, sym).Error(0)
}
func (m *MockPortfolio) GetActivePositionsCount() int { return m.Called().Int(0) }
func (m *MockPortfolio) GetTotalValue(ctx context.Context) (map[string]float64, error) {
	args := m.Called(ctx)
	return args.Get(0).(map[string]float64), args.Error(1)
}
func (m *MockPortfolio) GetPosition(ctx context.Context, ex, sym string) (repository.PositionData, error) {
	args := m.Called(ctx, ex, sym)
	return args.Get(0).(repository.PositionData), args.Error(1)
}
func (m *MockPortfolio) CreatePosition(ctx context.Context, ex, sym string, q, p float64, oid int64) error {
	return m.Called(ctx, ex, sym, q, p, oid).Error(0)
}
func (m *MockPortfolio) UpdatePosition(ctx context.Context, ex, sym string, u repository.PositionData) error {
	return m.Called(ctx, ex, sym, u).Error(0)
}
func (m *MockPortfolio) DeletePosition(ctx context.Context, ex, sym string) error {
	return m.Called(ctx, ex, sym).Error(0)
}

type MockReconciler struct{ mock.Mock }

func (m *MockReconciler) SyncOrders(ctx context.Context, ex, sym string) error {
	return m.Called(ctx, ex, sym).Error(0)
}
func (m *MockReconciler) SyncPositions(ctx context.Context, ex, sym string) error {
	return m.Called(ctx, ex, sym).Error(0)
}
func (m *MockReconciler) SyncTradeHistory(ctx context.Context, ex, sym string, lb time.Duration) error {
	return m.Called(ctx, ex, sym, lb).Error(0)
}

type MockExecutionService struct{ mock.Mock }

func (m *MockExecutionService) GetTicker(ctx context.Context, ex, sym string) (repository.MarketDataTick, error) {
	args := m.Called(ctx, ex, sym)
	return args.Get(0).(repository.MarketDataTick), args.Error(1)
}
func (m *MockExecutionService) GetBalance(ctx context.Context, ex, asset string) ([]repository.BalanceData, error) {
	args := m.Called(ctx, ex, asset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]repository.BalanceData), args.Error(1)
}
func (m *MockExecutionService) CreateOrder(ctx context.Context, ex, sym, side, typ string, amt, pr float64) (repository.OrderData, error) {
	args := m.Called(ctx, ex, sym, side, typ, amt, pr)
	return args.Get(0).(repository.OrderData), args.Error(1)
}
func (m *MockExecutionService) CreateStopOrder(ctx context.Context, ex, sym, side string, amt, stop, limit float64) (repository.OrderData, error) {
	args := m.Called(ctx, ex, sym, side, amt, stop, limit)
	return args.Get(0).(repository.OrderData), args.Error(1)
}
func (m *MockExecutionService) CancelOrder(ctx context.Context, ex, sym, id string) error {
	return m.Called(ctx, ex, sym, id).Error(0)
}
func (m *MockExecutionService) GetOrder(ctx context.Context, ex, sym, id string) (repository.OrderData, error) {
	args := m.Called(ctx, ex, sym, id)
	return args.Get(0).(repository.OrderData), args.Error(1)
}
func (m *MockExecutionService) GetOpenOrders(ctx context.Context, ex, sym string, lim int) ([]repository.OrderData, error) {
	args := m.Called(ctx, ex, sym, lim)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]repository.OrderData), args.Error(1)
}
func (m *MockExecutionService) GetRecentTrades(ctx context.Context, ex, sym string, since int64, lim int) ([]repository.OrderData, error) {
	args := m.Called(ctx, ex, sym, since, lim)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]repository.OrderData), args.Error(1)
}

// --- Helpers ---

func setupOrchestratorTest(t *testing.T) (*Orchestrator, *repository.Container, *MockPortfolio, *MockReconciler, *MockExecutionService) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mStrats := new(MockStrategiesRepo)
	mPf := new(MockPortfolio)
	mRecon := new(MockReconciler)
	mExec := new(MockExecutionService)
	mMD := new(MockMarketDataRepo)
	mRisk := new(MockRiskRepo)

	repo := &repository.Container{
		Strategies: mStrats,
		MarketData: mMD,
		Risks:      mRisk,
	}

	cfg := &config.Config{
		Server: config.ServerConfig{
			OrchestratorInterval: 100 * time.Millisecond,
			RefreshStratInterval: 1 * time.Minute,
		},
		Risk: config.RiskConfig{
			MaxOpenPositions: 3,
			MaxDailyLoss:     100.0,
		},
	}

	orch, err := New(logger, nil, repo, cfg, mPf, mRecon, mExec)
	require.NoError(t, err)

	return orch, repo, mPf, mRecon, mExec
}

func mockWorkerInit(repo *repository.Container, mPf *MockPortfolio, mExec *MockExecutionService, ex, sym string) {
	mMD := repo.MarketData.(*MockMarketDataRepo)
	mRisk := repo.Risks.(*MockRiskRepo)

	mMD.On("GetMarketDataTicks", mock.Anything, mock.Anything, ex, sym, mock.Anything).
		Return([]repository.MarketDataTick{}, nil).Maybe()
	mRisk.On("GetRiskPair", mock.Anything, mock.Anything, ex, sym).
		Return(repository.RiskPair{}, nil).Maybe()
	mPf.On("GetPosition", mock.Anything, ex, sym).
		Return(repository.PositionData{}, nil).Maybe()

	mExec.On("GetTicker", mock.Anything, ex, sym).
		Return(repository.MarketDataTick{Price: 100.0}, nil).Maybe()
	mExec.On("GetBalance", mock.Anything, ex, mock.Anything).
		Return([]repository.BalanceData{{AssetSymbol: "BTC", Total: 0}}, nil).Maybe()
	mPf.On("UpdatePosition", mock.Anything, ex, sym, mock.Anything).
		Return(nil).Maybe()
}

func TestNewOrchestrator(t *testing.T) {
	orch, _, _, _, _ := setupOrchestratorTest(t)
	assert.NotNil(t, orch)
	assert.NotNil(t, orch.risk)
	assert.NotNil(t, orch.signals)
}

func TestNewOrchestrator_InvalidConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := &repository.Container{}

	t.Run("zero OrchestratorInterval", func(t *testing.T) {
		cfg := &config.Config{
			Server: config.ServerConfig{
				OrchestratorInterval: 0,
				RefreshStratInterval: 1 * time.Minute,
			},
		}
		orch, err := New(logger, nil, repo, cfg, nil, nil, nil)
		assert.Error(t, err)
		assert.Nil(t, orch)
	})

	t.Run("zero RefreshStratInterval", func(t *testing.T) {
		cfg := &config.Config{
			Server: config.ServerConfig{
				OrchestratorInterval: 1 * time.Second,
				RefreshStratInterval: 0,
			},
		}
		orch, err := New(logger, nil, repo, cfg, nil, nil, nil)
		assert.Error(t, err)
		assert.Nil(t, orch)
	})
}

func TestOrchestrator_Start(t *testing.T) {
	orch, repo, mPf, _, mExec := setupOrchestratorTest(t)
	mStrats := repo.Strategies.(*MockStrategiesRepo)

	ctx, cancel := context.WithCancel(context.Background())

	mPf.On("LoadState", mock.Anything).Return(nil).Once()
	mStrats.On("GetStrategyPairs", mock.Anything, mock.Anything, mock.Anything).Return(
		[]repository.StrategyPair{{ExchangeName: "binance", InstrumentSymbol: "BTC/USDT"}}, nil,
	)

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	mockWorkerInit(repo, mPf, mExec, "binance", "BTC/USDT")

	err := orch.Start(ctx)
	assert.NoError(t, err)
	mPf.AssertExpectations(t)
}

func TestOrchestrator_Start_PortfolioFailure(t *testing.T) {
	orch, _, mPf, _, _ := setupOrchestratorTest(t)
	mPf.On("LoadState", mock.Anything).Return(errors.New("db error")).Once()

	err := orch.Start(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "load portfolio state failed")
}

func TestOrchestrator_Close(t *testing.T) {
	orch, _, _, _, _ := setupOrchestratorTest(t)
	sig, _ := signal_generator.NewSignalGenerator(
		orch.logger, repository.RiskPair{}, repository.StrategyPair{Type: "dummy"}, "test",
	)
	orch.signals["test"] = sig

	err := orch.Close()
	assert.NoError(t, err)
}

func TestOrchestrator_RefreshStrategies(t *testing.T) {
	orch, repo, mPf, _, mExec := setupOrchestratorTest(t)
	mStrats := repo.Strategies.(*MockStrategiesRepo)

	exchange := "binance"
	symbol := "BTC/USDT"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mStrats.On("GetStrategyPairs", mock.Anything, mock.Anything, mock.Anything).
		Return([]repository.StrategyPair{{ExchangeName: exchange, InstrumentSymbol: symbol}}, nil).Once()

	mockWorkerInit(repo, mPf, mExec, exchange, symbol)

	wg := &sync.WaitGroup{}
	orch.refreshStrategies(ctx, wg)
	cancel()
	wg.Wait()

	mStrats.AssertExpectations(t)
}

func TestOrchestrator_RefreshStrategies_Error(t *testing.T) {
	orch, repo, _, _, _ := setupOrchestratorTest(t)
	mStrats := repo.Strategies.(*MockStrategiesRepo)

	mStrats.On("GetStrategyPairs", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("db error")).Once()

	// refreshStrategies doesn't return an error, but logs it.
	// We check it handles the error gracefully.
	assert.NotPanics(t, func() {
		orch.refreshStrategies(context.Background(), &sync.WaitGroup{})
	})
}

func TestOrchestrator_LoadValidStrategies(t *testing.T) {
	orch, repo, _, _, _ := setupOrchestratorTest(t)
	mStrats := repo.Strategies.(*MockStrategiesRepo)
	ctx := context.Background()

	expectedStatuses := []string{repository.StrategyEnabled, repository.StrategyPendingDisabled}

	mStrats.On("GetStrategyPairs", ctx, mock.Anything, expectedStatuses).
		Return([]repository.StrategyPair{{InstrumentSymbol: "BTC/USDT"}}, nil).Once()

	pairs, err := orch.loadValidStrategies(ctx)
	assert.NoError(t, err)
	assert.Len(t, pairs, 1)
}

func TestOrchestrator_OrchestrateStrategies(t *testing.T) {
	orch, repo, mPf, _, mExec := setupOrchestratorTest(t)

	pair := repository.StrategyPair{
		ExchangeName:     "binance",
		InstrumentSymbol: "BTC/USDT",
		Type:             repository.StrategyDummy,
		Status:           repository.StrategyEnabled,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("Spawn new worker", func(t *testing.T) {
		mockWorkerInit(repo, mPf, mExec, pair.ExchangeName, pair.InstrumentSymbol)

		wg := &sync.WaitGroup{}
		orch.orchestrateStrategies(ctx, []repository.StrategyPair{pair}, wg)

		assert.Eventually(t, func() bool {
			orch.mu.Lock()
			defer orch.mu.Unlock()
			return len(orch.signals) == 1
		}, 500*time.Millisecond, 10*time.Millisecond)

		cancel()
		wg.Wait()
	})

	t.Run("Update existing worker", func(t *testing.T) {
		orch.orchestrateStrategies(context.Background(), []repository.StrategyPair{pair}, &sync.WaitGroup{})

		orch.mu.Lock()
		assert.Len(t, orch.signals, 1)
		orch.mu.Unlock()
	})
}

func TestOrchestrator_StartWorker(t *testing.T) {
	orch, repo, mPf, _, mExec := setupOrchestratorTest(t)
	pair := repository.StrategyPair{ExchangeName: "binance", InstrumentSymbol: "ETH/USDT", Type: "dummy"}

	mockWorkerInit(repo, mPf, mExec, pair.ExchangeName, pair.InstrumentSymbol)

	ctx, cancel := context.WithCancel(context.Background())
	wg := &sync.WaitGroup{}
	orch.startWorker(ctx, pair, "SignalGenerator-binance-ETH/USDT", wg)

	assert.Eventually(t, func() bool {
		orch.mu.Lock()
		defer orch.mu.Unlock()
		_, exists := orch.signals["SignalGenerator-binance-ETH/USDT"]
		return exists
	}, 1*time.Second, 10*time.Millisecond)

	cancel()
	wg.Wait()
}

func TestOrchestrator_WorkerPanicRecovery(t *testing.T) {
	orch, repo, _, _, _ := setupOrchestratorTest(t)
	mMD := repo.MarketData.(*MockMarketDataRepo)

	pair := repository.StrategyPair{ExchangeName: "binance", InstrumentSymbol: "PANIC/USDT"}
	name := "SignalGenerator-binance-PANIC/USDT"

	// Force initSignalHandler to panic by making a dependency panic
	mMD.On("GetMarketDataTicks", mock.Anything, mock.Anything, "binance", "PANIC/USDT", mock.Anything).
		Panic("intentional panic").Once()

	wg := &sync.WaitGroup{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	orch.startWorker(ctx, pair, name, wg)

	// Wait for the goroutine to finish (via recovery)
	wg.Wait()

	// Verify that the worker is NOT in the signals map (stopWorker was called or never added)
	orch.mu.Lock()
	defer orch.mu.Unlock()
	_, exists := orch.signals[name]
	assert.False(t, exists)
}

func TestOrchestrator_UpdateWorker(t *testing.T) {
	orch, _, _, _, _ := setupOrchestratorTest(t)
	pair := repository.StrategyPair{Type: "dummy"}
	sig, _ := signal_generator.NewSignalGenerator(orch.logger, repository.RiskPair{}, pair, "test")

	orch.updateWorker(pair, sig, "test")
}

func TestOrchestrator_RunWorker(t *testing.T) {
	orch, repo, mPf, _, mExec := setupOrchestratorTest(t)
	pair := repository.StrategyPair{ExchangeName: "binance", InstrumentSymbol: "BTC/USDT", Type: "dummy"}
	sig, _ := signal_generator.NewSignalGenerator(orch.logger, repository.RiskPair{}, pair, "test")

	mockWorkerInit(repo, mPf, mExec, pair.ExchangeName, pair.InstrumentSymbol)

	ctx, cancel := context.WithCancel(context.Background())
	orch.signals[sig.Name()] = sig

	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()

	orch.runWorker(ctx, sig)
}

func TestOrchestrator_StopWorker(t *testing.T) {
	orch, _, _, _, _ := setupOrchestratorTest(t)
	name := "test-worker"

	sig, _ := signal_generator.NewSignalGenerator(
		orch.logger, repository.RiskPair{}, repository.StrategyPair{Type: "dummy"}, name,
	)
	orch.signals[name] = sig

	orch.stopWorker(name)

	orch.mu.Lock()
	assert.NotContains(t, orch.signals, name)
	orch.mu.Unlock()
}
