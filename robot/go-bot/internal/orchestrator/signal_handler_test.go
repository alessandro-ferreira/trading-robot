//go:build unit

package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"trading/robot/go-bot/internal/components/signal_generator"
	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/strategy"
)

// initTestSignalGenerator creates a generator with MomentumProfit and moves it to a target state.
func initTestSignalGenerator(t *testing.T, o *Orchestrator, target strategy.StrategySignal) *signal_generator.SignalGenerator {
	momentum := repository.StrategyMomentum{
		WindowSeconds:   600,
		Windows:         []repository.MomentumWindow{{LookbackSeconds: 60, Threshold: 0.01}},
		StopLossPct:     0.05,
		ProfitTargetPct: sql.NullFloat64{Float64: 0.1, Valid: true},
	}
	pair := repository.StrategyPair{
		ExchangeName:     "binance",
		InstrumentSymbol: "BTC/USDT",
		Type:             repository.StrategyMomentumProfit,
		Momentum:         momentum,
	}
	sig, err := signal_generator.NewSignalGenerator(o.logger, repository.RiskPair{InstrumentSymbol: "BTC/USDT", AllocatedBudget: 100}, pair, "test")
	require.NoError(t, err)

	now := time.Now().Unix()
	// Use 1-second intervals to ensure maximum precision and avoid MAX_LOOKBACK_STALENESS (60s) issues.
	// Memory overhead is negligible (~11KB for 700 points).
	var history []repository.MarketDataTick
	for i := 700; i >= 1; i-- {
		history = append(history, repository.MarketDataTick{
			TickUnixAt: now - int64(i),
			Price:      100.0,
		})
	}
	require.NoError(t, sig.Warmup(history))

	switch target {
	case strategy.SignalSearchingBuyEntry:
		// State: IDLE
		_, _ = sig.GetSignal(100.0, now)

	case strategy.SignalBuy:
		// Trigger BUY: price 100 -> 102 (2% > 1% threshold).
		// State: Transitions from IDLE to PENDING_BUY
		s, err := sig.GetSignal(102.0, now)
		require.NoError(t, err)
		require.Equal(t, strategy.SignalBuy, s)

	case strategy.SignalWaitingBuyFill:
		// State: PENDING_BUY
		_, _ = sig.GetSignal(102.0, now)

	case strategy.SignalTrackingSellExit:
		// State: IN_POSITION
		sig.SetInPosition(true, 100.0, 100.0)

	case strategy.SignalSell:
		// Trigger SELL: Drop from 100 -> 94 (6% > 5% Stop Loss).
		// State: Transitions from IN_POSITION to PENDING_SELL
		sig.SetInPosition(true, 100.0, 100.0)
		_, _ = sig.GetSignal(97.0, now-1)
		s, err := sig.GetSignal(94.0, now)
		require.NoError(t, err)
		require.Equal(t, strategy.SignalSell, s)

	case strategy.SignalWaitingSellFill:
		// State: Transitions from IN_POSITION to PENDING_SELL
		sig.SetInPosition(true, 100.0, 100.0)
		_, _ = sig.GetSignal(97.0, now-1)  // Use intermediate step to avoid 5% jump rejection
		s, err := sig.GetSignal(94.0, now) // Transitions IN_POSITION -> PENDING_SELL: Return SIGNAL_SELL
		require.NoError(t, err)
		require.Equal(t, strategy.SignalSell, s)
		s, err = sig.GetSignal(94.0, now) // PENDING_SELL: Return SIGNAL_WAITING_SELL_FILL
		require.NoError(t, err)
		require.Equal(t, strategy.SignalWaitingSellFill, s)

	case strategy.SignalInvalid:
		// State: IN_POSITION but corrupted (EntryPrice 0)
		sig.SetInPosition(true, 0, 0)
		s, err := sig.GetSignal(100.0, now)
		require.NoError(t, err)
		require.Equal(t, strategy.SignalInvalid, s)
	}

	return sig
}

func TestOrchestrator_InitSignalHandler(t *testing.T) {
	tests := []struct {
		name      string
		pair      repository.StrategyPair
		setup     func(o *Orchestrator, repo *repository.Container, mPf *MockPortfolio, mExec *MockExecutionService)
		wantErr   bool
		errSubstr string
	}{
		{
			name: "Success Initialization",
			pair: repository.StrategyPair{ExchangeName: "binance", InstrumentSymbol: "BTC/USDT", Type: "dummy", WarmupWindowSeconds: 10},
			setup: func(o *Orchestrator, repo *repository.Container, mPf *MockPortfolio, mExec *MockExecutionService) {
				repo.MarketData.(*MockMarketDataRepo).On("GetMarketDataTicks", mock.Anything, mock.Anything, "binance", "BTC/USDT", mock.Anything).
					Return([]repository.MarketDataTick{{Price: 50000}}, nil).Once()
				repo.Risks.(*MockRiskRepo).On("GetRiskPair", mock.Anything, mock.Anything, "binance", "BTC/USDT").
					Return(repository.RiskPair{AllocatedBudget: 100}, nil).Once()
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").
					Return(repository.PositionData{EntryPrice: 49000, HighestPrice: 51000}, nil).Once()
			},
		},
		{
			name: "Warmup Data Failure",
			pair: repository.StrategyPair{ExchangeName: "binance", InstrumentSymbol: "BTC/USDT", WarmupWindowSeconds: 10},
			setup: func(o *Orchestrator, repo *repository.Container, mPf *MockPortfolio, mExec *MockExecutionService) {
				repo.MarketData.(*MockMarketDataRepo).On("GetMarketDataTicks", mock.Anything, mock.Anything, "binance", "BTC/USDT", mock.Anything).
					Return(nil, errors.New("db disconnect")).Once()
			},
			wantErr:   true,
			errSubstr: "fetch warmup data failed",
		},
		{
			name: "Risk Config Failure",
			pair: repository.StrategyPair{ExchangeName: "binance", InstrumentSymbol: "BTC/USDT", Type: "dummy"},
			setup: func(o *Orchestrator, repo *repository.Container, mPf *MockPortfolio, mExec *MockExecutionService) {
				repo.MarketData.(*MockMarketDataRepo).On("GetMarketDataTicks", mock.Anything, mock.Anything, "binance", "BTC/USDT", mock.Anything).
					Return([]repository.MarketDataTick{}, nil).Once()
				repo.Risks.(*MockRiskRepo).On("GetRiskPair", mock.Anything, mock.Anything, "binance", "BTC/USDT").
					Return(repository.RiskPair{}, errors.New("risk error")).Once()
			},
			wantErr:   true,
			errSubstr: "fetch risk config failed",
		},
		{
			name: "Create Signal Generator Failure",
			pair: repository.StrategyPair{ExchangeName: "binance", InstrumentSymbol: "BTC/USDT", Type: "invalid_type"},
			setup: func(o *Orchestrator, repo *repository.Container, mPf *MockPortfolio, mExec *MockExecutionService) {
				repo.MarketData.(*MockMarketDataRepo).On("GetMarketDataTicks", mock.Anything, mock.Anything, "binance", "BTC/USDT", mock.Anything).
					Return([]repository.MarketDataTick{}, nil).Once()
				repo.Risks.(*MockRiskRepo).On("GetRiskPair", mock.Anything, mock.Anything, "binance", "BTC/USDT").
					Return(repository.RiskPair{}, nil).Once()
			},
			wantErr:   true,
			errSubstr: "create signal generator failed",
		},
		{
			name: "Duplicate Handler Prevention",
			pair: repository.StrategyPair{ExchangeName: "binance", InstrumentSymbol: "BTC/USDT", Type: "dummy"},
			setup: func(o *Orchestrator, repo *repository.Container, mPf *MockPortfolio, mExec *MockExecutionService) {
				repo.MarketData.(*MockMarketDataRepo).On("GetMarketDataTicks", mock.Anything, mock.Anything, "binance", "BTC/USDT", mock.Anything).
					Return([]repository.MarketDataTick{}, nil).Once()
				repo.Risks.(*MockRiskRepo).On("GetRiskPair", mock.Anything, mock.Anything, "binance", "BTC/USDT").
					Return(repository.RiskPair{}, nil).Once()
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, pgx.ErrNoRows).Once()

				sig, _ := signal_generator.NewSignalGenerator(o.logger, repository.RiskPair{}, repository.StrategyPair{ExchangeName: "binance", InstrumentSymbol: "BTC/USDT", Type: "dummy"}, "SignalGenerator-binance-BTC/USDT")
				o.mu.Lock()
				o.signals[sig.Name()] = sig
				o.mu.Unlock()
			},
			wantErr:   true,
			errSubstr: "already exists",
		},
		{
			name: "Hydrate Unknown Position (Skip)",
			pair: repository.StrategyPair{ExchangeName: "binance", InstrumentSymbol: "BTC/USDT", Type: "dummy"},
			setup: func(o *Orchestrator, repo *repository.Container, mPf *MockPortfolio, mExec *MockExecutionService) {
				repo.MarketData.(*MockMarketDataRepo).On("GetMarketDataTicks", mock.Anything, mock.Anything, "binance", "BTC/USDT", mock.Anything).
					Return([]repository.MarketDataTick{}, nil).Once()
				repo.Risks.(*MockRiskRepo).On("GetRiskPair", mock.Anything, mock.Anything, "binance", "BTC/USDT").
					Return(repository.RiskPair{}, nil).Once()
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").
					Return(repository.PositionData{UnknownOrigin: true, EntryPrice: 50000}, nil).Once()
			},
		},
		{
			name: "Pending Disabled Status Sets Terminate",
			pair: repository.StrategyPair{ExchangeName: "binance", InstrumentSymbol: "BTC/USDT", Type: "dummy", Status: repository.StrategyPendingDisabled},
			setup: func(o *Orchestrator, repo *repository.Container, mPf *MockPortfolio, mExec *MockExecutionService) {
				repo.MarketData.(*MockMarketDataRepo).On("GetMarketDataTicks", mock.Anything, mock.Anything, "binance", "BTC/USDT", mock.Anything).
					Return([]repository.MarketDataTick{}, nil).Once()
				repo.Risks.(*MockRiskRepo).On("GetRiskPair", mock.Anything, mock.Anything, "binance", "BTC/USDT").
					Return(repository.RiskPair{}, nil).Once()
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, pgx.ErrNoRows).Once()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch, repo, mPf, _, mExec := setupOrchestratorTest(t)
			tt.setup(orch, repo, mPf, mExec)
			sig, err := orch.initSignalHandler(context.Background(), tt.pair, fmt.Sprintf("SignalGenerator-%s-%s", tt.pair.ExchangeName, tt.pair.InstrumentSymbol))
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, sig)
				orch.mu.Lock()
				assert.Contains(t, orch.signals, sig.Name())
				orch.mu.Unlock()

				if tt.pair.Status == repository.StrategyPendingDisabled {
					assert.True(t, sig.IsPendingTerminate())
				}
			}
		})
	}
}

func TestOrchestrator_ProcessSignal(t *testing.T) {
	tests := []struct {
		name  string
		setup func(mExec *MockExecutionService, sig *signal_generator.SignalGenerator, mPf *MockPortfolio, mStrats *MockStrategiesRepo)
	}{
		{
			name: "Ticker Fetch Error",
			setup: func(mExec *MockExecutionService, sig *signal_generator.SignalGenerator, mPf *MockPortfolio, mStrats *MockStrategiesRepo) {
				mExec.On("GetTicker", mock.Anything, "binance", "BTC/USDT").Return(repository.MarketDataTick{}, errors.New("rpc error")).Once()
			},
		},
		{
			name: "Termination Logic - No Position",
			setup: func(mExec *MockExecutionService, sig *signal_generator.SignalGenerator, mPf *MockPortfolio, mStrats *MockStrategiesRepo) {
				mExec.On("GetTicker", mock.Anything, "binance", "BTC/USDT").Return(repository.MarketDataTick{Price: 50000}, nil).Once()
				sig.SetPendingTerminate(true)
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, pgx.ErrNoRows).Twice()
				mStrats.On("ApplyStrategyDisable", mock.Anything, mock.Anything, "binance", "BTC/USDT").Return(nil).Once()
			},
		},
		{
			name: "Termination Logic - GetPosition Error",
			setup: func(mExec *MockExecutionService, sig *signal_generator.SignalGenerator, mPf *MockPortfolio, mStrats *MockStrategiesRepo) {
				mExec.On("GetTicker", mock.Anything, "binance", "BTC/USDT").Return(repository.MarketDataTick{Price: 50000}, nil).Once()
				sig.SetPendingTerminate(true)
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, pgx.ErrNoRows).Once()
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, errors.New("db error")).Once()
			},
		},
		{
			name: "Termination Logic - ApplyDisable Error",
			setup: func(mExec *MockExecutionService, sig *signal_generator.SignalGenerator, mPf *MockPortfolio, mStrats *MockStrategiesRepo) {
				mExec.On("GetTicker", mock.Anything, "binance", "BTC/USDT").Return(repository.MarketDataTick{Price: 50000}, nil).Once()
				sig.SetPendingTerminate(true)
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, pgx.ErrNoRows).Twice()
				mStrats.On("ApplyStrategyDisable", mock.Anything, mock.Anything, "binance", "BTC/USDT").Return(errors.New("db error")).Once()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch, repo, mPf, _, mExec := setupOrchestratorTest(t)
			mStrats := repo.Strategies.(*MockStrategiesRepo)
			sig, _ := signal_generator.NewSignalGenerator(orch.logger, repository.RiskPair{}, repository.StrategyPair{ExchangeName: "binance", InstrumentSymbol: "BTC/USDT", Type: "dummy"}, "test")
			tt.setup(mExec, sig, mPf, mStrats)
			orch.mu.Lock()
			orch.signals[sig.Name()] = sig
			orch.mu.Unlock()
			orch.processSignal(context.Background(), sig)
		})
	}
}

func TestOrchestrator_SignalSearchingBuyEntry(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(mPf *MockPortfolio, mExec *MockExecutionService, mRecon *MockReconciler)
		expectedSignal strategy.StrategySignal
	}{
		{
			name: "Sync State - Quantity Match",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mRecon *MockReconciler) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{Quantity: 1.0, EntryPrice: 100, HighestPrice: 100}, nil).Once()
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").Return([]repository.BalanceData{{AssetSymbol: "BTC", Total: 1.0}}, nil).Once()
			},
			expectedSignal: strategy.SignalTrackingSellExit,
		},
		{
			name: "Exit early on Position NoRows",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mRecon *MockReconciler) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, pgx.ErrNoRows).Once()
			},
			expectedSignal: strategy.SignalSearchingBuyEntry,
		},
		{
			name: "Exit early on Position DB error",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mRecon *MockReconciler) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, errors.New("db error")).Once()
			},
			expectedSignal: strategy.SignalSearchingBuyEntry,
		},
		{
			name: "Exit early if UnknownOrigin is already true",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mRecon *MockReconciler) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{UnknownOrigin: true}, nil).Once()
			},
			expectedSignal: strategy.SignalSearchingBuyEntry,
		},
		{
			name: "Exit on GetBalance error",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mRecon *MockReconciler) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{Quantity: 1.0, EntryPrice: 100}, nil).Once()
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").Return(nil, errors.New("rpc error")).Once()
			},
			expectedSignal: strategy.SignalSearchingBuyEntry,
		},
		{
			name: "Exit on empty balance slice",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mRecon *MockReconciler) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{Quantity: 1.0, EntryPrice: 100}, nil).Once()
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").Return([]repository.BalanceData{}, nil).Once()
			},
			expectedSignal: strategy.SignalSearchingBuyEntry,
		},
		{
			name: "Exit if reconciliation fails (SyncPositions)",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mRecon *MockReconciler) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{Quantity: 1.0, EntryPrice: 100}, nil).Once()
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").Return([]repository.BalanceData{{AssetSymbol: "BTC", Total: 0.5}}, nil).Once()
				mRecon.On("SyncPositions", mock.Anything, "binance", "BTC/USDT").Return(errors.New("sync error")).Once()
				mRecon.On("SyncTradeHistory", mock.Anything, "binance", "BTC/USDT", 15*time.Minute).Return(nil).Once()
			},
			expectedSignal: strategy.SignalSearchingBuyEntry,
		},
		{
			name: "Exit if reconciliation fails (SyncTradeHistory)",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mRecon *MockReconciler) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{Quantity: 1.0, EntryPrice: 100}, nil).Once()
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").Return([]repository.BalanceData{{AssetSymbol: "BTC", Total: 0.5}}, nil).Once()
				mRecon.On("SyncPositions", mock.Anything, "binance", "BTC/USDT").Return(nil).Once()
				mRecon.On("SyncTradeHistory", mock.Anything, "binance", "BTC/USDT", 15*time.Minute).Return(errors.New("sync error")).Once()
			},
			expectedSignal: strategy.SignalSearchingBuyEntry,
		},
		{
			name: "Mismatch Triggers Reconciliation",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mRecon *MockReconciler) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{Quantity: 1.0, EntryPrice: 100}, nil).Twice()
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").Return([]repository.BalanceData{{AssetSymbol: "BTC", Total: 0.5}}, nil).Twice()
				mRecon.On("SyncPositions", mock.Anything, "binance", "BTC/USDT").Return(nil).Once()
				mRecon.On("SyncTradeHistory", mock.Anything, "binance", "BTC/USDT", 15*time.Minute).Return(nil).Once()
				mPf.On("UpdatePosition", mock.Anything, "binance", "BTC/USDT", mock.MatchedBy(func(p repository.PositionData) bool { return p.UnknownOrigin })).Return(nil).Once()
			},
			expectedSignal: strategy.SignalSearchingBuyEntry,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch, _, mPf, mRecon, mExec := setupOrchestratorTest(t)
			sig := initTestSignalGenerator(t, orch, strategy.SignalSearchingBuyEntry)
			tt.setup(mPf, mExec, mRecon)
			orch.signalSearchingBuyEntry(context.Background(), orch.logger, sig)
			s, _ := sig.GetSignal(100, time.Now().Unix())
			assert.Equal(t, tt.expectedSignal, s)
		})
	}
}

func TestOrchestrator_SignalBuy(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(mPf *MockPortfolio, mExec *MockExecutionService, mBal *MockBalancesRepo)
		expectedSignal strategy.StrategySignal
	}{
		{
			name: "Skip if position exists",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mBal *MockBalancesRepo) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{Active: true}, nil).Once()
			},
			expectedSignal: strategy.SignalWaitingBuyFill,
		},
		{
			name: "Handle UnknownOrigin position - Reset strategy",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mBal *MockBalancesRepo) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").
					Return(repository.PositionData{UnknownOrigin: true, Active: true}, nil).Once()
			},
			expectedSignal: strategy.SignalBuy,
		},
		{
			name: "Query position error - Trigger retry",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mBal *MockBalancesRepo) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").
					Return(repository.PositionData{}, errors.New("db timeout")).Once()
			},
			expectedSignal: strategy.SignalBuy,
		},
		{
			name: "Risk manager pre-check rejection",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mBal *MockBalancesRepo) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, pgx.ErrNoRows).Once()
				// MaxOpenPositions is 3 in setupOrchestratorTest
				mPf.On("GetActivePositionsCount").Return(5).Once()
				mBal.On("GetBalance", mock.Anything, mock.Anything, "binance", "USDT").
					Return(repository.BalanceData{Total: 1000.0}, nil).Twice()
			},
			expectedSignal: strategy.SignalBuy,
		},
		{
			name: "Skip if pending buy order exists on exchange",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mBal *MockBalancesRepo) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, pgx.ErrNoRows).Once()
				mPf.On("GetActivePositionsCount").Return(0).Once()
				mBal.On("GetBalance", mock.Anything, mock.Anything, "binance", "USDT").
					Return(repository.BalanceData{Total: 1000.0}, nil).Twice()
				mExec.On("GetOpenOrders", mock.Anything, "binance", "BTC/USDT", 10).
					Return([]repository.OrderData{{Side: repository.OrderSideBuy}}, nil).Once()
			},
			expectedSignal: strategy.SignalWaitingBuyFill,
		},
		{
			name: "Skip if balance already exists on exchange",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mBal *MockBalancesRepo) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, pgx.ErrNoRows).Once()
				mPf.On("GetActivePositionsCount").Return(0).Once()
				mBal.On("GetBalance", mock.Anything, mock.Anything, "binance", "USDT").
					Return(repository.BalanceData{Total: 1000.0}, nil).Twice()
				mExec.On("GetOpenOrders", mock.Anything, "binance", "BTC/USDT", 10).Return([]repository.OrderData{}, nil).Once()
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").
					Return([]repository.BalanceData{{Total: 1.0}}, nil).Once()
			},
			expectedSignal: strategy.SignalWaitingBuyFill,
		},
		{
			name: "Risk manager final-check rejection (after exchange latency)",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mBal *MockBalancesRepo) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, pgx.ErrNoRows).Once()
				mPf.On("GetActivePositionsCount").Return(0).Once() // First check ok
				mBal.On("GetBalance", mock.Anything, mock.Anything, "binance", "USDT").
					Return(repository.BalanceData{Total: 1000.0}, nil).Twice()
				mExec.On("GetOpenOrders", mock.Anything, "binance", "BTC/USDT", 10).Return([]repository.OrderData{}, nil).Once()
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").Return([]repository.BalanceData{{Total: 0}}, nil).Once()
				mPf.On("GetActivePositionsCount").Return(5).Once() // Second check fails
			},
			expectedSignal: strategy.SignalBuy,
		},
		{
			name: "Market buy order API failure",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mBal *MockBalancesRepo) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, pgx.ErrNoRows).Once()
				mPf.On("GetActivePositionsCount").Return(0).Twice()
				mBal.On("GetBalance", mock.Anything, mock.Anything, "binance", "USDT").
					Return(repository.BalanceData{Total: 1000.0}, nil).Twice()
				mExec.On("GetOpenOrders", mock.Anything, "binance", "BTC/USDT", 10).Return([]repository.OrderData{}, nil).Once()
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").Return([]repository.BalanceData{{Total: 0}}, nil).Once()
				mExec.On("CreateOrder", mock.Anything, "binance", "BTC/USDT", repository.OrderSideBuy, repository.OrderTypeMarket, mock.Anything, float64(0)).
					Return(repository.OrderData{}, errors.New("exchange offline")).Once()
			},
			expectedSignal: strategy.SignalWaitingBuyFill,
		},
		{
			name: "Execute market buy - Partial fill / Open status",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mBal *MockBalancesRepo) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, pgx.ErrNoRows).Once()
				mPf.On("GetActivePositionsCount").Return(0).Twice()
				mBal.On("GetBalance", mock.Anything, mock.Anything, "binance", "USDT").
					Return(repository.BalanceData{Total: 1000.0}, nil).Twice()
				mExec.On("GetOpenOrders", mock.Anything, "binance", "BTC/USDT", 10).Return([]repository.OrderData{}, nil).Once()
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").Return([]repository.BalanceData{{Total: 0}}, nil).Once()
				mExec.On("CreateOrder", mock.Anything, "binance", "BTC/USDT", repository.OrderSideBuy, repository.OrderTypeMarket, mock.Anything, float64(0)).
					Return(repository.OrderData{Status: repository.OrderStatusOpen}, nil).Once()
				// CreatePosition should NOT be called
			},
			expectedSignal: strategy.SignalWaitingBuyFill,
		},
		{
			name: "Execute market buy",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mBal *MockBalancesRepo) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, pgx.ErrNoRows).Once()
				mPf.On("GetActivePositionsCount").Return(0).Twice()
				mBal.On("GetBalance", mock.Anything, mock.Anything, "binance", "USDT").
					Return(repository.BalanceData{Total: 1000.0}, nil).Twice()
				mExec.On("GetOpenOrders", mock.Anything, "binance", "BTC/USDT", 10).Return([]repository.OrderData{}, nil).Once()
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").Return([]repository.BalanceData{{Total: 0}}, nil).Once()
				mExec.On("CreateOrder", mock.Anything, "binance", "BTC/USDT", repository.OrderSideBuy, repository.OrderTypeMarket, mock.Anything, float64(0)).
					Return(repository.OrderData{Status: repository.OrderStatusClosed, Filled: 0.001, AveragePrice: sql.NullFloat64{Float64: 102, Valid: true}}, nil).Once()
				mPf.On("CreatePosition", mock.Anything, "binance", "BTC/USDT", 0.001, 102.0, mock.Anything).Return(nil).Once()
			},
			expectedSignal: strategy.SignalWaitingBuyFill,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch, repo, mPf, _, mExec := setupOrchestratorTest(t)
			sig := initTestSignalGenerator(t, orch, strategy.SignalBuy)
			mBal := repo.Balances.(*MockBalancesRepo)

			tt.setup(mPf, mExec, mBal)
			orch.signalBuy(context.Background(), orch.logger, sig, 102)
			s, _ := sig.GetSignal(102, time.Now().Unix())
			assert.Equal(t, tt.expectedSignal, s)
		})
	}
}

func TestOrchestrator_SignalWaitingBuyFill(t *testing.T) {
	tests := []struct {
		name           string
		price          float64
		setup          func(mPf *MockPortfolio, mExec *MockExecutionService, mRecon *MockReconciler, sig *signal_generator.SignalGenerator)
		expectedSignal strategy.StrategySignal
	}{
		{
			name:  "Happy flow - Position active found",
			price: 102,
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mRecon *MockReconciler, sig *signal_generator.SignalGenerator) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{EntryPrice: 102, HighestPrice: 102, UnknownOrigin: false}, nil).Once()
			},
			expectedSignal: strategy.SignalTrackingSellExit,
		},
		{
			name:  "Happy flow - Update highest price",
			price: 105,
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mRecon *MockReconciler, sig *signal_generator.SignalGenerator) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{EntryPrice: 102, HighestPrice: 102, UnknownOrigin: false}, nil).Once()
				mPf.On("UpdatePosition", mock.Anything, "binance", "BTC/USDT", repository.PositionData{EntryPrice: 102, HighestPrice: 105}).Return(nil).Once()
			},
			expectedSignal: strategy.SignalTrackingSellExit,
		},
		{
			name: "DB Error during position query",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mRecon *MockReconciler, sig *signal_generator.SignalGenerator) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, errors.New("db error")).Once()
			},
			expectedSignal: strategy.SignalWaitingBuyFill,
		},
		{
			name: "Wait if buy order exists",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mRecon *MockReconciler, sig *signal_generator.SignalGenerator) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, pgx.ErrNoRows).Once()
				mExec.On("GetOpenOrders", mock.Anything, "binance", "BTC/USDT", 10).Return([]repository.OrderData{{Side: repository.OrderSideBuy}}, nil).Once()
			},
			expectedSignal: strategy.SignalWaitingBuyFill,
		},
		{
			name: "Reset if no balance and no buy order",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mRecon *MockReconciler, sig *signal_generator.SignalGenerator) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, pgx.ErrNoRows).Once()
				mExec.On("GetOpenOrders", mock.Anything, "binance", "BTC/USDT", 10).Return([]repository.OrderData{}, nil).Once()
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").Return([]repository.BalanceData{{Total: 0}}, nil).Once()
			},
			expectedSignal: strategy.SignalBuy,
		},
		{
			name: "Trigger sync if balance exists but no local position",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mRecon *MockReconciler, sig *signal_generator.SignalGenerator) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, pgx.ErrNoRows).Once()
				mExec.On("GetOpenOrders", mock.Anything, "binance", "BTC/USDT", 10).Return([]repository.OrderData{}, nil).Once()
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").Return([]repository.BalanceData{{Total: 1.0}}, nil).Once()
				mRecon.On("SyncOrders", mock.Anything, "binance", "BTC/USDT").Return(nil).Once()
				mRecon.On("SyncPositions", mock.Anything, "binance", "BTC/USDT").Return(nil).Once()
			},
			expectedSignal: strategy.SignalWaitingBuyFill,
		},
		{
			name: "Try to promote unknown origin via trade history",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mRecon *MockReconciler, sig *signal_generator.SignalGenerator) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{UnknownOrigin: true}, nil).Once()
				mRecon.On("SyncTradeHistory", mock.Anything, "binance", "BTC/USDT", 15*time.Minute).Return(nil).Once()
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{UnknownOrigin: true}, nil).Once()
				// position stuck, reset strategy
			},
			expectedSignal: strategy.SignalBuy,
		},
		{
			name: "DB error during position re-query after history sync",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, mRecon *MockReconciler, sig *signal_generator.SignalGenerator) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{UnknownOrigin: true}, nil).Once()
				mRecon.On("SyncTradeHistory", mock.Anything, "binance", "BTC/USDT", 15*time.Minute).Return(nil).Once()
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, errors.New("db error")).Once()
			},
			expectedSignal: strategy.SignalWaitingBuyFill,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch, _, mPf, mRecon, mExec := setupOrchestratorTest(t)
			sig := initTestSignalGenerator(t, orch, strategy.SignalBuy)
			tt.setup(mPf, mExec, mRecon, sig)
			orch.signalWaitingBuyFill(context.Background(), orch.logger, sig, tt.price)
			s, _ := sig.GetSignal(102, time.Now().Unix())
			assert.Equal(t, tt.expectedSignal, s)
		})
	}
}

func TestOrchestrator_SignalTrackingSellExit(t *testing.T) {
	tests := []struct {
		name           string
		price          float64
		setup          func(mPf *MockPortfolio, mExec *MockExecutionService)
		expectedSignal strategy.StrategySignal
	}{
		{
			name:  "Happy flow - Position already with SL active",
			price: 102,
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{Active: true, HighestPrice: 102, StopLossActive: true}, nil).Once()
			},
			expectedSignal: strategy.SignalTrackingSellExit,
		},
		{
			name:  "Reset if position missing",
			price: 102,
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, pgx.ErrNoRows).Once()
			},
			expectedSignal: strategy.SignalSearchingBuyEntry,
		},
		{
			name:  "Reset if unknown origin",
			price: 102,
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{UnknownOrigin: true}, nil).Once()
			},
			expectedSignal: strategy.SignalSearchingBuyEntry,
		},
		{
			name:  "Activate if stop loss already exists",
			price: 102,
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{StopLossActive: false, EntryPrice: 102}, nil).Once()
				mExec.On("GetOpenOrders", mock.Anything, "binance", "BTC/USDT", 10).Return([]repository.OrderData{{Side: repository.OrderSideSell}}, nil).Once()
				mPf.On("UpdatePosition", mock.Anything, "binance", "BTC/USDT", mock.MatchedBy(func(p repository.PositionData) bool { return p.StopLossActive })).Return(nil).Once()
			},
			expectedSignal: strategy.SignalTrackingSellExit,
		},
		{
			name: "Place stop loss and activate",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{StopLossActive: false, EntryPrice: 102, Quantity: 1.0}, nil).Once()
				mExec.On("GetOpenOrders", mock.Anything, "binance", "BTC/USDT", 10).Return([]repository.OrderData{}, nil).Once()
				mExec.On("CreateStopOrder", mock.Anything, "binance", "BTC/USDT", repository.OrderSideSell, 1.0, mock.Anything, float64(0)).Return(repository.OrderData{}, nil).Once()
				mPf.On("UpdatePosition", mock.Anything, "binance", "BTC/USDT", mock.MatchedBy(func(p repository.PositionData) bool { return p.StopLossActive })).Return(nil).Once()
			},
			expectedSignal: strategy.SignalTrackingSellExit,
		},
		{
			name: "Failed to place stop loss order",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{StopLossActive: false, EntryPrice: 102, Quantity: 1.0}, nil).Once()
				mExec.On("GetOpenOrders", mock.Anything, "binance", "BTC/USDT", 10).Return([]repository.OrderData{}, nil).Once()
				mExec.On("CreateStopOrder", mock.Anything, "binance", "BTC/USDT", repository.OrderSideSell, 1.0, mock.Anything, float64(0)).Return(repository.OrderData{}, errors.New("rpc error")).Once()
			},
			expectedSignal: strategy.SignalTrackingSellExit,
		},
		{
			name:  "Position with stop loss active and higher price - Update HighestPrice",
			price: 105,
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{HighestPrice: 102, StopLossActive: true}, nil).Once()
				mPf.On("UpdatePosition", mock.Anything, "binance", "BTC/USDT", repository.PositionData{HighestPrice: 105, StopLossActive: true}).Return(nil).Once()
			},
			expectedSignal: strategy.SignalTrackingSellExit,
		},
		{
			name:  "Position with stop loss active and higher price - error on update",
			price: 105,
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{HighestPrice: 102, StopLossActive: true}, nil).Once()
				mPf.On("UpdatePosition", mock.Anything, "binance", "BTC/USDT", repository.PositionData{HighestPrice: 105, StopLossActive: true}).Return(errors.New("update error")).Once()
			},
			expectedSignal: strategy.SignalTrackingSellExit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch, _, mPf, _, mExec := setupOrchestratorTest(t)
			sig := initTestSignalGenerator(t, orch, strategy.SignalTrackingSellExit)
			tt.setup(mPf, mExec)
			orch.signalTrackingSellExit(context.Background(), orch.logger, sig, tt.price)

			s, _ := sig.GetSignal(100.0, time.Now().Unix())
			assert.Equal(t, tt.expectedSignal, s)
		})
	}
}

func TestOrchestrator_SignalSell(t *testing.T) {
	tests := []struct {
		name           string
		price          float64
		setup          func(mPf *MockPortfolio, mExec *MockExecutionService, sig *signal_generator.SignalGenerator)
		expectedSignal strategy.StrategySignal
	}{
		{
			name: "Reset if position missing",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, sig *signal_generator.SignalGenerator) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, pgx.ErrNoRows).Once()
			},
			expectedSignal: strategy.SignalSearchingBuyEntry,
		},
		{
			name:  "DB Error during position query",
			price: 94,
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, sig *signal_generator.SignalGenerator) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, errors.New("db error")).Once()
			},
			expectedSignal: strategy.SignalSell,
		},
		{
			name: "Reset if position from unknown origin",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, sig *signal_generator.SignalGenerator) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{UnknownOrigin: true}, nil).Once()
			},
			expectedSignal: strategy.SignalSearchingBuyEntry,
		},
		{
			name: "Delete position if balance zero",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, sig *signal_generator.SignalGenerator) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{Quantity: 1.0}, nil).Once()
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").Return([]repository.BalanceData{{Total: 0}}, nil).Once()
				mPf.On("DeletePosition", mock.Anything, "binance", "BTC/USDT").Return(nil).Once()
			},
			expectedSignal: strategy.SignalSearchingBuyEntry,
		},
		{
			name: "Skip if market sell in flight",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, sig *signal_generator.SignalGenerator) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{Quantity: 1.0}, nil).Once()
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").Return([]repository.BalanceData{{Total: 1.0}}, nil).Once()
				mExec.On("GetOpenOrders", mock.Anything, "binance", "BTC/USDT", 10).Return([]repository.OrderData{{Side: repository.OrderSideSell, OrderType: repository.OrderTypeMarket}}, nil).Once()
			},
			expectedSignal: strategy.SignalWaitingSellFill,
		},
		{
			name:  "Wait if stop loss exists and price below entry",
			price: 98,
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, sig *signal_generator.SignalGenerator) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{Quantity: 1.0, EntryPrice: 100}, nil).Once()
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").Return([]repository.BalanceData{{Total: 1.0}}, nil).Once()
				mExec.On("GetOpenOrders", mock.Anything, "binance", "BTC/USDT", 10).Return([]repository.OrderData{{Side: repository.OrderSideSell, OrderType: repository.OrderTypeStopMarket}}, nil).Once()
			},
			expectedSignal: strategy.SignalWaitingSellFill,
		},
		{
			name:  "Cancel stop loss and market sell on profit take",
			price: 51000,
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, sig *signal_generator.SignalGenerator) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{Quantity: 1.0, EntryPrice: 50000}, nil).Once()
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").Return([]repository.BalanceData{{Total: 1.0}}, nil).Once()
				mExec.On("GetOpenOrders", mock.Anything, "binance", "BTC/USDT", 10).Return([]repository.OrderData{{Side: repository.OrderSideSell, OrderType: repository.OrderTypeStopMarket, ExchangeOrderID: "sl-1"}}, nil).Once()
				mExec.On("CancelOrder", mock.Anything, "binance", "BTC/USDT", "sl-1").Return(nil).Once()
				mExec.On("CreateOrder", mock.Anything, "binance", "BTC/USDT", repository.OrderSideSell, repository.OrderTypeMarket, 1.0, float64(0)).Return(repository.OrderData{Status: repository.OrderStatusClosed}, nil).Once()
				mPf.On("DeletePosition", mock.Anything, "binance", "BTC/USDT").Return(nil).Once()
			},
			expectedSignal: strategy.SignalSearchingBuyEntry,
		},
		{
			name: "Place market sell happy flow",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, sig *signal_generator.SignalGenerator) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{Quantity: 1.0, EntryPrice: 50000}, nil).Once()
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").Return([]repository.BalanceData{{Total: 1.0}}, nil).Once()
				mExec.On("GetOpenOrders", mock.Anything, "binance", "BTC/USDT", 10).Return([]repository.OrderData{}, nil).Once()
				mExec.On("CreateOrder", mock.Anything, "binance", "BTC/USDT", repository.OrderSideSell, repository.OrderTypeMarket, 1.0, float64(0)).Return(repository.OrderData{Status: repository.OrderStatusClosed}, nil).Once()
				mPf.On("DeletePosition", mock.Anything, "binance", "BTC/USDT").Return(nil).Once()
			},
			expectedSignal: strategy.SignalSearchingBuyEntry,
		},
		{
			name:  "Market sell order remains open",
			price: 94,
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, sig *signal_generator.SignalGenerator) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{Quantity: 1.0, EntryPrice: 100}, nil).Once()
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").Return([]repository.BalanceData{{Total: 1.0}}, nil).Once()
				mExec.On("GetOpenOrders", mock.Anything, "binance", "BTC/USDT", 10).Return([]repository.OrderData{}, nil).Once()
				mExec.On("CreateOrder", mock.Anything, "binance", "BTC/USDT", repository.OrderSideSell, repository.OrderTypeMarket, 1.0, float64(0)).
					Return(repository.OrderData{Status: repository.OrderStatusOpen}, nil).Once()
			},
			expectedSignal: strategy.SignalWaitingSellFill,
		},
		{
			name:  "Market sell API error - Trigger retry",
			price: 94,
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService, sig *signal_generator.SignalGenerator) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{Quantity: 1.0, EntryPrice: 100}, nil).Once()
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").Return([]repository.BalanceData{{Total: 1.0}}, nil).Once()
				mExec.On("GetOpenOrders", mock.Anything, "binance", "BTC/USDT", 10).Return([]repository.OrderData{}, nil).Once()
				mExec.On("CreateOrder", mock.Anything, "binance", "BTC/USDT", repository.OrderSideSell, repository.OrderTypeMarket, 1.0, float64(0)).Return(repository.OrderData{}, errors.New("rpc error")).Once()
			},
			expectedSignal: strategy.SignalSell,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch, _, mPf, _, mExec := setupOrchestratorTest(t)
			sig := initTestSignalGenerator(t, orch, strategy.SignalSell)
			tt.setup(mPf, mExec, sig)
			orch.signalSell(context.Background(), orch.logger, sig, tt.price)

			s, _ := sig.GetSignal(94.0, time.Now().Unix())
			assert.Equal(t, tt.expectedSignal, s)
		})
	}
}

func TestOrchestrator_SignalWaitingSellFill(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(mPf *MockPortfolio, mExec *MockExecutionService)
		expectedSignal strategy.StrategySignal
	}{
		{
			name: "Filled - Position gone locally",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, pgx.ErrNoRows).Once()
			},
			expectedSignal: strategy.SignalSearchingBuyEntry,
		},
		{
			name: "Filled - Balance zero",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{Quantity: 1.0}, nil).Once()
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").Return([]repository.BalanceData{{Total: 0}}, nil).Once()
				mPf.On("DeletePosition", mock.Anything, "binance", "BTC/USDT").Return(nil).Once()
			},
			expectedSignal: strategy.SignalSearchingBuyEntry,
		},
		{
			name: "Processing - Order exists",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{Quantity: 1.0}, nil).Once()
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").Return([]repository.BalanceData{{Total: 1.0}}, nil).Once()
				mExec.On("GetOpenOrders", mock.Anything, "binance", "BTC/USDT", 10).Return([]repository.OrderData{{Side: repository.OrderSideSell}}, nil).Once()
			},
			expectedSignal: strategy.SignalWaitingSellFill,
		},
		{
			name: "Recovery - No order found",
			setup: func(mPf *MockPortfolio, mExec *MockExecutionService) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{Quantity: 1.0}, nil).Once()
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").Return([]repository.BalanceData{{Total: 1.0}}, nil).Once()
				mExec.On("GetOpenOrders", mock.Anything, "binance", "BTC/USDT", 10).Return([]repository.OrderData{}, nil).Once()
			},
			expectedSignal: strategy.SignalSell,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch, _, mPf, _, mExec := setupOrchestratorTest(t)
			sig := initTestSignalGenerator(t, orch, strategy.SignalWaitingSellFill)
			tt.setup(mPf, mExec)
			orch.signalWaitingSellFill(context.Background(), orch.logger, sig)

			// Verification price must be consistent with history (SignalWaitingSellFill ends at 94.0)
			s, _ := sig.GetSignal(94.0, time.Now().Unix())
			assert.Equal(t, tt.expectedSignal, s)
		})
	}
}

func TestOrchestrator_SignalInvalid(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(mPf *MockPortfolio)
		expectedSignal strategy.StrategySignal
	}{
		{
			name: "Reset on ErrNoRows",
			setup: func(mPf *MockPortfolio) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, pgx.ErrNoRows).Once()
			},
			expectedSignal: strategy.SignalSearchingBuyEntry,
		},
		{
			name: "Log on DB Error",
			setup: func(mPf *MockPortfolio) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{}, errors.New("db error")).Once()
			},
			expectedSignal: strategy.SignalInvalid,
		},
		{
			name: "Reset on UnknownOrigin",
			setup: func(mPf *MockPortfolio) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{UnknownOrigin: true}, nil).Once()
			},
			expectedSignal: strategy.SignalSearchingBuyEntry,
		},
		{
			name: "Hydrate on Valid Position",
			setup: func(mPf *MockPortfolio) {
				mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{EntryPrice: 100, HighestPrice: 100}, nil).Once()
			},
			expectedSignal: strategy.SignalTrackingSellExit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch, _, mPf, _, _ := setupOrchestratorTest(t)
			sig := initTestSignalGenerator(t, orch, strategy.SignalInvalid)
			tt.setup(mPf)
			orch.signalInvalid(context.Background(), orch.logger, sig)

			s, _ := sig.GetSignal(100.0, time.Now().Unix())
			assert.Equal(t, tt.expectedSignal, s)
		})
	}
}
