// Package clock provides the time abstraction the simulation runs on.
//
// Foundations F3: engine packages MUST NOT call time.Now() directly once
// they take a Clock. Tests use ManualClock to advance time
// deterministically; production uses RealClock.
//
// The interface is deliberately narrow: a clock tells you "now" and
// hands out tick pulses. The tick loop (internal/tick) owns the cadence
// math on top.
package clock

import "time"

// Clock is the time source for the engine. Implementations MUST be
// safe to call from multiple goroutines.
type Clock interface {
	// Now returns the current time according to this clock.
	Now() time.Time

	// Ticker returns a channel that delivers a value approximately
	// every d. The channel is closed when stop is invoked. The
	// returned stop function MUST be idempotent.
	Ticker(d time.Duration) (ch <-chan time.Time, stop func())
}

// RealClock is the production Clock backed by the stdlib time package.
type RealClock struct{}

// Now returns time.Now.
func (RealClock) Now() time.Time { return time.Now() }

// Ticker wraps time.NewTicker.
func (RealClock) Ticker(d time.Duration) (<-chan time.Time, func()) {
	t := time.NewTicker(d)
	return t.C, t.Stop
}
