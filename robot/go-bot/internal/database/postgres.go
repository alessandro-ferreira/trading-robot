package database

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"trading/robot/go-bot/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxConnIdleTime   = 5 * time.Minute
	maxConnLifetime   = 2 * time.Hour
	healthCheckPeriod = 5 * time.Minute

	defaultMaxConns               = 10
	defaultConnectTimeout         = 5 * time.Second
	defaultStatementTimeout       = 10 * time.Second
	defaultLockTimeout            = 5 * time.Second
	defaultIdleInTxSessionTimeout = 5 * time.Minute
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

	poolConfig.MaxConns = int32(dbConfig.MaxConns)
	if poolConfig.MaxConns <= 0 {
		poolConfig.MaxConns = defaultMaxConns
	}

	connectionTimeout := dbConfig.ConnectTimeout
	if connectionTimeout <= 0 {
		connectionTimeout = defaultConnectTimeout
	}
	poolConfig.ConnConfig.ConnectTimeout = connectionTimeout

	statementTimeout := dbConfig.StatementTimeout
	if statementTimeout <= 0 {
		statementTimeout = defaultStatementTimeout
	}
	poolConfig.ConnConfig.RuntimeParams["statement_timeout"] = strconv.FormatInt(
		statementTimeout.Milliseconds(), 10,
	)

	lockTimeout := dbConfig.LockTimeout
	if lockTimeout <= 0 {
		lockTimeout = defaultLockTimeout
	}
	poolConfig.ConnConfig.RuntimeParams["lock_timeout"] = strconv.FormatInt(
		lockTimeout.Milliseconds(), 10,
	)

	idleInTxSessionTimeout := dbConfig.IdleInTxSessionTimeout
	if idleInTxSessionTimeout <= 0 {
		idleInTxSessionTimeout = defaultIdleInTxSessionTimeout
	}
	poolConfig.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = strconv.FormatInt(
		idleInTxSessionTimeout.Milliseconds(), 10,
	)

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	return New(pool), nil
}
