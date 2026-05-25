package spawn

import "time"

// seedNanos returns the current wall-clock time in nanoseconds.
// Extracted as a function so tests can override it via build tags
// or wrapper if deterministic seeds are ever needed. Production
// callers route through it so the seed source is single-sourced.
func seedNanos() uint64 {
	return uint64(time.Now().UnixNano())
}
