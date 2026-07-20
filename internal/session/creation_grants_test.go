package session

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/feat"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// The 20/23 regression: a background whose granted feat raises max_hp (Toughness)
// must leave a freshly created character at FULL HP, not stranded below the new
// ceiling. This pins the load-bearing ordering in ApplyCharacterCreated — the
// fill-to-full runs AFTER the background feat grant. If the fills ever move back
// before the grant, current stays at the pre-feat max and this fails.
func TestApplyCharacterCreated_FillsAfterMaxHPFeat(t *testing.T) {
	mgr := NewManager()
	a, _ := newFakeActor("c1", "p1", "acc1", " Toughie", &world.Room{ID: "r"})

	// Wire the statBlock→vitals hp_max binding the live constructor sets up
	// (session.go ~L897): a raise to hp_max lifts the Vitals ceiling but never
	// auto-heals current — exactly the seam the ordering has to account for.
	a.feats = feat.NewRegistry()
	if err := a.feats.Register(&feat.Feat{
		ID:        "toughness",
		MultiTake: feat.MultiTakeStackable,
		Grants:    []feat.Grant{{Kind: feat.GrantMaxHP, Magnitude: 3}},
	}); err != nil {
		t.Fatalf("register toughness: %v", err)
	}
	a.statBlock.OnMaxChange(progression.StatHPMax, func(_, newMax int) {
		a.vitals.SetMax(newMax)
	})
	// Sanity: the actor starts full at the engine default (20/20).
	if a.vitals.Current() != combat.DefaultPlayerMaxHP || a.vitals.Max() != combat.DefaultPlayerMaxHP {
		t.Fatalf("precondition: vitals = %d/%d, want %d/%d",
			a.vitals.Current(), a.vitals.Max(), combat.DefaultPlayerMaxHP, combat.DefaultPlayerMaxHP)
	}
	mgr.Add(a)

	// An origin that auto-grants Toughness (single FeatOption ⇒ no pick step).
	a.backgroundID = "ex-security"
	backgrounds := progression.NewBackgroundRegistry()
	if err := backgrounds.Register(&progression.Background{
		ID:          "ex-security",
		FeatOptions: []string{"toughness"},
	}); err != nil {
		t.Fatalf("register background: %v", err)
	}
	granter := NewBackgroundGranter(mgr, nil, item.NewTemplates(), entities.NewStore(), nil)

	mgr.ApplyCharacterCreated(context.Background(), "p1", CreationGrants{
		Backgrounds: backgrounds,
		Granter:     granter,
	})

	wantMax := combat.DefaultPlayerMaxHP + 3
	if a.vitals.Max() != wantMax {
		t.Fatalf("max HP = %d, want %d (Toughness +3)", a.vitals.Max(), wantMax)
	}
	if a.vitals.Current() != wantMax {
		t.Errorf("current HP = %d, want %d (full) — fill ran BEFORE the max_hp feat grant (the 20/23 bug)",
			a.vitals.Current(), wantMax)
	}
}

// A character whose origin grants no max_hp feat still spawns full, and the fill
// is a no-op (nothing raised the ceiling). Guards that the reorder didn't regress
// the common case.
func TestApplyCharacterCreated_NoMaxHPFeatStaysFull(t *testing.T) {
	mgr := NewManager()
	a, _ := newFakeActor("c1", "p1", "acc1", "Plain", &world.Room{ID: "r"})
	a.feats = feat.NewRegistry()
	a.statBlock.OnMaxChange(progression.StatHPMax, func(_, newMax int) { a.vitals.SetMax(newMax) })
	mgr.Add(a)

	a.backgroundID = "street-kid"
	backgrounds := progression.NewBackgroundRegistry()
	_ = backgrounds.Register(&progression.Background{ID: "street-kid", FeatOptions: []string{"alertness"}})
	_ = a.feats.Register(&feat.Feat{ID: "alertness"}) // no grants
	granter := NewBackgroundGranter(mgr, nil, item.NewTemplates(), entities.NewStore(), nil)

	mgr.ApplyCharacterCreated(context.Background(), "p1", CreationGrants{
		Backgrounds: backgrounds,
		Granter:     granter,
	})

	if a.vitals.Current() != combat.DefaultPlayerMaxHP || a.vitals.Max() != combat.DefaultPlayerMaxHP {
		t.Errorf("vitals = %d/%d, want %d/%d (unchanged, full)",
			a.vitals.Current(), a.vitals.Max(), combat.DefaultPlayerMaxHP, combat.DefaultPlayerMaxHP)
	}
}

// An offline / unknown player id is a safe no-op.
func TestApplyCharacterCreated_OfflineNoop(t *testing.T) {
	mgr := NewManager()
	// No panic, no effect — the actor isn't in the manager.
	mgr.ApplyCharacterCreated(context.Background(), "ghost", CreationGrants{})
}
