package background

import (
	"context"
	"time"
)

// Background context timeout is a last-resort safeguard against unexpected hangs.
// Normal operation should rely on the more specific timeouts of each job.
const backgroundContextTimeout = 5 * time.Minute

func runWithContextTimeout(ctx context.Context, job func(context.Context) error) error {
	jobCtx, cancel := context.WithTimeout(ctx, backgroundContextTimeout)
	defer cancel()
	return job(jobCtx)
}
