package ai

import "fmt"

// Built-in behavior names. Content templates reference these via
// their `behavior` field. Constants exist so the typo-when-author-
// ing-content surface is small (a wrong name surfaces as
// ErrUnknownBehavior at dispatch time, which the dispatcher logs).
const (
	BehaviorNameStationary = "stationary"
	BehaviorNameWander     = "wander"
)

// RegisterEngineBaseline registers every built-in behavior into r.
// Mirrors slot.RegisterEngineBaseline / similar substrate seeds:
// callers (typically main.go) invoke this before pack loading so
// content packs that want to override a built-in fail loudly via
// ErrDuplicateBehavior rather than silently win.
func RegisterEngineBaseline(r *Registry) error {
	bindings := []struct {
		name string
		fn   Behavior
	}{
		{BehaviorNameStationary, BehaviorStationary},
		{BehaviorNameWander, BehaviorWander},
	}
	for _, b := range bindings {
		if err := r.Register(b.name, b.fn); err != nil {
			return fmt.Errorf("ai baseline %q: %w", b.name, err)
		}
	}
	return nil
}
