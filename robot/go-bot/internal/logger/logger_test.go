//go:build unit

package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"trading/robot/go-bot/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetup(t *testing.T) {
	// Setup modifies global state, so we restore it
	oldDefault := slog.Default()
	defer slog.SetDefault(oldDefault)

	cfg := config.LogConfig{Level: "info", Format: "text"}
	Setup(cfg)

	assert.NotNil(t, slog.Default())
	// We check if the default logger was indeed replaced
	assert.NotEqual(t, oldDefault, slog.Default())
}

func TestNew(t *testing.T) {
	testCases := []struct {
		name     string
		cfg      config.LogConfig
		logFunc  func(*slog.Logger)
		validate func(*testing.T, string)
	}{
		{
			name:    "JSON format - Info Level",
			cfg:     config.LogConfig{Level: "info", Format: "json", Source: true},
			logFunc: func(l *slog.Logger) { l.Info("msg", "key", "val") },
			validate: func(t *testing.T, out string) {
				var data map[string]any
				err := json.Unmarshal([]byte(out), &data)
				require.NoError(t, err)
				assert.Equal(t, "INFO", data["level"])
				assert.Equal(t, "msg", data["msg"])
				assert.Equal(t, "val", data["key"])
				assert.Contains(t, data, "source")
			},
		},
		{
			name:    "Text format - Debug Level",
			cfg:     config.LogConfig{Level: "debug", Format: "text", Source: false},
			logFunc: func(l *slog.Logger) { l.Debug("debug msg") },
			validate: func(t *testing.T, out string) {
				assert.Contains(t, out, "level=DEBUG")
				assert.Contains(t, out, "msg=\"debug msg\"")
			},
		},
		{
			name:    "Invalid Level defaults to Info",
			cfg:     config.LogConfig{Level: "invalid", Format: "text"},
			logFunc: func(l *slog.Logger) { l.Info("info msg") },
			validate: func(t *testing.T, out string) {
				assert.Contains(t, out, "level=INFO")
			},
		},
		{
			name: "Warn and Error levels",
			cfg:  config.LogConfig{Level: "debug", Format: "text"},
			logFunc: func(l *slog.Logger) {
				l.Warn("warn msg")
				l.Error("error msg")
			},
			validate: func(t *testing.T, out string) {
				assert.Contains(t, out, "level=WARN")
				assert.Contains(t, out, "level=ERROR")
			},
		},
		{
			name:    "Invalid Format defaults to Text",
			cfg:     config.LogConfig{Level: "info", Format: "unknown"},
			logFunc: func(l *slog.Logger) { l.Info("msg") },
			validate: func(t *testing.T, out string) {
				assert.Contains(t, out, "level=INFO")
				assert.NotContains(t, out, "{")
			},
		},
		{
			name:    "Time and Source Formatting",
			cfg:     config.LogConfig{Level: "info", Format: "json", Source: true},
			logFunc: func(l *slog.Logger) { l.Info("msg") },
			validate: func(t *testing.T, out string) {
				var data map[string]any
				_ = json.Unmarshal([]byte(out), &data)
				// Check time format (YYYY-MM-DD HH:MM:SS.mmm)
				timeStr, _ := data["time"].(string)
				assert.Regexp(t, `^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3}$`, timeStr)
				// Check source path (relative)
				source, _ := data["source"].(map[string]any)
				file, _ := source["file"].(string)
				assert.Contains(t, file, "internal/logger/logger_test.go")
				assert.NotContains(t, file, "go-bot/internal")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			l := New(&buf, tc.cfg)
			tc.logFunc(l)
			tc.validate(t, buf.String())
		})
	}
}
