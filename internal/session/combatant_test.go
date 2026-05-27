package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// M7.1: connActor must satisfy combat.Combatant. The compile-time
// assignment in this test pins the contract — if a future refactor
// breaks the interface, this test fails rather than the (currently
// absent) CombatManager call site.
func TestConnActorImplementsCombatant(t *testing.T) {
	r := &world.Room{ID: "x:1", Name: "X"}
	a, _ := newFakeActor("c1", "p-alice", "acc1", "Alice", r)

	var c combat.Combatant = a
	if c.Name() != "Alice" {
		t.Errorf("Name() = %q, want %q", c.Name(), "Alice")
	}
	want := string(combat.NewPlayerCombatantID("p-alice"))
	if got := string(c.CombatantID()); got != want {
		t.Errorf("CombatantID() = %q, want %q", got, want)
	}
	cur, max := c.Vitals().Snapshot()
	if cur != combat.DefaultPlayerMaxHP || max != combat.DefaultPlayerMaxHP {
		t.Errorf("Vitals at login = (%d, %d), want (%d, %d)",
			cur, max, combat.DefaultPlayerMaxHP, combat.DefaultPlayerMaxHP)
	}
	if c.Stats() != combat.DefaultPlayerStats() {
		t.Errorf("Stats() = %+v, want DefaultPlayerStats()", c.Stats())
	}
}

// TestRestorePlayerVitalsFloorsDeadAtLogin pins the safety floor in
// restorePlayerVitals: a save written with HP <= 0 (player killed in
// combat then disconnected before any §6.4 player-death subscriber
// existed) must restore to at least 1 HP so login produces a playable
// actor. Remove the floor when a real player-death subscriber
// guarantees no save ever serializes a dead player.
func TestRestorePlayerVitalsFloorsDeadAtLogin(t *testing.T) {
	cases := []struct {
		name   string
		state  *player.VitalsState
		wantHP int
	}{
		{"nil restores full HP", nil, combat.DefaultPlayerMaxHP},
		{"HP=0 floors to 1", &player.VitalsState{HP: 0, MaxHP: 40}, 1},
		{"HP=-5 floors to 1", &player.VitalsState{HP: -5, MaxHP: 40}, 1},
		{"HP in range preserved", &player.VitalsState{HP: 12, MaxHP: 40}, 12},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := restorePlayerVitals(tc.state)
			if got := v.Current(); got != tc.wantHP {
				t.Errorf("Current() = %d, want %d", got, tc.wantHP)
			}
		})
	}
}

// Damage applied through the Combatant surface must be observable
// through the actor's own Vitals accessor — i.e. the pointer is
// shared, not a copy. Pins the M7.1 design choice that Vitals
// returns a stable pointer for the life of the connActor so combat
// can mutate it without round-tripping through the session lock.
func TestConnActorVitalsSharedPointer(t *testing.T) {
	r := &world.Room{ID: "x:1", Name: "X"}
	a, _ := newFakeActor("c1", "p-bob", "acc1", "Bob", r)

	var c combat.Combatant = a
	c.Vitals().ApplyDamage(7)

	if got := a.Vitals().Current(); got != combat.DefaultPlayerMaxHP-7 {
		t.Errorf("after ApplyDamage(7), Current() = %d, want %d",
			got, combat.DefaultPlayerMaxHP-7)
	}
}
