package config

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// Define the path to the test config file
		configPath := filepath.Join("testdata", "success.toml")

		// Call the Load function
		cfg, err := Load(configPath)

		// Assertions
		require.NoError(t, err, "Load function returned an unexpected error")
		require.NotNil(t, cfg, "Config struct should not be nil")

		assert.Equal(t, 30*time.Second, cfg.Server.ShutdownTimeout)
		assert.Equal(t, "info", cfg.Log.Level)
		assert.Equal(t, "json", cfg.Log.Format)
		assert.False(t, cfg.Log.Source)
		assert.Equal(t, "localhost", cfg.Database.Host)
		assert.Equal(t, 5432, cfg.Database.Port)
		assert.Equal(t, "testuser", cfg.Database.User)
		assert.Equal(t, "testpassword", cfg.Database.Password)
		assert.Equal(t, "testdb", cfg.Database.DBName)
		assert.Equal(t, "disable", cfg.Database.SSLMode)
		assert.Equal(t, "localhost:50051", cfg.GRPC.PythonGatewayAddress)
		assert.Equal(t, "binance", cfg.Exchange.Name)
		assert.True(t, cfg.Exchange.SandboxMode)
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
