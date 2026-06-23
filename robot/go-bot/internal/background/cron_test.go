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

func TestCronTask(t *testing.T) {
	var mu sync.Mutex
	runCount := 0
	job := func(ctx context.Context) error {
		mu.Lock()
		runCount++
		mu.Unlock()
		return nil
	}

	// This spec runs every second.
	task := NewCronTask("test-cron", "* * * * * *", false, job)

	assert.Equal(t, "test-cron", task.Name(), "Task name should be set correctly")

	// We'll run the task for a short duration to check if it triggers.
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()

	// Run the task in a goroutine so we can observe it and then cancel it.
	go task.Run(ctx)

	<-ctx.Done() // Wait for the timeout.

	mu.Lock()
	defer mu.Unlock()

	// In 2.5 seconds, the job should have run at least twice.
	require.GreaterOrEqual(t, runCount, 2, "Job should have run at least twice")
	assert.LessOrEqual(t, runCount, 3, "Job should have run at most three times")
}

func TestCronTask_RunOnce(t *testing.T) {
	var mu sync.Mutex
	runCount := 0
	job := func(ctx context.Context) error {
		mu.Lock()
		runCount++
		mu.Unlock()
		return nil
	}

	// Create a task with runOnce=true. The cron spec is set to run infrequently (hourly).
	task := NewCronTask("test-cron-once", "0 * * * * *", true, job)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately to stop the scheduler, but allow the initial run
	cancel()
	task.Run(ctx)

	assert.Equal(t, 1, runCount, "Job should have run exactly once immediately")
}
