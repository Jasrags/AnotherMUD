package entities

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/tick"
)

// TagSwapHandlerName is the registration name used by RegisterTagSwap.
// Exported so tests can de-duplicate or inspect it.
const TagSwapHandlerName = "entities.tag_swap"

// RegisterTagSwap registers the SwapTagIndex tick handler on loop at
// cadence 1 (every tick) per world-rooms-movement §3.7. Returns the
// underlying tick.Register error so the caller decides whether a
// duplicate-name collision is fatal.
func RegisterTagSwap(loop *tick.Loop, s *Store) error {
	return loop.Register(TagSwapHandlerName, 1, func(ctx context.Context, n uint64) {
		_ = ctx // handler is purely synchronous; ctx unused today
		_ = n
		s.SwapTagIndex()
	})
}
