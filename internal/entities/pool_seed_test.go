package entities

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/channel"
	"github.com/Jasrags/AnotherMUD/internal/pool"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// TestSeedPoolInto_Formula is the SR-M3c-1 verification: a pool whose ceiling
// is a DERIVED formula ("8 + ceil(willpower / 2)") seeds at the evaluated value
// — not 0. StatBlock.Effective only returns base+modifiers, so this only works
// because SeedPoolInto routes a formula decl through channel/expr.
func TestSeedPoolInto_Formula(t *testing.T) {
	sb := progression.New()
	sb.SetBase("willpower", 5) // 8 + ceil(5/2) = 8 + 3 = 11

	set := pool.NewSet()
	SeedPoolInto(set, sb, "stun", "", "8 + ceil(willpower / 2)", pool.Rules{Floor: 0, Nonlethal: true})

	p, ok := set.Get("stun")
	if !ok {
		t.Fatal("stun pool was not seeded")
	}
	if got := p.Max(); got != 11 {
		t.Fatalf("stun max = %d; want 11 (8 + ceil(5/2))", got)
	}
	if got := p.Current(); got != 11 {
		t.Fatalf("stun current = %d; want 11 (seeds full)", got)
	}
}

// TestSeedPoolInto_FormulaReactive proves the ceiling re-derives when an input
// attribute changes: raising willpower re-evaluates the whole formula (not a
// flat delta), because SeedPoolInto binds OnMaxChange to each Vars() input.
func TestSeedPoolInto_FormulaReactive(t *testing.T) {
	sb := progression.New()
	sb.SetBase("willpower", 5) // -> 11

	set := pool.NewSet()
	SeedPoolInto(set, sb, "stun", "", "8 + ceil(willpower / 2)", pool.Rules{Floor: 0})
	p, _ := set.Get("stun")

	sb.AdjustBase("willpower", 3) // willpower 8 -> 8 + ceil(8/2) = 8 + 4 = 12
	if got := p.Max(); got != 12 {
		t.Fatalf("stun max after willpower 5->8 = %d; want 12 (8 + ceil(8/2))", got)
	}
}

// TestSeedPoolInto_FlatChannel keeps the pre-existing flat-stat path honest: a
// max_channel decl seeds from Effective(channel) and tracks it 1:1.
func TestSeedPoolInto_FlatChannel(t *testing.T) {
	sb := progression.New()
	sb.SetBase("resource_max", 7)

	set := pool.NewSet()
	SeedPoolInto(set, sb, "mana", "resource_max", "", pool.Rules{Floor: 0})
	p, _ := set.Get("mana")
	if got := p.Max(); got != 7 {
		t.Fatalf("mana max = %d; want 7", got)
	}
	sb.AdjustBase("resource_max", 2) // 7 -> 9, flat 1:1
	if got := p.Max(); got != 9 {
		t.Fatalf("mana max after +2 = %d; want 9", got)
	}
}

// TestSeedPoolInto_NoCeiling seeds an inert pool (max 0) when neither source is
// given — the substrate-only default.
func TestSeedPoolInto_NoCeiling(t *testing.T) {
	set := pool.NewSet()
	SeedPoolInto(set, progression.New(), "edge", "", "", pool.Rules{Floor: 0})
	p, ok := set.Get("edge")
	if !ok {
		t.Fatal("edge pool was not seeded")
	}
	if got := p.Max(); got != 0 {
		t.Fatalf("edge max = %d; want 0 (inert)", got)
	}
}

// TestMobMitigationReadsArmorInput proves the SR-M3c-2 `armor` synthetic
// channel input is wired: the Shadowrun soak formula `mitigation: body + armor`
// reads the mob's worn-armour rating (not just Body), through the same lookup
// special-case as the player's wornArmorBonus. Without the wiring, `armor` would
// resolve to 0 and mitigation would read Body alone.
func TestMobMitigationReadsArmorInput(t *testing.T) {
	s := NewStore()
	m, err := channel.NewMapping(map[channel.Channel]string{channel.Mitigation: "body + armor"})
	if err != nil {
		t.Fatalf("NewMapping: %v", err)
	}
	s.SetChannelMap(m)

	tpl := guardTpl()
	tpl.Stats = map[string]int{"body": 3, "hp_max": 40}
	tpl.Equipment = nil // no starter gear; armour set explicitly below
	inst, err := s.SpawnMob(tpl)
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}

	inst.SetArmorRating(4)
	if got := inst.Stats().Mitigation; got != 7 {
		t.Fatalf("mitigation = %d, want 7 (body 3 + armor 4 via the wired input)", got)
	}
	inst.SetArmorRating(0)
	if got := inst.Stats().Mitigation; got != 3 {
		t.Fatalf("mitigation with no armour = %d, want 3 (body 3 alone)", got)
	}
}
