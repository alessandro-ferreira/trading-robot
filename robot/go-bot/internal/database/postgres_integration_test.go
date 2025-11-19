//go:build integration

package database

import (
	"context"
	"os"
	"testing"

	"trading/robot/go-bot/internal/config"

	"github.com/stretchr/testify/require"
)

// TestNewDBPool_Integration requires a running PostgreSQL database.
// preferably started via `docker-compose up test-db`.
// It uses default credentials for the Docker container but allows overriding
// via environment variables: DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME.
//
// To run this test, use the 'integration' build tag:
// go test -v -tags=integration ./...
func TestNewDBPool_Integration(t *testing.T) {
	// getEnv returns the value of an environment variable or a default value.
	getEnv := func(key, defaultValue string) string {
		if value, exists := os.LookupEnv(key); exists {
			return value
		}
		return defaultValue
	}

	dbConfig := config.DatabaseConfig{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     5433, // Default to the port exposed by Docker Compose
		User:     getEnv("DB_USER", "testuser"),
		Password: getEnv("DB_PASSWORD", "testpassword"),
		DBName:   getEnv("DB_NAME", "testdb"),
		SSLMode:  getEnv("DB_SSLMODE", "disable"),
	}

	// A simple check to see if we can connect. If not, we skip the test.
	// This is useful for environments where Docker might not be running.
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
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
}
