package background

import (
	"context"
	"log/slog"
	"time"
)

// WithRetry wraps a job with retry logic.
// It will retry the job up to 'attempts' times if it returns an error.
// It waits for 'delay' between attempts.
func WithRetry(job func(context.Context) error, attempts int, delay time.Duration) func(context.Context) error {
	return func(ctx context.Context) error {
		var err error
		for i := 0; i < attempts; i++ {
			if i > 0 {
				slog.Info("Retrying background job", "attempt", i+1, "max_attempts", attempts)
			}

			err = job(ctx)
			if err == nil {
				return nil
			}

			// If context is canceled, stop retrying immediately
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				// Continue to next attempt
			}
		}
		return err
	}
}
