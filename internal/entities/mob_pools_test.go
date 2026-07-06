package entities

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/pool"
)

// stunMobTpl is a guard whose Stats carry an hp_stun value, so a Stun-monitor
// pool decl (max_channel: hp_stun) seeds a non-zero ceiling.
func stunMobTpl() *mob.Template {
	return &mob.Template{
		ID:    "tapestry-core:stun-dummy",
		Name:  "a stun dummy",
		Type:  "npc",
		Stats: map[string]int{"hp_max": 40, "hp_stun": 12},
	}
}

func stunDecls() []*pool.Decl {
	return []*pool.Decl{
		{Kind: "stun", Rules: pool.Rules{Floor: 0, Nonlethal: true, DepletionEvent: true}, MaxChannel: "hp_stun", SeedOnMob: true},
	}
}

// A mob spawned from a Store carrying mob-seed decls gets those pools seeded,
// with the ceiling from the max_channel stat and the declared Rules — the seam
// that makes a Shadowrun mob stunnable (SR-M3a step 4).
func TestSpawnMob_SeedsMobPools(t *testing.T) {
	s := NewStore()
	s.SetMobPools(stunDecls())

	inst, err := s.SpawnMob(stunMobTpl())
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	pools := inst.Pools()
	if pools == nil {
		t.Fatal("Pools() is nil; a seeded mob must carry a pool.Set")
	}
	stun, ok := pools.Get("stun")
	if !ok {
		t.Fatal("stun monitor not seeded onto the mob")
	}
	if stun.Max() != 12 {
		t.Errorf("stun max = %d, want 12 (hp_stun)", stun.Max())
	}
	if r := stun.Rules(); !r.Nonlethal || !r.DepletionEvent {
		t.Errorf("stun rules = %+v, want Nonlethal+DepletionEvent (SR-M2 KO routing reads these)", r)
	}
}

// A fantasy/WoT mob (no mob-seed decls) spawns with an empty-but-non-nil set:
// combat treats an empty set the same as the nil Pools() returned pre-M3a.
func TestSpawnMob_NoDeclsLeavesEmptySet(t *testing.T) {
	s := NewStore() // no SetMobPools
	inst, err := s.SpawnMob(stunMobTpl())
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	if inst.Pools() == nil {
		t.Fatal("Pools() is nil; buildMobFromTemplate must init an empty set")
	}
	if _, ok := inst.Pools().Get("stun"); ok {
		t.Error("a mob in a world with no mob-seed pools must carry no monitors")
	}
}

// SetMobPools retro-seeds mobs already tracked (spawned during pack Load, before
// the decls were built from content) — the same retro-stamp SetChannelMap does.
func TestSetMobPools_RetroSeedsTrackedMob(t *testing.T) {
	s := NewStore()
	// Spawn BEFORE the decls exist (mirrors a mob spawned during Load).
	inst, err := s.SpawnMob(stunMobTpl())
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	if _, ok := inst.Pools().Get("stun"); ok {
		t.Fatal("mob should have no stun monitor before SetMobPools")
	}

	s.SetMobPools(stunDecls())

	stun, ok := inst.Pools().Get("stun")
	if !ok {
		t.Fatal("SetMobPools did not retro-seed the tracked mob's stun monitor")
	}
	if stun.Max() != 12 {
		t.Errorf("retro-seeded stun max = %d, want 12", stun.Max())
	}
}
