package database

import (
	"context"
	"fmt"
	"time"

	"trading/robot/go-bot/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxConnIdleTime   = 5 * time.Minute
	maxConnLifetime   = 2 * time.Hour
	healthCheckPeriod = 5 * time.Minute
	connectTimeout    = 5 * time.Second
)

// NewDBPool creates a new database connection pool and returns it wrapped in our DB struct.
func NewDBPool(ctx context.Context, dbConfig config.DatabaseConfig) (*DB, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		dbConfig.Host, dbConfig.Port, dbConfig.User, dbConfig.Password, dbConfig.DBName, dbConfig.SSLMode)

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	poolConfig.MaxConnIdleTime = maxConnIdleTime
	poolConfig.MaxConnLifetime = maxConnLifetime
	poolConfig.HealthCheckPeriod = healthCheckPeriod
	poolConfig.ConnConfig.ConnectTimeout = connectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return New(pool), nil
}
