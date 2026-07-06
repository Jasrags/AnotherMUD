package pack

import (
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/pool"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// The core pack's mana/movement pools are the REGRESSION GATE for SR-M3a step 2
// (shadowrun-mvp.md): they must reproduce the hardcoded seedResourcePools binds
// exactly — `pool.New(kind, Effective(stat), pool.Rules{Floor: 0})` for
// {mana→resource_max} and {movement→movement_max}, seeded on players only — so
// making the seed data-driven (step 3) is a no-op for every existing world. If
// this drifts, characters would seed different resource pools than today.
func TestCorePack_PoolsMatchHardcodedSeed(t *testing.T) {
	content := repoContentDir(t)
	matches, err := filepath.Glob(filepath.Join(content, "core", "pools", "*.yaml"))
	if err != nil {
		t.Fatalf("glob core pools: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("core/pools/*.yaml resolves to 0 files — the engine pools are unauthored")
	}

	// Decode + Register through the real loader path (exercises the content too).
	reg := pool.NewRegistry()
	for _, m := range matches {
		d, err := decodePool(m, "tapestry-core")
		if err != nil {
			t.Fatalf("decode %s: %v", m, err)
		}
		if err := reg.Register(d); err != nil {
			t.Fatalf("register %s: %v", m, err)
		}
	}

	// Core must declare EXACTLY the two engine pools — an accidental third would
	// seed onto every character in every world.
	if got := len(reg.All()); got != 2 {
		t.Fatalf("core declares %d pools, want exactly 2 (mana + movement)", got)
	}

	// The hardcoded binds: {mana→resource_max}, {movement→movement_max}, both
	// pool.Rules{Floor: 0}, player-seeded (mobs carry no resource pool today).
	want := []struct {
		kind    pool.Kind
		channel progression.StatType
	}{
		{"mana", progression.StatResourceMax},
		{"movement", progression.StatMovementMax},
	}
	for _, w := range want {
		d, ok := reg.Get(w.kind)
		if !ok {
			t.Errorf("core declares no %q pool", w.kind)
			continue
		}
		if d.Rules != (pool.Rules{Floor: 0}) {
			t.Errorf("%s rules = %+v, want {Floor:0} (the hardcoded seed)", w.kind, d.Rules)
		}
		if d.MaxChannel != string(w.channel) {
			t.Errorf("%s max_channel = %q, want %q", w.kind, d.MaxChannel, w.channel)
		}
		if !d.SeedOnPlayer {
			t.Errorf("%s must seed on players (the hardcoded seed does)", w.kind)
		}
		if d.SeedOnMob {
			t.Errorf("%s must NOT seed on mobs (mobs carry no resource pool today)", w.kind)
		}
	}
}
