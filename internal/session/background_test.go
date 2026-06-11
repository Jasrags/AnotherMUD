package session

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/feat"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

func TestBackgroundGranter_GrantsSkillsAndItems(t *testing.T) {
	mgr := NewManager()
	a, _ := newFakeActor("c1", "p1", "acc1", "Hero", &world.Room{ID: "r"})
	mgr.Add(a)

	store := entities.NewStore()
	tpls := item.NewTemplates()
	tpls.Add(&item.Template{ID: "core:lockpicks", Name: "lockpicks", Type: "item"})

	abilities := progression.NewAbilityRegistry()
	_ = abilities.Register(&progression.Ability{ID: "open-lock", Type: progression.AbilityPassive, Category: progression.AbilitySkill, DefaultCap: 100})
	prof := progression.NewProficiencyManager(abilities, progression.DefaultProficiencyConfig())

	g := NewBackgroundGranter(mgr, prof, tpls, store, nil) // nil currency → gold skipped
	bg := &progression.Background{
		ID:     "thief",
		Skills: []progression.BackgroundSkill{{AbilityID: "open-lock", Proficiency: 15}},
		Items:  []string{"core:lockpicks"},
		Gold:   50, // skipped (nil currency)
	}
	g.Grant(context.Background(), "p1", bg)

	// Skill learned at the declared starting proficiency.
	if v, ok := prof.Proficiency("p1", "open-lock"); !ok || v != 15 {
		t.Errorf("open-lock proficiency = (%d, %v), want (15, true)", v, ok)
	}
	// Item spawned into inventory.
	inv := a.Inventory()
	if len(inv) != 1 {
		t.Fatalf("inventory = %d items, want 1", len(inv))
	}
	if ent, ok := store.GetByID(inv[0]); !ok {
		t.Error("granted item not tracked in store")
	} else if inst, ok := ent.(*entities.ItemInstance); !ok || string(inst.TemplateID()) != "core:lockpicks" {
		t.Errorf("granted item template = %v", ent)
	}
}

func TestBackgroundGranter_DefaultsProficiencyAndSkipsMissing(t *testing.T) {
	mgr := NewManager()
	a, _ := newFakeActor("c1", "p1", "acc1", "Hero", &world.Room{ID: "r"})
	mgr.Add(a)

	abilities := progression.NewAbilityRegistry()
	_ = abilities.Register(&progression.Ability{ID: "open-lock", Type: progression.AbilityPassive, Category: progression.AbilitySkill, DefaultCap: 100})
	prof := progression.NewProficiencyManager(abilities, progression.DefaultProficiencyConfig())

	g := NewBackgroundGranter(mgr, prof, item.NewTemplates(), entities.NewStore(), nil)
	g.Grant(context.Background(), "p1", &progression.Background{
		ID: "soldier",
		// proficiency 0 → defaults to baseline 1.
		Skills: []progression.BackgroundSkill{{AbilityID: "open-lock", Proficiency: 0}},
		Items:  []string{"core:nope"}, // missing template → skipped silently
	})

	if v, _ := prof.Proficiency("p1", "open-lock"); v != 1 {
		t.Errorf("default proficiency = %d, want 1", v)
	}
	if len(a.Inventory()) != 0 {
		t.Error("a missing item template should grant nothing")
	}
}

func TestBackgroundGranter_OfflineAndNilAreNoops(t *testing.T) {
	mgr := NewManager() // actor never Added → offline
	g := NewBackgroundGranter(mgr, nil, item.NewTemplates(), entities.NewStore(), nil)
	// Offline recipient: silent no-op, no panic.
	g.Grant(context.Background(), "ghost", &progression.Background{ID: "x", Gold: 10})
	// nil background: no-op.
	g.Grant(context.Background(), "p1", nil)
}

// The creation wizard offers a background step when backgrounds are loaded, and
// the chosen id is committed to the save.
func TestRunCreation_CommitsBackground(t *testing.T) {
	rr, cr := twoRaceOneClass(t)
	br := progression.NewBackgroundRegistry()
	_ = br.Register(&progression.Background{ID: "soldier", DisplayName: "Soldier"})
	cfg := Config{CreationFlow: NewCreationFlow(rr, cr, br)}
	loaded := newPlayerLoaded("Bob")
	conn := &scriptedConn{inputs: []string{"elf", "fighter", "soldier", "yes"}}

	if err := runCreation(context.Background(), conn, cfg, loaded); err != nil {
		t.Fatalf("runCreation: %v", err)
	}
	if loaded.Player.Background != "soldier" {
		t.Errorf("save background = %q, want soldier", loaded.Player.Background)
	}
}

// feats §2 (EPIC S4 Phase 5): a background grants authored feats free at
// creation — recorded + applied, no slot spent, no prereq checked.
func TestBackgroundGranter_GrantsFeats(t *testing.T) {
	mgr := NewManager()
	a, _ := newFakeActor("c1", "p1", "acc1", "Hero", &world.Room{ID: "r"})
	a.featCredits = 1 // the creation slot — must NOT be consumed by the grant
	reg := feat.NewRegistry()
	_ = reg.Register(&feat.Feat{ID: "great-fortitude",
		Grants: []feat.Grant{{Kind: feat.GrantSaveBonus, Target: "fortitude", Magnitude: 2}}})
	a.feats = reg
	mgr.Add(a)

	g := NewBackgroundGranter(mgr, nil, item.NewTemplates(), entities.NewStore(), nil)
	g.Grant(context.Background(), "p1", &progression.Background{
		ID: "soldier", Feats: []string{"great-fortitude", "ghost-feat"}, // ghost skipped fail-soft
	})

	if len(a.save.KnownFeats) != 1 || a.save.KnownFeats[0].FeatID != "great-fortitude" {
		t.Fatalf("KnownFeats = %+v, want [great-fortitude]", a.save.KnownFeats)
	}
	if a.FeatCredits() != 1 {
		t.Errorf("FeatCredits = %d, want 1 (an authored grant must not spend a slot)", a.FeatCredits())
	}
	if a.Saves().Fortitude < 2 {
		t.Errorf("Fortitude = %d, want >= 2 from the granted feat", a.Saves().Fortitude)
	}
	// Re-granting (e.g. a relog re-fire) is idempotent for a single feat.
	a.GrantFeat("great-fortitude", "")
	if len(a.save.KnownFeats) != 1 {
		t.Errorf("re-grant duplicated a single feat: %+v", a.save.KnownFeats)
	}
}

// An authored grant is GRANT-ONCE even for a stackable feat: re-firing must not
// inflate the stack (a background grants "you have this feat", not "+1 stack").
func TestGrantFeat_StackableIsGrantOnce(t *testing.T) {
	a, _ := newFakeActor("c1", "p1", "acc1", "Hero", &world.Room{ID: "r"})
	reg := feat.NewRegistry()
	_ = reg.Register(&feat.Feat{ID: "toughness", MultiTake: feat.MultiTakeStackable,
		Grants: []feat.Grant{{Kind: feat.GrantMaxHP, Magnitude: 3}}})
	a.feats = reg

	base := a.statBlock.Effective(progression.StatHPMax)
	a.GrantFeat("toughness", "")
	a.GrantFeat("toughness", "") // re-fire — must be a no-op, not a second stack
	if len(a.save.KnownFeats) != 1 || a.save.KnownFeats[0].Count != 1 {
		t.Errorf("stackable authored grant stacked on re-fire: %+v", a.save.KnownFeats)
	}
	if got := a.statBlock.Effective(progression.StatHPMax); got != base+3 {
		t.Errorf("hp_max = %d, want %d (granted once, +3 only)", got, base+3)
	}
}

// feats §2.2 (EPIC S4 Phase 2): CreditFeats banks slots, syncs the save, and
// ignores non-positive credits.
func TestCreditFeats_BanksAndSyncsSave(t *testing.T) {
	a, _ := newFakeActor("c1", "p1", "acc1", "Hero", &world.Room{ID: "r"})

	if a.FeatCredits() != 0 {
		t.Fatalf("initial FeatCredits = %d, want 0", a.FeatCredits())
	}
	a.CreditFeats(1) // creation slot
	a.CreditFeats(2) // a 6th-level jump, say
	if got := a.FeatCredits(); got != 3 {
		t.Errorf("FeatCredits = %d, want 3", got)
	}
	if a.save.FeatCredits != 3 {
		t.Errorf("save.FeatCredits = %d, want 3 (synced)", a.save.FeatCredits)
	}
	// Non-positive credits are no-ops (the spend side lives in the feat verb).
	a.CreditFeats(0)
	a.CreditFeats(-5)
	if got := a.FeatCredits(); got != 3 {
		t.Errorf("FeatCredits after no-op credits = %d, want 3", got)
	}
}

// With no backgrounds loaded, creation skips the background step entirely and
// the character is background-less (backgrounds §3, last acceptance criterion).
func TestRunCreation_NoBackgroundsSkipsStep(t *testing.T) {
	rr, cr := twoRaceOneClass(t)
	br := progression.NewBackgroundRegistry() // empty → no background step
	cfg := Config{CreationFlow: NewCreationFlow(rr, cr, br)}
	loaded := newPlayerLoaded("Bob")
	// Only race + class + confirm are prompted — no background input.
	conn := &scriptedConn{inputs: []string{"elf", "fighter", "yes"}}

	if err := runCreation(context.Background(), conn, cfg, loaded); err != nil {
		t.Fatalf("runCreation: %v", err)
	}
	if loaded.Player.Background != "" {
		t.Errorf("save background = %q, want empty (no backgrounds loaded)", loaded.Player.Background)
	}
}
