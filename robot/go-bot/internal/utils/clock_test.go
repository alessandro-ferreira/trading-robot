//go:build unit

package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSystemClock(t *testing.T) {
	clock := NewSystemClock()
	now := clock.Now()

	// Should be close to current time
	require.WithinDuration(t, time.Now(), now, 1*time.Second)
}
