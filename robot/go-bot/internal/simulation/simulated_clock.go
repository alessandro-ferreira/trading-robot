package simulation

import (
	"time"
	"trading/robot/go-bot/internal/utils"
)

// simulatedClock implements Clock using timestamps from a SimulatedClient.
// It progresses through time as the simulated client steps through price history.
type simulatedClock struct {
	client *SimulatedClient
}

// NewSimulatedClock creates a clock that progresses through the simulated client's price history.
func NewSimulatedClock(client *SimulatedClient) utils.Clock {
	return &simulatedClock{client: client}
}

// Now returns the current time from the simulated client's price history.
// This will be the timestamp of the current price candle being processed.
func (sc *simulatedClock) Now() time.Time {
	return sc.client.CurrentTime()
}
