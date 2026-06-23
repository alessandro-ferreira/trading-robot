//go:build unit

package background

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPeriodicTask(t *testing.T) {
	var mu sync.Mutex
	runCount := 0
	job := func(ctx context.Context) error {
		mu.Lock()
		runCount++
		mu.Unlock()
		return nil
	}

	// Create a task that runs every 100ms
	task := NewPeriodicTask("test-periodic", 100*time.Millisecond, false, job)

	assert.Equal(t, "test-periodic", task.Name(), "Task name should be correct")

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	// Run in background
	go task.Run(ctx)

	<-ctx.Done()

	mu.Lock()
	defer mu.Unlock()

	// Should run at least twice (at 100ms and 200ms)
	require.GreaterOrEqual(t, runCount, 2, "Job should have run at least twice")
}

func TestPeriodicTask_RunOnce(t *testing.T) {
	var mu sync.Mutex
	runCount := 0
	job := func(ctx context.Context) error {
		mu.Lock()
		runCount++
		mu.Unlock()
		return nil
	}

	// Create a task with runOnce=true
	task := NewPeriodicTask("test-periodic-once", 100*time.Millisecond, true, job)

	// Cancel immediately to stop the ticker loop, but allow the initial run
	ctx, cancel := context.WithCancel(context.Background())

	// Run synchronously since we cancel immediately
	cancel()
	task.Run(ctx)

	assert.Equal(t, 1, runCount, "Job should have run exactly once immediately")
}
