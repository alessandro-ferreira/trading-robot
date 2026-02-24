package background

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockTask is a simple task for testing purposes.
type mockTask struct {
	name     string
	runCount int
	running  chan struct{}
}

func (m *mockTask) Name() string {
	return m.name
}

func (m *mockTask) Run(ctx context.Context) {
	close(m.running) // Signal that we started
	<-ctx.Done()     // Block until canceled
	m.runCount++
}

func TestManager_Lifecycle(t *testing.T) {
	logger := slog.Default()
	mgr := NewManager(logger)

	task1 := &mockTask{name: "task1", running: make(chan struct{})}
	task2 := &mockTask{name: "task2", running: make(chan struct{})}

	mgr.Add(task1)
	mgr.Add(task2)

	ctx, cancel := context.WithCancel(context.Background())

	// Start tasks
	mgr.Start(ctx)

	// Wait for tasks to signal they are running
	<-task1.running
	<-task2.running

	// Stop tasks
	cancel()
	mgr.Wait()

	// Verify they finished
	assert.Equal(t, 1, task1.runCount)
	assert.Equal(t, 1, task2.runCount)
}
