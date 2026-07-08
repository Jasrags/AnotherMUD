package entities

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/item"
)

// TestItemInstancePropertiesConcurrentAccess pins the m11-5 fix: with
// the unguarded live-map Properties() this races (and `go test -race`
// fails); with the propsMu-guarded snapshot/Property/SetProperty it is
// safe. Mirrors the MobInstance guard.
func TestItemInstancePropertiesConcurrentAccess(t *testing.T) {
	s := NewStore()
	it, err := s.Spawn(&item.Template{
		ID: "core:potion", Name: "a potion", Type: "item",
		Properties: map[string]any{"charges": 5},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := range 200 {
				it.SetProperty("charges", n+j) // writer
				_, _ = it.Property("charges")  // single-key reader
				_ = it.Properties()            // snapshot reader
			}
		}(i)
	}
	wg.Wait()
}

// Weapon-identity §2: the category + proficiency tier are lifted onto the
// instance at build so the equip path reads them without the template
// registry (mirrors weaponDamage). Untyped weapons expose empty strings.
func TestItemInstance_WeaponIdentityFields(t *testing.T) {
	s := NewStore()
	it, err := s.Spawn(&item.Template{
		ID: "wot:longsword", Name: "a longsword", Type: "weapon",
		WeaponCategory: "longsword", ProficiencyTier: "martial",
		CritThreatLow: 19, CritMultiplier: 2,
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if it.WeaponCategory() != "longsword" {
		t.Errorf("WeaponCategory() = %q, want longsword", it.WeaponCategory())
	}
	if it.ProficiencyTier() != "martial" {
		t.Errorf("ProficiencyTier() = %q, want martial", it.ProficiencyTier())
	}
	if it.CritThreatLow() != 19 || it.CritMultiplier() != 2 {
		t.Errorf("crit params = (%d,%d), want (19,2)", it.CritThreatLow(), it.CritMultiplier())
	}

	rock, err := s.Spawn(&item.Template{ID: "wot:rock", Name: "a rock", Type: "item"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if rock.WeaponCategory() != "" || rock.ProficiencyTier() != "" {
		t.Errorf("non-weapon should expose empty identity, got cat=%q tier=%q",
			rock.WeaponCategory(), rock.ProficiencyTier())
	}
}

// shadowrun-mvp SR-M3b: an ItemInstance snapshots its template's target_pool
// and exposes it via TargetPool() for the holder's Stats() builder; an ordinary
// weapon reports "" (the hp path).
func TestItemInstance_TargetPool(t *testing.T) {
	s := NewStore()
	baton, err := s.Spawn(&item.Template{
		ID: "shadowrun:stun-baton", Name: "a stun baton", Type: "weapon",
		WeaponDamage: "1d6", TargetPool: "stun",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if got := baton.TargetPool(); got != "stun" {
		t.Errorf("TargetPool() = %q, want stun", got)
	}

	sword, err := s.Spawn(&item.Template{ID: "wot:sword", Name: "a sword", Type: "weapon", WeaponDamage: "1d8"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if got := sword.TargetPool(); got != "" {
		t.Errorf("TargetPool() = %q, want empty (hp path)", got)
	}
}

// armor-depth §2/§4: weapon damage types and armor resistances snapshot
// onto the instance and the accessors return fresh, unaliased copies.
func TestItemInstance_DamageTypesAndResistances(t *testing.T) {
	s := NewStore()
	sword, err := s.Spawn(&item.Template{
		ID: "wot:sword", Name: "a sword", Type: "weapon",
		WeaponDamage: "1d8", DamageTypes: []string{"slashing"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if got := sword.DamageTypes(); len(got) != 1 || got[0] != "slashing" {
		t.Errorf("DamageTypes() = %v, want [slashing]", got)
	}
	// Mutating the returned slice must not affect the instance.
	sword.DamageTypes()[0] = "mutated"
	if sword.DamageTypes()[0] != "slashing" {
		t.Error("DamageTypes() returned an aliased slice; mutation leaked")
	}

	armor, err := s.Spawn(&item.Template{
		ID: "wot:plate", Name: "plate", Type: "armor",
		Resistances: map[string]int{"slashing": 3, "piercing": 1},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	got := armor.Resistances()
	if got["slashing"] != 3 || got["piercing"] != 1 {
		t.Errorf("Resistances() = %v, want slashing:3 piercing:1", got)
	}
	got["slashing"] = 99 // mutate the copy
	if armor.Resistances()["slashing"] != 3 {
		t.Error("Resistances() returned an aliased map; mutation leaked")
	}

	rock, _ := s.Spawn(&item.Template{ID: "wot:pebble", Name: "a pebble", Type: "item"})
	if rock.DamageTypes() != nil || rock.Resistances() != nil {
		t.Errorf("plain item should expose nil types/resistances, got types=%v res=%v",
			rock.DamageTypes(), rock.Resistances())
	}
}

// Armor-depth §3/§5: ArmorBonus / ArmorMaxDex / ArmorTier lift onto the
// instance from the template, ArmorMaxDex returns a defensive copy, and a
// plain item exposes the zero/nil defaults.
func TestItemInstance_ArmorDepthMetadata(t *testing.T) {
	s := NewStore()
	cap := 1
	helm, err := s.Spawn(&item.Template{
		ID: "wot:great-helm", Name: "a great helm", Type: "item",
		ArmorBonus: 4, ArmorMaxDex: &cap, ArmorTier: "heavy",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if helm.ArmorBonus() != 4 {
		t.Errorf("ArmorBonus() = %d, want 4", helm.ArmorBonus())
	}
	if helm.ArmorTier() != "heavy" {
		t.Errorf("ArmorTier() = %q, want heavy", helm.ArmorTier())
	}
	mdx := helm.ArmorMaxDex()
	if mdx == nil || *mdx != 1 {
		t.Fatalf("ArmorMaxDex() = %v, want ptr to 1", mdx)
	}
	*mdx = 99 // mutate the returned copy
	if got := helm.ArmorMaxDex(); got == nil || *got != 1 {
		t.Error("ArmorMaxDex() returned an aliased pointer; mutation leaked")
	}

	plain, _ := s.Spawn(&item.Template{ID: "wot:rock", Name: "a rock", Type: "item"})
	if plain.ArmorBonus() != 0 || plain.ArmorTier() != "" || plain.ArmorMaxDex() != nil {
		t.Errorf("plain item armor metadata = (bonus %d, tier %q, maxDex %v), want (0, \"\", nil)",
			plain.ArmorBonus(), plain.ArmorTier(), plain.ArmorMaxDex())
	}
}

func TestDecrementInt(t *testing.T) {
	s := NewStore()
	it, err := s.Spawn(&item.Template{
		ID: "core:torch", Name: "a torch", Type: "light",
		Properties: map[string]any{"fuel": 3},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	if rem, zero := it.DecrementInt("fuel", 1); rem != 2 || zero {
		t.Fatalf("DecrementInt 3-1 = (%d,%v), want (2,false)", rem, zero)
	}
	if rem, zero := it.DecrementInt("fuel", 5); rem != 0 || !zero {
		t.Fatalf("DecrementInt 2-5 = (%d,%v), want (0,true) (floored)", rem, zero)
	}
	// Absent key is treated as zero and written as zero.
	if rem, zero := it.DecrementInt("missing", 1); rem != 0 || !zero {
		t.Fatalf("DecrementInt on absent = (%d,%v), want (0,true)", rem, zero)
	}
	if v, ok := it.Property("missing"); !ok || v.(int) != 0 {
		t.Fatalf("absent key after DecrementInt = (%v,%v), want (0,true)", v, ok)
	}
}

func TestDecrementIntConcurrent(t *testing.T) {
	// Two goroutines decrementing concurrently must not corrupt the map
	// or lose the floor invariant (race detector + final value check).
	s := NewStore()
	it, err := s.Spawn(&item.Template{
		ID: "core:torch", Name: "a torch", Type: "light",
		Properties: map[string]any{"fuel": 1000},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 250 {
				it.DecrementInt("fuel", 1)
			}
		})
	}
	wg.Wait()
	if v, _ := it.Property("fuel"); v.(int) != 0 {
		t.Fatalf("fuel after 1000 concurrent decrements = %v, want 0", v)
	}
}

func TestSpawnAssignsFreshIDAndCopiesFields(t *testing.T) {
	s := NewStore()
	tpl := &item.Template{
		ID:       "tapestry-core:short-sword",
		Name:     "a short sword",
		Type:     "item",
		Tags:     []string{"weapon", "metal"},
		Keywords: []string{"sword", "short"},
		Properties: map[string]any{
			"damage": 4,
		},
		Modifiers: []item.Modifier{{Stat: "str", Value: 1}},
	}

	a, err := s.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	b, err := s.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn second: %v", err)
	}

	if a.ID() == b.ID() {
		t.Fatalf("ids collided: %q == %q", a.ID(), b.ID())
	}
	if !strings.HasPrefix(string(a.ID()), "entity-") {
		t.Errorf("id prefix: %q", a.ID())
	}
	if a.Name() != tpl.Name {
		t.Errorf("Name = %q", a.Name())
	}
	if a.Type() != tpl.Type {
		t.Errorf("Type = %q", a.Type())
	}
	if a.TemplateID() != tpl.ID {
		t.Errorf("TemplateID = %q", a.TemplateID())
	}
	if got := a.Properties()[PropTemplateID]; got != string(tpl.ID) {
		t.Errorf("Properties[%s] = %v, want %q", PropTemplateID, got, tpl.ID)
	}
}

// TestSpawnLiftsEligibleAndCompanionSlots verifies R5: the template's
// equipment slot eligibility + footprint (§3.3) are copied onto the
// instance at spawn, and the accessors return fresh (non-aliasing)
// slices so callers cannot mutate instance state.
func TestSpawnLiftsEligibleAndCompanionSlots(t *testing.T) {
	s := NewStore()
	tpl := &item.Template{
		ID:             "tapestry-core:greatsword",
		Name:           "a greatsword",
		Type:           "item",
		EligibleSlots:  []string{"wield"},
		CompanionSlots: []string{"offhand"},
	}
	it, err := s.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if got := it.EligibleSlots(); len(got) != 1 || got[0] != "wield" {
		t.Errorf("EligibleSlots = %v, want [wield]", got)
	}
	if got := it.CompanionSlots(); len(got) != 1 || got[0] != "offhand" {
		t.Errorf("CompanionSlots = %v, want [offhand]", got)
	}
	// Mutating the returned slice must not corrupt instance state.
	got := it.EligibleSlots()
	got[0] = "tampered"
	if again := it.EligibleSlots(); again[0] != "wield" {
		t.Errorf("EligibleSlots aliased backing array: %v", again)
	}
}

// TestSpawnLiftsLegacySlotProperty: a template carrying only the legacy
// `properties.slot` string (no EligibleSlots) is still eligible for that
// one slot — the §3.2 bridge applied at instance build.
func TestSpawnLiftsLegacySlotProperty(t *testing.T) {
	s := NewStore()
	tpl := &item.Template{
		ID:         "tapestry-core:leather-cap",
		Name:       "a leather cap",
		Type:       "item",
		Properties: map[string]any{"slot": "head"},
	}
	it, err := s.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if got := it.EligibleSlots(); len(got) != 1 || got[0] != "head" {
		t.Errorf("EligibleSlots = %v, want [head] from legacy slot property", got)
	}
}

// TestSpawnNonEquippableHasNoSlots: a template with neither EligibleSlots
// nor a legacy slot property yields an empty eligible set — the item is
// not equippable (§3.4 step 3).
func TestSpawnNonEquippableHasNoSlots(t *testing.T) {
	s := NewStore()
	it, err := s.Spawn(&item.Template{
		ID:   "tapestry-core:quest-token",
		Name: "a wax seal",
		Type: "item",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if got := it.EligibleSlots(); len(got) != 0 {
		t.Errorf("EligibleSlots = %v, want empty", got)
	}
}

func TestSpawnFiltersRoomIDFromProperties(t *testing.T) {
	s := NewStore()
	tpl := &item.Template{
		ID:   "tapestry-core:foo",
		Name: "foo",
		Type: "item",
		Properties: map[string]any{
			"room_id": "tapestry-core:somewhere",
			"keep":    "ok",
		},
	}
	a, err := s.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if _, present := a.Properties()[PropRoomID]; present {
		t.Errorf("room_id leaked into instance properties: %+v", a.Properties())
	}
	if a.Properties()["keep"] != "ok" {
		t.Errorf("non-reserved property dropped: %+v", a.Properties())
	}
}

func TestSpawnDropsImplicitTypeTag(t *testing.T) {
	// §2.3 step 2: tag matching the entity's own type is implicit.
	s := NewStore()
	tpl := &item.Template{
		ID:   "tapestry-core:foo",
		Name: "foo",
		Type: "item",
		Tags: []string{"item", "weapon"},
	}
	a, err := s.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if got := a.Tags(); !reflect.DeepEqual(got, []string{"weapon"}) {
		t.Errorf("Tags = %v, want [weapon]", got)
	}
}

func TestSpawnNormalizesNestedUntypedMaps(t *testing.T) {
	// yaml.v3 can produce map[any]any for nested maps in some shapes.
	// §2.3 step 4 requires recursive normalization.
	s := NewStore()
	tpl := &item.Template{
		ID:   "tapestry-core:foo",
		Name: "foo",
		Type: "item",
		Properties: map[string]any{
			"nested": map[any]any{
				"inner": map[any]any{"k": 1},
				42:      "dropped non-string key",
			},
		},
	}
	a, err := s.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	nested, ok := a.Properties()["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested not normalized: %T", a.Properties()["nested"])
	}
	inner, ok := nested["inner"].(map[string]any)
	if !ok {
		t.Fatalf("inner not normalized: %T", nested["inner"])
	}
	if inner["k"] != 1 {
		t.Errorf("inner[k] = %v", inner["k"])
	}
	if _, present := nested["42"]; present {
		t.Errorf("non-string key was promoted to string: %+v", nested)
	}
}

func TestSpawnModifiersTaggedByEntityID(t *testing.T) {
	s := NewStore()
	tpl := &item.Template{
		ID:        "tapestry-core:sword",
		Name:      "sword",
		Type:      "item",
		Modifiers: []item.Modifier{{Stat: "str", Value: 2}, {Stat: "dex", Value: 1}},
	}
	a, _ := s.Spawn(tpl)
	b, _ := s.Spawn(tpl)

	for _, m := range a.Modifiers() {
		want := SourceKey("entity:" + string(a.ID()))
		if m.Source != want {
			t.Errorf("a modifier source = %q, want %q", m.Source, want)
		}
	}
	if a.Modifiers()[0].Source == b.Modifiers()[0].Source {
		t.Errorf("two instances share a source key: %q", a.Modifiers()[0].Source)
	}
}

func TestSpawnNilTemplateReturnsError(t *testing.T) {
	s := NewStore()
	_, err := s.Spawn(nil)
	if !errors.Is(err, ErrUnknownTemplate) {
		t.Errorf("err = %v, want ErrUnknownTemplate", err)
	}
}

func TestTakeCharge(t *testing.T) {
	it := &ItemInstance{properties: map[string]any{"charges": 2}}
	if r, ok := it.TakeCharge("charges"); !ok || r != 1 {
		t.Fatalf("first take = (%d, %v), want (1, true)", r, ok)
	}
	if r, ok := it.TakeCharge("charges"); !ok || r != 0 {
		t.Fatalf("second take = (%d, %v), want (0, true)", r, ok)
	}
	// Empty now: further takes refuse and never go negative.
	if r, ok := it.TakeCharge("charges"); ok || r != 0 {
		t.Errorf("take on empty = (%d, %v), want (0, false)", r, ok)
	}
	// Absent key → no charge to take.
	if _, ok := it.TakeCharge("missing"); ok {
		t.Error("take on absent key should be false")
	}
}
