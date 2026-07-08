//go:build unit

package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestSetup_File(t *testing.T) {
	oldDefault := slog.Default()
	defer slog.SetDefault(oldDefault)

	tmpFile, err := os.CreateTemp("", "test_log_*.log")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	cfg := config.LogConfig{Level: "info", Format: "text", Path: tmpFile.Name()}
	Setup(cfg)

	slog.Info("file log message")

	content, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)
	assert.Contains(t, string(content), "level=INFO")
	assert.Contains(t, string(content), "msg=\"file log message\"")
}

func TestSetup_Rotate(t *testing.T) {
	oldDefault := slog.Default()
	defer slog.SetDefault(oldDefault)

	tmpDir, err := os.MkdirTemp("", "test_rotate_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "bot.log")
	cfg := config.LogConfig{Level: "info", Format: "text", Path: logPath, Rotate: true}
	Setup(cfg)

	slog.Info("rotate log message")

	// Expect bot-YYYY-MM-DD.log
	date := time.Now().Format("2006-01-02")
	expectedPath := filepath.Join(tmpDir, "bot-"+date+".log")

	content, err := os.ReadFile(expectedPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "msg=\"rotate log message\"")
}

func TestDailyRotatingWriter_Rotation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test_dw_rotate_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	basePath := filepath.Join(tmpDir, "test.log")
	writer := NewDailyRotatingWriter(basePath, true)

	// Mock lastDate to yesterday
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	writer.lastDate = yesterday

	// Manually open an "old" file
	oldPath := filepath.Join(tmpDir, "test-"+yesterday+".log")
	oldFile, err := os.OpenFile(oldPath, os.O_CREATE|os.O_WRONLY, 0644)
	require.NoError(t, err)
	writer.current = oldFile

	// Write something - should trigger rotation
	msg := []byte("new day message\n")
	_, err = writer.Write(msg)
	require.NoError(t, err)

	date := time.Now().Format("2006-01-02")
	newPath := filepath.Join(tmpDir, "test-"+date+".log")

	// Check if new file exists and has content
	content, err := os.ReadFile(newPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "new day message")

	// Check if old file was closed (we can try to write to it or just trust the logic, but usually closing is enough)
	assert.NotEqual(t, yesterday, writer.lastDate)
	assert.Equal(t, date, writer.lastDate)
}

func TestDailyRotatingWriter_ErrorStdout(t *testing.T) {
	// Root or some protected path that should fail
	badPath := "/root_no_access/test.log"
	writer := NewDailyRotatingWriter(badPath, false)

	msg := []byte("error message should go to stdout\n")
	// This should not return error but fall back to stdout
	n, err := writer.Write(msg)
	require.NoError(t, err)
	assert.Equal(t, len(msg), n)
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
