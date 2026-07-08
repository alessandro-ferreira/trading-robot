package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"trading/robot/go-bot/internal/config"
)

// DailyRotatingWriter implements io.Writer and handles daily log rotation.
type DailyRotatingWriter struct {
	mu       sync.Mutex
	basePath string
	rotate   bool
	current  *os.File
	lastDate string
}

func NewDailyRotatingWriter(basePath string, rotate bool) *DailyRotatingWriter {
	return &DailyRotatingWriter{
		basePath: basePath,
		rotate:   rotate,
	}
}

func (w *DailyRotatingWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	date := time.Now().Format("2006-01-02")

	if w.current != nil && (w.lastDate != date && w.rotate) {
		w.current.Close()
		w.current = nil
	}

	if w.current == nil {
		logPath := w.basePath
		if w.rotate {
			ext := filepath.Ext(w.basePath)
			base := strings.TrimSuffix(w.basePath, ext)
			logPath = fmt.Sprintf("%s-%s%s", base, date, ext)
		}

		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			// If we fail to open the log file, write to stdout to avoid failing silently
			os.Stdout.Write([]byte(fmt.Sprintf("failed to open log file %s: %v\n", logPath, err)))
			return os.Stdout.Write(p)
		}
		w.current = f
		w.lastDate = date
	}

	return w.current.Write(p)
}

// Setup configures the global structured logger.
func Setup(cfg config.LogConfig) {
	var w io.Writer = os.Stdout

	if cfg.Path != "" {
		w = NewDailyRotatingWriter(cfg.Path, cfg.Rotate)
	}

	logger := New(w, cfg)
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
			// Format time in a human-readable way
			if a.Key == slog.TimeKey {
				a.Value = slog.StringValue(a.Value.Time().Format("2006-01-02 15:04:05.000"))
			}
			// Shorten the source file path to the relative path from the project root
			if a.Key == slog.SourceKey {
				if source, ok := a.Value.Any().(*slog.Source); ok {
					if idx := strings.Index(source.File, "go-bot/"); idx != -1 {
						source.File = source.File[idx+len("go-bot/"):]
					} else {
						source.File = filepath.Base(source.File)
					}
					source.Function = "" // Forces slog to completely omit the function field
				}
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
