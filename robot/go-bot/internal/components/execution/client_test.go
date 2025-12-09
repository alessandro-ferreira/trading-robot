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
	pingResponse *pb.PingResponse
	pingError    error
}

func (s *mockExchangeServer) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
	if s.pingError != nil {
		return nil, s.pingError
	}
	return s.pingResponse, nil
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

func TestNewGatewayClient_ConnectionFailure(t *testing.T) {
	// This is a small integration test to ensure NewGatewayClient fails fast.
	// It attempts to connect to a port that is presumed to be closed.
	cfg := &config.GRPCConfig{PythonGatewayAddress: "localhost:9999"} // Invalid port
	_, err := NewGatewayClient(cfg)
	require.Error(t, err, "NewGatewayClient should fail when the gateway is unreachable")
	assert.Contains(t, err.Error(), "initial health check to python-gateway failed")
}
