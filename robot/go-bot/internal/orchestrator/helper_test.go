//go:build unit

package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/strategy"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestOrchestrator_CheckPosition(t *testing.T) {
	tests := []struct {
		name           string
		position       repository.PositionData
		positionErr    error
		expected       repository.PositionData
		expectedErr    string
		expectedSignal strategy.StrategySignal
		checkSignal    bool
	}{
		{
			name:     "Return position",
			position: repository.PositionData{Active: true, Quantity: 1.0, EntryPrice: 100},
			expected: repository.PositionData{Active: true, Quantity: 1.0, EntryPrice: 100},
		},
		{
			name:        "Treat missing position as empty",
			positionErr: pgx.ErrNoRows,
			expected:    repository.PositionData{},
		},
		{
			name:        "Wrap position query error",
			positionErr: errors.New("db error"),
			expected:    repository.PositionData{},
			expectedErr: "failed to query position: db error",
		},
		{
			name:           "Reset strategy for unknown origin",
			position:       repository.PositionData{UnknownOrigin: true},
			expected:       repository.PositionData{UnknownOrigin: true},
			expectedSignal: strategy.SignalSearchingBuyEntry,
			checkSignal:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch, _, mPf, _, _ := setupOrchestratorTest(t)
			sig := initTestSignalGenerator(t, orch, strategy.SignalTrackingSellExit)
			mPf.On("GetPosition", mock.Anything, "binance", "BTC/USDT").Return(tt.position, tt.positionErr).Once()

			position, err := orch.checkPosition(context.Background(), orch.logger, "binance", "BTC/USDT", sig)

			assert.Equal(t, tt.expected, position)
			if tt.expectedErr == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.expectedErr)
			}

			if tt.checkSignal {
				s, _ := sig.GetSignal(100.0, time.Now().Unix())
				assert.Equal(t, tt.expectedSignal, s)
			}
		})
	}
}

func TestOrchestrator_CheckPendingOrder(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(mExec *MockExecutionService, mOrders *MockOrdersRepo, cancel context.CancelFunc)
		expectedErr string
	}{
		{
			name: "No pending order",
			setup: func(mExec *MockExecutionService, mOrders *MockOrdersRepo, cancel context.CancelFunc) {
				mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "BTC/USDT", []string{repository.OrderStatusNew}, []string{}, []string{}, 1).
					Return([]repository.OrderData{}, nil).Once()
			},
		},
		{
			name: "Already linked order",
			setup: func(mExec *MockExecutionService, mOrders *MockOrdersRepo, cancel context.CancelFunc) {
				mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "BTC/USDT", []string{repository.OrderStatusNew}, []string{}, []string{}, 1).
					Return([]repository.OrderData{{ExchangeOrderID: sql.NullString{String: "exchange-1", Valid: true}}}, nil).Once()
			},
		},
		{
			name: "Database error",
			setup: func(mExec *MockExecutionService, mOrders *MockOrdersRepo, cancel context.CancelFunc) {
				mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "BTC/USDT", []string{repository.OrderStatusNew}, []string{}, []string{}, 1).
					Return([]repository.OrderData{}, errors.New("db error")).Once()
			},
			expectedErr: "failed to fetch pending orders from database: db error",
		},
		{
			name: "Link matching exchange order",
			setup: func(mExec *MockExecutionService, mOrders *MockOrdersRepo, cancel context.CancelFunc) {
				pendingOrder := repository.OrderData{ID: 1, Side: repository.OrderSideBuy, Amount: 1.5, Status: repository.OrderStatusNew}
				exchangeOrder := repository.OrderData{Side: repository.OrderSideBuy, Amount: 1.5, ExchangeOrderID: sql.NullString{String: "exchange-1", Valid: true}}
				mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "BTC/USDT", []string{repository.OrderStatusNew}, []string{}, []string{}, 1).
					Return([]repository.OrderData{pendingOrder}, nil).Once()
				mExec.On("GetOrders", mock.Anything, "binance", "BTC/USDT", 10, false).
					Return([]repository.OrderData{exchangeOrder}, nil).Once()
				updatedOrder := pendingOrder
				updatedOrder.ExchangeOrderID = exchangeOrder.ExchangeOrderID
				mOrders.On("UpdateOrder", mock.Anything, mock.Anything, updatedOrder).Return(int64(1), nil).Once()
			},
		},
		{
			name: "Exchange error",
			setup: func(mExec *MockExecutionService, mOrders *MockOrdersRepo, cancel context.CancelFunc) {
				pendingOrder := repository.OrderData{Side: repository.OrderSideBuy, Amount: 1.5, Status: repository.OrderStatusNew}
				mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "BTC/USDT", []string{repository.OrderStatusNew}, []string{}, []string{}, 1).
					Return([]repository.OrderData{pendingOrder}, nil).Once()
				mExec.On("GetOrders", mock.Anything, "binance", "BTC/USDT", 10, false).
					Return([]repository.OrderData{}, errors.New("exchange error")).Once()
			},
			expectedErr: "failed to fetch orders from exchange: exchange error",
		},
		{
			name: "Pending order not found - Mark rejected",
			setup: func(mExec *MockExecutionService, mOrders *MockOrdersRepo, cancel context.CancelFunc) {
				pendingOrder := repository.OrderData{ID: 1, Side: repository.OrderSideBuy, Amount: 1.5, Status: repository.OrderStatusNew}
				mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "BTC/USDT", []string{repository.OrderStatusNew}, []string{}, []string{}, 1).
					Return([]repository.OrderData{pendingOrder}, nil).Once()
				mExec.On("GetOrders", mock.Anything, "binance", "BTC/USDT", 10, false).
					Return([]repository.OrderData{}, nil).Once()
				updatedOrder := pendingOrder
				updatedOrder.Status = repository.OrderStatusRejected
				mOrders.On("UpdateOrder", mock.Anything, mock.Anything, updatedOrder).
					Return(int64(1), nil).Once()
			},
		},
		{
			name: "Update exchange order ID error",
			setup: func(mExec *MockExecutionService, mOrders *MockOrdersRepo, cancel context.CancelFunc) {
				pendingOrder := repository.OrderData{ID: 1, Side: repository.OrderSideBuy, Amount: 1.5, Status: repository.OrderStatusNew}
				exchangeOrder := repository.OrderData{Side: repository.OrderSideBuy, Amount: 1.5, ExchangeOrderID: sql.NullString{String: "exchange-1", Valid: true}}
				mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "BTC/USDT", []string{repository.OrderStatusNew}, []string{}, []string{}, 1).
					Return([]repository.OrderData{pendingOrder}, nil).Once()
				mExec.On("GetOrders", mock.Anything, "binance", "BTC/USDT", 10, false).
					Return([]repository.OrderData{exchangeOrder}, nil).Once()
				updatedOrder := pendingOrder
				updatedOrder.ExchangeOrderID = exchangeOrder.ExchangeOrderID
				mOrders.On("UpdateOrder", mock.Anything, mock.Anything, updatedOrder).
					Return(int64(0), errors.New("update error")).Once()
				rejectedOrder := pendingOrder
				rejectedOrder.Status = repository.OrderStatusRejected
				mOrders.On("UpdateOrder", mock.Anything, mock.Anything, rejectedOrder).
					Return(int64(0), errors.New("rejection update error")).Once()
			},
			expectedErr: "failed to update pending order to rejected: rejection update error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch, repo, _, _, mExec := setupOrchestratorTest(t)
			mOrders := repo.Orders.(*MockOrdersRepo)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tt.setup(mExec, mOrders, cancel)

			err := orch.checkPendingOrder(ctx, orch.logger, "binance", "BTC/USDT")
			if tt.expectedErr == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.expectedErr)
			}
		})
	}
}

func TestOrchestrator_GetDbOpenOrders(t *testing.T) {
	tests := []struct {
		name          string
		types         []string
		sides         []string
		dbOrders      []repository.OrderData
		dbErr         error
		expected      []repository.OrderData
		expectedError string
	}{
		{
			name:     "Returns open orders",
			types:    []string{repository.OrderTypeMarket},
			sides:    []string{repository.OrderSideBuy},
			dbOrders: []repository.OrderData{{ID: 1, Side: repository.OrderSideBuy, OrderType: repository.OrderTypeMarket}},
			expected: []repository.OrderData{{ID: 1, Side: repository.OrderSideBuy, OrderType: repository.OrderTypeMarket}},
		},
		{
			name:          "Wraps database error",
			types:         []string{repository.OrderTypeMarket},
			sides:         []string{repository.OrderSideSell},
			dbErr:         errors.New("db error"),
			expectedError: "failed to fetch open orders from database: db error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch, repo, _, _, _ := setupOrchestratorTest(t)
			mOrders := repo.Orders.(*MockOrdersRepo)
			mOrders.On("GetOrders", mock.Anything, mock.Anything, "binance", "BTC/USDT",
				[]string{repository.OrderStatusNew, repository.OrderStatusOpen}, tt.types, tt.sides, 1,
			).Return(tt.dbOrders, tt.dbErr).Once()

			orders, err := orch.getDbOpenOrders(context.Background(), orch.logger, "binance", "BTC/USDT", tt.types, tt.sides)

			if tt.expectedError == "" {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, orders)
			} else {
				assert.Nil(t, orders)
				assert.EqualError(t, err, tt.expectedError)
			}
		})
	}
}

func TestOrchestrator_GetBalance(t *testing.T) {
	tests := []struct {
		name          string
		balances      []repository.BalanceData
		balanceErr    error
		expected      repository.BalanceData
		expectedError string
		expectError   bool
	}{
		{
			name:     "Returns first asset balance",
			balances: []repository.BalanceData{{AssetSymbol: "BTC", Total: 1.5}, {AssetSymbol: "BTC", Total: 2.5}},
			expected: repository.BalanceData{AssetSymbol: "BTC", Total: 1.5},
		},
		{
			name:          "Wraps exchange error",
			balanceErr:    errors.New("exchange error"),
			expectedError: "failed to get balance: exchange error",
			expectError:   true,
		},
		{
			name:        "Returns error when balance is missing",
			balances:    []repository.BalanceData{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch, _, _, _, mExec := setupOrchestratorTest(t)
			mExec.On("GetBalance", mock.Anything, "binance", "BTC").
				Return(tt.balances, tt.balanceErr).Once()

			balance, err := orch.getBalance(context.Background(), orch.logger, "binance", "BTC/USDT")

			assert.Equal(t, tt.expected, balance)
			if !tt.expectError {
				assert.NoError(t, err)
			} else if tt.expectedError == "" {
				assert.Error(t, err)
			} else {
				assert.EqualError(t, err, tt.expectedError)
			}
		})
	}
}

func TestOrchestrator_GetStopLossOrder(t *testing.T) {
	tests := []struct {
		name          string
		openOrders    []repository.OrderData
		ordersErr     error
		expected      repository.OrderData
		expectedError string
	}{
		{
			name: "Returns stop loss order",
			openOrders: []repository.OrderData{
				{ID: 1, Side: repository.OrderSideSell, OrderType: repository.OrderTypeMarket},
				{ID: 2, Side: repository.OrderSideSell, OrderType: repository.OrderTypeStopMarket},
			},
			expected: repository.OrderData{ID: 2, Side: repository.OrderSideSell, OrderType: repository.OrderTypeStopMarket},
		},
		{
			name:       "Returns empty order when stop loss is missing",
			openOrders: []repository.OrderData{{ID: 1, Side: repository.OrderSideSell, OrderType: repository.OrderTypeMarket}},
		},
		{
			name:          "Wraps exchange error",
			ordersErr:     errors.New("exchange error"),
			expectedError: "failed to fetch open orders for stop loss check: exchange error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch, _, _, _, mExec := setupOrchestratorTest(t)
			mExec.On("GetOpenOrders", mock.Anything, "binance", "BTC/USDT", 10, true).
				Return(tt.openOrders, tt.ordersErr).Once()

			order, err := orch.getStopLossOrder(context.Background(), orch.logger, "binance", "BTC/USDT")

			assert.Equal(t, tt.expected, order)
			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.expectedError)
			}
		})
	}
}
