package command_test

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/action"
	"github.com/Jasrags/AnotherMUD/internal/command"
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
