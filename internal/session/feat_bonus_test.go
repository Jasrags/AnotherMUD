package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/feat"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// A held save-bonus feat lifts exactly its axis in connActor.Saves() (EPIC S4
// Phase 3a) — the conferred bonus adds on top of the class-base + ability-mod
// derivation, leaving the other axes untouched.
func TestSaves_FeatSaveBonusLiftsAxis(t *testing.T) {
	a, _ := newFakeActor("c1", "p1", "acc1", "Hero", &world.Room{ID: "r"})

	reg := feat.NewRegistry()
	_ = reg.Register(&feat.Feat{
		ID:     "iron-will",
		Grants: []feat.Grant{{Kind: feat.GrantSaveBonus, Target: "will", Magnitude: 2}},
	})
	a.feats = reg

	// Baseline (no feats taken).
	base := a.Saves()

	// Take Iron Will.
	a.save.KnownFeats = []player.KnownFeat{{FeatID: "iron-will"}}
	withFeat := a.Saves()

	if got := withFeat.Will - base.Will; got != 2 {
		t.Errorf("Will delta = %d, want +2 from Iron Will", got)
	}
	if withFeat.Fortitude != base.Fortitude || withFeat.Reflex != base.Reflex {
		t.Errorf("non-Will axes drifted: base %+v, withFeat %+v", base, withFeat)
	}
}

// With no feat registry wired (headless/test actor) or no known feats, Saves()
// is unaffected — the feat path is a no-op, never a panic.
func TestSaves_NoFeatsIsNoop(t *testing.T) {
	a, _ := newFakeActor("c1", "p1", "acc1", "Hero", &world.Room{ID: "r"})
	a.feats = nil // headless
	_ = a.Saves() // must not panic

	a.feats = feat.NewRegistry()
	// A known feat that isn't registered (removed content) is skipped fail-soft.
	a.save.KnownFeats = []player.KnownFeat{{FeatID: "ghost"}}
	if s := a.Saves(); s.Will != 0 {
		// classless fake actor has 0 base will; a ghost feat adds nothing.
		t.Errorf("ghost feat should add nothing, Will = %d", s.Will)
	}
}
