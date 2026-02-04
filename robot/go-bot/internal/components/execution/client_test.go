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
)

// mockExchangeServer is a mock implementation of the ExchangeServiceServer.
type mockExchangeServer struct {
	pb.UnimplementedExchangeServiceServer
	pingResponse          *pb.PingResponse
	pingError             error
	tickerResponse        *pb.TickerResponse
	tickerError           error
	balanceResponse       *pb.BalanceResponse
	balanceError          error
	createOrderResponse   *pb.OrderResponse
	createOrderError      error
	cancelOrderResponse   *pb.CancelOrderResponse
	cancelOrderError      error
	getOrderResponse      *pb.OrderResponse
	getOrderError         error
	getOpenOrdersResponse *pb.OpenOrdersResponse
	getOpenOrdersError    error
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

func (s *mockExchangeServer) GetOpenOrders(ctx context.Context, req *pb.GetOpenOrdersRequest) (*pb.OpenOrdersResponse, error) {
	if s.getOpenOrdersError != nil {
		return nil, s.getOpenOrdersError
	}
	return s.getOpenOrdersResponse, nil
}

// setupTest creates a mock gRPC server and returns a client connected to it via an in-memory buffer.
func setupTest(t *testing.T, mockSrv *mockExchangeServer) (*GatewayClient, func()) {
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
	conn, err := grpc.Dial("bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	client := &GatewayClient{
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

func TestGatewayClient_Ping(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Arrange
		mockSrv := &mockExchangeServer{
			pingResponse: &pb.PingResponse{Message: "mock pong"},
		}
		client, cleanup := setupTest(t, mockSrv)
		defer cleanup()

		// Act
		resp, err := client.Ping(context.Background())

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "mock pong", resp)
	})

	t.Run("Server Error", func(t *testing.T) {
		// Arrange
		mockSrv := &mockExchangeServer{
			pingError: status.Error(codes.Internal, "internal server error"),
		}
		client, cleanup := setupTest(t, mockSrv)
		defer cleanup()

		// Act
		_, err := client.Ping(context.Background())

		// Assert
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
		assert.Contains(t, st.Message(), "internal server error")
	})
}

func TestGatewayClient_CreateOrder(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockSrv := &mockExchangeServer{
			createOrderResponse: &pb.OrderResponse{Id: "123", Symbol: "BTC/USDT", Status: "open"},
		}
		client, cleanup := setupTest(t, mockSrv)
		defer cleanup()

		req := &pb.CreateOrderRequest{Symbol: "BTC/USDT", Side: "buy", Type: "limit", Amount: 1.0, Price: 20000.0}
		resp, err := client.CreateOrder(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, "123", resp.Id)
		assert.Equal(t, "open", resp.Status)
	})

	t.Run("Server Error", func(t *testing.T) {
		mockSrv := &mockExchangeServer{
			createOrderError: status.Error(codes.Internal, "internal server error"),
		}
		client, cleanup := setupTest(t, mockSrv)
		defer cleanup()

		req := &pb.CreateOrderRequest{Symbol: "BTC/USDT"}
		_, err := client.CreateOrder(context.Background(), req)

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
	})
}

func TestGatewayClient_CancelOrder(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockSrv := &mockExchangeServer{
			cancelOrderResponse: &pb.CancelOrderResponse{Id: "123", Status: "canceled"},
		}
		client, cleanup := setupTest(t, mockSrv)
		defer cleanup()

		resp, err := client.CancelOrder(context.Background(), "123", "BTC/USDT", "binance")

		require.NoError(t, err)
		assert.Equal(t, "123", resp.Id)
		assert.Equal(t, "canceled", resp.Status)
	})

	t.Run("Server Error", func(t *testing.T) {
		mockSrv := &mockExchangeServer{
			cancelOrderError: status.Error(codes.Internal, "internal server error"),
		}
		client, cleanup := setupTest(t, mockSrv)
		defer cleanup()

		_, err := client.CancelOrder(context.Background(), "123", "BTC/USDT", "binance")

		require.Error(t, err)
	})
}

func TestGatewayClient_GetOrder(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockSrv := &mockExchangeServer{
			getOrderResponse: &pb.OrderResponse{Id: "123", Symbol: "BTC/USDT", Status: "closed"},
		}
		client, cleanup := setupTest(t, mockSrv)
		defer cleanup()

		resp, err := client.GetOrder(context.Background(), "123", "BTC/USDT", "binance")

		require.NoError(t, err)
		assert.Equal(t, "123", resp.Id)
		assert.Equal(t, "closed", resp.Status)
	})

	t.Run("Server Error", func(t *testing.T) {
		mockSrv := &mockExchangeServer{
			getOrderError: status.Error(codes.Internal, "internal server error"),
		}
		client, cleanup := setupTest(t, mockSrv)
		defer cleanup()

		_, err := client.GetOrder(context.Background(), "123", "BTC/USDT", "binance")

		require.Error(t, err)
	})
}

func TestGatewayClient_GetOpenOrders(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockSrv := &mockExchangeServer{
			getOpenOrdersResponse: &pb.OpenOrdersResponse{
				Orders: []*pb.OrderResponse{
					{Id: "123", Symbol: "BTC/USDT", Status: "open"},
					{Id: "124", Symbol: "BTC/USDT", Status: "open"},
				},
			},
		}
		client, cleanup := setupTest(t, mockSrv)
		defer cleanup()

		resp, err := client.GetOpenOrders(context.Background(), "BTC/USDT", "binance")

		require.NoError(t, err)
		assert.Len(t, resp.Orders, 2)
	})

	t.Run("Server Error", func(t *testing.T) {
		mockSrv := &mockExchangeServer{
			getOpenOrdersError: status.Error(codes.Internal, "internal server error"),
		}
		client, cleanup := setupTest(t, mockSrv)
		defer cleanup()

		_, err := client.GetOpenOrders(context.Background(), "BTC/USDT", "binance")

		require.Error(t, err)
	})
}

func TestNewGatewayClient_ConnectionFailure(t *testing.T) {
	// This is a small integration test to ensure NewGatewayClient fails fast.
	// It attempts to connect to a port that is presumed to be closed.
	cfg := &config.GRPCConfig{PythonGatewayAddress: "localhost:9999"} // Invalid port
	_, err := NewGatewayClient(cfg)
	require.Error(t, err, "NewGatewayClient should fail when the gateway is unreachable")
	assert.Contains(t, err.Error(), "initial health check to python-gateway failed")
}

func TestGatewayClient_GetTicker(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Arrange
		mockSrv := &mockExchangeServer{
			tickerResponse: &pb.TickerResponse{Symbol: "BTC/USDT", Price: 20000.00},
		}
		client, cleanup := setupTest(t, mockSrv)
		defer cleanup()

		// Act
		resp, err := client.GetTicker(context.Background(), "BTC/USDT", "binance")

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "BTC/USDT", resp.Symbol)
		assert.Equal(t, 20000.00, resp.Price)
	})

	t.Run("Server Error", func(t *testing.T) {
		// Arrange
		mockSrv := &mockExchangeServer{
			tickerError: status.Error(codes.Internal, "internal server error"),
		}
		client, cleanup := setupTest(t, mockSrv)
		defer cleanup()

		// Act
		_, err := client.GetTicker(context.Background(), "BTC/USDT", "binance")

		// Assert
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
		assert.Contains(t, st.Message(), "internal server error")
	})
}

func TestGatewayClient_GetBalance(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Arrange
		mockSrv := &mockExchangeServer{
			balanceResponse: &pb.BalanceResponse{
				Total: map[string]float64{"USDT": 1000.00},
				Free:  map[string]float64{"USDT": 500.00},
			},
		}
		client, cleanup := setupTest(t, mockSrv)
		defer cleanup()

		// Act
		resp, err := client.GetBalance(context.Background(), "USDT", "binance")

		// Assert
		require.NoError(t, err)
		assert.Equal(t, 1000.00, resp.Total["USDT"])
		assert.Equal(t, 500.00, resp.Free["USDT"])
	})

	t.Run("Server Error", func(t *testing.T) {
		mockSrv := &mockExchangeServer{
			balanceError: status.Error(codes.Internal, "internal server error"),
		}
		client, cleanup := setupTest(t, mockSrv)
		defer cleanup()

		_, err := client.GetBalance(context.Background(), "USDT", "binance")

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
		assert.Contains(t, st.Message(), "internal server error")
	})
}
