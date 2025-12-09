package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"

	"trading/robot/go-bot/internal/config"
)

// Setup configures the global structured logger to write to os.Stdout.
func Setup(cfg config.LogConfig) {
	logger := New(os.Stdout, cfg)
	slog.SetDefault(logger)
}

// New creates a new slog.Logger for the given writer and configuration.
// This is useful for testing where the output can be directed to a buffer.
func New(w io.Writer, cfg config.LogConfig) *slog.Logger {
	var level slog.Level
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo // Default to info level
	}

	opts := &slog.HandlerOptions{
		AddSource: cfg.Source, // Enable source file and line number in logs
		Level:     level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				a.Value = slog.StringValue(a.Value.Time().Format("2006-01-02 15:04:05.000"))
			}
			return a
		},
	}

	var handler slog.Handler
	switch strings.ToLower(cfg.Format) {
	case "json":
		handler = slog.NewJSONHandler(w, opts)
	case "text":
		fallthrough
	default:
		handler = slog.NewTextHandler(w, opts)
	}

	return slog.New(handler)
}
