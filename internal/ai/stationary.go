package ai

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// BehaviorStationary is the no-op behavior used by mobs that stand
// in place. The village guard ships with it. It exists primarily
// to give content authors a non-empty `behavior` field to use for
// "stays put, no AI" without the dispatcher having to special-case
// missing handlers.
//
// Returning nil unconditionally satisfies the spec's implicit
// "no-op behavior is permitted" contract (§4.2 — "Behavior is a
// string name; the dispatcher may register handlers that do
// nothing").
func BehaviorStationary(_ context.Context, _ *entities.MobInstance, _ Deps) error {
	return nil
}
