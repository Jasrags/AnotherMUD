package emote

import (
	"strings"
	"testing"
)

func validNoTarget() Emote {
	return Emote{
		ID: "x:smile", DisplayName: "smile",
		NoTarget: View{ActorView: "You smile.", RoomView: "$n smiles."},
		Targeted: View{
			ActorView:  "You smile at $N.",
			TargetView: "$n smiles at you.",
			RoomView:   "$n smiles at $N.",
		},
	}
}

func TestRegistry_RegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(validNoTarget()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if e, ok := r.ByVerb("SMILE"); !ok || e.ID != "x:smile" {
		t.Errorf("ByVerb(SMILE) miss")
	}
	if r.Len() != 1 {
		t.Errorf("Len = %d", r.Len())
	}
}

func TestRegistry_Aliases(t *testing.T) {
	r := NewRegistry()
	e := validNoTarget()
	e.Aliases = []string{"grin"}
	if err := r.Register(e); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, ok := r.ByVerb("grin"); !ok {
		t.Errorf("alias grin did not resolve")
	}
}

func TestRegistry_RejectsDuplicateID(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(validNoTarget())
	err := r.Register(validNoTarget())
	if err == nil || !strings.Contains(err.Error(), "duplicate id") {
		t.Errorf("dup id: %v", err)
	}
}

func TestRegistry_RejectsVerbCollision(t *testing.T) {
	r := NewRegistry()
	a := validNoTarget()
	_ = r.Register(a)
	b := validNoTarget()
	b.ID = "x:smirk"
	b.Aliases = []string{"smile"} // collides with first's display name
	err := r.Register(b)
	if err == nil || !strings.Contains(err.Error(), "verb collision") {
		t.Errorf("verb collision: %v", err)
	}
}

func TestEmote_ValidateRequiresTargetedViews(t *testing.T) {
	e := Emote{ID: "x:bad", DisplayName: "bad"}
	if err := e.Validate(); err == nil {
		t.Errorf("empty templates: want error")
	}
}

func TestEmote_ValidateNoTargetForm(t *testing.T) {
	e := validNoTarget()
	e.NoTarget = View{} // missing
	if err := e.Validate(); err == nil {
		t.Errorf("missing no_target form: want error")
	}
}

func TestEmote_ValidateRequiresTargetSkipsNoTargetCheck(t *testing.T) {
	e := validNoTarget()
	e.RequiresTarget = true
	e.NoTarget = View{} // intentionally empty
	if err := e.Validate(); err != nil {
		t.Errorf("RequiresTarget should skip no_target validation: %v", err)
	}
}
