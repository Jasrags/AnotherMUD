package entities

import (
	"errors"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/item"
)

// armorHost spawns a modifiable armor host with the given capacity and an
// "armor" tag (the host-compat key mods match against, item-modification §4).
func armorHost(t *testing.T, s *Store, capacity, armorBonus int) *ItemInstance {
	t.Helper()
	it, err := s.Spawn(&item.Template{
		ID: "sr:vest", Name: "an armor vest", Type: "item",
		Tags: []string{"armor"}, Capacity: capacity, ArmorBonus: armorBonus,
	})
	if err != nil {
		t.Fatalf("spawn host: %v", err)
	}
	return it
}

// armorMod spawns an armor modification with the given cost, resistances, and AC.
func armorMod(t *testing.T, s *Store, id string, cost int, resist map[string]int, armorBonus int) *ItemInstance {
	t.Helper()
	it, err := s.Spawn(&item.Template{
		ID: item.TemplateID(id), Name: id, Type: "item",
		ModHost: "armor", ModCapacityCost: cost, Resistances: resist, ArmorBonus: armorBonus,
	})
	if err != nil {
		t.Fatalf("spawn mod: %v", err)
	}
	return it
}

func TestInstallMod_HappyPath_CapacityAndEffectiveFields(t *testing.T) {
	s := NewStore()
	host := armorHost(t, s, 9, 2) // capacity 9, own AC 2
	fireResist := armorMod(t, s, "sr:fire-resistance", 3, map[string]int{"fire": 4}, 0)

	if got := host.FreeCapacity(); got != 9 {
		t.Fatalf("FreeCapacity before = %d, want 9", got)
	}
	if err := host.InstallMod(fireResist); err != nil {
		t.Fatalf("InstallMod: %v", err)
	}
	// Capacity accounting (§2).
	if got := host.UsedCapacity(); got != 3 {
		t.Fatalf("UsedCapacity = %d, want 3", got)
	}
	if got := host.FreeCapacity(); got != 6 {
		t.Fatalf("FreeCapacity after = %d, want 6", got)
	}
	// Effective resistances = host (none) + mod (fire 4) (§6, typed field).
	if got := host.Resistances()["fire"]; got != 4 {
		t.Fatalf("effective fire resistance = %d, want 4", got)
	}
	// A mod granting no AC leaves the host's own ArmorBonus untouched.
	if got := host.ArmorBonus(); got != 2 {
		t.Fatalf("ArmorBonus = %d, want 2 (host only)", got)
	}
	if got := len(host.InstalledMods()); got != 1 {
		t.Fatalf("InstalledMods len = %d, want 1", got)
	}
}

func TestInstallMod_MultipleMods_StackWithinBudget(t *testing.T) {
	s := NewStore()
	host := armorHost(t, s, 9, 0)
	if err := host.InstallMod(armorMod(t, s, "sr:fire", 3, map[string]int{"fire": 4}, 0)); err != nil {
		t.Fatalf("install 1: %v", err)
	}
	if err := host.InstallMod(armorMod(t, s, "sr:plate", 4, map[string]int{"physical": 2}, 3)); err != nil {
		t.Fatalf("install 2: %v", err)
	}
	if got := host.UsedCapacity(); got != 7 {
		t.Fatalf("UsedCapacity = %d, want 7", got)
	}
	// Effective fields aggregate across both mods (§6).
	r := host.Resistances()
	if r["fire"] != 4 || r["physical"] != 2 {
		t.Fatalf("resistances = %v, want fire:4 physical:2", r)
	}
	if got := host.ArmorBonus(); got != 3 {
		t.Fatalf("ArmorBonus = %d, want 3 (from plate mod)", got)
	}
}

func TestInstallMod_RefusedOverCapacity(t *testing.T) {
	s := NewStore()
	host := armorHost(t, s, 4, 0)
	if err := host.InstallMod(armorMod(t, s, "sr:a", 3, nil, 0)); err != nil {
		t.Fatalf("install 1: %v", err)
	}
	// A 2-cost mod does not fit the remaining 1.
	err := host.InstallMod(armorMod(t, s, "sr:b", 2, nil, 0))
	if !errors.Is(err, ErrModNoCapacity) {
		t.Fatalf("over-capacity install err = %v, want ErrModNoCapacity", err)
	}
	if got := host.UsedCapacity(); got != 3 {
		t.Fatalf("UsedCapacity after refusal = %d, want 3 (unchanged)", got)
	}
}

func TestInstallMod_RefusedIncompatibleAndNonModAndNonModifiable(t *testing.T) {
	s := NewStore()
	host := armorHost(t, s, 9, 0)

	// A weapon mod (host key "weapon") does not fit an armor host.
	weaponMod, _ := s.Spawn(&item.Template{ID: "sr:scope", Name: "scope", Type: "item", ModHost: "weapon", ModCapacityCost: 1})
	if err := host.InstallMod(weaponMod); !errors.Is(err, ErrModIncompatible) {
		t.Fatalf("incompatible install err = %v, want ErrModIncompatible", err)
	}
	// A plain item is not a modification.
	plain, _ := s.Spawn(&item.Template{ID: "sr:rock", Name: "rock", Type: "item"})
	if err := host.InstallMod(plain); !errors.Is(err, ErrNotAModification) {
		t.Fatalf("non-mod install err = %v, want ErrNotAModification", err)
	}
	// A host with no capacity is unmodifiable.
	plainHost := armorHost(t, s, 0, 0)
	if err := plainHost.InstallMod(armorMod(t, s, "sr:m", 1, nil, 0)); !errors.Is(err, ErrNotModifiable) {
		t.Fatalf("unmodifiable install err = %v, want ErrNotModifiable", err)
	}
}

func TestRemoveMod_RestoresCapacityAndReversesEffect(t *testing.T) {
	s := NewStore()
	host := armorHost(t, s, 9, 0)
	_ = host.InstallMod(armorMod(t, s, "sr:fire-resistance", 3, map[string]int{"fire": 4}, 0))

	removed, ok := host.RemoveMod("fire")
	if !ok {
		t.Fatal("RemoveMod returned ok=false")
	}
	if removed.TemplateID != "sr:fire-resistance" {
		t.Fatalf("removed TemplateID = %q, want sr:fire-resistance", removed.TemplateID)
	}
	if got := host.FreeCapacity(); got != 9 {
		t.Fatalf("FreeCapacity after remove = %d, want 9", got)
	}
	if r := host.Resistances(); r != nil {
		t.Fatalf("resistances after remove = %v, want nil", r)
	}
	// Removing again with no match returns ok=false.
	if _, ok := host.RemoveMod("nope"); ok {
		t.Fatal("RemoveMod matched a non-existent mod")
	}
}

func TestRestoreInstalledMod_RebuildsFromTemplate(t *testing.T) {
	s := NewStore()
	host := armorHost(t, s, 9, 0)
	modTpl := &item.Template{
		ID: "sr:fire-resistance", Name: "fire resistance coating", Type: "item",
		ModHost: "armor", ModCapacityCost: 3, Resistances: map[string]int{"fire": 4},
	}
	host.RestoreInstalledMod(modTpl)

	if got := host.UsedCapacity(); got != 3 {
		t.Fatalf("UsedCapacity = %d, want 3", got)
	}
	if got := host.Resistances()["fire"]; got != 4 {
		t.Fatalf("effective fire resistance = %d, want 4", got)
	}
	if mods := host.InstalledMods(); len(mods) != 1 || mods[0].TemplateID != "sr:fire-resistance" {
		t.Fatalf("InstalledMods = %+v, want one sr:fire-resistance", mods)
	}
}

func TestInstallMod_GrantsAndReversesProtection(t *testing.T) {
	s := NewStore()
	host := armorHost(t, s, 9, 0)
	if got := host.GrantedProtections(); got != nil {
		t.Fatalf("GrantedProtections before = %v, want nil", got)
	}
	// A chemical-seal-style mod grants the rad-shielded protection key while worn
	// (item-modification §6 → area-effects §4.6 immunity).
	seal, _ := s.Spawn(&item.Template{
		ID: "sr:chem-seal", Name: "a chemical seal", Type: "item",
		ModHost: "armor", ModCapacityCost: 6, Protection: []string{"rad-shielded"},
	})
	if err := host.InstallMod(seal); err != nil {
		t.Fatalf("InstallMod: %v", err)
	}
	if got := host.GrantedProtections(); len(got) != 1 || got[0] != "rad-shielded" {
		t.Fatalf("GrantedProtections after install = %v, want [rad-shielded]", got)
	}
	// Removing the mod removes the protection.
	if _, ok := host.RemoveMod("seal"); !ok {
		t.Fatal("RemoveMod failed")
	}
	if got := host.GrantedProtections(); got != nil {
		t.Fatalf("GrantedProtections after remove = %v, want nil", got)
	}
}

func TestInstallMod_GrantsCapability(t *testing.T) {
	s := NewStore()
	// A cybereye host + a smartlink enhancement that grants the "smartlink"
	// capability. (Uses the armor rule here — the mechanic is host-agnostic.)
	host := armorHost(t, s, 9, 0)
	link, _ := s.Spawn(&item.Template{
		ID: "sr:smartlink", Name: "a smartlink", Type: "item",
		ModHost: "armor", ModCapacityCost: 3, Grants: []string{"smartlink"},
	})
	if host.ProvidesCapability("smartlink") {
		t.Fatal("host provides smartlink before install")
	}
	if err := host.InstallMod(link); err != nil {
		t.Fatalf("InstallMod: %v", err)
	}
	if got := host.GrantedCapabilities(); len(got) != 1 || got[0] != "smartlink" {
		t.Fatalf("GrantedCapabilities = %v, want [smartlink]", got)
	}
	if !host.ProvidesCapability("SMARTLINK") { // case-insensitive
		t.Fatal("ProvidesCapability(smartlink) = false after install")
	}
	if _, ok := host.RemoveMod("smartlink"); !ok {
		t.Fatal("RemoveMod failed")
	}
	if host.ProvidesCapability("smartlink") {
		t.Fatal("capability not reversed on remove")
	}
}

// A monolithic worn capability item (no installed mods) provides its capability
// through an intrinsic TAG — the path low-light goggles / a racial-grant item
// rely on, distinct from the installed-mod grant path above.
func TestProvidesCapability_IntrinsicTag(t *testing.T) {
	s := NewStore()
	goggles, err := s.Spawn(&item.Template{
		ID: "sr:low-light-goggles", Name: "a pair of low-light goggles", Type: "item",
		Tags: []string{"eyewear", "low-light"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if !goggles.ProvidesCapability("low-light") {
		t.Error("a worn item must provide a capability declared as an intrinsic tag")
	}
	if goggles.ProvidesCapability("thermographic") {
		t.Error("must not provide a capability it does not carry")
	}
	// The intrinsic-tag path is case-SENSITIVE (HasTag exact match), unlike the
	// installed-mod path (EqualFold). Harmless here — capability keys are fixed
	// lowercase constants and content tags are authored lowercase — but asserted
	// so the asymmetry is a documented, intentional contract, not a latent bug.
	if goggles.ProvidesCapability("LOW-LIGHT") {
		t.Error("intrinsic-tag path is exact-match; a case variant should not match (documents the asymmetry)")
	}
}

func TestRestoreInstalledMod_RebuildsProtection(t *testing.T) {
	s := NewStore()
	host := armorHost(t, s, 9, 0)
	host.RestoreInstalledMod(&item.Template{
		ID: "sr:chem-seal", Name: "a chemical seal", Type: "item",
		ModHost: "armor", ModCapacityCost: 6, Protection: []string{"rad-shielded"},
	})
	if got := host.GrantedProtections(); len(got) != 1 || got[0] != "rad-shielded" {
		t.Fatalf("GrantedProtections after restore = %v, want [rad-shielded]", got)
	}
}

func TestUnmodifiedItem_UnchangedBehavior(t *testing.T) {
	s := NewStore()
	// An item with no capacity + no mods returns exactly its intrinsic fields.
	it, _ := s.Spawn(&item.Template{
		ID: "sr:plain-vest", Name: "vest", Type: "item", Tags: []string{"armor"},
		ArmorBonus: 5, Resistances: map[string]int{"physical": 1},
	})
	if it.Capacity() != 0 || it.FreeCapacity() != 0 {
		t.Fatalf("capacity = %d/%d, want 0/0", it.Capacity(), it.FreeCapacity())
	}
	if it.ArmorBonus() != 5 {
		t.Fatalf("ArmorBonus = %d, want 5", it.ArmorBonus())
	}
	if it.Resistances()["physical"] != 1 {
		t.Fatalf("physical resistance = %d, want 1", it.Resistances()["physical"])
	}
}
