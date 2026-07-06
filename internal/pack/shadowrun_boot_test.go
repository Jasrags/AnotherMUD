package pack

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/channel"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/pool"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/slot"
)

// TestLoad_ShadowrunBootSlice is the SR-M3c-1 gate: selecting the `shadowrun`
// pack boots {tapestry-core, shadowrun} via dependency closure, seeds a runner
// on the eight Shadowrun primaries + Edge, and stands them on a street corner —
// with the Stun monitor deriving its ceiling from Willpower (the SR-M3c-1 build
// step 0 fix: a formula-driven pool max that Effective alone can't evaluate).
func TestLoad_ShadowrunBootSlice(t *testing.T) {
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
	// Select only shadowrun; the dependency closure adds tapestry-core.
	if err := Load(context.Background(), root, []string{"shadowrun"}, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load shadowrun: %v", err)
	}

	// The starter district loaded — a bootable room + its area.
	if _, err := regs.World.Room("shadowrun:street-corner"); err != nil {
		t.Errorf("shadowrun starter room not loaded: %v", err)
	}
	if _, err := regs.World.Area("shadowrun:seattle"); err != nil {
		t.Errorf("shadowrun area not loaded: %v", err)
	}

	// The shadowrun `human` overrode the core baseline (priority 1) — proven by
	// a cap key only the SR metatype declares (agility isn't a classic attribute).
	human, ok := regs.Races.Get("human")
	if !ok {
		t.Fatal("human metatype not loaded")
	}
	if _, hasAgility := human.StatCaps["agility"]; !hasAgility {
		t.Errorf("human StatCaps = %v, want an 'agility' cap (the SR override should win over core human)", human.StatCaps)
	}

	// The world selects the Shadowrun attribute set (manifest attribute_set:).
	if got := regs.WorldAttributeSets["shadowrun"]; got != "shadowrun-primaries" {
		t.Errorf("WorldAttributeSets[shadowrun] = %q, want shadowrun-primaries", got)
	}
	srSet, ok := regs.AttributeSets.Get("shadowrun-primaries")
	if !ok {
		t.Fatal("shadowrun-primaries attribute set not loaded")
	}
	if got := len(srSet.Keys()); got != 9 {
		t.Errorf("shadowrun-primaries has %d attributes, want 9 (8 primaries + edge)", got)
	}

	// The Stun monitor loaded as a nonlethal, formula-driven pool.
	stun, ok := regs.Pools.Get("stun")
	if !ok {
		t.Fatal("stun pool not declared")
	}
	if stun.MaxFormula != "8 + ceil(willpower / 2)" {
		t.Errorf("stun MaxFormula = %q, want the willpower formula", stun.MaxFormula)
	}
	if !stun.Rules.Nonlethal || !stun.Rules.DepletionEvent {
		t.Errorf("stun rules = %+v, want nonlethal + depletion_event", stun.Rules)
	}
	if !stun.SeedOnPlayer || !stun.SeedOnMob {
		t.Errorf("stun seeds player=%v mob=%v, want both true", stun.SeedOnPlayer, stun.SeedOnMob)
	}

	// The combat channel map remapped defense onto SR primaries (not the core
	// baseline `ac`). Reaction 3 + Intuition 3 = 6.
	mapping, err := regs.ChannelMap.Build()
	if err != nil {
		t.Fatalf("build channel map: %v", err)
	}
	srBase := progression.SeedBaseFromSet(srSet)
	sb := progression.NewWithBase(srBase)
	lookup := func(name string) int { return sb.Effective(progression.StatType(name)) }
	// All four channels derive off the SR primaries (defaults 3 each), not the
	// core baseline (which reads hit_mod/ac/str this world doesn't seed).
	if got := mapping.Value(channel.Attack, lookup); got != 3 {
		t.Errorf("attack channel = %d, want 3 (agility 3; weapon skill adds via proficiency)", got)
	}
	if got := mapping.Value(channel.Defense, lookup); got != 6 {
		t.Errorf("defense channel = %d, want 6 (reaction 3 + intuition 3)", got)
	}
	if got := mapping.Value(channel.DamageBonus, lookup); got != 3 {
		t.Errorf("damage_bonus channel = %d, want 3 (strength 3)", got)
	}
	// mitigation = body + armor; `armor` is unwired until SR-M3c-2, so it reads
	// 0 → body alone (3). This asserts the c-1 degradation is intentional.
	if got := mapping.Value(channel.Mitigation, lookup); got != 3 {
		t.Errorf("mitigation channel = %d, want 3 (body 3 + armor 0, armor unwired in c-1)", got)
	}

	// END-TO-END: seed the Stun monitor onto the SR-seeded stat block through the
	// real seeder. Willpower defaults to 3, so 8 + ceil(3/2) = 8 + 2 = 10 — NOT 0,
	// which is exactly the SR-M3c-1 build-step-0 failure this proves is fixed.
	set := pool.NewSet()
	entities.SeedPoolInto(set, sb, "stun", progression.StatType(stun.MaxChannel), stun.MaxFormula, stun.Rules)
	p, ok := set.Get("stun")
	if !ok {
		t.Fatal("stun pool was not seeded")
	}
	if got := p.Max(); got != 10 {
		t.Fatalf("seeded stun max = %d, want 10 (8 + ceil(willpower 3 / 2)); 0 would mean the formula seam is broken", got)
	}
}
