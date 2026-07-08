//go:build unit

package config

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWithDefaults(t *testing.T) {
	cfg := newWithDefaults()
	assert.Equal(t, 10*time.Second, cfg.Server.OrchestratorInterval)
	assert.Equal(t, 10*time.Second, cfg.Server.DefaultExchangeTimeout)
	assert.Equal(t, 10*time.Second, cfg.Server.ShutdownTimeout)
	assert.Equal(t, "info", cfg.Log.Level)
	assert.Equal(t, "text", cfg.Log.Format)
	assert.Equal(t, "", cfg.Log.Path)
	assert.False(t, cfg.Log.Rotate)
	assert.False(t, cfg.Log.Source)
	assert.Equal(t, "disable", cfg.Database.SSLMode)
	assert.Equal(t, 5*time.Second, cfg.GRPC.ConnectionTimeout)
}

func TestLoad(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// Define the path to the test config file
		configPath := filepath.Join("testdata", "success.toml")

		// Call the Load function
		cfg, err := Load(configPath)

		// Assertions
		require.NoError(t, err, "Load function returned an unexpected error")
		require.NotNil(t, cfg, "Config struct should not be nil")

		assert.Equal(t, 10*time.Second, cfg.Server.OrchestratorInterval)
		assert.Equal(t, 1*time.Minute, cfg.Server.RefreshStratInterval)
		assert.Equal(t, 10*time.Second, cfg.Server.DefaultExchangeTimeout)
		assert.Equal(t, 30*time.Second, cfg.Server.ShutdownTimeout)
		assert.Equal(t, "info", cfg.Log.Level)
		assert.Equal(t, "json", cfg.Log.Format)
		assert.Equal(t, "/var/log/go-bot.log", cfg.Log.Path)
		assert.True(t, cfg.Log.Rotate)
		assert.False(t, cfg.Log.Source)
		assert.Equal(t, "localhost", cfg.Database.Host)
		assert.Equal(t, 5432, cfg.Database.Port)
		assert.Equal(t, "testuser", cfg.Database.User)
		assert.Equal(t, "testpassword", cfg.Database.Password)
		assert.Equal(t, "testdb", cfg.Database.DBName)
		assert.Equal(t, "disable", cfg.Database.SSLMode)
		assert.Equal(t, "localhost:50050", cfg.GRPC.GoBotAddress)
		assert.Equal(t, "localhost:50051", cfg.GRPC.PythonGatewayAddress)
		assert.Equal(t, "0.0.0.0:50052", cfg.GRPC.ManagementAddress)
		assert.Equal(t, 5*time.Second, cfg.GRPC.ConnectionTimeout)

		// Verify Health Check Config
		assert.Equal(t, "BTC", cfg.Health.Asset)
		assert.Equal(t, 3*time.Minute, cfg.Health.Interval)
		assert.Equal(t, 2, cfg.Health.RetryAttempts)
		assert.Equal(t, 1*time.Second, cfg.Health.RetryDelay)

		require.Len(t, cfg.Exchanges, 2)
		assert.Equal(t, "binance", cfg.Exchanges[0].Name)
		assert.True(t, cfg.Exchanges[0].SandboxMode)
		assert.True(t, cfg.Exchanges[0].HealthCheck)
		assert.Equal(t, "coinbase", cfg.Exchanges[1].Name)
		assert.False(t, cfg.Exchanges[1].SandboxMode)
		assert.False(t, cfg.Exchanges[1].HealthCheck)

		// Verify Risk Config
		assert.Equal(t, 3, cfg.Risk.MaxOpenPositions)
		assert.Equal(t, 100.0, cfg.Risk.MaxDailyLoss)
	})

	t.Run("file not found", func(t *testing.T) {
		// Call Load with a non-existent file path
		_, err := Load("non_existent_config.toml")

		// Assert that an error is returned
		require.Error(t, err, "Expected an error for a non-existent file")
		assert.Contains(t, err.Error(), "config file not found", "Error message should indicate file not found")
	})

	t.Run("invalid toml format", func(t *testing.T) {
		configPath := filepath.Join("testdata", "invalid.toml")
		// Call Load with the invalid file
		_, err := Load(configPath)

		// Assert that a decoding error occurred
		require.Error(t, err, "Expected an error for invalid TOML format")
		assert.Contains(t, err.Error(), "failed to decode config file", "Error message should indicate decoding failure")
	})
}
