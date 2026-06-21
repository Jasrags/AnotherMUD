package command_test

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/action"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// Out of combat with timed actions enabled, donning slow armor is DEFERRED: the
// equip does not land immediately — a don action goes in flight, the item stays
// in inventory, and the action-complete sweep (replaying the command with
// ReplayAction) performs the actual equip (action-economy.md §7.2).
func TestEquip_SlowArmorDefersOutOfCombat(t *testing.T) {
	r := newRegistry(t)
	f := newEqFixture(t)

	inner := newTestActor(f.room)
	a := &namedActor{testActor: inner, name: "Alice", playerID: "p-1"}
	helm, err := f.store.Spawn(heavyHelmTpl())
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	a.AddToInventory(helm.ID())

	tr := action.NewTracker()
	var now uint64 = 100
	env := f.env()
	env.Actions = tr
	env.NowTick = func() uint64 { return now }
	env.DonTicks = 5
	ctx := context.Background()

	// Phase 1: the equip defers. Helm not worn, still in inventory, action armed.
	if err := r.Dispatch(ctx, env, a, "equip helm"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if _, worn := a.Equipment()["head"]; worn {
		t.Fatal("slow armor should not equip instantly with timed actions on")
	}
	if !containsID(a.Inventory(), helm.ID()) {
		t.Error("helm should remain in inventory mid-don")
	}
	act, busy := tr.Active("p-1")
	if !busy {
		t.Fatal("a don action should be in flight")
	}
	if act.Kind != command.KindArmorDon {
		t.Errorf("action kind = %q, want %q", act.Kind, command.KindArmorDon)
	}
	if act.ReadyAt != now+5 {
		t.Errorf("ReadyAt = %d, want %d", act.ReadyAt, now+5)
	}
	if !act.Interruptible {
		t.Error("a don action should be interruptible")
	}

	// Phase 2: the sweep claims the action and replays its command.
	raw, ok := act.Payload.(string)
	if !ok || raw != "equip helm" {
		t.Fatalf("payload = %v, want the original command", act.Payload)
	}
	now += 5
	if _, due := tr.CompleteReady("p-1", now); !due { // the sweep clears it
		t.Fatal("action should be due at ReadyAt")
	}
	env.ReplayAction = true
	if err := r.Dispatch(ctx, env, a, raw); err != nil {
		t.Fatalf("replay dispatch: %v", err)
	}
	if _, worn := a.Equipment()["head"]; !worn {
		t.Error("the replayed don should equip the helm")
	}
}

// Light armor is unaffected — it equips instantly even with timed actions on
// (only medium/heavy armor is "slow").
func TestEquip_LightArmorInstantWithTimedActions(t *testing.T) {
	r := newRegistry(t)
	f := newEqFixture(t)
	inner := newTestActor(f.room)
	a := &namedActor{testActor: inner, name: "Alice", playerID: "p-1"}
	cap, err := f.store.Spawn(lightCapTpl())
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	a.AddToInventory(cap.ID())

	env := f.env()
	env.Actions = action.NewTracker()
	env.NowTick = func() uint64 { return 1 }
	if err := r.Dispatch(context.Background(), env, a, "equip cap"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if _, worn := a.Equipment()["head"]; !worn {
		t.Error("light armor should equip instantly even with timed actions on")
	}
	if env.Actions.IsBusy("p-1") {
		t.Error("light armor should not arm a don action")
	}
}

// hastydon arms a FASTER timer than a normal don and, on completion, wears the
// armor one step worse on both AC and the check penalty (armor-depth §7).
func TestHastyDon_FasterTimerAndDegradedMods(t *testing.T) {
	r := newRegistry(t)
	f := newEqFixture(t)
	inner := newTestActor(f.room)
	a := &namedActor{testActor: inner, name: "Alice", playerID: "p-1"}
	helm, err := f.store.Spawn(heavyHelmTpl()) // ArmorBonus 4, heavy, no base check penalty
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	a.AddToInventory(helm.ID())

	var now uint64 = 100
	env := f.env()
	env.Actions = action.NewTracker()
	env.NowTick = func() uint64 { return now }
	env.DonTicks = 30 // hasty = 30/3 = 10
	ctx := context.Background()

	// Phase 1: a hasty don is in flight, faster than the full 30-tick don.
	if err := r.Dispatch(ctx, env, a, "hastydon helm"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	act, busy := env.Actions.Active("p-1")
	if !busy {
		t.Fatal("hastydon should arm a timed action")
	}
	if act.ReadyAt != now+10 {
		t.Errorf("hasty ReadyAt = %d, want %d (a third of the 30-tick don)", act.ReadyAt, now+10)
	}

	// Phase 2: the sweep replays "hastydon helm" → degraded equip.
	now += 10
	if _, due := env.Actions.CompleteReady("p-1", now); !due {
		t.Fatal("hasty don should be due")
	}
	env.ReplayAction = true
	if err := r.Dispatch(ctx, env, a, "hastydon helm"); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if _, worn := a.Equipment()["head"]; !worn {
		t.Fatal("the helm should be worn after the hasty don completes")
	}

	mods := a.mods[entities.EquipmentSourceKey(helm.ID())]
	var ac, check int
	var sawCheck bool
	for _, m := range mods {
		switch m.Stat {
		case "ac":
			ac = m.Value
		case "armor_check":
			check = m.Value
			sawCheck = true
		}
	}
	if ac != 3 {
		t.Errorf("hasty AC modifier = %d, want 3 (armor bonus 4 − 1)", ac)
	}
	if !sawCheck || check != 1 {
		t.Errorf("hasty check penalty = %d (present=%v), want 1 (base 0 + 1)", check, sawCheck)
	}
}

// A normal don of the same helm applies the FULL armor bonus and no check
// penalty — proving the degradation above is the hasty path, not the helm.
func TestNormalDon_FullMods(t *testing.T) {
	r := newRegistry(t)
	f := newEqFixture(t)
	a := newTestActor(f.room) // no timed actions → instant equip
	helm := f.spawnInInventory(t, heavyHelmTpl(), a)

	dispatch(t, r, f.env(), a, "equip helm")

	mods := a.mods[entities.EquipmentSourceKey(helm.ID())]
	var ac int
	for _, m := range mods {
		if m.Stat == "ac" {
			ac = m.Value
		}
		if m.Stat == "armor_check" {
			t.Errorf("a normal don should apply no check penalty, got %d", m.Value)
		}
	}
	if ac != 4 {
		t.Errorf("normal AC modifier = %d, want the full 4", ac)
	}
}
