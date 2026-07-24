package execution

import "time"

// Clock abstracts the time source for the system.
// This allows production code to use the system clock,
// while backtesting uses a simulated clock that progresses through CSV timestamps.
type Clock interface {
	// Now returns the current time according to this clock.
	Now() time.Time
}

// systemClock implements Clock using the system's actual time.
type systemClock struct{}

// NewSystemClock creates a clock that returns the system's current time.
func NewSystemClock() Clock {
	return &systemClock{}
}

// Now returns the current system time.
func (sc *systemClock) Now() time.Time {
	return time.Now()
}

// simulatedClock implements Clock using timestamps from a SimulatedClient.
// It progresses through time as the simulated client steps through price history.
type simulatedClock struct {
	client *SimulatedClient
}

// NewSimulatedClock creates a clock that progresses through the simulated client's price history.
func NewSimulatedClock(client *SimulatedClient) Clock {
	return &simulatedClock{client: client}
}

// Now returns the current time from the simulated client's price history.
// This will be the timestamp of the current price candle being processed.
func (sc *simulatedClock) Now() time.Time {
	return sc.client.CurrentTime()
}
