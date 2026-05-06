package api

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	pb "trading/robot/go-bot/gen/go/v1"
	"trading/robot/go-bot/internal/database/repository"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ManagementServer implements the ManagementService gRPC server.
type ManagementServer struct {
	repos  *repository.Container
	db     repository.DBExecutor
	logger *slog.Logger

	pb.UnimplementedManagementServiceServer
}

func NewManagementServer(
	logger *slog.Logger,
	db repository.DBExecutor,
	repos *repository.Container,
) *ManagementServer {
	return &ManagementServer{
		repos:  repos,
		db:     db,
		logger: logger.With("service", "management_server"),
	}
}

func (s *ManagementServer) UpdateStrategy(
	ctx context.Context, req *pb.UpdateStrategyRequest,
) (*pb.UpdateStrategyResponse, error) {
	s.logger.Info("received strategy update request",
		"symbol", req.GetSymbol(), "type", req.GetStrategyType(), "enabled", req.GetEnabled(),
	)

	// Handle deactivation request early to avoid nil pointer dereference on momentum_params
	if !req.GetEnabled() {
		err := s.repos.Strategies.RequestStrategyDisable(
			ctx, s.db, req.GetExchange(), req.GetSymbol(), req.GetStrategyType(),
		)
		if err != nil {
			s.logger.Error("failed to disable strategy", "error", err)
			return nil, status.Errorf(codes.Internal, "database update failed")
		}

		return &pb.UpdateStrategyResponse{
			Success: true,
			Message: fmt.Sprintf("strategy %s disabled for %s", req.GetStrategyType(), req.GetSymbol()),
		}, nil
	}

	var momentum repository.StrategyMomentum
	var label string

	if req.GetStrategyType() != repository.StrategyDummy {
		params := req.GetMomentumParams()
		if params == nil {
			return nil, status.Errorf(
				codes.InvalidArgument, "momentum_params are required for momentum strategies",
			)
		}

		// Validate type-specific requirements
		switch req.GetStrategyType() {
		case repository.StrategyMomentumProfit:
			if params.ProfitTargetPct == nil {
				return nil, status.Errorf(
					codes.InvalidArgument, "profit_target_pct is required for momentum_profit",
				)
			}
		case repository.StrategyMomentumTrailing:
			if params.ActivationPct == nil {
				return nil, status.Errorf(
					codes.InvalidArgument, "activation_pct is required for momentum_trailing",
				)
			}
			if params.TrailingStopPct == nil {
				return nil, status.Errorf(
					codes.InvalidArgument, "trailing_stop_pct is required for momentum_trailing",
				)
			}
		}

		label = params.GetLabel()
		momentum = repository.StrategyMomentum{
			WindowSeconds: int(params.GetWindowSeconds()),
			RequireAll:    params.GetRequireAll(),
			StopLossPct:   params.GetStopLossPct(),
			ProfitTargetPct: sql.NullFloat64{
				Float64: params.GetProfitTargetPct(),
				Valid:   params.ProfitTargetPct != nil,
			},
			ActivationPct: sql.NullFloat64{
				Float64: params.GetActivationPct(),
				Valid:   params.ActivationPct != nil,
			},
			TrailingStopPct: sql.NullFloat64{
				Float64: params.GetTrailingStopPct(),
				Valid:   params.TrailingStopPct != nil,
			},
		}

		for _, w := range params.GetWindows() {
			momentum.Windows = append(momentum.Windows, repository.MomentumWindow{
				LookbackSeconds: int(w.GetLookbackSeconds()),
				Threshold:       w.GetThreshold(),
			})
		}
	}

	err := s.repos.Strategies.UpsertEnabledStrategy(
		ctx, s.db, req.GetExchange(), req.GetSymbol(), req.GetStrategyType(), label, momentum,
	)

	if err != nil {
		s.logger.Error("failed to update strategy config", "error", err)
		return nil, status.Errorf(codes.Internal, "database update failed")
	}

	return &pb.UpdateStrategyResponse{
		Success: true,
		Message: fmt.Sprintf("strategy %s updated for %s", req.GetStrategyType(), req.GetSymbol()),
	}, nil
}

func (s *ManagementServer) UpdateRisk(
	ctx context.Context, req *pb.UpdateRiskRequest,
) (*pb.UpdateRiskResponse, error) {
	s.logger.Info("received risk update request",
		"symbol", req.GetSymbol(),
		"risk_per_trade", req.GetRiskPerTrade())

	riskPair := repository.RiskPair{
		ExchangeName:     req.GetExchange(),
		InstrumentSymbol: req.GetSymbol(),
		RiskPerTrade:     req.GetRiskPerTrade(),
		MaxPositionSize: sql.NullFloat64{
			Float64: req.GetMaxPositionSize(), Valid: req.GetMaxPositionSize() > 0,
		},
	}

	if err := s.repos.Risks.UpsertRiskPair(ctx, s.db, riskPair); err != nil {
		s.logger.Error("failed to update risk config", "error", err)
		return nil, status.Errorf(codes.Internal, "database update failed")
	}

	return &pb.UpdateRiskResponse{
		Success: true,
		Message: fmt.Sprintf("risk parameters updated for %s on %s", req.GetSymbol(), req.GetExchange()),
	}, nil
}
