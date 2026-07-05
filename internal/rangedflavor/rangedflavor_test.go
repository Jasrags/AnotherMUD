package rangedflavor

import "testing"

// With no registry (nil receiver) every key resolves off the engine floor and
// substitutes params — callers never have to nil-guard.
func TestResolve_NilFallsToFloor(t *testing.T) {
	var r *Registry
	self, room := r.Resolve("bow", KeyDry, map[string]string{"ammo": "arrows"})
	if self != "You are out of arrows." {
		t.Errorf("self = %q", self)
	}
	if room != "{actor} is out of arrows." {
		t.Errorf("room = %q (actor token left for caller-less floor)", room)
	}
}

// A registered style overrides the floor for its keys.
func TestResolve_StyleOverridesFloor(t *testing.T) {
	r := NewRegistry()
	r.Register(Style{ID: "bow", Msgs: map[string]Line{
		KeyDry: {Self: "Your quiver is empty.", Room: "{actor} grasps at an empty quiver."},
	}})
	self, room := r.Resolve("bow", KeyDry, map[string]string{"actor": "Rand"})
	if self != "Your quiver is empty." {
		t.Errorf("self = %q, want the bow override", self)
	}
	if room != "Rand grasps at an empty quiver." {
		t.Errorf("room = %q, want substituted bow override", room)
	}
}

// A style missing a key falls through to the default style, then the floor —
// per audience line, so a style may override Self and inherit Room.
func TestResolve_FallthroughChain(t *testing.T) {
	r := NewRegistry()
	r.Register(Style{ID: DefaultStyleID, Msgs: map[string]Line{
		KeyLoad: {Self: "You ready {weapon}.", Room: "{actor} readies {weapon}."},
	}})
	r.Register(Style{ID: "crossbow", Msgs: map[string]Line{
		// Only overrides Self; Room must fall through to the default style.
		KeyLoad: {Self: "*click* — {weapon} is spanned."},
	}})
	self, room := r.Resolve("crossbow", KeyLoad, map[string]string{"weapon": "a crossbow", "actor": "Mat"})
	if self != "*click* — a crossbow is spanned." {
		t.Errorf("self = %q, want crossbow Self override", self)
	}
	if room != "Mat readies a crossbow." {
		t.Errorf("room = %q, want default-style Room inherited", room)
	}
}

// An unknown style id resolves off default/floor, not an error.
func TestResolve_UnknownStyle(t *testing.T) {
	r := NewRegistry()
	self, _ := r.Resolve("nonesuch", KeyUnloaded, map[string]string{"weapon": "a sling"})
	if self != "a sling isn't loaded. (load it first)" {
		t.Errorf("self = %q, want floor for unknown style", self)
	}
}

// Substitution leaves unknown tokens intact and handles a missing close brace.
func TestSubstitute_UnknownAndMalformed(t *testing.T) {
	if got := substitute("hit {target} with {weapon}", map[string]string{"target": "a rat"}); got != "hit a rat with {weapon}" {
		t.Errorf("unknown token not passed through: %q", got)
	}
	if got := substitute("open {brace", map[string]string{"brace": "x"}); got != "open {brace" {
		t.Errorf("malformed template mishandled: %q", got)
	}
}

// Register is last-writer-wins for a repeated id (baseline-then-pack override).
func TestRegister_Replaces(t *testing.T) {
	r := NewRegistry()
	r.Register(Style{ID: "bow", Msgs: map[string]Line{KeyDry: {Self: "first"}}})
	r.Register(Style{ID: "bow", Msgs: map[string]Line{KeyDry: {Self: "second"}}})
	if self, _ := r.Resolve("bow", KeyDry, nil); self != "second" {
		t.Errorf("self = %q, want the second registration", self)
	}
	if ids := r.IDs(); len(ids) != 1 {
		t.Errorf("ids = %v, want a single entry after replace", ids)
	}
}
