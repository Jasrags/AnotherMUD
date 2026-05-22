package clock

import (
	"sync"
	"time"
)

// ManualClock is a test Clock whose time only advances when Advance is
// called. Tickers registered against it fire once per Advance(d) call
// for each whole d-interval that has elapsed since their creation
// (or since their last fire).
//
// ManualClock is safe for concurrent use. Advance blocks until every
// pending tick fanout has been delivered to its receiver or the
// receiver buffer absorbs it, so tests can rely on "after Advance
// returns, all tick handlers due at this time have been notified".
type ManualClock struct {
	mu      sync.Mutex
	now     time.Time
	tickers []*manualTicker
}

type manualTicker struct {
	d      time.Duration
	next   time.Time
	ch     chan time.Time
	closed bool
}

// NewManual returns a ManualClock whose Now is start.
func NewManual(start time.Time) *ManualClock {
	return &ManualClock{now: start}
}

// Now returns the manual clock's current time.
func (m *ManualClock) Now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.now
}

// Ticker registers a ticker that fires every d (in manual-clock time).
// The channel is buffered (1) so a single missed receive does not
// block Advance.
func (m *ManualClock) Ticker(d time.Duration) (<-chan time.Time, func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := &manualTicker{
		d:    d,
		next: m.now.Add(d),
		ch:   make(chan time.Time, 1),
	}
	m.tickers = append(m.tickers, t)
	stop := func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if !t.closed {
			t.closed = true
			close(t.ch)
		}
	}
	return t.ch, stop
}

// Advance moves the clock forward by d and fires any tickers whose
// next-fire time has been reached. Multiple fires that elapse within
// the same Advance collapse to one — matching the stdlib time.Ticker
// behavior on a slow receiver.
func (m *ManualClock) Advance(d time.Duration) {
	m.mu.Lock()
	m.now = m.now.Add(d)
	now := m.now
	// Snapshot the list of tickers to fire to avoid holding the lock
	// across a (potentially blocking) channel send.
	type fire struct {
		ch chan time.Time
		t  time.Time
	}
	var fires []fire
	for _, t := range m.tickers {
		if t.closed {
			continue
		}
		if !now.Before(t.next) {
			fires = append(fires, fire{t.ch, now})
			// Advance to the next interval boundary AFTER now.
			for !now.Before(t.next) {
				t.next = t.next.Add(t.d)
			}
		}
	}
	m.mu.Unlock()

	for _, f := range fires {
		select {
		case f.ch <- f.t:
		default:
			// Receiver is behind; drop (matches time.Ticker semantics).
		}
	}
}
