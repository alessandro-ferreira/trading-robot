//go:build unit

package reconcil

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"trading/robot/go-bot/internal/database/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// --- Mocks ---

type MockExecutionService struct{ mock.Mock }

func (m *MockExecutionService) GetTicker(ctx context.Context, ex, sym string) (repository.MarketDataTick, error) {
	args := m.Called(ctx, ex, sym)
	return args.Get(0).(repository.MarketDataTick), args.Error(1)
}
func (m *MockExecutionService) GetBalance(ctx context.Context, ex, asset string) ([]repository.BalanceData, error) {
	args := m.Called(ctx, ex, asset)
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
func (m *MockExecutionService) GetOrders(ctx context.Context, ex, sym string, lim int, updb bool) ([]repository.OrderData, error) {
	args := m.Called(ctx, ex, sym, lim, updb)
	return args.Get(0).([]repository.OrderData), args.Error(1)
}
func (m *MockExecutionService) GetOpenOrders(ctx context.Context, ex, sym string, lim int, updb bool) ([]repository.OrderData, error) {
	args := m.Called(ctx, ex, sym, lim, updb)
	return args.Get(0).([]repository.OrderData), args.Error(1)
}

type MockOrdersRepo struct{ mock.Mock }

func (m *MockOrdersRepo) GetOrder(ctx context.Context, db repository.DBExecutor, ex, id string) (repository.OrderData, error) {
	args := m.Called(ctx, db, ex, id)
	return args.Get(0).(repository.OrderData), args.Error(1)
}
func (m *MockOrdersRepo) GetOrders(ctx context.Context, db repository.DBExecutor, ex, sym string, st, tp, sd []string, lim int) ([]repository.OrderData, error) {
	args := m.Called(ctx, db, ex, sym, st, tp, sd, lim)
	return args.Get(0).([]repository.OrderData), args.Error(1)
}
func (m *MockOrdersRepo) CreateOrder(ctx context.Context, db repository.DBExecutor, o repository.OrderData) (int64, error) {
	args := m.Called(ctx, db, o)
	return args.Get(0).(int64), args.Error(1)
}
func (m *MockOrdersRepo) UpdateOrder(ctx context.Context, db repository.DBExecutor, o repository.OrderData) (int64, error) {
	args := m.Called(ctx, db, o)
	return args.Get(0).(int64), args.Error(1)
}

type MockBalancesRepo struct{ mock.Mock }

func (m *MockBalancesRepo) GetBalance(ctx context.Context, db repository.DBExecutor, exchange, asset string) (repository.BalanceData, error) {
	return repository.BalanceData{}, nil
}
func (m *MockBalancesRepo) GetAllBalances(ctx context.Context, db repository.DBExecutor, exchange string) ([]repository.BalanceData, error) {
	args := m.Called(ctx, db, exchange)
	return args.Get(0).([]repository.BalanceData), args.Error(1)
}
func (m *MockBalancesRepo) UpsertBalance(ctx context.Context, db repository.DBExecutor, b repository.BalanceData) (int64, error) {
	args := m.Called(ctx, db, b)
	return args.Get(0).(int64), args.Error(1)
}

type MockPositionsRepo struct{ mock.Mock }

func (m *MockPositionsRepo) GetPositions(ctx context.Context, db repository.DBExecutor, ex, sym string) ([]repository.PositionData, error) {
	args := m.Called(ctx, db, ex, sym)
	return args.Get(0).([]repository.PositionData), args.Error(1)
}
func (m *MockPositionsRepo) DeletePosition(ctx context.Context, db repository.DBExecutor, ex, sym string) error {
	return m.Called(ctx, db, ex, sym).Error(0)
}
func (m *MockPositionsRepo) UpsertPosition(ctx context.Context, db repository.DBExecutor, p repository.PositionData) error {
	return m.Called(ctx, db, p).Error(0)
}
func (m *MockPositionsRepo) GetPosition(ctx context.Context, db repository.DBExecutor, ex, sym string) (repository.PositionData, error) {
	args := m.Called(ctx, db, ex, sym)
	return args.Get(0).(repository.PositionData), args.Error(1)
}

type MockStrategiesRepo struct{ mock.Mock }

func (m *MockStrategiesRepo) GetStrategyPairs(ctx context.Context, db repository.DBExecutor, statuses []string) ([]repository.StrategyPair, error) {
	args := m.Called(ctx, db, statuses)
	return args.Get(0).([]repository.StrategyPair), args.Error(1)
}

func (m *MockStrategiesRepo) UpsertEnabledStrategy(ctx context.Context, db repository.DBExecutor, exchangeName string, symbol string, strategyType string, label string, momentum repository.StrategyMomentum) error {
	args := m.Called(ctx, db, exchangeName, symbol, strategyType, label, momentum)
	return args.Error(0)
}

func (m *MockStrategiesRepo) RequestStrategyDisable(ctx context.Context, db repository.DBExecutor, exchange, symbol, strategyType string) error {
	args := m.Called(ctx, db, exchange, symbol, strategyType)
	return args.Error(0)
}

func (m *MockStrategiesRepo) ApplyStrategyDisable(ctx context.Context, db repository.DBExecutor, exchange, symbol string) error {
	args := m.Called(ctx, db, exchange, symbol)
	return args.Error(0)
}

type MockPortfolio struct {
	mock.Mock
}

func (m *MockPortfolio) LoadState(ctx context.Context) error { return m.Called(ctx).Error(0) }
func (m *MockPortfolio) RefreshState(ctx context.Context, ex, sym string) error {
	return m.Called(ctx, ex, sym).Error(0)
}
func (m *MockPortfolio) GetPositionsCount() int { return m.Called().Int(0) }
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

// --- Helpers ---

func setupReconciler(t *testing.T) (Reconciler, *MockExecutionService, *MockPortfolio, *repository.Container) {
	mExec := new(MockExecutionService)
	mOrders := new(MockOrdersRepo)
	mBalances := new(MockBalancesRepo)
	mPositions := new(MockPositionsRepo)
	mStrategies := new(MockStrategiesRepo)
	mPf := new(MockPortfolio)

	repo := &repository.Container{
		Orders:     mOrders,
		Balances:   mBalances,
		Positions:  mPositions,
		Strategies: mStrategies,
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := NewReconciler(logger, nil, repo, mExec, mPf)
	return r, mExec, mPf, repo
}

func toNullFloat64(f float64) sql.NullFloat64 {
	return sql.NullFloat64{Float64: f, Valid: f > 0}
}

func toNullTime(t time.Time) sql.NullTime {
	return sql.NullTime{Time: t, Valid: !t.IsZero()}
}

func TestReconciler_SyncOrders(t *testing.T) {
	testCases := []struct {
		name     string
		exchange string
		symbol   string
		setup    func(mExec *MockExecutionService, mPf *MockPortfolio, mOrders *MockOrdersRepo)
		wantErr  bool
	}{
		{
			name:     "Investigation of Buy vanished order - creates position in portfolio",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mExec *MockExecutionService, mPf *MockPortfolio, mOrders *MockOrdersRepo) {
				mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "BTC/USDT",
					[]string{"new", "open"}, []string{"limit", "market"}, []string{}, 100).Return([]repository.OrderData{
					{ID: 10, ExchangeOrderID: sql.NullString{String: "closed-1", Valid: true}, InstrumentSymbol: "BTC/USDT", Status: "open"},
				}, nil)
				mExec.On("GetOrder", mock.Anything, "binance", "BTC/USDT", "closed-1").Return(repository.OrderData{
					ExchangeOrderID: sql.NullString{String: "closed-1", Valid: true}, InstrumentSymbol: "BTC/USDT", Side: repository.OrderSideBuy, Status: repository.OrderStatusClosed,
					Filled: 1.0, Price: toNullFloat64(50000),
				}, nil)
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").Return([]repository.BalanceData{{Total: 0.999}}, nil)
				mPf.On("CreatePosition", mock.Anything, "binance", "BTC/USDT", 1.0, 50000.0, int64(10)).Return(nil)
				mPf.On("UpdatePosition", mock.Anything, "binance", "BTC/USDT", repository.PositionData{Quantity: 0.999}).Return(nil)
			},
		},
		{
			name:     "Investigation of Buy vanished order - error getting balance from exchange",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mExec *MockExecutionService, mPf *MockPortfolio, mOrders *MockOrdersRepo) {
				mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "BTC/USDT",
					[]string{"new", "open"}, []string{"limit", "market"}, []string{}, 100).Return([]repository.OrderData{
					{ID: 10, ExchangeOrderID: sql.NullString{String: "closed-1", Valid: true}, InstrumentSymbol: "BTC/USDT", Status: "open"},
				}, nil)
				mExec.On("GetOrder", mock.Anything, "binance", "BTC/USDT", "closed-1").Return(repository.OrderData{
					ExchangeOrderID: sql.NullString{String: "closed-1", Valid: true}, InstrumentSymbol: "BTC/USDT", Side: repository.OrderSideBuy, Status: repository.OrderStatusClosed,
					Filled: 1.0, Price: toNullFloat64(50000),
				}, nil)
				mExec.On("GetBalance", mock.Anything, "binance", "BTC").Return([]repository.BalanceData{}, errors.New("exchange error"))
				mPf.On("CreatePosition", mock.Anything, "binance", "BTC/USDT", 1.0, 50000.0, int64(10)).Return(nil)
			},
		},
		{
			name:     "Get Orders failure",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mExec *MockExecutionService, mPf *MockPortfolio, mOrders *MockOrdersRepo) {
				mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "BTC/USDT", mock.Anything, mock.Anything, mock.Anything, 100).Return([]repository.OrderData{}, errors.New("db disconnect"))
			},
			wantErr: true,
		},
		{
			name:     "Invalid exchange order ID",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mExec *MockExecutionService, mPf *MockPortfolio, mOrders *MockOrdersRepo) {
				mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "BTC/USDT", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]repository.OrderData{
					{ExchangeOrderID: sql.NullString{Valid: false}, InstrumentSymbol: "BTC/USDT"},
				}, nil)
			},
		},
		{
			name:     "Gateway failure - GetOrder fails",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mExec *MockExecutionService, mPf *MockPortfolio, mOrders *MockOrdersRepo) {
				mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "BTC/USDT", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]repository.OrderData{
					{ExchangeOrderID: sql.NullString{String: "fail-1", Valid: true}, InstrumentSymbol: "BTC/USDT"},
				}, nil)
				mExec.On("GetOrder", mock.Anything, "binance", "BTC/USDT", "fail-1").Return(repository.OrderData{
					ExchangeOrderID: sql.NullString{String: "fail-1", Valid: true}, InstrumentSymbol: "BTC/USDT",
				}, errors.New("rpc fail"))
			},
		},
		{
			name:     "Investigation of Sell vanished order",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mExec *MockExecutionService, mPf *MockPortfolio, mOrders *MockOrdersRepo) {
				mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "BTC/USDT",
					[]string{"new", "open"}, []string{"limit", "market"}, []string{}, 100).Return([]repository.OrderData{
					{ID: 10, ExchangeOrderID: sql.NullString{String: "closed-2", Valid: true}, InstrumentSymbol: "BTC/USDT", Status: "open"},
				}, nil)
				mExec.On("GetOrder", mock.Anything, "binance", "BTC/USDT", "closed-2").Return(repository.OrderData{
					ExchangeOrderID: sql.NullString{String: "closed-2", Valid: true}, InstrumentSymbol: "BTC/USDT", Status: repository.OrderStatusClosed,
					Filled: 1.0, Price: toNullFloat64(50000),
				}, nil)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r, mExec, mPf, repo := setupReconciler(t)
			mOrders := repo.Orders.(*MockOrdersRepo)
			tc.setup(mExec, mPf, mOrders)

			err := r.SyncOrders(context.Background(), tc.exchange, tc.symbol)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestReconciler_SyncStopOrders(t *testing.T) {
	testCases := []struct {
		name     string
		exchange string
		symbol   string
		setup    func(mExec *MockExecutionService, mPf *MockPortfolio, mOrders *MockOrdersRepo, mBalances *MockBalancesRepo, mPositions *MockPositionsRepo)
		wantErr  bool
	}{
		{
			name:     "Stop order inactive and position active - resets stop loss active and adjusts quantity",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mExec *MockExecutionService, mPf *MockPortfolio, mOrders *MockOrdersRepo, mBalances *MockBalancesRepo, mPositions *MockPositionsRepo) {
				mPositions.On("GetPositions", mock.Anything, mock.Anything, "binance", "BTC/USDT").Return([]repository.PositionData{
					{InstrumentSymbol: "BTC/USDT", Quantity: 1.0, StopLossActive: true},
				}, nil)
				mBalances.On("GetAllBalances", mock.Anything, mock.Anything, "binance").Return([]repository.BalanceData{
					{ExchangeName: "binance", AssetSymbol: "BTC", Total: 0.8},
				}, nil)
				mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "BTC/USDT",
					[]string{"new", "open"}, []string{"stop_limit", "stop_market"}, []string{"sell"}, 100).Return([]repository.OrderData{
					{ID: 20, ExchangeOrderID: sql.NullString{String: "stop-1", Valid: true}, InstrumentSymbol: "BTC/USDT", Status: "open"},
				}, nil)
				mExec.On("GetOrder", mock.Anything, "binance", "BTC/USDT", "stop-1").Return(repository.OrderData{
					ExchangeOrderID: sql.NullString{String: "stop-1", Valid: true}, InstrumentSymbol: "BTC/USDT", Status: repository.OrderStatusExpired,
				}, nil)
				mPf.On("UpdatePosition", mock.Anything, "binance", "BTC/USDT", mock.MatchedBy(func(p repository.PositionData) bool {
					return p.Quantity == 0.8 && !p.StopLossActive
				})).Return(nil)
			},
		},
		{
			name:     "Stop order remains open - no position update",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mExec *MockExecutionService, mPf *MockPortfolio, mOrders *MockOrdersRepo, mBalances *MockBalancesRepo, mPositions *MockPositionsRepo) {
				mPositions.On("GetPositions", mock.Anything, mock.Anything, "binance", "BTC/USDT").Return([]repository.PositionData{
					{InstrumentSymbol: "BTC/USDT", Quantity: 1.0, StopLossActive: true},
				}, nil)
				mBalances.On("GetAllBalances", mock.Anything, mock.Anything, "binance").Return([]repository.BalanceData{
					{ExchangeName: "binance", AssetSymbol: "BTC", Total: 1.0},
				}, nil)
				mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "BTC/USDT",
					[]string{"new", "open"}, []string{"stop_limit", "stop_market"}, []string{"sell"}, 100).Return([]repository.OrderData{
					{ID: 20, ExchangeOrderID: sql.NullString{String: "stop-2", Valid: true}, InstrumentSymbol: "BTC/USDT", Status: "open"},
				}, nil)
				mExec.On("GetOrder", mock.Anything, "binance", "BTC/USDT", "stop-2").Return(repository.OrderData{
					ExchangeOrderID: sql.NullString{String: "stop-2", Valid: true}, InstrumentSymbol: "BTC/USDT", Status: repository.OrderStatusOpen,
				}, nil)
			},
		},
		{
			name:     "Active position has StopLossActive false - skipped",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mExec *MockExecutionService, mPf *MockPortfolio, mOrders *MockOrdersRepo, mBalances *MockBalancesRepo, mPositions *MockPositionsRepo) {
				mPositions.On("GetPositions", mock.Anything, mock.Anything, "binance", "BTC/USDT").Return([]repository.PositionData{
					{InstrumentSymbol: "BTC/USDT", Quantity: 1.0, StopLossActive: false},
				}, nil)
				mBalances.On("GetAllBalances", mock.Anything, mock.Anything, "binance").Return([]repository.BalanceData{
					{ExchangeName: "binance", AssetSymbol: "BTC", Total: 1.0},
				}, nil)
				mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "BTC/USDT",
					[]string{"new", "open"}, []string{"stop_limit", "stop_market"}, []string{"sell"}, 100).Return([]repository.OrderData{}, nil)
			},
		},
		{
			name:     "GetPositions failure returns error",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mExec *MockExecutionService, mPf *MockPortfolio, mOrders *MockOrdersRepo, mBalances *MockBalancesRepo, mPositions *MockPositionsRepo) {
				mPositions.On("GetPositions", mock.Anything, mock.Anything, "binance", "BTC/USDT").Return([]repository.PositionData{}, errors.New("db error"))
			},
			wantErr: true,
		},
		{
			name:     "GetAllBalances failure returns error",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mExec *MockExecutionService, mPf *MockPortfolio, mOrders *MockOrdersRepo, mBalances *MockBalancesRepo, mPositions *MockPositionsRepo) {
				mPositions.On("GetPositions", mock.Anything, mock.Anything, "binance", "BTC/USDT").Return([]repository.PositionData{}, nil)
				mBalances.On("GetAllBalances", mock.Anything, mock.Anything, "binance").Return([]repository.BalanceData{}, errors.New("balance error"))
			},
			wantErr: true,
		},
		{
			name:     "GetOrders failure returns error",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mExec *MockExecutionService, mPf *MockPortfolio, mOrders *MockOrdersRepo, mBalances *MockBalancesRepo, mPositions *MockPositionsRepo) {
				mPositions.On("GetPositions", mock.Anything, mock.Anything, "binance", "BTC/USDT").Return([]repository.PositionData{}, nil)
				mBalances.On("GetAllBalances", mock.Anything, mock.Anything, "binance").Return([]repository.BalanceData{}, nil)
				mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "BTC/USDT", mock.Anything, mock.Anything, mock.Anything, 100).Return([]repository.OrderData{}, errors.New("orders error"))
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r, mExec, mPf, repo := setupReconciler(t)
			mOrders := repo.Orders.(*MockOrdersRepo)
			mBalances := repo.Balances.(*MockBalancesRepo)
			mPositions := repo.Positions.(*MockPositionsRepo)
			tc.setup(mExec, mPf, mOrders, mBalances, mPositions)

			err := r.SyncStopOrders(context.Background(), tc.exchange, tc.symbol)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestReconciler_SyncPositions(t *testing.T) {
	testCases := []struct {
		name     string
		exchange string
		symbol   string
		setup    func(mBalances *MockBalancesRepo, mPositions *MockPositionsRepo, mStrategies *MockStrategiesRepo, mOrders *MockOrdersRepo, mPf *MockPortfolio)
		wantErr  bool
	}{
		{
			name:     "External liquidation - Wallet zero",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mBalances *MockBalancesRepo, mPositions *MockPositionsRepo, mStrategies *MockStrategiesRepo, mOrders *MockOrdersRepo, mPf *MockPortfolio) {
				mBalances.On("GetAllBalances", mock.Anything, mock.Anything, mock.Anything).Return([]repository.BalanceData{
					{ExchangeName: "binance", AssetSymbol: "BTC", Total: 0.0},
				}, nil)
				mPositions.On("GetPositions", mock.Anything, mock.Anything, "binance", "BTC/USDT").Return([]repository.PositionData{
					{InstrumentSymbol: "BTC/USDT", Quantity: 1.0},
				}, nil)
				mPf.On("DeletePosition", mock.Anything, "binance", "BTC/USDT").Return(nil)
				mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "BTC/USDT", mock.Anything, mock.Anything, mock.Anything, 1).Return([]repository.OrderData{}, nil)
			},
		},
		{
			name:     "Ghost balance adoption",
			exchange: "binance",
			symbol:   "LTC/USDT",
			setup: func(mBalances *MockBalancesRepo, mPositions *MockPositionsRepo, mStrategies *MockStrategiesRepo, mOrders *MockOrdersRepo, mPf *MockPortfolio) {
				mBalances.On("GetAllBalances", mock.Anything, mock.Anything, mock.Anything).Return([]repository.BalanceData{
					{ExchangeName: "binance", AssetSymbol: "LTC", Total: 10.0},
				}, nil)
				mPositions.On("GetPositions", mock.Anything, mock.Anything, "binance", "LTC/USDT").Return([]repository.PositionData{}, nil)
				mStrategies.On("GetStrategyPairs", mock.Anything, mock.Anything, mock.Anything).Return([]repository.StrategyPair{
					{ExchangeName: "binance", InstrumentSymbol: "LTC/USDT"},
				}, nil)
				mPf.On("CreatePosition", mock.Anything, "binance", "LTC/USDT", 10.0, 0.0, int64(0)).Return(nil)
				mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "LTC/USDT", mock.Anything, mock.Anything, mock.Anything, 1).Return([]repository.OrderData{}, nil)
			},
		},
		{
			name:     "Quantity drift adjustment",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mBalances *MockBalancesRepo, mPositions *MockPositionsRepo, mStrategies *MockStrategiesRepo, mOrders *MockOrdersRepo, mPf *MockPortfolio) {
				mBalances.On("GetAllBalances", mock.Anything, mock.Anything, mock.Anything).Return([]repository.BalanceData{
					{ExchangeName: "binance", AssetSymbol: "BTC", Total: 1.0005},
				}, nil)
				mPositions.On("GetPositions", mock.Anything, mock.Anything, "binance", "BTC/USDT").Return([]repository.PositionData{
					{InstrumentSymbol: "BTC/USDT", Quantity: 1.0},
				}, nil)
				mPf.On("UpdatePosition", mock.Anything, "binance", "BTC/USDT", mock.MatchedBy(func(p repository.PositionData) bool {
					return p.Quantity == 1.0005
				})).Return(nil)
				mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "BTC/USDT", mock.Anything, mock.Anything, mock.Anything, 1).Return([]repository.OrderData{}, nil)
			},
		},
		{
			name:     "Empty symbol - ghost adoption via enabled strategy pairs",
			exchange: "binance",
			symbol:   "",
			setup: func(mBalances *MockBalancesRepo, mPositions *MockPositionsRepo, mStrategies *MockStrategiesRepo, mOrders *MockOrdersRepo, mPf *MockPortfolio) {
				mBalances.On("GetAllBalances", mock.Anything, mock.Anything, mock.Anything).Return([]repository.BalanceData{
					{ExchangeName: "binance", AssetSymbol: "ETH", Total: 2.0},
				}, nil)
				mPositions.On("GetPositions", mock.Anything, mock.Anything, "binance", "").Return([]repository.PositionData{}, nil)
				mStrategies.On("GetStrategyPairs", mock.Anything, mock.Anything, []string{"enabled", "pending_disabled"}).Return([]repository.StrategyPair{
					{ExchangeName: "binance", InstrumentSymbol: "ETH/USDT"},
				}, nil)
				mPf.On("CreatePosition", mock.Anything, "binance", "ETH/USDT", 2.0, 0.0, int64(0)).Return(nil)
				mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "ETH/USDT", mock.Anything, mock.Anything, mock.Anything, 1).Return([]repository.OrderData{}, nil)
			},
		},
		{
			name:     "Ghost adoption skip - Open orders exist",
			exchange: "binance",
			symbol:   "ETH/USDT",
			setup: func(mBalances *MockBalancesRepo, mPositions *MockPositionsRepo, mStrategies *MockStrategiesRepo, mOrders *MockOrdersRepo, mPf *MockPortfolio) {
				mBalances.On("GetAllBalances", mock.Anything, mock.Anything, mock.Anything).Return([]repository.BalanceData{
					{ExchangeName: "binance", AssetSymbol: "ETH", Total: 2.0},
				}, nil)
				mPositions.On("GetPositions", mock.Anything, mock.Anything, "binance", "ETH/USDT").Return([]repository.PositionData{}, nil)
				// Mock an open order existing for this symbol
				mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "ETH/USDT", mock.Anything, mock.Anything, mock.Anything, 1).Return([]repository.OrderData{
					{ExchangeOrderID: sql.NullString{String: "pending-1", Valid: true}},
				}, nil)
			},
		},
		{
			name:     "DB Error",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mBalances *MockBalancesRepo, mPositions *MockPositionsRepo, mStrategies *MockStrategiesRepo, mOrders *MockOrdersRepo, mPf *MockPortfolio) {
				mBalances.On("GetAllBalances", mock.Anything, mock.Anything, mock.Anything).Return([]repository.BalanceData{}, errors.New("db down"))
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r, _, mPf, repo := setupReconciler(t)
			mBalances := repo.Balances.(*MockBalancesRepo)
			mPositions := repo.Positions.(*MockPositionsRepo)
			mStrategies := repo.Strategies.(*MockStrategiesRepo)
			mOrders := repo.Orders.(*MockOrdersRepo)
			tc.setup(mBalances, mPositions, mStrategies, mOrders, mPf)

			err := r.SyncPositions(context.Background(), tc.exchange, tc.symbol)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
