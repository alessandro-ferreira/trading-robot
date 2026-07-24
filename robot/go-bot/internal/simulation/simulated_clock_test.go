package simulation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSimulatedClock(t *testing.T) {
	// Create a minimal SimulatedClient for testing
	// Since we are in the same package 'execution', we can access non-exported fields
	client := &SimulatedClient{
		priceHistory: []PriceCandle{
			{UnixTimestamp: 1700000000, Price: 50000.0},
			{UnixTimestamp: 1700000060, Price: 50100.0},
		},
	}

	clock := NewSimulatedClock(client)

	// Case 1: Before any candles have been "served" (historyIndex = 0)
	// CurrentTime defaults to first candle (idx=0)
	client.historyIndex = 0
	expected1 := time.Unix(1700000000, 0).UTC()
	require.Equal(t, expected1, clock.Now())

	// Case 2: After first candle served (historyIndex = 1)
	// CurrentTime uses idx = 1 - 1 = 0
	client.historyIndex = 1
	require.Equal(t, expected1, clock.Now())

	// Case 3: After second candle served (historyIndex = 2)
	// CurrentTime uses idx = 2 - 1 = 1
	client.historyIndex = 2
	expected2 := time.Unix(1700000060, 0).UTC()
	require.Equal(t, expected2, clock.Now())
}
