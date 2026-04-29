package reconciliation

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	pb "trading/robot/go-bot/gen/go/v1"
	"trading/robot/go-bot/internal/database/repository"
)

// --- Mocks ---

type MockExecutionService struct{ mock.Mock }

func (m *MockExecutionService) GetTicker(ctx context.Context, ex, sym string) (*pb.TickerResponse, error) {
	args := m.Called(ctx, ex, sym)
	return args.Get(0).(*pb.TickerResponse), args.Error(1)
}
func (m *MockExecutionService) GetBalance(ctx context.Context, ex, asset string) (*pb.BalanceResponse, error) {
	args := m.Called(ctx, ex, asset)
	return args.Get(0).(*pb.BalanceResponse), args.Error(1)
}
func (m *MockExecutionService) CreateOrder(ctx context.Context, ex, sym, side, typ string, amt, pr float64) (*pb.OrderResponse, error) {
	args := m.Called(ctx, ex, sym, side, typ, amt, pr)
	return args.Get(0).(*pb.OrderResponse), args.Error(1)
}
func (m *MockExecutionService) CancelOrder(ctx context.Context, ex, sym, id string) error {
	return m.Called(ctx, ex, sym, id).Error(0)
}
func (m *MockExecutionService) GetOrder(ctx context.Context, ex, sym, id string) (*pb.OrderResponse, error) {
	args := m.Called(ctx, ex, sym, id)
	return args.Get(0).(*pb.OrderResponse), args.Error(1)
}
func (m *MockExecutionService) GetOpenOrders(ctx context.Context, ex, sym string, lim int) (*pb.OrdersResponse, error) {
	args := m.Called(ctx, ex, sym, lim)
	return args.Get(0).(*pb.OrdersResponse), args.Error(1)
}
func (m *MockExecutionService) GetRecentTrades(ctx context.Context, ex, sym string, since int64, lim int) (*pb.OrdersResponse, error) {
	args := m.Called(ctx, ex, sym, since, lim)
	return args.Get(0).(*pb.OrdersResponse), args.Error(1)
}

type MockOrdersRepo struct{ mock.Mock }

func (m *MockOrdersRepo) GetOrder(ctx context.Context, db repository.DBExecutor, id, ex string) (repository.OrderData, error) {
	args := m.Called(ctx, db, id, ex)
	return args.Get(0).(repository.OrderData), args.Error(1)
}
func (m *MockOrdersRepo) GetOrders(ctx context.Context, db repository.DBExecutor, ex, sym string, st []string, lim int) ([]repository.OrderData, error) {
	args := m.Called(ctx, db, ex, sym, st, lim)
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

func (m *MockBalancesRepo) GetAllBalances(ctx context.Context, db repository.DBExecutor) ([]repository.BalanceData, error) {
	args := m.Called(ctx, db)
	return args.Get(0).([]repository.BalanceData), args.Error(1)
}
func (m *MockBalancesRepo) UpsertBalance(ctx context.Context, db repository.DBExecutor, b repository.BalanceData) (int64, error) {
	args := m.Called(ctx, db, b)
	return args.Get(0).(int64), args.Error(1)
}

type MockPositionsRepo struct{ mock.Mock }

func (m *MockPositionsRepo) GetOpenPositions(ctx context.Context, db repository.DBExecutor, ex, sym string) ([]repository.PositionData, error) {
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

func (m *MockStrategiesRepo) GetStrategyPairs(ctx context.Context, db repository.DBExecutor, onlyEnabled bool) ([]repository.StrategyPair, error) {
	args := m.Called(ctx, db, onlyEnabled)
	return args.Get(0).([]repository.StrategyPair), args.Error(1)
}

func (m *MockStrategiesRepo) UpsertEnabledStrategy(ctx context.Context, db repository.DBExecutor, exchangeName string, symbol string, strategyType string, label string, momentum repository.StrategyMomentum) error {
	args := m.Called(ctx, db, exchangeName, symbol, strategyType, label, momentum)
	return args.Error(0)
}

func (m *MockStrategiesRepo) DisableStrategy(ctx context.Context, db repository.DBExecutor, exchange, symbol, strategyType string) error {
	args := m.Called(ctx, db, exchange, symbol, strategyType)
	return args.Error(0)
}

// --- Helpers ---

func setupReconciler(t *testing.T) (*Reconciler, *MockExecutionService, *repository.Container) {
	mExec := new(MockExecutionService)
	mOrders := new(MockOrdersRepo)
	mBalances := new(MockBalancesRepo)
	mPositions := new(MockPositionsRepo)
	mStrategies := new(MockStrategiesRepo)

	container := &repository.Container{
		Orders:     mOrders,
		Balances:   mBalances,
		Positions:  mPositions,
		Strategies: mStrategies,
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := NewReconciler(logger, nil, container, mExec)
	return r, mExec, container
}

func TestReconciler_SyncOrders(t *testing.T) {
	testCases := []struct {
		name     string
		exchange string
		symbol   string
		setup    func(mExec *MockExecutionService, mOrders *MockOrdersRepo)
		wantErr  bool
	}{
		{
			name:     "Investigation of vanished order",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mExec *MockExecutionService, mOrders *MockOrdersRepo) {
				mExec.On("GetOpenOrders", mock.Anything, "binance", "BTC/USDT", 100).Return(&pb.OrdersResponse{
					Orders: []*pb.OrderResponse{},
				}, nil)
				mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "BTC/USDT", []string{"new", "open"}, 100).Return([]repository.OrderData{
					{ExchangeOrderID: "gone-1", InstrumentSymbol: "BTC/USDT", Status: "open"},
				}, nil)
				mExec.On("GetOrder", mock.Anything, "binance", "BTC/USDT", "gone-1").Return(&pb.OrderResponse{Id: "gone-1"}, nil)
			},
		},
		{
			name:     "Gateway failure returns error",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mExec *MockExecutionService, mOrders *MockOrdersRepo) {
				mExec.On("GetOpenOrders", mock.Anything, "binance", "BTC/USDT", 100).Return((*pb.OrdersResponse)(nil), errors.New("rpc fail"))
			},
			wantErr: true,
		},
		{
			name:     "DB query failure",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mExec *MockExecutionService, mOrders *MockOrdersRepo) {
				mExec.On("GetOpenOrders", mock.Anything, "binance", "BTC/USDT", 100).Return(&pb.OrdersResponse{}, nil)
				mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "BTC/USDT", mock.Anything, 100).Return([]repository.OrderData{}, errors.New("db disconnect"))
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r, mExec, repo := setupReconciler(t)
			mOrders := repo.Orders.(*MockOrdersRepo)
			tc.setup(mExec, mOrders)

			err := r.SyncOrders(context.Background(), tc.exchange, tc.symbol)
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
		setup    func(mBalances *MockBalancesRepo, mPositions *MockPositionsRepo, mStrategies *MockStrategiesRepo)
		wantErr  bool
	}{
		{
			name:     "External liquidation - Wallet zero",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mBalances *MockBalancesRepo, mPositions *MockPositionsRepo, mStrategies *MockStrategiesRepo) {
				mBalances.On("GetAllBalances", mock.Anything, mock.Anything).Return([]repository.BalanceData{
					{ExchangeName: "binance", AssetSymbol: "BTC", Total: 0.0},
				}, nil)
				mPositions.On("GetOpenPositions", mock.Anything, mock.Anything, "binance", "").Return([]repository.PositionData{
					{InstrumentSymbol: "BTC/USDT", Quantity: 1.0},
				}, nil)
				mPositions.On("DeletePosition", mock.Anything, mock.Anything, "binance", "BTC/USDT").Return(nil)
			},
		},
		{
			name:     "Ghost balance adoption",
			exchange: "binance",
			symbol:   "LTC/USDT",
			setup: func(mBalances *MockBalancesRepo, mPositions *MockPositionsRepo, mStrategies *MockStrategiesRepo) {
				mBalances.On("GetAllBalances", mock.Anything, mock.Anything).Return([]repository.BalanceData{
					{ExchangeName: "binance", AssetSymbol: "LTC", Total: 10.0},
				}, nil)
				mPositions.On("GetOpenPositions", mock.Anything, mock.Anything, "binance", "").Return([]repository.PositionData{}, nil)
				mPositions.On("UpsertPosition", mock.Anything, mock.Anything, mock.MatchedBy(func(p repository.PositionData) bool {
					return p.InstrumentSymbol == "LTC/USDT" && p.StrategyState == "unmanaged" && p.Quantity == 10.0
				})).Return(nil)
			},
		},
		{
			name:     "Quantity drift adjustment",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mBalances *MockBalancesRepo, mPositions *MockPositionsRepo, mStrategies *MockStrategiesRepo) {
				mBalances.On("GetAllBalances", mock.Anything, mock.Anything).Return([]repository.BalanceData{
					{ExchangeName: "binance", AssetSymbol: "BTC", Total: 1.0005},
				}, nil)
				mPositions.On("GetOpenPositions", mock.Anything, mock.Anything, "binance", "").Return([]repository.PositionData{
					{InstrumentSymbol: "BTC/USDT", Quantity: 1.0},
				}, nil)
				mPositions.On("UpsertPosition", mock.Anything, mock.Anything, mock.MatchedBy(func(p repository.PositionData) bool {
					return p.Quantity == 1.0005
				})).Return(nil)
			},
		},
		{
			name:     "Empty symbol - adoption via strategy pairs",
			exchange: "binance",
			symbol:   "",
			setup: func(mBalances *MockBalancesRepo, mPositions *MockPositionsRepo, mStrategies *MockStrategiesRepo) {
				mBalances.On("GetAllBalances", mock.Anything, mock.Anything).Return([]repository.BalanceData{
					{ExchangeName: "binance", AssetSymbol: "ETH", Total: 2.0},
				}, nil)
				mPositions.On("GetOpenPositions", mock.Anything, mock.Anything, "binance", "").Return([]repository.PositionData{}, nil)
				mStrategies.On("GetStrategyPairs", mock.Anything, mock.Anything, false).Return([]repository.StrategyPair{
					{ExchangeName: "binance", InstrumentSymbol: "ETH/USDT"},
				}, nil)
				mPositions.On("UpsertPosition", mock.Anything, mock.Anything, mock.MatchedBy(func(p repository.PositionData) bool {
					return p.InstrumentSymbol == "ETH/USDT"
				})).Return(nil)
			},
		},
		{
			name:     "DB Error",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mBalances *MockBalancesRepo, mPositions *MockPositionsRepo, mStrategies *MockStrategiesRepo) {
				mBalances.On("GetAllBalances", mock.Anything, mock.Anything).Return([]repository.BalanceData{}, errors.New("db down"))
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r, _, repo := setupReconciler(t)
			mBalances := repo.Balances.(*MockBalancesRepo)
			mPositions := repo.Positions.(*MockPositionsRepo)
			mStrategies := repo.Strategies.(*MockStrategiesRepo)
			tc.setup(mBalances, mPositions, mStrategies)

			err := r.SyncPositions(context.Background(), tc.exchange, tc.symbol)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestReconciler_SyncTradeHistory(t *testing.T) {
	testCases := []struct {
		name     string
		exchange string
		symbol   string
		setup    func(mExec *MockExecutionService, mPositions *MockPositionsRepo, mStrategies *MockStrategiesRepo)
		wantErr  bool
	}{
		{
			name:     "Promotion - Success",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mExec *MockExecutionService, mPositions *MockPositionsRepo, mStrategies *MockStrategiesRepo) {
				now := time.Now().UnixMilli()
				mExec.On("GetRecentTrades", mock.Anything, "binance", "BTC/USDT", mock.Anything, 100).Return(&pb.OrdersResponse{
					Orders: []*pb.OrderResponse{
						{Id: "trade-1", Symbol: "BTC/USDT", Filled: 1.0, Average: 50000.0, Timestamp: now},
					},
				}, nil)
				mPositions.On("GetPosition", mock.Anything, mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{
					InstrumentSymbol: "BTC/USDT", Quantity: 1.0, StrategyState: "unmanaged",
				}, nil)
				mPositions.On("UpsertPosition", mock.Anything, mock.Anything, mock.MatchedBy(func(p repository.PositionData) bool {
					return p.StrategyState == "active" && p.EntryPrice == 50000.0
				})).Return(nil)
			},
		},
		{
			name:     "Promotion Skip - Trade too old",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mExec *MockExecutionService, mPositions *MockPositionsRepo, mStrategies *MockStrategiesRepo) {
				old := time.Now().Add(-1 * time.Hour).UnixMilli()
				mExec.On("GetRecentTrades", mock.Anything, "binance", "BTC/USDT", mock.Anything, 100).Return(&pb.OrdersResponse{
					Orders: []*pb.OrderResponse{
						{Id: "old-1", Symbol: "BTC/USDT", Filled: 1.0, Average: 50000.0, Timestamp: old},
					},
				}, nil)
				mPositions.On("GetPosition", mock.Anything, mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{
					InstrumentSymbol: "BTC/USDT", Quantity: 1.0, StrategyState: "unmanaged",
				}, nil)
			},
		},
		{
			name:     "Promotion Skip - Quantity mismatch",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mExec *MockExecutionService, mPositions *MockPositionsRepo, mStrategies *MockStrategiesRepo) {
				mExec.On("GetRecentTrades", mock.Anything, "binance", "BTC/USDT", mock.Anything, 100).Return(&pb.OrdersResponse{
					Orders: []*pb.OrderResponse{
						{Id: "mismatch-1", Symbol: "BTC/USDT", Filled: 1.0, Timestamp: time.Now().UnixMilli()},
					},
				}, nil)
				mPositions.On("GetPosition", mock.Anything, mock.Anything, "binance", "BTC/USDT").Return(repository.PositionData{
					InstrumentSymbol: "BTC/USDT", Quantity: 0.5, StrategyState: "unmanaged",
				}, nil)
			},
		},
		{
			name:     "Empty symbol - Batch Audit",
			exchange: "binance",
			symbol:   "",
			setup: func(mExec *MockExecutionService, mPositions *MockPositionsRepo, mStrategies *MockStrategiesRepo) {
				mExec.On("GetRecentTrades", mock.Anything, "binance", "", mock.Anything, 100).Return(&pb.OrdersResponse{
					Orders: []*pb.OrderResponse{},
				}, nil)
				mStrategies.On("GetStrategyPairs", mock.Anything, mock.Anything, false).Return([]repository.StrategyPair{
					{ExchangeName: "binance", InstrumentSymbol: "ETH/USDT"},
				}, nil)
				mExec.On("GetRecentTrades", mock.Anything, "binance", "ETH/USDT", mock.Anything, 100).Return(&pb.OrdersResponse{
					Orders: []*pb.OrderResponse{},
				}, nil)
			},
		},
		{
			name:     "Gateway Error",
			exchange: "binance",
			symbol:   "BTC/USDT",
			setup: func(mExec *MockExecutionService, mPositions *MockPositionsRepo, mStrategies *MockStrategiesRepo) {
				mExec.On("GetRecentTrades", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return((*pb.OrdersResponse)(nil), errors.New("fail"))
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r, mExec, repo := setupReconciler(t)
			mPositions := repo.Positions.(*MockPositionsRepo)
			mStrategies := repo.Strategies.(*MockStrategiesRepo)
			tc.setup(mExec, mPositions, mStrategies)

			err := r.SyncTradeHistory(context.Background(), tc.exchange, tc.symbol)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
