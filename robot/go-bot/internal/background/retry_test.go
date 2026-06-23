//go:build unit

package background

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithRetry(t *testing.T) {
	testCases := []struct {
		name             string
		attempts         int
		delay            time.Duration
		jobFactory       func() func(context.Context) error
		expectedAttempts int
		expectError      bool
	}{
		{
			name:     "Success on first try",
			attempts: 3,
			delay:    1 * time.Millisecond,
			jobFactory: func() func(context.Context) error {
				return func(ctx context.Context) error {
					return nil
				}
			},
			expectedAttempts: 1,
			expectError:      false,
		},
		{
			name:     "Success on third try",
			attempts: 3,
			delay:    1 * time.Millisecond,
			jobFactory: func() func(context.Context) error {
				count := 0
				return func(ctx context.Context) error {
					count++
					if count < 3 {
						return errors.New("temporary failure")
					}
					return nil
				}
			},
			expectedAttempts: 3,
			expectError:      false,
		},
		{
			name:     "Exhaust all attempts",
			attempts: 2,
			delay:    1 * time.Millisecond,
			jobFactory: func() func(context.Context) error {
				return func(ctx context.Context) error {
					return errors.New("permanent failure")
				}
			},
			expectedAttempts: 2,
			expectError:      true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			count := 0
			actualJob := tc.jobFactory()
			trackingJob := func(ctx context.Context) error {
				count++
				return actualJob(ctx)
			}

			fn := WithRetry(trackingJob, tc.attempts, tc.delay)
			err := fn(context.Background())

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.expectedAttempts, count)
		})
	}
}

func TestWithRetry_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	count := 0
	job := func(ctx context.Context) error {
		count++
		return errors.New("fail")
	}

	// Use a long delay so we can cancel during sleep
	fn := WithRetry(job, 5, 100*time.Millisecond)

	// Cancel context after the first failure triggers the sleep
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err := fn(ctx)
	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	assert.Equal(t, 1, count, "Should stop after first failure because context was cancelled during delay")
}
