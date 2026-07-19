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
	g.Grant(context.Background(), "p1", bg, BackgroundChoices{})

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

// GrantStartingItems is the class role-"floor" grant path (role×origin creation):
// it spawns the given templates into the online character's inventory, no-ops on
// an empty list or an offline recipient, and skips a missing template fail-soft.
func TestBackgroundGranter_GrantStartingItems(t *testing.T) {
	mgr := NewManager()
	a, _ := newFakeActor("c1", "p1", "acc1", "Hero", &world.Room{ID: "r"})
	mgr.Add(a)

	store := entities.NewStore()
	tpls := item.NewTemplates()
	tpls.Add(&item.Template{ID: "shadowrun:stun-baton", Name: "stun baton", Type: "item"})

	g := NewBackgroundGranter(mgr, nil, tpls, store, nil)

	// Empty list: no-op.
	g.GrantStartingItems("p1", nil)
	if len(a.Inventory()) != 0 {
		t.Fatalf("empty list granted %d items, want 0", len(a.Inventory()))
	}
	// Offline recipient: silent no-op, no panic.
	g.GrantStartingItems("ghost", []string{"shadowrun:stun-baton"})

	// The floor weapon spawns; a missing template in the same call is skipped.
	g.GrantStartingItems("p1", []string{"shadowrun:stun-baton", "shadowrun:nope"})
	inv := a.Inventory()
	if len(inv) != 1 {
		t.Fatalf("inventory = %d items, want 1 (floor weapon; missing template skipped)", len(inv))
	}
	if ent, ok := store.GetByID(inv[0]); !ok || string(ent.(*entities.ItemInstance).TemplateID()) != "shadowrun:stun-baton" {
		t.Errorf("granted item = %v, want shadowrun:stun-baton", ent)
	}
}

// Role×origin skill merge (creation): the class path teaches an overlapping
// skill at the baseline floor (grantDefaultAbilityProf = 1) BEFORE the background
// grant runs — the character.created order — and the background's declared
// proficiency overwrites it. Because the class floor is the minimum and a
// background clamps to max(prof, 1), the origin's trained value always wins on an
// overlap (last-wins == higher-wins). Pins that a role+origin overlap doesn't
// strand the character at the class baseline.
func TestBackgroundGranter_OriginSkillWinsOverClassFloor(t *testing.T) {
	mgr := NewManager()
	a, _ := newFakeActor("c1", "p1", "acc1", "Hero", &world.Room{ID: "r"})
	mgr.Add(a)

	abilities := progression.NewAbilityRegistry()
	_ = abilities.Register(&progression.Ability{ID: "perception", Type: progression.AbilityPassive, Category: progression.AbilitySkill, DefaultCap: 100})
	prof := progression.NewProficiencyManager(abilities, progression.DefaultProficiencyConfig())

	// The class path teaches the overlapping skill at the baseline floor first.
	prof.Learn("p1", "perception", 1)

	// Then the origin grants it at its trained value (mirrors the subscriber order).
	g := NewBackgroundGranter(mgr, prof, item.NewTemplates(), entities.NewStore(), nil)
	g.Grant(context.Background(), "p1", &progression.Background{
		ID:     "corporate-dropout",
		Skills: []progression.BackgroundSkill{{AbilityID: "perception", Proficiency: 10}},
	}, BackgroundChoices{})

	if v, ok := prof.Proficiency("p1", "perception"); !ok || v != 10 {
		t.Errorf("perception = (%d,%v), want (10,true) — origin's trained value must win over the class floor", v, ok)
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
	}, BackgroundChoices{})

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
	g.Grant(context.Background(), "ghost", &progression.Background{ID: "x", Gold: 10}, BackgroundChoices{})
	// nil background: no-op.
	g.Grant(context.Background(), "p1", nil, BackgroundChoices{})
}

// The creation wizard offers a background step when backgrounds are loaded, and
// the chosen id is committed to the save.
func TestRunCreation_CommitsBackground(t *testing.T) {
	rr, cr := twoRaceOneClass(t)
	br := progression.NewBackgroundRegistry()
	_ = br.Register(&progression.Background{ID: "soldier", DisplayName: "Soldier"})
	cfg := Config{CreationFlow: NewCreationFlow(rr, cr, br, nil)}
	loaded := newPlayerLoaded("Bob")
	conn := &scriptedConn{inputs: []string{"male", "elf", "fighter", "soldier", "yes"}}

	if err := runCreation(context.Background(), conn, cfg, loaded); err != nil {
		t.Fatalf("runCreation: %v", err)
	}
	if loaded.Player.Background != "soldier" {
		t.Errorf("save background = %q, want soldier", loaded.Player.Background)
	}
}

// The pick-one chooser: a background with FeatOptions + EquipmentPackages grants
// the CHOSEN feat + the CHOSEN package, not all of them (backgrounds §2).
func TestBackgroundGranter_GrantsChosenFeatAndPackage(t *testing.T) {
	mgr := NewManager()
	a, _ := newFakeActor("c1", "p1", "acc1", "Hero", &world.Room{ID: "r"})
	reg := feat.NewRegistry()
	_ = reg.Register(&feat.Feat{ID: "stealthy"})
	_ = reg.Register(&feat.Feat{ID: "blooded"})
	a.feats = reg
	mgr.Add(a)

	store := entities.NewStore()
	tpls := item.NewTemplates()
	tpls.Add(&item.Template{ID: "wot:studded-leather", Name: "studded leather", Type: "item"})
	tpls.Add(&item.Template{ID: "wot:mail-shirt", Name: "mail shirt", Type: "item"})

	g := NewBackgroundGranter(mgr, nil, tpls, store, nil)
	bg := &progression.Background{
		ID:                "borderlander",
		FeatOptions:       []string{"stealthy", "blooded"},
		EquipmentPackages: [][]string{{"wot:studded-leather"}, {"wot:mail-shirt"}},
	}
	// Choose Blooded + the second package (mail shirt).
	g.Grant(context.Background(), "p1", bg, BackgroundChoices{Feat: "blooded", EquipmentIndex: 1})

	if len(a.save.KnownFeats) != 1 || a.save.KnownFeats[0].FeatID != "blooded" {
		t.Errorf("KnownFeats = %+v, want only [blooded]", a.save.KnownFeats)
	}
	inv := a.Inventory()
	if len(inv) != 1 {
		t.Fatalf("inventory = %d items, want 1 (only the chosen package)", len(inv))
	}
	if ent, ok := store.GetByID(inv[0]); !ok || string(ent.(*entities.ItemInstance).TemplateID()) != "wot:mail-shirt" {
		t.Errorf("granted item = %v, want wot:mail-shirt", ent)
	}
}

// A single-option / no-choice background auto-grants the first feat option and
// the first package without any recorded choice (the step is skipped at creation).
func TestBackgroundGranter_AutoGrantsSingleOption(t *testing.T) {
	mgr := NewManager()
	a, _ := newFakeActor("c2", "p2", "acc2", "Solo", &world.Room{ID: "r"})
	reg := feat.NewRegistry()
	_ = reg.Register(&feat.Feat{ID: "militia"})
	a.feats = reg
	mgr.Add(a)

	g := NewBackgroundGranter(mgr, nil, item.NewTemplates(), entities.NewStore(), nil)
	bg := &progression.Background{ID: "cairhienin", FeatOptions: []string{"militia"}}
	g.Grant(context.Background(), "p2", bg, BackgroundChoices{}) // no choice recorded

	if len(a.save.KnownFeats) != 1 || a.save.KnownFeats[0].FeatID != "militia" {
		t.Errorf("KnownFeats = %+v, want [militia] (single option auto-granted)", a.save.KnownFeats)
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
	}, BackgroundChoices{})

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

// languages.md §3: a background grants its home language at creation —
// idempotent, fail-soft, and skipped when unset.
func TestBackgroundGranter_GrantsHomeLanguage(t *testing.T) {
	mgr := NewManager()
	a, _ := newFakeActor("c1", "p1", "acc1", "Hero", &world.Room{ID: "r"})
	mgr.Add(a)
	g := NewBackgroundGranter(mgr, nil, nil, nil, nil) // nil prof/tpls/store/currency: only the language grant under test

	g.Grant(context.Background(), "p1", &progression.Background{
		ID: "aiel", HomeLanguage: "wot:common-aiel",
	}, BackgroundChoices{})
	if len(a.save.KnownLanguages) != 1 || a.save.KnownLanguages[0] != "wot:common-aiel" {
		t.Fatalf("KnownLanguages = %+v, want [wot:common-aiel]", a.save.KnownLanguages)
	}
	// Re-granting (a relog re-fire) does not duplicate the home language.
	g.Grant(context.Background(), "p1", &progression.Background{ID: "aiel", HomeLanguage: "wot:common-aiel"}, BackgroundChoices{})
	if len(a.save.KnownLanguages) != 1 {
		t.Errorf("re-grant duplicated the home language: %+v", a.save.KnownLanguages)
	}

	// A background with no home language grants none (and does not error).
	a2, _ := newFakeActor("c2", "p2", "acc2", "Hero2", &world.Room{ID: "r"})
	mgr.Add(a2)
	g.Grant(context.Background(), "p2", &progression.Background{ID: "languageless"}, BackgroundChoices{})
	if len(a2.save.KnownLanguages) != 0 {
		t.Errorf("a home-language-less background granted %+v", a2.save.KnownLanguages)
	}
}

// backgrounds.md §Restrictions: a background's weapon restriction is DERIVED at
// login from the registry (no save field) and refuses the forbidden categories.
func TestApplyBackground_DerivesWeaponRestriction(t *testing.T) {
	reg := progression.NewBackgroundRegistry()
	_ = reg.Register(&progression.Background{
		ID:                       "aiel",
		WeaponRestrictions:       []string{"Longsword", "Short-Sword"}, // mixed case → lowercased at Register
		WeaponRestrictionMessage: "Not the Aiel way.",
	})

	a, _ := newFakeActor("c1", "p1", "acc1", "Hero", &world.Room{ID: "r"})
	applyBackground(a, "Aiel", reg)

	// A restricted category is refused with the authored message (case-insensitive).
	if msg := a.WeaponRestrictionRefusal("longsword"); msg != "Not the Aiel way." {
		t.Errorf("longsword refusal = %q, want the authored message", msg)
	}
	if msg := a.WeaponRestrictionRefusal("SHORT-SWORD"); msg == "" {
		t.Error("short-sword should be refused (case-insensitive)")
	}
	// An allowed weapon + a non-weapon (empty category) are never refused.
	if msg := a.WeaponRestrictionRefusal("shortbow"); msg != "" {
		t.Errorf("shortbow (allowed) refusal = %q, want empty", msg)
	}
	if msg := a.WeaponRestrictionRefusal(""); msg != "" {
		t.Errorf("non-weapon refusal = %q, want empty", msg)
	}

	// A background with no restriction (or a nil registry) refuses nothing.
	a2, _ := newFakeActor("c2", "p2", "acc2", "Plain", &world.Room{ID: "r"})
	applyBackground(a2, "aiel", nil) // nil registry → no restriction derived
	if msg := a2.WeaponRestrictionRefusal("longsword"); msg != "" {
		t.Errorf("nil-registry actor refused %q, want empty", msg)
	}
}

// The generic fallback fires when a background restricts a category but authors
// no custom message.
func TestWeaponRestrictionRefusal_GenericFallback(t *testing.T) {
	reg := progression.NewBackgroundRegistry()
	_ = reg.Register(&progression.Background{ID: "stoic", WeaponRestrictions: []string{"rapier"}})
	a, _ := newFakeActor("c1", "p1", "acc1", "Hero", &world.Room{ID: "r"})
	applyBackground(a, "stoic", reg)
	if msg := a.WeaponRestrictionRefusal("rapier"); msg == "" {
		t.Error("a restricted category with no authored message should use the generic refusal")
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
	cfg := Config{CreationFlow: NewCreationFlow(rr, cr, br, nil)}
	loaded := newPlayerLoaded("Bob")
	// Only gender + race + class + confirm are prompted — no background input.
	conn := &scriptedConn{inputs: []string{"male", "elf", "fighter", "yes"}}

	if err := runCreation(context.Background(), conn, cfg, loaded); err != nil {
		t.Fatalf("runCreation: %v", err)
	}
	if loaded.Player.Background != "" {
		t.Errorf("save background = %q, want empty (no backgrounds loaded)", loaded.Player.Background)
	}
}
