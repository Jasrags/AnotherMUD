package emote

import "fmt"

// View is the per-perspective template trio. ActorView is what the
// emoter sees, TargetView is what the target sees (when there is
// one), RoomView is what every other observer in the room sees.
//
// All three may contain substitution tokens (spec §2.2). TargetView
// is required only in TargetedTemplates; NoTargetTemplates omits it.
type View struct {
	ActorView  string
	TargetView string
	RoomView   string
}

// Emote is a single registered emote.
//
// Spec: docs/specs/emotes.md §2.
type Emote struct {
	// ID is the namespaced identifier (e.g. "tapestry-core:smile").
	ID string

	// DisplayName is the verb the player types ("smile"). Used as
	// the registry's lookup key and as the verb keyword.
	DisplayName string

	// Aliases are extra verb names that route to the same emote
	// (e.g. "grins" → "grin"). Lower-cased; lookup is case-
	// insensitive.
	Aliases []string

	// RequiresTarget forbids the no-target form. Useful for emotes
	// that only make sense aimed at someone (e.g. "introduce").
	RequiresTarget bool

	// NoTarget holds the actor+room templates used when no target
	// is supplied. Required unless RequiresTarget is true.
	NoTarget View

	// Targeted holds the actor+target+room templates used when a
	// target IS supplied. Always required.
	Targeted View
}

// Validate reports a load-time error if the templates are not
// internally consistent: missing required views, missing target
// view in a targeted form, or a TargetedView template that
// references no-target paths (handled at substitution time, not
// here).
//
// Spec: §2.1 ("a template missing a required view is a load-time
// error").
func (e Emote) Validate() error {
	if e.ID == "" {
		return fmt.Errorf("emote: empty ID")
	}
	if e.DisplayName == "" {
		return fmt.Errorf("emote %q: empty DisplayName", e.ID)
	}
	if e.Targeted.ActorView == "" || e.Targeted.TargetView == "" || e.Targeted.RoomView == "" {
		return fmt.Errorf("emote %q: targeted form requires actor / target / room views", e.ID)
	}
	if !e.RequiresTarget {
		if e.NoTarget.ActorView == "" || e.NoTarget.RoomView == "" {
			return fmt.Errorf("emote %q: no-target form requires actor + room views", e.ID)
		}
	}
	return nil
}
