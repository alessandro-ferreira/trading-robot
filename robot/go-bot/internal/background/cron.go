package background

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/robfig/cron/v3"
)

// CronTask wraps a job to be executed based on a cron expression.
type CronTask struct {
	name    string
	spec    string
	job     func(context.Context) error
	runOnce bool
	parser  cron.Parser
}

// NewCronTask creates a new task that runs according to the provided cron expression.
// The spec follows the standard cron syntax (e.g., "0 0 * * *" for daily at midnight).
// It uses the standard parser which includes seconds (second, minute, hour, dom, month, dow).
func NewCronTask(name, spec string, runOnce bool, job func(context.Context) error) *CronTask {
	return &CronTask{
		name:    name,
		spec:    spec,
		job:     job,
		runOnce: runOnce,
		parser:  cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor),
	}
}

func (t *CronTask) Name() string {
	return t.name
}

func (t *CronTask) Run(ctx context.Context) {
	if t.runOnce {
		if err := t.job(ctx); err != nil {
			slog.Error("Cron task failed (initial run)", "task", t.name, "error", err)
		}
	}

	c := cron.New(cron.WithParser(t.parser))

	// Wrap the job to pass the context
	_, err := c.AddFunc(t.spec, func() {
		if err := t.job(ctx); err != nil {
			slog.Error("Cron task failed", "task", t.name, "error", err)
		}
	})
	if err != nil {
		// This will crash the app if the spec is invalid, which is desirable for fail-fast behavior on startup.
		panic(fmt.Sprintf("failed to add cron job %s with spec '%s': %v", t.name, t.spec, err))
	}

	c.Start()

	// Block until context is canceled
	<-ctx.Done()

	// Stop the cron scheduler
	ctxStop := c.Stop()
	<-ctxStop.Done()
}
