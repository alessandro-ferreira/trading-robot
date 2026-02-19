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
		assertLogs          func(*testing.T, *bytes.Buffer)
	}{
		{
			name: "Success",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.balanceResponse = &pb.BalanceResponse{
					Free:  map[string]float64{"BTC": 1.5},
					Used:  map[string]float64{"BTC": 0.5},
					Total: map[string]float64{"BTC": 2.0},
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
					Free:  map[string]float64{"BTC": 1.0},
					Used:  map[string]float64{"BTC": 0.0},
					Total: map[string]float64{"BTC": 1.0},
				}
			},
			setupRepoMock: func(mockRepo *MockBalancesRepo) {
				mockRepo.On("UpsertBalance", mock.Anything, mock.Anything, mock.Anything).Return(int64(0), errors.New("db error"))
			},
			expectedErrContains: "failed to persist balance",
		},
		{
			name: "Success with balance inconsistency warning",
			setupGatewayMock: func(mockSrv *mockExchangeServer) {
				mockSrv.balanceResponse = &pb.BalanceResponse{
					Free:  map[string]float64{"BTC": 1.0},
					Used:  map[string]float64{"BTC": 1.0},
					Total: map[string]float64{"BTC": 2.000000001}, // total > free + used, triggers warning
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
			err := svc.GetBalance(context.Background(), "binance", "BTC")

			// Assert
			if tc.expectedErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrContains)
			} else {
				require.NoError(t, err)
			}

			if tc.assertLogs != nil {
				tc.assertLogs(t, &logBuffer)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}
