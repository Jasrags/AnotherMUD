package session

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/pool"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

func regenServices() (*economy.SustenanceService, *economy.RestService) {
	return economy.NewSustenanceService(economy.DefaultSustenanceConfig()),
		economy.NewRestService(economy.DefaultRestConfig(), nil, nil)
}

// full + awake + plain room → heals exactly BaseHP.
func TestRegenTick_FullAwakeHealsBase(t *testing.T) {
	mgr := NewManager()
	sust, rest := regenServices()
	cfg := economy.DefaultRegenConfig() // BaseHP 2
	a, _ := newFakeActor("c1", "p1", "acc1", "Alice", &world.Room{ID: "x:1"})
	a.sustenance = 100 // full → 1.0
	a.vitals.ApplyDamage(10)
	before, _ := a.vitals.Snapshot()
	mgr.Add(a)

	mgr.RegenTick(context.Background(), sust, rest, cfg)

	after, _ := a.vitals.Snapshot()
	if after-before != cfg.BaseHP {
		t.Fatalf("healed %d, want %d", after-before, cfg.BaseHP)
	}
}

// resting doubles regen (rest multiplier 2.0).
func TestRegenTick_RestingDoubles(t *testing.T) {
	mgr := NewManager()
	sust, rest := regenServices()
	cfg := economy.DefaultRegenConfig()
	a, _ := newFakeActor("c1", "p1", "acc1", "Alice", &world.Room{ID: "x:1"})
	a.sustenance = 100
	a.restState = string(economy.StateResting)
	a.vitals.ApplyDamage(10)
	before, _ := a.vitals.Snapshot()
	mgr.Add(a)

	mgr.RegenTick(context.Background(), sust, rest, cfg)

	after, _ := a.vitals.Snapshot()
	if after-before != cfg.BaseHP*2 {
		t.Fatalf("resting healed %d, want %d", after-before, cfg.BaseHP*2)
	}
}

// famished (sustenance 0 → multiplier 0) regenerates nothing, even in a
// healing room.
func TestRegenTick_FamishedHealsNothing(t *testing.T) {
	mgr := NewManager()
	sust, rest := regenServices()
	cfg := economy.DefaultRegenConfig()
	a, _ := newFakeActor("c1", "p1", "acc1", "Alice", &world.Room{ID: "x:1", HealingRate: 5})
	a.sustenance = 0 // famished → 0.0
	a.vitals.ApplyDamage(10)
	before, _ := a.vitals.Snapshot()
	mgr.Add(a)

	mgr.RegenTick(context.Background(), sust, rest, cfg)

	after, _ := a.vitals.Snapshot()
	if after != before {
		t.Fatalf("famished healed %d, want 0", after-before)
	}
}

// room healing_rate is an additive bonus on top of the scaled base.
func TestRegenTick_RoomHealingRateAdds(t *testing.T) {
	mgr := NewManager()
	sust, rest := regenServices()
	cfg := economy.DefaultRegenConfig() // BaseHP 2
	a, _ := newFakeActor("c1", "p1", "acc1", "Alice", &world.Room{ID: "x:1", HealingRate: 3})
	a.sustenance = 100 // full → 1.0, awake → 1.0
	a.vitals.ApplyDamage(10)
	before, _ := a.vitals.Snapshot()
	mgr.Add(a)

	mgr.RegenTick(context.Background(), sust, rest, cfg)

	after, _ := a.vitals.Snapshot()
	if after-before != cfg.BaseHP+3 {
		t.Fatalf("healed %d, want %d (base+room)", after-before, cfg.BaseHP+3)
	}
}

// mana regenerates independently of HP fullness: a full-HP channeler with
// a drained Power pool still refills it (the bug the old structure had —
// it `continue`d the whole actor on full HP).
func TestRegenTick_RefillsManaAtFullHP(t *testing.T) {
	mgr := NewManager()
	sust, rest := regenServices()
	cfg := economy.DefaultRegenConfig() // BaseMana 1
	a, _ := newFakeActor("c1", "p1", "acc1", "Alice", &world.Room{ID: "x:1"})
	a.sustenance = 100 // full → 1.0
	// Full HP (fixture default) but a drained mana pool.
	a.pools = pool.NewSet()
	a.pools.Add(pool.NewAt(poolKindMana, 3, 20, pool.Rules{Floor: 0}))
	mgr.Add(a)

	mgr.RegenTick(context.Background(), sust, rest, cfg)

	if mn := a.Mana(); mn != 3+cfg.BaseMana {
		t.Fatalf("mana regen at full HP = %d, want %d", mn, 3+cfg.BaseMana)
	}
}

// a full mana pool is not over-restored, and an unseeded actor never panics.
func TestRegenTick_FullManaUnchanged(t *testing.T) {
	mgr := NewManager()
	sust, rest := regenServices()
	a, _ := newFakeActor("c1", "p1", "acc1", "Alice", &world.Room{ID: "x:1"})
	a.sustenance = 100
	a.pools = pool.NewSet()
	a.pools.Add(pool.New(poolKindMana, 20, pool.Rules{Floor: 0})) // full
	mgr.Add(a)

	mgr.RegenTick(context.Background(), sust, rest, economy.DefaultRegenConfig())

	if mn := a.Mana(); mn != 20 {
		t.Fatalf("full mana over-restored to %d, want 20", mn)
	}
}

// a full-HP player is not over-healed.
func TestRegenTick_SkipsFullHP(t *testing.T) {
	mgr := NewManager()
	sust, rest := regenServices()
	a, _ := newFakeActor("c1", "p1", "acc1", "Alice", &world.Room{ID: "x:1"})
	a.sustenance = 100
	before, max := a.vitals.Snapshot()
	if before != max {
		t.Fatalf("fixture should start at full HP, got %d/%d", before, max)
	}
	mgr.Add(a)

	mgr.RegenTick(context.Background(), sust, rest, economy.DefaultRegenConfig())

	after, _ := a.vitals.Snapshot()
	if after != max {
		t.Fatalf("full-HP player healed to %d past max %d", after, max)
	}
}

// nil services make the tick a no-op.
func TestRegenTick_NilServiceNoop(t *testing.T) {
	mgr := NewManager()
	a, _ := newFakeActor("c1", "p1", "acc1", "Alice", &world.Room{ID: "x:1"})
	a.sustenance = 100
	a.vitals.ApplyDamage(10)
	before, _ := a.vitals.Snapshot()
	mgr.Add(a)

	mgr.RegenTick(context.Background(), nil, nil, economy.DefaultRegenConfig())

	after, _ := a.vitals.Snapshot()
	if after != before {
		t.Fatalf("nil-service regen healed %d", after-before)
	}
}
