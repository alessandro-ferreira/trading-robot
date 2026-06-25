package portfolio

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"trading/robot/go-bot/internal/database"
	"trading/robot/go-bot/internal/database/repository"
)

type Portfolio interface {
	// --- Portfolio Management ---
	LoadState(ctx context.Context) error
	RefreshState(ctx context.Context, exchange, instrumentSymbol string) error
	GetActivePositionsCount() int
	GetTotalValue(ctx context.Context) (map[string]float64, error)

	// --- Position Operations (CRUD) ---
	GetPosition(ctx context.Context, exchange, instrumentSymbol string) (repository.PositionData, error)
	CreatePosition(
		ctx context.Context, exchange, instrumentSymbol string, quantity, price float64, orderID int64,
	) error
	UpdatePosition(
		ctx context.Context, exchange, instrumentSymbol string, updates repository.PositionData,
	) error
	DeletePosition(ctx context.Context, exchange, instrumentSymbol string) error
}

// portfolio manages the porfollio of actives holdings of assets.
type portfolio struct {
	mu     sync.RWMutex
	logger *slog.Logger
	db     *database.DB
	repo   *repository.Container

	// positions maps "exchange|symbol" to the actives positions.
	positions map[string]struct{}
}

// NewPortfolio creates a new Portfolio instance.
func NewPortfolio(logger *slog.Logger, db *database.DB, repo *repository.Container) Portfolio {
	return &portfolio{
		logger:    logger,
		db:        db,
		repo:      repo,
		positions: make(map[string]struct{}),
	}
}

// LoadState
func (p *portfolio) LoadState(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	positions, err := p.repo.Positions.GetActivePositions(ctx, p.db, "", "")
	if err != nil {
		return fmt.Errorf("load state: failed to fetch positions: %w", err)
	}

	p.positions = make(map[string]struct{})
	for _, posData := range positions {
		key := makeKey(posData.ExchangeName, posData.InstrumentSymbol)
		p.positions[key] = struct{}{}
	}
	return nil
}

// RefreshState reloads the presence of a specific position from the database.
func (p *portfolio) RefreshState(ctx context.Context, exchange, instrumentSymbol string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := makeKey(exchange, instrumentSymbol)
	if _, err := p.repo.Positions.GetPosition(ctx, p.db, exchange, instrumentSymbol); err != nil {
		delete(p.positions, key)
		return nil
	}
	p.positions[key] = struct{}{}
	return nil
}

// GetActivePositionsCount returns the number of currently active positions.
func (p *portfolio) GetActivePositionsCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.positions)
}

// GetTotalValue returns the aggregated totals for all assets held across all exchanges based on the balances table.
func (p *portfolio) GetTotalValue(ctx context.Context) (map[string]float64, error) {
	totals := make(map[string]float64)

	// Aggregate Cash Balances
	balances, err := p.repo.Balances.GetAllBalances(ctx, p.db, "")
	if err != nil {
		return nil, fmt.Errorf("total value: failed to fetch balances: %w", err)
	}
	for _, b := range balances {
		totals[b.AssetSymbol] += b.Total
	}

	return totals, nil
}

// makeKey creates a unique key for the positions map.
func makeKey(exchange, instrumentSymbol string) string {
	return fmt.Sprintf("%s|%s", exchange, instrumentSymbol)
}
