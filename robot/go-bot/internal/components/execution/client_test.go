package execution

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	pb "trading/robot/go-bot/gen/go/v1"
	"trading/robot/go-bot/internal/config"
	"trading/robot/go-bot/internal/database/repository"
)

// mockExchangeServer is a mock implementation of the ExchangeServiceServer.
type mockExchangeServer struct {
	pb.UnimplementedExchangeServiceServer
	pingResponse            *pb.PingResponse
	pingError               error
	tickerResponse          *pb.TickerResponse
	tickerError             error
	balanceResponse         *pb.BalanceResponse
	balanceError            error
	createOrderResponse     *pb.OrderResponse
	createOrderError        error
	createStopOrderResponse *pb.OrderResponse
	createStopOrderError    error
	cancelOrderResponse     *pb.CancelOrderResponse
	cancelOrderError        error
	getOrderResponse        *pb.OrderResponse
	getOrderError           error
	getOpenOrdersResponse   *pb.OrdersResponse
	getOpenOrdersError      error
	resetStateResponse      *pb.ResetStateResponse
	resetStateError         error
}

func (s *mockExchangeServer) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
	if s.pingError != nil {
		return nil, s.pingError
	}
	return s.pingResponse, nil
}

func (s *mockExchangeServer) GetTicker(ctx context.Context, req *pb.GetTickerRequest) (*pb.TickerResponse, error) {
	if s.tickerError != nil {
		return nil, s.tickerError
	}
	return s.tickerResponse, nil
}

func (s *mockExchangeServer) GetBalance(ctx context.Context, req *pb.GetBalanceRequest) (*pb.BalanceResponse, error) {
	if s.balanceError != nil {
		return nil, s.balanceError
	}
	return s.balanceResponse, nil
}

func (s *mockExchangeServer) CreateOrder(ctx context.Context, req *pb.CreateOrderRequest) (*pb.OrderResponse, error) {
	if s.createOrderError != nil {
		return nil, s.createOrderError
	}
	return s.createOrderResponse, nil
}

func (s *mockExchangeServer) CreateStopOrder(ctx context.Context, req *pb.CreateStopOrderRequest) (*pb.OrderResponse, error) {
	if s.createStopOrderError != nil {
		return nil, s.createStopOrderError
	}
	return s.createStopOrderResponse, nil
}

func (s *mockExchangeServer) CancelOrder(ctx context.Context, req *pb.CancelOrderRequest) (*pb.CancelOrderResponse, error) {
	if s.cancelOrderError != nil {
		return nil, s.cancelOrderError
	}
	return s.cancelOrderResponse, nil
}

func (s *mockExchangeServer) GetOrder(ctx context.Context, req *pb.GetOrderRequest) (*pb.OrderResponse, error) {
	if s.getOrderError != nil {
		return nil, s.getOrderError
	}
	return s.getOrderResponse, nil
}

func (s *mockExchangeServer) GetOpenOrders(ctx context.Context, req *pb.GetOpenOrdersRequest) (*pb.OrdersResponse, error) {
	if s.getOpenOrdersError != nil {
		return nil, s.getOpenOrdersError
	}
	return s.getOpenOrdersResponse, nil
}

func (s *mockExchangeServer) GetRecentTrades(ctx context.Context, req *pb.GetRecentTradesRequest) (*pb.OrdersResponse, error) {
	if s.getOpenOrdersError != nil {
		return nil, s.getOpenOrdersError
	}
	return s.getOpenOrdersResponse, nil
}

func (s *mockExchangeServer) ResetState(ctx context.Context, req *pb.ResetStateRequest) (*pb.ResetStateResponse, error) {
	if s.resetStateError != nil {
		return nil, s.resetStateError
	}
	return s.resetStateResponse, nil
}

// setupTest creates a mock gRPC server and returns a client connected to it via an in-memory buffer.
func setupTest(t *testing.T, mockSrv *mockExchangeServer) (GatewayClient, func()) {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	pb.RegisterExchangeServiceServer(grpcServer, mockSrv)

	go func() {
		if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Logf("server exited with error: %v", err)
		}
	}()

	// Create a client connection to the in-memory listener instead of a real network address.
	conn, err := grpc.NewClient("passthrough://bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	client := &gatewayClient{
		conn:   conn,
		client: pb.NewExchangeServiceClient(conn),
	}

	// Teardown function to close resources.
	cleanup := func() {
		conn.Close()
		grpcServer.Stop()
	}

	return client, cleanup
}

func TestNewGatewayClient_Success(t *testing.T) {
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	addr := lis.Addr().String()

	mockSrv := &mockExchangeServer{
		pingResponse: &pb.PingResponse{Message: "mock pong"},
	}
	grpcServer := grpc.NewServer()
	pb.RegisterExchangeServiceServer(grpcServer, mockSrv)

	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	cfg := &config.GRPCConfig{PythonGatewayAddress: addr}
	client, err := NewGatewayClient(cfg)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Test Close for coverage
	err = client.Close()
	require.NoError(t, err)
}

func TestNewGatewayClient_ConnectionFailure(t *testing.T) {
	// This is a small integration test to ensure NewGatewayClient fails fast.
	// It attempts to connect to a port that is presumed to be closed.
	cfg := &config.GRPCConfig{PythonGatewayAddress: "localhost:9999"} // Invalid port
	_, err := NewGatewayClient(cfg)
	require.Error(t, err, "NewGatewayClient should fail when the gateway is unreachable")
	assert.Contains(t, err.Error(), "initial health check to python-gateway failed")
}

func TestGatewayClient_Ping(t *testing.T) {
	testCases := []struct {
		name            string
		setupMock       func(*mockExchangeServer)
		expectedMessage string
		expectError     bool
		errorCode       codes.Code
	}{
		{
			name: "Success",
			setupMock: func(s *mockExchangeServer) {
				s.pingResponse = &pb.PingResponse{Message: "mock pong"}
			},
			expectedMessage: "mock pong",
		},
		{
			name: "Server Error",
			setupMock: func(s *mockExchangeServer) {
				s.pingError = status.Error(codes.Internal, "internal server error")
			},
			expectError: true,
			errorCode:   codes.Internal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockSrv := &mockExchangeServer{}
			if tc.setupMock != nil {
				tc.setupMock(mockSrv)
			}
			client, cleanup := setupTest(t, mockSrv)
			defer cleanup()

			resp, err := client.Ping(context.Background())

			if tc.expectError {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				assert.Equal(t, tc.errorCode, st.Code())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedMessage, resp)
			}
		})
	}
}

func TestGatewayClient_GetTicker(t *testing.T) {
	testCases := []struct {
		name          string
		setupMock     func(*mockExchangeServer)
		expectedPrice float64
		expectError   bool
		errorCode     codes.Code
	}{
		{
			name: "Success",
			setupMock: func(s *mockExchangeServer) {
				s.tickerResponse = &pb.TickerResponse{Symbol: "BTC/USDT", Price: 20000.00}
			},
			expectedPrice: 20000.00,
		},
		{
			name: "Server Error",
			setupMock: func(s *mockExchangeServer) {
				s.tickerError = status.Error(codes.Internal, "internal server error")
			},
			expectError: true,
			errorCode:   codes.Internal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockSrv := &mockExchangeServer{}
			if tc.setupMock != nil {
				tc.setupMock(mockSrv)
			}
			client, cleanup := setupTest(t, mockSrv)
			defer cleanup()

			resp, err := client.GetTicker(context.Background(), "binance", "BTC/USDT")

			if tc.expectError {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				assert.Equal(t, tc.errorCode, st.Code())
			} else {
				require.NoError(t, err)
				assert.Equal(t, "BTC/USDT", resp.Symbol)
				assert.Equal(t, tc.expectedPrice, resp.Price)
			}
		})
	}
}

func TestGatewayClient_GetBalance(t *testing.T) {
	testCases := []struct {
		name          string
		setupMock     func(*mockExchangeServer)
		expectedTotal float64
		expectedFree  float64
		expectError   bool
		errorCode     codes.Code
	}{
		{
			name: "Success",
			setupMock: func(s *mockExchangeServer) {
				s.balanceResponse = &pb.BalanceResponse{
					Balances: []*pb.BalanceObject{{Asset: "USDT", Free: 500.00, Total: 1000.00}},
				}
			},
			expectedTotal: 1000.00,
			expectedFree:  500.00,
		},
		{
			name: "Server Error",
			setupMock: func(s *mockExchangeServer) {
				s.balanceError = status.Error(codes.Internal, "internal server error")
			},
			expectError: true,
			errorCode:   codes.Internal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockSrv := &mockExchangeServer{}
			if tc.setupMock != nil {
				tc.setupMock(mockSrv)
			}
			client, cleanup := setupTest(t, mockSrv)
			defer cleanup()

			resp, err := client.GetBalance(context.Background(), "binance", "USDT")

			if tc.expectError {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				assert.Equal(t, tc.errorCode, st.Code())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedTotal, resp.Balances[0].Total)
				assert.Equal(t, tc.expectedFree, resp.Balances[0].Free)
			}
		})
	}
}

func TestGatewayClient_CreateOrder(t *testing.T) {
	testCases := []struct {
		name           string
		setupMock      func(*mockExchangeServer)
		req            *pb.CreateOrderRequest
		expectedID     string
		expectedStatus string
		expectError    bool
		errorCode      codes.Code
	}{
		{
			name: "Success",
			setupMock: func(s *mockExchangeServer) {
				s.createOrderResponse = &pb.OrderResponse{Id: "123", Symbol: "BTC/USDT", Status: repository.OrderStatusOpen}
			},
			req:            &pb.CreateOrderRequest{Symbol: "BTC/USDT", Side: repository.OrderSideBuy, Type: repository.OrderTypeLimit, Amount: 1.0},
			expectedID:     "123",
			expectedStatus: repository.OrderStatusOpen,
		},
		{
			name: "Server Error",
			setupMock: func(s *mockExchangeServer) {
				s.createOrderError = status.Error(codes.Internal, "internal server error")
			},
			req:         &pb.CreateOrderRequest{Symbol: "BTC/USDT"},
			expectError: true,
			errorCode:   codes.Internal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockSrv := &mockExchangeServer{}
			if tc.setupMock != nil {
				tc.setupMock(mockSrv)
			}
			client, cleanup := setupTest(t, mockSrv)
			defer cleanup()

			resp, err := client.CreateOrder(context.Background(), tc.req)

			if tc.expectError {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				assert.Equal(t, tc.errorCode, st.Code())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedID, resp.Id)
				assert.Equal(t, tc.expectedStatus, resp.Status)
			}
		})
	}
}

func TestGatewayClient_CreateStopOrder(t *testing.T) {
	testCases := []struct {
		name           string
		setupMock      func(*mockExchangeServer)
		req            *pb.CreateStopOrderRequest
		expectedID     string
		expectedStatus string
		expectError    bool
		errorCode      codes.Code
	}{
		{
			name: "Success",
			setupMock: func(s *mockExchangeServer) {
				s.createStopOrderResponse = &pb.OrderResponse{Id: "stop-123", Symbol: "BTC/USDT", Status: repository.OrderStatusOpen}
			},
			req:            &pb.CreateStopOrderRequest{Symbol: "BTC/USDT", Side: repository.OrderSideBuy, Amount: 1.0, StopPrice: 50000.0},
			expectedID:     "stop-123",
			expectedStatus: repository.OrderStatusOpen,
		},
		{
			name: "Server Error",
			setupMock: func(s *mockExchangeServer) {
				s.createStopOrderError = status.Error(codes.Internal, "internal server error")
			},
			req:         &pb.CreateStopOrderRequest{Symbol: "BTC/USDT"},
			expectError: true,
			errorCode:   codes.Internal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockSrv := &mockExchangeServer{}
			if tc.setupMock != nil {
				tc.setupMock(mockSrv)
			}
			client, cleanup := setupTest(t, mockSrv)
			defer cleanup()

			resp, err := client.CreateStopOrder(context.Background(), tc.req)

			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedID, resp.Id)
				assert.Equal(t, tc.expectedStatus, resp.Status)
			}
		})
	}
}

func TestGatewayClient_CancelOrder(t *testing.T) {
	testCases := []struct {
		name           string
		setupMock      func(*mockExchangeServer)
		expectedID     string
		expectedStatus string
		expectError    bool
	}{
		{
			name: "Success",
			setupMock: func(s *mockExchangeServer) {
				s.cancelOrderResponse = &pb.CancelOrderResponse{Id: "123", Status: repository.OrderStatusCanceled}
			},
			expectedID:     "123",
			expectedStatus: repository.OrderStatusCanceled,
		},
		{
			name: "Server Error",
			setupMock: func(s *mockExchangeServer) {
				s.cancelOrderError = status.Error(codes.Internal, "internal server error")
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockSrv := &mockExchangeServer{}
			if tc.setupMock != nil {
				tc.setupMock(mockSrv)
			}
			client, cleanup := setupTest(t, mockSrv)
			defer cleanup()

			resp, err := client.CancelOrder(context.Background(), "binance", "BTC/USDT", "123")

			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedID, resp.Id)
				assert.Equal(t, tc.expectedStatus, resp.Status)
			}
		})
	}
}

func TestGatewayClient_GetOrder(t *testing.T) {
	testCases := []struct {
		name           string
		setupMock      func(*mockExchangeServer)
		expectedID     string
		expectedStatus string
		expectError    bool
	}{
		{
			name: "Success",
			setupMock: func(s *mockExchangeServer) {
				s.getOrderResponse = &pb.OrderResponse{Id: "123", Symbol: "BTC/USDT", Status: repository.OrderStatusClosed}
			},
			expectedID:     "123",
			expectedStatus: repository.OrderStatusClosed,
		},
		{
			name: "Server Error",
			setupMock: func(s *mockExchangeServer) {
				s.getOrderError = status.Error(codes.Internal, "internal server error")
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockSrv := &mockExchangeServer{}
			if tc.setupMock != nil {
				tc.setupMock(mockSrv)
			}
			client, cleanup := setupTest(t, mockSrv)
			defer cleanup()

			resp, err := client.GetOrder(context.Background(), "binance", "BTC/USDT", "123")

			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedID, resp.Id)
				assert.Equal(t, tc.expectedStatus, resp.Status)
			}
		})
	}
}

func TestGatewayClient_GetOpenOrders(t *testing.T) {
	testCases := []struct {
		name        string
		setupMock   func(*mockExchangeServer)
		expectedLen int
		expectError bool
	}{
		{
			name: "Success",
			setupMock: func(s *mockExchangeServer) {
				s.getOpenOrdersResponse = &pb.OrdersResponse{
					Orders: []*pb.OrderResponse{
						{Id: "123", Symbol: "BTC/USDT", Status: repository.OrderStatusOpen},
						{Id: "124", Symbol: "BTC/USDT", Status: repository.OrderStatusOpen},
					},
				}
			},
			expectedLen: 2,
		},
		{
			name: "Server Error",
			setupMock: func(s *mockExchangeServer) {
				s.getOpenOrdersError = status.Error(codes.Internal, "internal server error")
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockSrv := &mockExchangeServer{}
			if tc.setupMock != nil {
				tc.setupMock(mockSrv)
			}
			client, cleanup := setupTest(t, mockSrv)
			defer cleanup()

			resp, err := client.GetOpenOrders(context.Background(), "binance", "BTC/USDT", 10)

			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, resp.Orders, tc.expectedLen)
			}
		})
	}
}

func TestGatewayClient_GetRecentTrades(t *testing.T) {
	testCases := []struct {
		name        string
		setupMock   func(*mockExchangeServer)
		expectedLen int
		expectError bool
	}{
		{
			name: "Success",
			setupMock: func(s *mockExchangeServer) {
				s.getOpenOrdersResponse = &pb.OrdersResponse{
					Orders: []*pb.OrderResponse{
						{Id: "123", Symbol: "BTC/USDT", Status: repository.OrderStatusClosed},
						{Id: "124", Symbol: "BTC/USDT", Status: repository.OrderStatusClosed},
					},
				}
			},
			expectedLen: 2,
		},
		{
			name: "Server Error",
			setupMock: func(s *mockExchangeServer) {
				s.getOpenOrdersError = status.Error(codes.Internal, "internal server error")
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockSrv := &mockExchangeServer{}
			if tc.setupMock != nil {
				tc.setupMock(mockSrv)
			}
			client, cleanup := setupTest(t, mockSrv)
			defer cleanup()

			resp, err := client.GetRecentTrades(context.Background(), "binance", "BTC/USDT", 0, 10)

			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, resp.Orders, tc.expectedLen)
			}
		})
	}
}

func TestGatewayClient_ResetState(t *testing.T) {
	testCases := []struct {
		name        string
		setupMock   func(*mockExchangeServer)
		expectError bool
	}{
		{
			name: "Success",
			setupMock: func(s *mockExchangeServer) {
				s.resetStateResponse = &pb.ResetStateResponse{Status: "OK"}
			},
		},
		{
			name: "Server Error",
			setupMock: func(s *mockExchangeServer) {
				s.resetStateError = status.Error(codes.Internal, "internal server error")
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockSrv := &mockExchangeServer{}
			if tc.setupMock != nil {
				tc.setupMock(mockSrv)
			}
			client, cleanup := setupTest(t, mockSrv)
			defer cleanup()

			_, err := client.ResetState(context.Background())

			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
