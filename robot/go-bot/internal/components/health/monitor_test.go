//go:build unit

package health

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMonitor_CheckHealth(t *testing.T) {
	logger := slog.Default()
	exchanges := []string{"binance", "kraken"}

	monitor := NewMonitor(logger, exchanges)

	// Define the check logic as a closure
	checkFunc := func(ctx context.Context, exchange string) error {
		if exchange == "binance" {
			return nil
		}
		if exchange == "kraken" {
			return errors.New("connection refused")
		}
		return errors.New("unknown exchange")
	}

	err := monitor.CheckHealth(context.Background(), checkFunc)
	assert.NoError(t, err)

	// Verify Binance
	statusBinance, ok := monitor.GetStatus("binance")
	assert.True(t, ok)
	assert.True(t, statusBinance.IsHealthy)
	assert.Empty(t, statusBinance.LastError)
	assert.NotZero(t, statusBinance.Latency)

	// Verify Kraken
	statusKraken, ok := monitor.GetStatus("kraken")
	assert.True(t, ok)
	assert.False(t, statusKraken.IsHealthy)
	assert.Equal(t, "connection refused", statusKraken.LastError)
	assert.NotZero(t, statusKraken.Latency)
}
