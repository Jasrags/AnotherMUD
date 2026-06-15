package pack

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// TestLoad_WotPackSelectionBootSwap is the M0.5 proof: selecting the `wot`
// pack boots {tapestry-core, wot} via dependency closure — it pulls in the
// engine baseline but NOT the demo `starter-world` — and spawns into the WoT
// starter room. This is the whole point of the setting-unblock work: a
// different world boots in the demo's place by pack selection alone.
func TestLoad_WotPackSelectionBootSwap(t *testing.T) {
	root, err := filepath.Abs("../../content")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	regs := NewRegistries()
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatalf("baseline properties: %v", err)
	}
	if err := slot.RegisterEngineBaseline(regs.Slots); err != nil {
		t.Fatalf("baseline slots: %v", err)
	}
	// Select only wot; the loader's dependency closure adds tapestry-core.
	if err := Load(context.Background(), root, []string{"wot"}, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load wot: %v", err)
	}

	// The WoT starter room loaded and is the kind of room a character spawns in.
	if _, err := regs.World.Room("wot:the-green"); err != nil {
		t.Errorf("wot starter room not loaded: %v", err)
	}
	if _, err := regs.World.Area("wot:emonds-field"); err != nil {
		t.Errorf("wot area not loaded: %v", err)
	}

	// The engine baseline came along via the dependency closure.
	if !regs.Races.Has("human") {
		t.Error("baseline race 'human' not loaded (tapestry-core dependency not pulled in)")
	}
	if _, ok := regs.Channels.Get("tapestry-core:ooc"); !ok {
		t.Error("baseline ooc channel not loaded (tapestry-core dependency not pulled in)")
	}

	// The demo world was NOT selected — its rooms must be absent.
	if _, err := regs.World.Room(world.RoomID("starter-world:town-square")); err == nil {
		t.Error("starter-world:town-square loaded, but only wot+baseline were selected")
	}

	// Lamp-lit village: emonds-field's area `light_floor: dim` baked onto
	// the Green, so it resolves Dim (navigable, full render) at night —
	// while a westwood wilds room stays Gloom (description withheld, bring
	// a torch). This is the village/hamlet light split (light-and-darkness
	// §2.4 floor cascade).
	green, err := regs.World.Room("wot:the-green")
	if err != nil {
		t.Fatalf("the-green missing: %v", err)
	}
	if got, ok := green.PropertyString("light_floor"); !ok || got != "dim" {
		t.Errorf("the-green light_floor = (%q,%v), want (dim,true) baked from emonds-field", got, ok)
	}
	res := light.NewResolver(light.DefaultConfig(), fixedNight{})
	if got := res.Effective(green, light.Black, light.Black); got != light.Dim {
		t.Errorf("the-green at night = %v, want Dim (village lamps lift gloom)", got)
	}

	// Map POI derivation: the forge holds Haral the smith (shop + trainer)
	// so it resolves to a shop marker; the open Green has no fixture.
	if forge, err := regs.World.Room("wot:the-forge"); err == nil {
		if forge.POI != "shop" {
			t.Errorf("the-forge POI = %q, want shop (Haral is a shop+trainer NPC)", forge.POI)
		}
	}
	if green.POI != "" {
		t.Errorf("the-green POI = %q, want empty (no fixture)", green.POI)
	}
	// The Winespring Inn rest rooms (healing_rate, no vendor) read as inns.
	for _, id := range []world.RoomID{"wot:inn-common-room", "wot:inn-guestroom"} {
		if r, err := regs.World.Room(id); err == nil && r.POI != "inn" {
			t.Errorf("%s POI = %q, want inn (healing_rate set, no shop)", id, r.POI)
		}
	}
	if wild, err := regs.World.Room("wot:deep-westwood"); err == nil {
		if got := res.Effective(wild, light.Black, light.Black); got != light.Gloom {
			t.Errorf("deep-westwood at night = %v, want Gloom (wilds stay dark)", got)
		}
	}
}

// TestLoad_WotChannelerInitiateWilderSplit verifies the WoT S2 Initiate/Wilder
// split end-to-end on real content: both channeling classes load, share the
// One Power pool + starter weave path, and diverge on the two meaningful axes
// the split is built around — the pool's governing stat (Initiate→INT studied
// discipline, Wilder→WIS raw instinct) and Fortitude resilience (the Wilder's
// strong Fort is the translation of d20's "wilders are more practiced at
// overchanneling"; our shipped overchannel→Fort→cascade reads it for free).
// The single legacy `channeler` class id must be gone (replaced, not kept).
func TestLoad_WotChannelerInitiateWilderSplit(t *testing.T) {
	root, err := filepath.Abs("../../content")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	regs := NewRegistries()
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatalf("baseline properties: %v", err)
	}
	if err := slot.RegisterEngineBaseline(regs.Slots); err != nil {
		t.Fatalf("baseline slots: %v", err)
	}
	if err := Load(context.Background(), root, []string{"wot"}, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load wot: %v", err)
	}

	// The pre-split single class id is gone — the split replaces it, it does
	// not add a third channeling class.
	if _, ok := regs.Classes.Get("channeler"); ok {
		t.Error("legacy 'channeler' class still registered; the Initiate/Wilder split should replace it")
	}

	initiate, ok := regs.Classes.Get("initiate")
	if !ok {
		t.Fatal("initiate class not loaded")
	}
	wilder, ok := regs.Classes.Get("wilder")
	if !ok {
		t.Fatal("wilder class not loaded")
	}

	// Shared identity: both bind the one-power track and grant a One Power pool.
	for _, c := range []*progression.Class{initiate, wilder} {
		if c.BoundTrack != "one-power" {
			t.Errorf("%s bound_track = %q, want one-power", c.ID, c.BoundTrack)
		}
		if c.StartingStats[progression.StatResourceMax] != 30 {
			t.Errorf("%s starting resource_max = %d, want 30 (the One Power pool)",
				c.ID, c.StartingStats[progression.StatResourceMax])
		}
	}

	// Divergence axis 1 — the pool's governing stat (the build choice).
	if got := initiate.GrowthBonuses[progression.StatResourceMax]; got != progression.StatINT {
		t.Errorf("initiate resource_max growth keyed to %q, want int (studied discipline)", got)
	}
	if got := wilder.GrowthBonuses[progression.StatResourceMax]; got != progression.StatWIS {
		t.Errorf("wilder resource_max growth keyed to %q, want wis (raw instinct)", got)
	}

	// Divergence axis 2 — Fortitude resilience to overchannel backlash. Both
	// share a strong Will (mentally disciplined channelers); only the Wilder
	// has strong Fort.
	if initiate.SaveProgressions[progression.SaveFortitude] != progression.SaveWeak {
		t.Errorf("initiate fortitude = %q, want weak (brittle, refined)",
			initiate.SaveProgressions[progression.SaveFortitude])
	}
	if wilder.SaveProgressions[progression.SaveFortitude] != progression.SaveStrong {
		t.Errorf("wilder fortitude = %q, want strong (hardy against backlash)",
			wilder.SaveProgressions[progression.SaveFortitude])
	}
	if initiate.SaveProgressions[progression.SaveWill] != progression.SaveStrong ||
		wilder.SaveProgressions[progression.SaveWill] != progression.SaveStrong {
		t.Error("both channeling classes should have a strong Will save")
	}
}

// fixedNight is a light.PeriodSource pinned to night for the boot test.
type fixedNight struct{}

func (fixedNight) CurrentPeriod() string { return "night" }

// TestLoad_WotWeaponIdentityChain verifies the weapon-identity feature
// (EPIC S1) end-to-end on real content: the demo weapons decode with their
// tier / crit / damage-type fields, the fighter class grants simple+martial,
// and the proficiency rule resolves correctly — a fighter is proficient
// with the martial longsword (no penalty) but NOT the exotic ashandarei
// (which takes the non-proficient to-hit penalty).
func TestLoad_WotWeaponIdentityChain(t *testing.T) {
	root, err := filepath.Abs("../../content")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	regs := NewRegistries()
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatalf("baseline properties: %v", err)
	}
	if err := slot.RegisterEngineBaseline(regs.Slots); err != nil {
		t.Fatalf("baseline slots: %v", err)
	}
	if err := Load(context.Background(), root, []string{"wot"}, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load wot: %v", err)
	}

	// The martial longsword decoded with its full identity (§2, §4).
	longsword, err := regs.Items.Get("wot:two-rivers-longsword")
	if err != nil {
		t.Fatalf("longsword: %v", err)
	}
	if longsword.ProficiencyTier != "martial" || longsword.WeaponCategory != "longsword" {
		t.Errorf("longsword identity = (%q,%q), want (martial, longsword)",
			longsword.ProficiencyTier, longsword.WeaponCategory)
	}
	if longsword.CritThreatLow != 19 || longsword.CritMultiplier != 2 {
		t.Errorf("longsword crit = (%d,%d), want (19,2)", longsword.CritThreatLow, longsword.CritMultiplier)
	}
	if !slices.Equal(longsword.DamageTypes, []string{"slashing"}) {
		t.Errorf("longsword damage types = %v, want [slashing]", longsword.DamageTypes)
	}

	// The exotic ashandarei decoded at the exotic tier.
	ashandarei, err := regs.Items.Get("wot:ashandarei")
	if err != nil {
		t.Fatalf("ashandarei: %v", err)
	}
	if ashandarei.ProficiencyTier != "exotic" {
		t.Errorf("ashandarei tier = %q, want exotic", ashandarei.ProficiencyTier)
	}

	// The fighter class grants simple + martial proficiency.
	fighter, ok := regs.Classes.Get("fighter")
	if !ok {
		t.Fatal("fighter class not loaded")
	}
	if !slices.Equal(fighter.ProficiencyTiers, []string{"simple", "martial"}) {
		t.Errorf("fighter ProficiencyTiers = %v, want [simple martial]", fighter.ProficiencyTiers)
	}

	// End-to-end proficiency rule (weapon-identity §3): proficient with the
	// martial longsword, NOT with the exotic ashandarei.
	if !item.Proficient(fighter.ProficiencyTiers, fighter.ProficiencyCategories,
		longsword.ProficiencyTier, longsword.WeaponCategory) {
		t.Error("a fighter should be proficient with the martial longsword")
	}
	if item.Proficient(fighter.ProficiencyTiers, fighter.ProficiencyCategories,
		ashandarei.ProficiencyTier, ashandarei.WeaponCategory) {
		t.Error("a fighter should NOT be proficient with the exotic ashandarei")
	}
}
