package pack

import (
	"context"
	"path/filepath"
	"testing"
)

// A pack declaring a `pools:` glob must register the pool end-to-end
// (shadowrun-mvp SR-M3a). Mirrors the attribute-sets / languages glob trap: the
// loader enumerates by manifest, so without the glob the files are silently
// ignored.
func TestLoad_RegistersPool(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  pools: [pools/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "pools/stun.yaml"), `
id: stun
floor: 0
nonlethal: true
depletion_event: true
overflow_to: physical
max_channel: hp_stun
seed_on_player: true
seed_on_mob: true
`)

	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}

	d, ok := regs.Pools.Get("stun")
	if !ok {
		t.Fatal("pool 'stun' not registered")
	}
	if d.Rules.Floor != 0 || !d.Rules.Nonlethal || !d.Rules.DepletionEvent {
		t.Errorf("stun rules decoded wrong: %+v", d.Rules)
	}
	if d.Rules.OverflowTo != "physical" {
		t.Errorf("overflow_to = %q, want physical", d.Rules.OverflowTo)
	}
	if d.MaxChannel != "hp_stun" || !d.SeedOnPlayer || !d.SeedOnMob {
		t.Errorf("stun seed/channel decoded wrong: %+v", d)
	}
	if d.Pack != "tapestry-core" {
		t.Errorf("Pack = %q, want tapestry-core", d.Pack)
	}
}

// A world pack overrides a core pool by declaring a higher priority for the same
// kind (later-wins semantics, like every other content registry).
func TestLoad_PoolPriorityOverride(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "core/pack.yaml"), "name: core\ncontent:\n  pools: [pools/*.yaml]\n")
	writeFile(t, filepath.Join(root, "core/pools/mana.yaml"), "id: mana\nfloor: 0\nmax_channel: resource_max\nseed_on_player: true\n")
	writeFile(t, filepath.Join(root, "world/pack.yaml"), "name: world\nkind: world\nsplash: splash.txt\ndependencies:\n  core: \"*\"\ncontent:\n  pools: [pools/*.yaml]\n")
	writeFile(t, filepath.Join(root, "world/splash.txt"), "{Y}W{x}\n")
	writeFile(t, filepath.Join(root, "world/pools/mana.yaml"), "id: mana\nfloor: -3\nseed_on_player: true\npriority: 1\n")

	regs := NewRegistries()
	if err := Load(context.Background(), root, []string{"world"}, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}

	d, ok := regs.Pools.Get("mana")
	if !ok {
		t.Fatal("pool 'mana' not registered")
	}
	if d.Rules.Floor != -3 || d.Pack != "world" {
		t.Fatalf("higher-priority world decl should override core: %+v", d)
	}
}

// A malformed pool (missing id) must fail at load with attribution rather than
// registering a nameless pool.
func TestLoad_MalformedPoolFails(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), "name: core\ncontent:\n  pools: [pools/*.yaml]\n")
	writeFile(t, filepath.Join(pack, "pools/bad.yaml"), "floor: 0\nnonlethal: true\n")

	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err == nil {
		t.Fatal("Load: expected error for a pool missing 'id', got nil")
	}
}
