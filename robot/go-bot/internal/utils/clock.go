package utils

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
