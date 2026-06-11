package session

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
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
