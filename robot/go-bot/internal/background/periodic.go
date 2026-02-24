package background

import (
	"context"
	"log/slog"
	"time"
)

// PeriodicTask is a generic implementation of Task that runs a job at a fixed interval.
type PeriodicTask struct {
	name     string
	interval time.Duration
	job      func(context.Context) error
	runOnce  bool
}

// NewPeriodicTask creates a new periodic task.
// If runOnce is true, the job will be executed immediately upon starting,
// in addition to running on the schedule.
func NewPeriodicTask(name string, interval time.Duration, runOnce bool, job func(context.Context) error) *PeriodicTask {
	return &PeriodicTask{
		name:     name,
		interval: interval,
		job:      job,
		runOnce:  runOnce,
	}
}

// Name returns the name of the task.
func (t *PeriodicTask) Name() string {
	return t.name
}

// Run starts the ticker and executes the job periodically.
func (t *PeriodicTask) Run(ctx context.Context) {
	if t.runOnce {
		if err := t.job(ctx); err != nil {
			slog.Error("Periodic task failed (initial run)", "task", t.name, "error", err)
		}
	}

	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := t.job(ctx); err != nil {
				slog.Error("Periodic task failed", "task", t.name, "error", err)
			}
		}
	}
}
