//go:build integration

package database

import (
	"context"
	"os"
	"testing"
	"time"

	"trading/robot/go-bot/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestNewDBPool_Integration(t *testing.T) {
	// getEnv returns the value of an environment variable or a default value.
	getEnv := func(key, defaultValue string) string {
		if value, exists := os.LookupEnv(key); exists {
			return value
		}
		return defaultValue
	}

	// DB Config - matches docker-compose.yml test-db
	dbConfig := config.DatabaseConfig{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     5433, // Default to the port exposed by Docker Compose
		User:     getEnv("DB_USER", "testuser"),
		Password: getEnv("DB_PASSWORD", "testpassword"),
		DBName:   getEnv("DB_NAME", "testdb"),
		SSLMode:  getEnv("DB_SSLMODE", "disable"),

		// Set some non-default values to test configuration handling
		MaxConns:               5,
		ConnectTimeout:         time.Second,
		StatementTimeout:       50 * time.Millisecond,
		LockTimeout:            50 * time.Millisecond,
		IdleInTxSessionTimeout: 80 * time.Millisecond,
	}

	// A simple check to see if we can connect. If not, we skip the test.
	// This is useful for environments where Docker might not be running.
	ctx, cancel := context.WithTimeout(context.Background(), dbConfig.ConnectTimeout)
	defer cancel()

	db, err := NewDBPool(ctx, dbConfig)
	// NewDBPool is non-blocking and may not return an error immediately.
	// We must ping the database to confirm a successful connection.
	if err != nil {
		t.Fatalf("NewDBPool failed unexpectedly: %v", err)
	}
	require.NotNil(t, db, "The returned DB struct should not be nil")
	defer db.Close()

	// Ping the database to ensure the connection is alive.
	err = db.Ping(ctx)
	if err != nil {
		t.Skipf("Skipping integration test: could not ping database: %v", err)
	}

	t.Run("max_conns_configured", func(t *testing.T) {
		pool, ok := db.Pool.(*pgxpool.Pool)
		require.True(t, ok, "expected a pgxpool.Pool")
		require.Equal(t, int32(5), pool.Stat().MaxConns())
	})

	t.Run("connect_timeout", func(t *testing.T) {
		unreachableConfig := dbConfig
		unreachableConfig.Host = "10.255.255.1" // Non-routable IP
		unreachableConfig.ConnectTimeout = 100 * time.Millisecond

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		start := time.Now()
		_, err := NewDBPool(ctx, unreachableConfig)

		require.Error(t, err)
		require.Less(t, time.Since(start), 1*time.Second, "connection attempt took too long to time out")
		t.Logf("Connect timeout triggered as expected: %v", err)
	})

	t.Run("statement_timeout", func(t *testing.T) {
		// pg_sleep for 1s should trigger the 50ms statement_timeout
		_, err := db.Exec(context.Background(), "SELECT pg_sleep(1)")
		require.Error(t, err)
		t.Logf("Statement timeout triggered as expected: %v", err)
	})

	t.Run("lock_timeout", func(t *testing.T) {
		// Start a transaction and acquire an exclusive lock on an existing table
		tx1, err := db.Begin(context.Background())
		require.NoError(t, err)
		defer tx1.Rollback(context.Background())

		_, err = tx1.Exec(context.Background(), "LOCK TABLE trading.exchanges IN ACCESS EXCLUSIVE MODE")
		require.NoError(t, err)

		// Start another transaction that attempts to acquire the same lock and should time out
		tx2, err := db.Begin(context.Background())
		require.NoError(t, err)
		defer tx2.Rollback(context.Background())

		start := time.Now()
		_, err = tx2.Exec(context.Background(), "LOCK TABLE trading.exchanges IN ACCESS EXCLUSIVE MODE")
		require.Error(t, err)
		require.Less(t, time.Since(start), 1*time.Second, "lock attempt took too long to time out")
		t.Logf("Lock timeout triggered as expected: %v", err)
	})

	t.Run("idle_in_tx_session_timeout", func(t *testing.T) {
		// Start a transaction and do not commit or rollback
		tx, err := db.Begin(context.Background())
		require.NoError(t, err)
		defer tx.Rollback(context.Background())

		// Wait for longer than the idle_in_tx_session_timeout
		time.Sleep(100 * time.Millisecond)

		// Attempt to execute another query in the same transaction
		_, err = tx.Exec(context.Background(), "SELECT 1")
		require.Error(t, err)
		t.Logf("Idle in transaction session timeout triggered as expected: %v", err)
	})
}
