package logger

import (
	"bytes"
	"encoding/json"
	"testing"

	"trading/robot/go-bot/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Run("json format", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := config.LogConfig{
			Level:  "info",
			Format: "json",
		}

		logger := New(&buf, cfg)
		logger.Info("test message", "key", "value")

		// Unmarshal the JSON output and check its fields
		var logOutput map[string]interface{}
		err := json.Unmarshal(buf.Bytes(), &logOutput)
		require.NoError(t, err, "Log output should be valid JSON")

		assert.Equal(t, "INFO", logOutput["level"])
		assert.Equal(t, "test message", logOutput["msg"])
		assert.Equal(t, "value", logOutput["key"])
		assert.Contains(t, logOutput, "time", "Log should contain a time field")
		assert.Contains(t, logOutput, "source", "Log should contain a source field")
	})

	t.Run("text format", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := config.LogConfig{
			Level:  "debug",
			Format: "text",
		}

		logger := New(&buf, cfg)
		// Use a debug message to also test the level
		logger.Debug("test message", "key", "value")

		output := buf.String()

		// Check for the presence of essential parts, making the test robust
		assert.Contains(t, output, "level=DEBUG", "Log output should contain the correct level")
		assert.Contains(t, output, `msg="test message"`, "Log output should contain the correct message")
		assert.Contains(t, output, "key=value", "Log output should contain the key-value pair")
		assert.Contains(t, output, "time=", "Log should contain a time field")
		assert.Contains(t, output, "source=", "Log should contain a source field")
	})
}
