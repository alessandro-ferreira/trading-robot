package execution

import (
	"context"
	"fmt"
	"log/slog"
	"math"

	"trading/robot/go-bot/internal/database"
	"trading/robot/go-bot/internal/database/repository"
)

// Service provides methods for trade execution and order management.
type Service struct {
	logger *slog.Logger
	db     *database.DB
	client *GatewayClient
	repo   *repository.Container
}

// NewService creates a new execution Service.
func NewService(logger *slog.Logger, db *database.DB, client *GatewayClient, repo *repository.Container) *Service {
	return &Service{
		logger: logger,
		db:     db,
		client: client,
		repo:   repo,
	}
}

// GetBalance retrieves the balance for a specific asset on a specific exchange.
func (s *Service) GetBalance(ctx context.Context, exchangeName, assetSymbol string) error {
	s.logger.Info("Fetching balance from exchange", "exchange", exchangeName, "asset", assetSymbol)

	// Fetch from Exchange via gRPC
	resp, err := s.client.GetBalance(ctx, assetSymbol, exchangeName)
	if err != nil {
		return fmt.Errorf("failed to fetch balance from gateway: %w", err)
	}

	// Extract values (default to 0 if missing)
	free := resp.Free[assetSymbol]
	used := resp.Used[assetSymbol]
	total := resp.Total[assetSymbol]

	// Validate that the numbers add up, accounting for float precision.
	const epsilon = 1e-9
	if math.Abs(total-(free+used)) > epsilon {
		s.logger.Warn(
			"Balance inconsistency detected from exchange",
			"asset", assetSymbol,
			"free", free,
			"used", used,
			"total", total,
			"discrepancy", total-(free+used),
		)
	}

	s.logger.Info("Balance received", "asset", assetSymbol, "free", free, "used", used, "total", total)

	// Persist to Database
	balance := repository.BalanceData{
		ExchangeName: exchangeName,
		AssetSymbol:  assetSymbol,
		Free:         free,
		Used:         used,
		Total:        total,
	}
	id, err := s.repo.Balances.UpsertBalance(ctx, s.db, balance)
	if err != nil {
		return fmt.Errorf("failed to persist balance: %w", err)
	}

	s.logger.Info("Balance persisted successfully", "balance_id", id)
	return nil
}
