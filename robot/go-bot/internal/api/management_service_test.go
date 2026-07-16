//go:build unit

package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	pb "trading/robot/go-bot/gen/go/v1"
	"trading/robot/go-bot/internal/database/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockStrategiesRepo is a mock implementation of the StrategiesRepo.
type MockStrategiesRepo struct {
	mock.Mock
}

func (m *MockStrategiesRepo) GetStrategyPairs(ctx context.Context, db repository.DBExecutor, statuses []string) ([]repository.StrategyPair, error) {
	args := m.Called(ctx, db, statuses)
	return args.Get(0).([]repository.StrategyPair), args.Error(1)
}

func (m *MockStrategiesRepo) UpsertEnabledStrategy(ctx context.Context, exec repository.DBExecutor, exchange, symbol, strategyType, label string, momentum repository.StrategyMomentum) error {
	args := m.Called(ctx, exec, exchange, symbol, strategyType, label, momentum)
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

// MockRiskRepo is a mock implementation of the RiskRepo.
type MockRiskRepo struct {
	mock.Mock
}

// optionalFloat is a helper to get a pointer to a float64 literal for optional gRPC fields.
func optionalFloat(f float64) *float64 {
	return &f
}

func (m *MockRiskRepo) GetRiskPair(ctx context.Context, db repository.DBExecutor, exchange, symbol string) (repository.RiskPair, error) {
	args := m.Called(ctx, db, exchange, symbol)
	return args.Get(0).(repository.RiskPair), args.Error(1)
}

func (m *MockRiskRepo) UpsertRiskPair(ctx context.Context, exec repository.DBExecutor, riskPair repository.RiskPair) error {
	args := m.Called(ctx, exec, riskPair)
	return args.Error(0)
}

// --- Helpers ---

func setupManagementServer(t *testing.T) (*ManagementServer, *MockStrategiesRepo, *MockRiskRepo) {
	mockLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockStrategies := new(MockStrategiesRepo)
	mockRisks := new(MockRiskRepo)
	repos := &repository.Container{
		Strategies: mockStrategies,
		Risks:      mockRisks,
	}
	server := NewManagementServer(mockLogger, nil, repos)
	return server, mockStrategies, mockRisks
}

func TestManagementServer_UpdateStrategy(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name        string
		req         *pb.UpdateStrategyRequest
		setup       func(m *MockStrategiesRepo)
		wantErr     bool
		errContains string
		expectedMsg string
	}{
		{
			name: "successful strategy update",
			req: &pb.UpdateStrategyRequest{
				Exchange:     "binance",
				Symbol:       "BTC/USDT",
				StrategyType: "momentum_trailing",
				Enabled:      true,
				MomentumParams: &pb.MomentumParams{
					Label:           "default",
					WindowSeconds:   10,
					RequireAll:      false,
					StopLossPct:     0.1,
					ProfitTargetPct: optionalFloat(0.5 * 0.01),
					ActivationPct:   optionalFloat(0.5 * 0.01),
					TrailingStopPct: optionalFloat(0.2 * 0.01),
					Windows: []*pb.MomentumWindow{
						{LookbackSeconds: 10, Threshold: 0.1 * 0.01},
					},
				},
			},
			setup: func(m *MockStrategiesRepo) {
				m.On("UpsertEnabledStrategy", ctx, nil, "binance", "BTC/USDT", "momentum_trailing", "default", mock.AnythingOfType("repository.StrategyMomentum")).Return(nil).Once()
			},
			expectedMsg: "strategy momentum_trailing updated for BTC/USDT",
		},
		{
			name: "successful strategy disable",
			req: &pb.UpdateStrategyRequest{
				Exchange:     "binance",
				Symbol:       "BTC/USDT",
				StrategyType: "momentum_trailing",
				Enabled:      false,
			},
			setup: func(m *MockStrategiesRepo) {
				m.On("RequestStrategyDisable", ctx, nil, "binance", "BTC/USDT", "momentum_trailing").Return(nil).Once()
			},
			expectedMsg: "strategy momentum_trailing disabled for BTC/USDT",
		},
		{
			name: "strategy update with database error",
			req: &pb.UpdateStrategyRequest{
				Exchange:     "binance",
				Symbol:       "BTC/USDT",
				StrategyType: "momentum_trailing",
				Enabled:      true,
				MomentumParams: &pb.MomentumParams{
					Label:           "default",
					WindowSeconds:   10,
					ActivationPct:   optionalFloat(0.05),
					TrailingStopPct: optionalFloat(0.02),
				},
			},
			setup: func(m *MockStrategiesRepo) {
				m.On("UpsertEnabledStrategy", ctx, nil, "binance", "BTC/USDT", "momentum_trailing", "default", mock.AnythingOfType("repository.StrategyMomentum")).Return(errors.New("db error")).Once()
			},
			wantErr:     true,
			errContains: "database update failed",
		},
		{
			name: "strategy disable with database error",
			req: &pb.UpdateStrategyRequest{
				Exchange:     "binance",
				Symbol:       "BTC/USDT",
				StrategyType: "momentum_trailing",
				Enabled:      false,
			},
			setup: func(m *MockStrategiesRepo) {
				m.On("RequestStrategyDisable", ctx, nil, "binance", "BTC/USDT", "momentum_trailing").Return(errors.New("db error")).Once()
			},
			wantErr:     true,
			errContains: "database update failed",
		},
		{
			name: "successful dummy strategy update without params",
			req: &pb.UpdateStrategyRequest{
				Exchange:     "binance",
				Symbol:       "BTC/USDT",
				StrategyType: "dummy",
				Enabled:      true,
			},
			setup: func(m *MockStrategiesRepo) {
				m.On("UpsertEnabledStrategy", ctx, nil, "binance", "BTC/USDT", "dummy", "", mock.AnythingOfType("repository.StrategyMomentum")).Return(nil).Once()
			},
			expectedMsg: "strategy dummy updated for BTC/USDT",
		},
		{
			name: "momentum strategy update without params failure",
			req: &pb.UpdateStrategyRequest{
				Exchange:     "binance",
				Symbol:       "BTC/USDT",
				StrategyType: "momentum_trailing",
				Enabled:      true,
			},
			setup:       func(m *MockStrategiesRepo) {},
			wantErr:     true,
			errContains: "momentum_params are required",
		},
		{
			name: "momentum_profit without profit_target_pct failure",
			req: &pb.UpdateStrategyRequest{
				Exchange:     "binance",
				Symbol:       "BTC/USDT",
				StrategyType: "momentum_profit",
				Enabled:      true,
				MomentumParams: &pb.MomentumParams{
					Label:         "default",
					WindowSeconds: 10,
				},
			},
			setup:       func(m *MockStrategiesRepo) {},
			wantErr:     true,
			errContains: "profit_target_pct is required",
		},
		{
			name: "momentum_trailing without activation_pct failure",
			req: &pb.UpdateStrategyRequest{
				Exchange:     "binance",
				Symbol:       "BTC/USDT",
				StrategyType: "momentum_trailing",
				Enabled:      true,
				MomentumParams: &pb.MomentumParams{
					Label:           "default",
					WindowSeconds:   10,
					TrailingStopPct: optionalFloat(0.02),
				},
			},
			setup:       func(m *MockStrategiesRepo) {},
			wantErr:     true,
			errContains: "activation_pct is required",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server, mockStrategies, _ := setupManagementServer(t)
			tc.setup(mockStrategies)
			resp, err := server.UpdateStrategy(ctx, tc.req)
			if tc.wantErr {
				assert.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				} else {
					assert.Contains(t, err.Error(), "database update failed")
				}
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
				assert.True(t, resp.GetSuccess())
				assert.Contains(t, resp.GetMessage(), tc.expectedMsg)
			}
			mockStrategies.AssertExpectations(t)
		})
	}
}

func TestManagementServer_UpdateRisk(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name    string
		req     *pb.UpdateRiskRequest
		setup   func(m *MockRiskRepo)
		wantErr bool
	}{
		{
			name: "successful risk update with max position size",
			req: &pb.UpdateRiskRequest{
				Exchange:        "binance",
				Symbol:          "BTC/USDT",
				AllocatedBudget: 100.0,
				MaxAssetUnits:   1.0,
			},
			setup: func(m *MockRiskRepo) {
				m.On("UpsertRiskPair", ctx, nil, mock.MatchedBy(func(p repository.RiskPair) bool {
					return p.AllocatedBudget == 100.0 && p.MaxAssetUnits.Valid && p.MaxAssetUnits.Float64 == 1.0
				})).Return(nil).Once()
			},
		},
		{
			name: "successful risk update without max position size",
			req: &pb.UpdateRiskRequest{
				Exchange:        "binance",
				Symbol:          "ETH/USDT",
				AllocatedBudget: 50.0,
			},
			setup: func(m *MockRiskRepo) {
				m.On("UpsertRiskPair", ctx, nil, mock.MatchedBy(func(p repository.RiskPair) bool {
					return p.AllocatedBudget == 50.0 && !p.MaxAssetUnits.Valid
				})).Return(nil).Once()
			},
		},
		{
			name: "risk update with database error",
			req: &pb.UpdateRiskRequest{
				Exchange:        "binance",
				Symbol:          "BTC/USDT",
				AllocatedBudget: 100.0,
			},
			setup: func(m *MockRiskRepo) {
				m.On("UpsertRiskPair", ctx, nil, mock.Anything).Return(errors.New("db error")).Once()
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server, _, mockRisks := setupManagementServer(t)
			tc.setup(mockRisks)
			resp, err := server.UpdateRisk(ctx, tc.req)
			if tc.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "database update failed")
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
				assert.True(t, resp.GetSuccess())
			}
			mockRisks.AssertExpectations(t)
		})
	}
}
