package health

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// HealthStatus represents the health state of an exchange.
type HealthStatus struct {
	IsHealthy   bool
	LastChecked time.Time
	LastError   string
	Latency     time.Duration
}

// Monitor checks and stores the health status of exchanges.
type Monitor struct {
	logger    *slog.Logger
	exchanges []string

	mu     sync.RWMutex
	status map[string]HealthStatus
}

// NewMonitor creates a new health monitor.
func NewMonitor(logger *slog.Logger, exchanges []string) *Monitor {
	return &Monitor{
		logger:    logger,
		exchanges: exchanges,
		status:    make(map[string]HealthStatus),
	}
}

// CheckHealth performs the health check for all configured exchanges using the provided check function.
func (m *Monitor) CheckHealth(ctx context.Context, checkMethod func(context.Context, string) error) error {
	var wg sync.WaitGroup

	for _, exchange := range m.exchanges {
		wg.Add(1)
		go func(ex string) {
			defer wg.Done()
			m.checkExchange(ctx, ex, checkMethod)
		}(exchange)
	}

	wg.Wait()
	return nil
}

func (m *Monitor) checkExchange(ctx context.Context, exchange string, checkMethod func(context.Context, string) error) {
	start := time.Now()
	err := checkMethod(ctx, exchange)
	latency := time.Since(start)

	status := HealthStatus{
		LastChecked: time.Now(),
		Latency:     latency,
	}

	if err != nil {
		m.logger.Error("Health check failed", "exchange", exchange, "error", err)
		status.IsHealthy = false
		status.LastError = err.Error()
	} else {
		// Log at debug level to avoid spamming logs on every tick
		m.logger.Debug("Health check passed", "exchange", exchange, "latency", latency)
		status.IsHealthy = true
	}

	m.mu.Lock()
	m.status[exchange] = status
	m.mu.Unlock()
}

// GetStatus returns the current health status for a specific exchange.
func (m *Monitor) GetStatus(exchange string) (HealthStatus, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	status, ok := m.status[exchange]
	return status, ok
}
