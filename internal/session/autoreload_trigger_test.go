package session

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/action"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// gunnerManager registers actor `a` (playerID "p-gunner", autoreload on, wielding
// a holder-fed pistol) in a minimal Manager with a live action tracker, for
// exercising AutoReloadOnDry's busy-gate branch.
func gunnerManager(t *testing.T, store *entities.Store) (*Manager, *connActor) {
	t.Helper()
	a := newEqActor(t, store)
	a.playerID = "p-gunner"
	a.autoreload.Store(true)
	spawnHolderGun(t, store, a)
	m := &Manager{
		byPlayerID:    map[string]*connActor{a.playerID: a},
		actionTracker: action.NewTracker(),
	}
	return m, a
}

// AutoReloadOnDry must gate the "already reloading, own the moment" short-circuit
// on the action KIND, not merely "busy": a player mid-don (a non-reload busy
// action) can still take a dry ranged swing, and mistaking that for an in-flight
// reload would swallow the dry moment with no reload armed and no message. Busy
// with a non-reload action falls through (returns false → default narration).
func TestAutoReloadOnDry_BusyWithNonReloadFallsThrough(t *testing.T) {
	store := entities.NewStore()
	m, a := gunnerManager(t, store)

	// Occupy the single busy slot with a NON-reload action (armor don).
	if !m.actionTracker.Begin(a.PlayerID(), action.Action{Kind: command.KindArmorDon, ReadyAt: 100}) {
		t.Fatal("could not arm the don action")
	}
	if owned := m.AutoReloadOnDry(context.Background(), "p-gunner", 0); owned {
		t.Error("busy with a non-reload action must fall through (return false), not own the moment")
	}
}

// Busy with an actual reload short-circuits to true (owns the moment; the in-flight
// reload completes on its own — no re-arm, no spam).
func TestAutoReloadOnDry_BusyReloadingOwnsTheMoment(t *testing.T) {
	store := entities.NewStore()
	m, a := gunnerManager(t, store)

	if !m.actionTracker.Begin(a.PlayerID(), action.Action{Kind: command.KindReload, ReadyAt: 100}) {
		t.Fatal("could not arm the reload action")
	}
	if owned := m.AutoReloadOnDry(context.Background(), "p-gunner", 0); !owned {
		t.Error("busy reloading should own the moment (return true) to suppress the dry-fire spam")
	}
}

// Pref off falls through regardless of weapon/ammo state.
func TestAutoReloadOnDry_PrefOffFallsThrough(t *testing.T) {
	store := entities.NewStore()
	m, a := gunnerManager(t, store)
	a.autoreload.Store(false)

	if owned := m.AutoReloadOnDry(context.Background(), "p-gunner", 0); owned {
		t.Error("autoreload off must fall through to the default dry narration")
	}
}
