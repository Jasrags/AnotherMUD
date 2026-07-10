// Package guard holds cross-cutting, source-level invariant tests (guardrails)
// that belong to no single feature package — e.g. "player-facing currency text
// must flow through the currency-label seam, never a hardcoded 'gold'". These are
// build-time regression gates: they fail `go test` when a convention is broken,
// rather than relying on a reviewer to catch it by eye.
package guard
