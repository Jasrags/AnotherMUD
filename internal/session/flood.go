package session

import (
	"sync"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
)

// FloodConfig parameterizes the per-session token-bucket rate limiter
// described in docs/specs/session-lifecycle.md §4.
//
// Zero CommandsPerSecond (or zero BurstSize) disables flood protection
// — the gate becomes a no-op that always permits input. This is the
// test default; production wires DefaultFloodConfig.
type FloodConfig struct {
	CommandsPerSecond  float64
	BurstSize          float64
	StrikeThreshold    int
	StrikeDecaySeconds float64
}

// DefaultFloodConfig returns the spec's default policy (15 commands/s,
// 30-token burst, 3 strikes to disconnect, strikes decay after 10s
// of clean input).
func DefaultFloodConfig() FloodConfig {
	return FloodConfig{
		CommandsPerSecond:  15,
		BurstSize:          30,
		StrikeThreshold:    3,
		StrikeDecaySeconds: 10,
	}
}

// gmcpFloodConfig derives the inbound-GMCP rate limit from the command
// flood policy. Inbound GMCP (e.g. tab-completion firing per keystroke) is
// lighter and more frequent than commands, so it gets a higher rate. Abuse
// is only ever DROPPED, never disconnected (StrikeThreshold 0) — a client
// hammering Tab keeps its command channel; only its excess GMCP is shed. A
// zero command config (the test default) yields a disabled gate, so tests
// see no throttling.
func gmcpFloodConfig(cmd FloodConfig) FloodConfig {
	return FloodConfig{
		CommandsPerSecond: cmd.CommandsPerSecond * 2,
		BurstSize:         cmd.BurstSize * 2,
		StrikeThreshold:   0,
	}
}

// floodDecision is the result of evaluating one input against the gate.
type floodDecision int

const (
	// floodAllow: input passes; the caller proceeds to dispatch.
	floodAllow floodDecision = iota
	// floodDrop: input dropped silently. The gate may have already sent
	// a "Slow down." reply; the caller must not dispatch.
	floodDrop
	// floodDisconnect: the strike threshold has been reached. The
	// caller must inform the user, tear down the connection, and stop
	// reading.
	floodDisconnect
)

// floodGate is the per-session token-bucket rate limiter. Safe for
// concurrent use; in practice it is only called from one goroutine
// (the session's read loop) but the mutex protects against future
// callers and keeps the race detector happy.
//
// Zero-value FloodConfig means the gate is disabled and Check is a
// no-op that always returns floodAllow.
type floodGate struct {
	cfg   FloodConfig
	clock clock.Clock

	mu          sync.Mutex
	initialized bool
	tokens      float64
	lastRefill  time.Time
	strikes     int
	lastStrike  time.Time
	warned      bool
	floodedOut  bool
}

func newFloodGate(cfg FloodConfig, c clock.Clock) *floodGate {
	if c == nil {
		c = clock.RealClock{}
	}
	return &floodGate{cfg: cfg, clock: c}
}

// disabled reports whether this gate is a no-op.
func (f *floodGate) disabled() bool {
	return f.cfg.CommandsPerSecond <= 0 || f.cfg.BurstSize <= 0
}

// Check evaluates one input. The second return is true when the
// caller should emit the "Slow down." reply (at most once per strike-
// decay cycle). The gate's mutex is released before Check returns, so
// the caller is free to write through any sink without risking a lock
// cycle through the gate.
func (f *floodGate) Check() (floodDecision, bool) {
	if f.disabled() {
		return floodAllow, false
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.floodedOut {
		return floodDisconnect, false
	}

	now := f.clock.Now()
	if !f.initialized {
		f.tokens = f.cfg.BurstSize
		f.lastRefill = now
		f.initialized = true
	} else {
		if elapsed := now.Sub(f.lastRefill).Seconds(); elapsed > 0 {
			f.tokens += elapsed * f.cfg.CommandsPerSecond
			if f.tokens > f.cfg.BurstSize {
				f.tokens = f.cfg.BurstSize
			}
			f.lastRefill = now
		}
	}

	// Strike decay: a clean window resets the strike count and clears
	// the warned flag so the next abuse cycle gets a fresh warning.
	if f.strikes > 0 && f.cfg.StrikeDecaySeconds > 0 &&
		now.Sub(f.lastStrike).Seconds() >= f.cfg.StrikeDecaySeconds {
		f.strikes = 0
		f.warned = false
	}

	if f.tokens >= 1.0 {
		f.tokens -= 1.0
		return floodAllow, false
	}

	// Bucket empty: drop the input. Warn once per decay cycle.
	warn := false
	if !f.warned {
		warn = true
		f.warned = true
	}
	f.strikes++
	f.lastStrike = now

	if f.cfg.StrikeThreshold > 0 && f.strikes >= f.cfg.StrikeThreshold {
		f.floodedOut = true
		return floodDisconnect, warn
	}
	return floodDrop, warn
}
