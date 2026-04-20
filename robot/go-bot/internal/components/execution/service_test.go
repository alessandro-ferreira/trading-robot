package execution

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "trading/robot/go-bot/gen/go/v1"
	"trading/robot/go-bot/internal/database/repository"
)

// MockMarketDataRepo is a mock implementation of repository.MarketDataRepo
type MockMarketDataRepo struct {
	mock.Mock
}

func (m *MockMarketDataRepo) GetMarketDataTicks(ctx context.Context, db repository.DBExecutor, exchangeName, symbol string, limit int) ([]repository.MarketDataTick, error) {
	args := m.Called(ctx, db, exchangeName, symbol, limit)
	return args.Get(0).([]repository.MarketDataTick), args.Error(1)
}

func (m *MockMarketDataRepo) InsertTick(ctx context.Context, db repository.DBExecutor, tick repository.MarketDataTick) error {
	args := m.Called(ctx, db, tick)
	return args.Error(0)
}

func TestService_GetTicker(t *testing.T) {
	testCases := []struct {
		name                string
		setupGatewayMock    func(*mockExchangeServer)
		setupRepoMock       func(*MockMarketDataRepo)
		expectedErrContains string
		expectedPrice       float64
	}{
		{
			name: "Success",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.tickerResponse = &pb.TickerResponse{
					Symbol: "BTC/USDT",
					Price:  50000.0,
				}
			},
			setupRepoMock: func(mockRepo *MockMarketDataRepo) {
				mockRepo.On("InsertTick", mock.Anything, mock.Anything, mock.MatchedBy(func(tick repository.MarketDataTick) bool {
					return tick.ExchangeName == "binance" && tick.Symbol == "BTC/USDT" && tick.Price == 50000.0
				})).Return(nil)
			},
			expectedPrice: 50000.0,
		},
		{
			name: "Gateway Error",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.tickerError = status.Error(codes.Unavailable, "gateway down")
			},
			setupRepoMock:       func(mockRepo *MockMarketDataRepo) {},
			expectedErrContains: "gateway down",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			mockSrv := &mockExchangeServer{}
			tc.setupGatewayMock(mockSrv)
			client, cleanup := setupTest(t, mockSrv)
			defer cleanup()

			mockRepo := new(MockMarketDataRepo)
			if tc.setupRepoMock != nil {
				tc.setupRepoMock(mockRepo)
			}

			container := &repository.Container{MarketData: mockRepo}
			svc := NewService(slog.Default(), nil, client, container)

			// Act
			resp, err := svc.GetTicker(context.Background(), "binance", "BTC/USDT")

			// Assert
			if tc.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrContains)
				assert.Nil(t, resp)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, resp)
				assert.Equal(t, tc.expectedPrice, resp.Price)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

// MockBalancesRepo is a mock implementation of repository.BalancesRepo
type MockBalancesRepo struct {
	mock.Mock
}

func (m *MockBalancesRepo) GetAllBalances(ctx context.Context, db repository.DBExecutor) ([]repository.BalanceData, error) {
	args := m.Called(ctx, db)
	return args.Get(0).([]repository.BalanceData), args.Error(1)
}

func (m *MockBalancesRepo) UpsertBalance(ctx context.Context, db repository.DBExecutor, balance repository.BalanceData) (int64, error) {
	args := m.Called(ctx, db, balance)
	return args.Get(0).(int64), args.Error(1)
}

func TestService_GetBalance(t *testing.T) {
	testCases := []struct {
		name                string
		setupGatewayMock    func(*mockExchangeServer)
		setupRepoMock       func(*MockBalancesRepo)
		expectedErrContains string
		expectedBalanceID   int64
		assertLogs          func(*testing.T, *bytes.Buffer)
	}{
		{
			name: "Success",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.balanceResponse = &pb.BalanceResponse{
					Balances: []*pb.BalanceObject{{Asset: "BTC", Free: 1.5, Used: 0.5, Total: 2.0}},
				}
			},
			setupRepoMock: func(mockRepo *MockBalancesRepo) {
				mockRepo.On("UpsertBalance", mock.Anything, mock.Anything, mock.MatchedBy(func(b repository.BalanceData) bool {
					return b.AssetSymbol == "BTC" && b.Free == 1.5 && b.Used == 0.5 && b.Total == 2.0
				})).Return(int64(1), nil)
			},
		},
		{
			name: "Gateway Error",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.balanceError = status.Error(codes.Unavailable, "gateway down")
			},
			setupRepoMock:       func(mockRepo *MockBalancesRepo) {},
			expectedErrContains: "gateway down",
		},
		{
			name: "DB Persistence Error",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.balanceResponse = &pb.BalanceResponse{
					Balances: []*pb.BalanceObject{{Asset: "BTC", Free: 1.0, Used: 0.0, Total: 1.0}},
				}
			},
			setupRepoMock: func(mockRepo *MockBalancesRepo) {
				mockRepo.On("UpsertBalance", mock.Anything, mock.Anything, mock.Anything).Return(int64(0), errors.New("db error"))
			},
			// We don't error the whole call if one asset fails to persist, we log and continue
			expectedErrContains: "",
		},
		{
			name: "Success with balance inconsistency warning",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.balanceResponse = &pb.BalanceResponse{
					Balances: []*pb.BalanceObject{{Asset: "BTC", Free: 1.0, Used: 1.0, Total: 2.000000001}},
				}
			},
			setupRepoMock: func(mockRepo *MockBalancesRepo) {
				mockRepo.On("UpsertBalance", mock.Anything, mock.Anything, mock.Anything).Return(int64(1), nil)
			},
			assertLogs: func(t *testing.T, logBuffer *bytes.Buffer) {
				assert.Contains(t, logBuffer.String(), "Balance inconsistency detected")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			mockSrv := &mockExchangeServer{}
			tc.setupGatewayMock(mockSrv)
			client, cleanup := setupTest(t, mockSrv)
			defer cleanup()

			mockRepo := new(MockBalancesRepo)
			tc.setupRepoMock(mockRepo)

			var logBuffer bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logBuffer, nil))

			container := &repository.Container{Balances: mockRepo}
			svc := NewService(logger, nil, client, container)

			// Act
			resp, err := svc.GetBalance(context.Background(), "binance", "BTC")

			// Assert
			if tc.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrContains)
				assert.Nil(t, resp)
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp)
				assert.NotEmpty(t, resp.Balances)
			}

			if tc.assertLogs != nil {
				tc.assertLogs(t, &logBuffer)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

// MockOrdersRepo is a mock implementation of repository.OrdersRepo
type MockOrdersRepo struct {
	mock.Mock
}

func (m *MockOrdersRepo) GetOrder(ctx context.Context, db repository.DBExecutor, exchangeOrderID, exchangeName string) (repository.OrderData, error) {
	args := m.Called(ctx, db, exchangeOrderID, exchangeName)
	return args.Get(0).(repository.OrderData), args.Error(1)
}

func (m *MockOrdersRepo) GetOrders(ctx context.Context, db repository.DBExecutor, exchangeName, symbol string, limit int) ([]repository.OrderData, error) {
	args := m.Called(ctx, db, exchangeName, symbol, limit)
	return args.Get(0).([]repository.OrderData), args.Error(1)
}

func (m *MockOrdersRepo) CreateOrder(ctx context.Context, db repository.DBExecutor, order repository.OrderData) (int64, error) {
	args := m.Called(ctx, db, order)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockOrdersRepo) UpdateOrder(ctx context.Context, db repository.DBExecutor, order repository.OrderData) (int64, error) {
	args := m.Called(ctx, db, order)
	return args.Get(0).(int64), args.Error(1)
}

func TestService_CreateOrder(t *testing.T) {
	testCases := []struct {
		name                string
		setupGatewayMock    func(*mockExchangeServer)
		setupRepoMock       func(*MockOrdersRepo)
		expectedErrContains string
		expectedOrderID     string
	}{
		{
			name: "Success",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.createOrderResponse = &pb.OrderResponse{
					Id:        "order-123",
					Symbol:    "BTC/USDT",
					Status:    repository.OrderStatusOpen,
					Filled:    0,
					Remaining: 1.5,
					Cost:      0,
					Timestamp: 1678886400000,
				}
			},
			setupRepoMock: func(mockRepo *MockOrdersRepo) {
				mockRepo.On("CreateOrder", mock.Anything, mock.Anything, mock.MatchedBy(func(o repository.OrderData) bool {
					return o.ExchangeOrderID == "order-123" &&
						o.InstrumentSymbol == "BTC/USDT" &&
						o.Status == repository.OrderStatusOpen &&
						o.Amount == 1.5
				})).Return(int64(1), nil)
			},
			expectedOrderID: "order-123",
		},
		{
			name: "Gateway Error",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.createOrderError = status.Error(codes.Unavailable, "gateway down")
			},
			setupRepoMock:       func(mockRepo *MockOrdersRepo) {},
			expectedErrContains: "gateway down",
		},
		{
			name: "DB Persistence Error",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.createOrderResponse = &pb.OrderResponse{
					Id:     "order-123",
					Symbol: "BTC/USDT",
					Status: repository.OrderStatusOpen,
				}
			},
			setupRepoMock: func(mockRepo *MockOrdersRepo) {
				mockRepo.On("CreateOrder", mock.Anything, mock.Anything, mock.Anything).Return(int64(0), errors.New("db error"))
			},
			expectedErrContains: "order created but failed to persist",
			expectedOrderID:     "order-123",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockSrv := &mockExchangeServer{}
			tc.setupGatewayMock(mockSrv)
			client, cleanup := setupTest(t, mockSrv)
			defer cleanup()

			mockRepo := new(MockOrdersRepo)
			tc.setupRepoMock(mockRepo)

			container := &repository.Container{Orders: mockRepo}
			svc := NewService(slog.Default(), nil, client, container)

			resp, err := svc.CreateOrder(context.Background(), "binance", "BTC/USDT", repository.OrderSideBuy, repository.OrderTypeLimit, 1.5, 50000.0)

			if tc.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrContains)
			} else {
				require.NoError(t, err)
			}

			if resp != nil {
				assert.Equal(t, tc.expectedOrderID, resp.Id)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestService_CancelOrder(t *testing.T) {
	testCases := []struct {
		name                string
		setupGatewayMock    func(*mockExchangeServer)
		setupRepoMock       func(*MockOrdersRepo)
		expectedErrContains string
	}{
		{
			name: "Success",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.cancelOrderResponse = &pb.CancelOrderResponse{
					Id:     "order-123",
					Status: repository.OrderStatusCanceled,
				}
				mockSrv.getOrderResponse = &pb.OrderResponse{
					Id:        "order-123",
					Symbol:    "BTC/USDT",
					Status:    repository.OrderStatusCanceled,
					Filled:    0.5,
					Remaining: 1.0,
					Cost:      10000.0,
					Timestamp: 1678886400000,
				}
			},
			setupRepoMock: func(mockRepo *MockOrdersRepo) {
				mockRepo.On("UpdateOrder", mock.Anything, mock.Anything, mock.MatchedBy(func(o repository.OrderData) bool {
					return o.ExchangeOrderID == "order-123" &&
						o.Status == repository.OrderStatusCanceled &&
						o.Filled == 0.5
				})).Return(int64(1), nil)
			},
		},
		{
			name: "Gateway Cancel Error",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.cancelOrderError = status.Error(codes.Unavailable, "gateway down")
			},
			setupRepoMock:       func(mockRepo *MockOrdersRepo) {},
			expectedErrContains: "failed to cancel order on gateway",
		},
		{
			name: "Gateway GetOrder Error",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.cancelOrderResponse = &pb.CancelOrderResponse{Id: "order-123", Status: repository.OrderStatusCanceled}
				mockSrv.getOrderError = status.Error(codes.Unavailable, "gateway down")
			},
			setupRepoMock:       func(mockRepo *MockOrdersRepo) {},
			expectedErrContains: "failed to fetch order details",
		},
		{
			name: "DB Update Error",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.cancelOrderResponse = &pb.CancelOrderResponse{Id: "order-123", Status: "canceled"}
				mockSrv.getOrderResponse = &pb.OrderResponse{Id: "order-123", Status: "canceled"}
			},
			setupRepoMock: func(mockRepo *MockOrdersRepo) {
				mockRepo.On("UpdateOrder", mock.Anything, mock.Anything, mock.Anything).Return(int64(0), errors.New("db error"))
			},
			expectedErrContains: "failed to update db",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockSrv := &mockExchangeServer{}
			tc.setupGatewayMock(mockSrv)
			client, cleanup := setupTest(t, mockSrv)
			defer cleanup()

			mockRepo := new(MockOrdersRepo)
			tc.setupRepoMock(mockRepo)

			container := &repository.Container{Orders: mockRepo}
			svc := NewService(slog.Default(), nil, client, container)

			err := svc.CancelOrder(context.Background(), "binance", "BTC/USDT", "order-123")

			if tc.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrContains)
			} else {
				require.NoError(t, err)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestService_GetOrder(t *testing.T) {
	testCases := []struct {
		name                string
		setupGatewayMock    func(*mockExchangeServer)
		setupRepoMock       func(*MockOrdersRepo)
		expectedErrContains string
		expectedOrderID     string
	}{
		{
			name: "Success",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.getOrderResponse = &pb.OrderResponse{
					Id:        "order-123",
					Symbol:    "BTC/USDT",
					Status:    repository.OrderStatusClosed,
					Filled:    1.5,
					Remaining: 0,
					Cost:      75000.0,
					Timestamp: 1678886400000,
				}
			},
			setupRepoMock: func(mockRepo *MockOrdersRepo) {
				mockRepo.On("UpdateOrder", mock.Anything, mock.Anything, mock.MatchedBy(func(o repository.OrderData) bool {
					return o.ExchangeOrderID == "order-123" &&
						o.Status == repository.OrderStatusClosed &&
						o.Filled == 1.5
				})).Return(int64(1), nil)
			},
			expectedOrderID: "order-123",
		},
		{
			name: "Gateway Error",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.getOrderError = status.Error(codes.Unavailable, "gateway down")
			},
			setupRepoMock:       func(mockRepo *MockOrdersRepo) {},
			expectedErrContains: "failed to fetch order from gateway",
		},
		{
			name: "DB Update Error",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.getOrderResponse = &pb.OrderResponse{Id: "order-123", Status: repository.OrderStatusClosed}
			},
			setupRepoMock: func(mockRepo *MockOrdersRepo) {
				mockRepo.On("UpdateOrder", mock.Anything, mock.Anything, mock.Anything).Return(int64(0), errors.New("db error"))
			},
			expectedErrContains: "order fetched but failed to update db",
			expectedOrderID:     "order-123",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockSrv := &mockExchangeServer{}
			tc.setupGatewayMock(mockSrv)
			client, cleanup := setupTest(t, mockSrv)
			defer cleanup()

			mockRepo := new(MockOrdersRepo)
			tc.setupRepoMock(mockRepo)

			container := &repository.Container{Orders: mockRepo}
			svc := NewService(slog.Default(), nil, client, container)

			resp, err := svc.GetOrder(context.Background(), "binance", "BTC/USDT", "order-123")

			if tc.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrContains)
			} else {
				require.NoError(t, err)
			}

			if tc.expectedOrderID != "" {
				require.NotNil(t, resp)
				assert.Equal(t, tc.expectedOrderID, resp.Id)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestService_GetOpenOrders(t *testing.T) {
	testCases := []struct {
		name                string
		setupGatewayMock    func(*mockExchangeServer)
		setupRepoMock       func(*MockOrdersRepo)
		expectedErrContains string
		expectedCount       int
	}{
		{
			name: "Success",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.getOpenOrdersResponse = &pb.OpenOrdersResponse{
					Orders: []*pb.OrderResponse{
						{
							Id:        "order-1",
							Symbol:    "BTC/USDT",
							Status:    repository.OrderStatusOpen,
							Filled:    0,
							Remaining: 1.0,
						},
						{
							Id:        "order-2",
							Symbol:    "BTC/USDT",
							Status:    repository.OrderStatusOpen,
							Filled:    0.5,
							Remaining: 0.5,
						},
					},
				}
			},
			setupRepoMock: func(mockRepo *MockOrdersRepo) {
				mockRepo.On("UpdateOrder", mock.Anything, mock.Anything, mock.MatchedBy(func(o repository.OrderData) bool {
					return o.ExchangeOrderID == "order-1"
				})).Return(int64(1), nil)
				mockRepo.On("UpdateOrder", mock.Anything, mock.Anything, mock.MatchedBy(func(o repository.OrderData) bool {
					return o.ExchangeOrderID == "order-2"
				})).Return(int64(2), nil)
			},
			expectedCount: 2,
		},
		{
			name: "Gateway Error",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.getOpenOrdersError = status.Error(codes.Unavailable, "gateway down")
			},
			setupRepoMock:       func(mockRepo *MockOrdersRepo) {},
			expectedErrContains: "failed to fetch open orders",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockSrv := &mockExchangeServer{}
			tc.setupGatewayMock(mockSrv)
			client, cleanup := setupTest(t, mockSrv)
			defer cleanup()

			mockRepo := new(MockOrdersRepo)
			tc.setupRepoMock(mockRepo)

			container := &repository.Container{Orders: mockRepo}
			svc := NewService(slog.Default(), nil, client, container)

			resp, err := svc.GetOpenOrders(context.Background(), "binance", "BTC/USDT")

			if tc.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrContains)
			} else {
				require.NoError(t, err)
				assert.Len(t, resp.Orders, tc.expectedCount)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}
