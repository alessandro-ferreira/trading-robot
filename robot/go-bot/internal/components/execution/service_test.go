//go:build unit

package execution

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"
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

func (m *MockMarketDataRepo) GetMarketDataTicks(ctx context.Context, db repository.DBExecutor, exchangeName, symbol string, since int64) ([]repository.MarketDataTick, error) {
	args := m.Called(ctx, db, exchangeName, symbol, since)
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
		{
			name: "Insert Error",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.tickerResponse = &pb.TickerResponse{
					Symbol: "BTC/USDT",
					Price:  50000.0,
				}
			},
			setupRepoMock: func(mockRepo *MockMarketDataRepo) {
				mockRepo.On("InsertTick", mock.Anything, mock.Anything, mock.Anything).Return(
					errors.New("db insert error"),
				)
			},
			expectedErrContains: "failed to persist tick",
			expectedPrice:       50000.0,
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
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			svc := NewService(logger, nil, client, container)

			// Act
			resp, err := svc.GetTicker(context.Background(), "binance", "BTC/USDT")

			// Assert
			if tc.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrContains)
			} else {
				require.NoError(t, err)
			}

			if tc.expectedPrice > 0 {
				require.NotNil(t, resp)
				assert.Equal(t, tc.expectedPrice, resp.Price)
			} else {
				assert.Equal(t, repository.MarketDataTick{}, resp)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

// MockBalancesRepo is a mock implementation of repository.BalancesRepo
type MockBalancesRepo struct {
	mock.Mock
}

func (m *MockBalancesRepo) GetBalance(ctx context.Context, db repository.DBExecutor, exchange, asset string) (repository.BalanceData, error) {
	return repository.BalanceData{}, nil
}

func (m *MockBalancesRepo) GetAllBalances(ctx context.Context, db repository.DBExecutor, exchange string) ([]repository.BalanceData, error) {
	args := m.Called(ctx, db, exchange)
	return args.Get(0).([]repository.BalanceData), args.Error(1)
}

func (m *MockBalancesRepo) UpsertBalance(ctx context.Context, db repository.DBExecutor, balance repository.BalanceData) (int64, error) {
	args := m.Called(ctx, db, balance)
	return args.Get(0).(int64), args.Error(1)
}

func TestService_GetBalance(t *testing.T) {
	testCases := []struct {
		name                string
		assetSymbol         string
		setupGatewayMock    func(*mockExchangeServer)
		setupRepoMock       func(*MockBalancesRepo)
		expectedErrContains string
		expectedBalanceID   int64
		expectedCount       int
		assertLogs          func(*testing.T, *bytes.Buffer)
	}{
		{
			name:        "Success",
			assetSymbol: "BTC",
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
			expectedCount: 1,
		},
		{
			name:        "Gateway Error",
			assetSymbol: "BTC",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.balanceError = status.Error(codes.Unavailable, "gateway down")
			},
			setupRepoMock:       func(mockRepo *MockBalancesRepo) {},
			expectedErrContains: "gateway down",
			expectedCount:       0,
		},
		{
			name:        "DB Persistence Error",
			assetSymbol: "", // Empty assetSymbol means continue on error
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.balanceResponse = &pb.BalanceResponse{
					Balances: []*pb.BalanceObject{{Asset: "BTC", Free: 1.0, Used: 0.0, Total: 1.0}},
				}
			},
			setupRepoMock: func(mockRepo *MockBalancesRepo) {
				mockRepo.On("UpsertBalance", mock.Anything, mock.Anything, mock.Anything).Return(int64(0), errors.New("db error"))
			},
			// We don't error the whole call if one asset fails to persist, we log and continue
			expectedCount:       0,
			expectedErrContains: "",
		},
		{
			name:        "DB Persistence Error - Specific Asset",
			assetSymbol: "BTC", // Specific asset means fail on error
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.balanceResponse = &pb.BalanceResponse{
					Balances: []*pb.BalanceObject{{Asset: "BTC", Free: 1.0, Used: 0.0, Total: 1.0}},
				}
			},
			setupRepoMock: func(mockRepo *MockBalancesRepo) {
				mockRepo.On("UpsertBalance", mock.Anything, mock.Anything, mock.Anything).Return(int64(0), errors.New("db error"))
			},
			// When assetSymbol is passed ("BTC"), persistence failure should return an error
			expectedErrContains: "failed to persist balance",
			expectedCount:       0,
		},
		{
			name:        "Success with balance inconsistency warning",
			assetSymbol: "BTC",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.balanceResponse = &pb.BalanceResponse{
					Balances: []*pb.BalanceObject{{Asset: "BTC", Free: 1.0, Used: 1.0, Total: 2.000000001}},
				}
			},
			setupRepoMock: func(mockRepo *MockBalancesRepo) {
				mockRepo.On("UpsertBalance", mock.Anything, mock.Anything, mock.Anything).Return(int64(1), nil)
			},
			expectedCount: 1,
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
			resp, err := svc.GetBalance(context.Background(), "binance", tc.assetSymbol)

			// Assert
			if tc.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrContains)
				assert.Empty(t, resp)
			} else {
				require.NoError(t, err)
				assert.Len(t, resp, tc.expectedCount)
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

func (m *MockOrdersRepo) GetOrder(ctx context.Context, db repository.DBExecutor, exchangeName, exchangeOrderID string) (repository.OrderData, error) {
	args := m.Called(ctx, db, exchangeName, exchangeOrderID)
	return args.Get(0).(repository.OrderData), args.Error(1)
}

func (m *MockOrdersRepo) GetOrders(ctx context.Context, db repository.DBExecutor, exchangeName, symbol string, statuses, types, sides []string, limit int) ([]repository.OrderData, error) {
	args := m.Called(ctx, db, exchangeName, symbol, statuses, types, sides, limit)
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
					Id:          "order-123",
					Symbol:      "BTC/USDT",
					Status:      repository.OrderStatusOpen,
					Filled:      0,
					Remaining:   1.5,
					Cost:        0,
					Fee:         0.1,
					FeeCurrency: "USDT",
					Timestamp:   1678886400000,
				}
			},
			setupRepoMock: func(mockRepo *MockOrdersRepo) {
				mockRepo.On("CreateOrder", mock.Anything, mock.Anything, mock.MatchedBy(func(o repository.OrderData) bool {
					return o.ExchangeOrderID == "order-123" &&
						o.InstrumentSymbol == "BTC/USDT" &&
						o.Status == repository.OrderStatusOpen &&
						o.Amount == 1.5 &&
						o.Fee.Float64 == 0.1 &&
						o.FeeAssetSymbol.String == "USDT"
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
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			svc := NewService(logger, nil, client, container)

			resp, err := svc.CreateOrder(context.Background(), "binance", "BTC/USDT", repository.OrderSideBuy, repository.OrderTypeLimit, 1.5, 50000.0)

			if tc.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrContains)
			} else {
				require.NoError(t, err)
			}

			if tc.expectedOrderID != "" {
				assert.Equal(t, tc.expectedOrderID, resp.ExchangeOrderID)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestService_CreateStopOrder(t *testing.T) {
	testCases := []struct {
		name                string
		limitPrice          float64
		setupGatewayMock    func(*mockExchangeServer)
		setupRepoMock       func(*MockOrdersRepo)
		expectedErrContains string
		expectedOrderID     string
	}{
		{
			name:       "Success Stop Market",
			limitPrice: 0,
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.createStopOrderResponse = &pb.OrderResponse{
					Id:     "stop-123",
					Symbol: "BTC/USDT",
					Status: repository.OrderStatusOpen,
					Fee:    0,
				}
			},
			setupRepoMock: func(mockRepo *MockOrdersRepo) {
				mockRepo.On("CreateOrder", mock.Anything, mock.Anything, mock.MatchedBy(func(o repository.OrderData) bool {
					return o.ExchangeOrderID == "stop-123" &&
						o.OrderType == repository.OrderTypeStopMarket &&
						o.Price.Float64 == 50000.0 &&
						!o.Fee.Valid
				})).Return(int64(1), nil)
			},
			expectedOrderID: "stop-123",
		},
		{
			name:       "Success Stop Limit",
			limitPrice: 49500.0,
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.createStopOrderResponse = &pb.OrderResponse{
					Id:     "stop-limit-123",
					Symbol: "BTC/USDT",
					Status: repository.OrderStatusOpen,
				}
			},
			setupRepoMock: func(mockRepo *MockOrdersRepo) {
				mockRepo.On("CreateOrder", mock.Anything, mock.Anything, mock.MatchedBy(func(o repository.OrderData) bool {
					return o.ExchangeOrderID == "stop-limit-123" &&
						o.OrderType == repository.OrderTypeStopLimit &&
						o.Price.Float64 == 50000.0
				})).Return(int64(2), nil)
			},
			expectedOrderID: "stop-limit-123",
		},
		{
			name: "Gateway Error",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.createStopOrderError = status.Error(codes.Unavailable, "gateway down")
			},
			setupRepoMock:       func(mockRepo *MockOrdersRepo) {},
			expectedErrContains: "gateway down",
		},
		{
			name: "DB Persistence Error",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.createStopOrderResponse = &pb.OrderResponse{
					Id:     "stop-123",
					Symbol: "BTC/USDT",
					Status: repository.OrderStatusOpen,
				}
			},
			setupRepoMock: func(mockRepo *MockOrdersRepo) {
				mockRepo.On("CreateOrder", mock.Anything, mock.Anything, mock.Anything).Return(int64(0), errors.New("db error"))
			},
			expectedErrContains: "stop order created but failed to persist",
			expectedOrderID:     "stop-123",
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
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			svc := NewService(logger, nil, client, container)

			resp, err := svc.CreateStopOrder(context.Background(), "binance", "BTC/USDT", repository.OrderSideSell, 0.1, 50000.0, tc.limitPrice)

			if tc.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedOrderID, resp.ExchangeOrderID)
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
					Id:          "order-123",
					Symbol:      "BTC/USDT",
					Status:      repository.OrderStatusCanceled,
					Filled:      0.5,
					Remaining:   1.0,
					Cost:        10000.0,
					Fee:         0.05,
					FeeCurrency: "USDT",
					Timestamp:   1678886400000,
				}
			},
			setupRepoMock: func(mockRepo *MockOrdersRepo) {
				mockRepo.On("UpdateOrder", mock.Anything, mock.Anything, mock.MatchedBy(func(o repository.OrderData) bool {
					return o.ExchangeOrderID == "order-123" &&
						o.Status == repository.OrderStatusCanceled &&
						o.Filled == 0.5 &&
						o.Fee.Float64 == 0.05
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
		{
			name: "DB Not Found Fallback",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.cancelOrderResponse = &pb.CancelOrderResponse{Id: "order-123", Status: repository.OrderStatusCanceled}
				mockSrv.getOrderResponse = &pb.OrderResponse{
					Id:     "order-123",
					Symbol: "BTC/USDT",
					Status: repository.OrderStatusCanceled,
				}
			},
			setupRepoMock: func(mockRepo *MockOrdersRepo) {
				mockRepo.On("UpdateOrder", mock.Anything, mock.Anything, mock.Anything).Return(
					int64(0), pgx.ErrNoRows,
				)
				mockRepo.On("CreateOrder", mock.Anything, mock.Anything, mock.Anything).Return(
					int64(1), nil,
				)
			},
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
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			svc := NewService(logger, nil, client, container)

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
					Id:          "order-123",
					Symbol:      "BTC/USDT",
					Status:      repository.OrderStatusClosed,
					Filled:      1.5,
					Remaining:   0,
					Cost:        75000.0,
					Fee:         0.2,
					FeeCurrency: "USDT",
					Timestamp:   1678886400000,
				}
			},
			setupRepoMock: func(mockRepo *MockOrdersRepo) {
				mockRepo.On("UpdateOrder", mock.Anything, mock.Anything, mock.MatchedBy(func(o repository.OrderData) bool {
					return o.ExchangeOrderID == "order-123" &&
						o.Status == repository.OrderStatusClosed &&
						o.Filled == 1.5 &&
						o.Fee.Float64 == 0.2
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
		{
			name: "DB Not Found Fallback",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.getOrderResponse = &pb.OrderResponse{
					Id:     "order-123",
					Symbol: "BTC/USDT",
					Status: repository.OrderStatusClosed,
				}
			},
			setupRepoMock: func(mockRepo *MockOrdersRepo) {
				mockRepo.On("UpdateOrder", mock.Anything, mock.Anything, mock.Anything).Return(
					int64(0), pgx.ErrNoRows,
				)
				mockRepo.On("CreateOrder", mock.Anything, mock.Anything, mock.Anything).Return(
					int64(1), nil,
				)
			},
			expectedOrderID: "order-123",
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
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			svc := NewService(logger, nil, client, container)

			resp, err := svc.GetOrder(context.Background(), "binance", "BTC/USDT", "order-123")

			if tc.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrContains)
			} else {
				require.NoError(t, err)
			}

			if tc.expectedOrderID != "" {
				assert.Equal(t, tc.expectedOrderID, resp.ExchangeOrderID)
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
				mockSrv.getOpenOrdersResponse = &pb.OrdersResponse{
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
			name: "Success with CreateOrder fallback",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.getOpenOrdersResponse = &pb.OrdersResponse{
					Orders: []*pb.OrderResponse{
						{
							Id:     "order-new",
							Symbol: "BTC/USDT",
							Status: repository.OrderStatusOpen,
						},
					},
				}
			},
			setupRepoMock: func(mockRepo *MockOrdersRepo) {
				mockRepo.On("UpdateOrder", mock.Anything, mock.Anything, mock.Anything).Return(int64(0), pgx.ErrNoRows)
				mockRepo.On("CreateOrder", mock.Anything, mock.Anything, mock.MatchedBy(func(o repository.OrderData) bool {
					return o.ExchangeOrderID == "order-new"
				})).Return(int64(10), nil)
			},
			expectedCount: 1,
		},
		{
			name: "Gateway Error",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.getOpenOrdersError = status.Error(codes.Unavailable, "gateway down")
			},
			setupRepoMock:       func(mockRepo *MockOrdersRepo) {},
			expectedErrContains: "failed to fetch open orders",
		},
		{
			name: "DB Generic Error Continue",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.getOpenOrdersResponse = &pb.OrdersResponse{
					Orders: []*pb.OrderResponse{
						{Id: "order-1", Symbol: "BTC/USDT", Status: repository.OrderStatusOpen},
					},
				}
			},
			setupRepoMock: func(mockRepo *MockOrdersRepo) {
				// If UpdateOrder returns an error other than ErrNoRows, it logs a warning and continues.
				mockRepo.On("UpdateOrder", mock.Anything, mock.Anything, mock.Anything).Return(
					int64(0), errors.New("db generic error"),
				)
			},
			expectedCount: 0,
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
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			svc := NewService(logger, nil, client, container)

			resp, err := svc.GetOpenOrders(context.Background(), "binance", "BTC/USDT", 10)

			if tc.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrContains)
			} else {
				require.NoError(t, err)
				assert.Len(t, resp, tc.expectedCount)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestService_GetRecentTrades(t *testing.T) {
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
				mockSrv.getOpenOrdersResponse = &pb.OrdersResponse{ // Reuse mock field for simplicity
					Orders: []*pb.OrderResponse{
						{
							Id:          "trade-1",
							Symbol:      "BTC/USDT",
							Status:      repository.OrderStatusClosed,
							Filled:      1.0,
							Fee:         0.1,
							FeeCurrency: "USDT",
						},
					},
				}
			},
			setupRepoMock: func(mockRepo *MockOrdersRepo) {
				mockRepo.On("UpdateOrder", mock.Anything, mock.Anything, mock.MatchedBy(func(o repository.OrderData) bool {
					return o.ExchangeOrderID == "trade-1" && o.Fee.Float64 == 0.1
				})).Return(int64(1), nil)
			},
			expectedCount: 1,
		},
		{
			name: "Success with CreateOrder fallback",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.getOpenOrdersResponse = &pb.OrdersResponse{
					Orders: []*pb.OrderResponse{
						{
							Id:     "trade-new",
							Symbol: "BTC/USDT",
							Status: repository.OrderStatusClosed,
						},
					},
				}
			},
			setupRepoMock: func(mockRepo *MockOrdersRepo) {
				mockRepo.On("UpdateOrder", mock.Anything, mock.Anything, mock.Anything).Return(int64(0), pgx.ErrNoRows)
				mockRepo.On("CreateOrder", mock.Anything, mock.Anything, mock.MatchedBy(func(o repository.OrderData) bool {
					return o.ExchangeOrderID == "trade-new"
				})).Return(int64(10), nil)
			},
			expectedCount: 1,
		},
		{
			name: "Gateway Error",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.getOpenOrdersError = status.Error(codes.Unavailable, "gateway down")
			},
			setupRepoMock:       func(mockRepo *MockOrdersRepo) {},
			expectedErrContains: "failed to fetch trades from gateway",
		},
		{
			name: "DB Generic Error Continue",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.getOpenOrdersResponse = &pb.OrdersResponse{
					Orders: []*pb.OrderResponse{
						{Id: "trade-1", Symbol: "BTC/USDT", Status: repository.OrderStatusClosed},
					},
				}
			},
			setupRepoMock: func(mockRepo *MockOrdersRepo) {
				// If UpdateOrder returns an error other than ErrNoRows, it logs a warning and continues.
				mockRepo.On("UpdateOrder", mock.Anything, mock.Anything, mock.Anything).Return(
					int64(0), errors.New("db generic error"),
				)
			},
			expectedCount: 0,
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
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			svc := NewService(logger, nil, client, container)

			resp, err := svc.GetRecentTrades(context.Background(), "binance", "BTC/USDT", 0, 10)

			if tc.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrContains)
			} else {
				require.NoError(t, err)
				assert.Len(t, resp, tc.expectedCount)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}
