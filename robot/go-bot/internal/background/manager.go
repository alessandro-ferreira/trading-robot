package background

import (
	"context"
	"log/slog"
	"sync"
)

// Task represents a background process that can be managed.
type Task interface {
	// Name returns a human-readable name for the task.
	Name() string
	// Run starts the task. It should block until the context is canceled.
	Run(ctx context.Context)
}

// Manager handles the lifecycle of background tasks.
type Manager struct {
	logger *slog.Logger
	tasks  []Task
	wg     sync.WaitGroup
}

// NewManager creates a new background task manager.
func NewManager(logger *slog.Logger) *Manager {
	return &Manager{
		logger: logger,
		tasks:  make([]Task, 0),
	}
}

// Add registers a new task with the manager.
func (m *Manager) Add(t Task) {
	m.tasks = append(m.tasks, t)
}

// Start runs all registered tasks in separate goroutines.
func (m *Manager) Start(ctx context.Context) {
	for _, t := range m.tasks {
		m.wg.Add(1)
		go func(task Task) {
			defer m.wg.Done()
			m.logger.Info("Starting background task", "task", task.Name())
			task.Run(ctx)
			m.logger.Info("Background task stopped", "task", task.Name())
		}(t)
	}
}

// Wait blocks until all tasks have stopped.
// This should be called after the context passed to Start is canceled.
func (m *Manager) Wait() {
	m.wg.Wait()
}

// Stop is a convenience method if you want to manage cancellation internally,
// but typically the context passed to Start controls the lifecycle.
// For now, we rely on the context cancellation in main.go.
