package command_test

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
)

// adminCombatActor is a combatActor (Combatant + the embedded testActor's
// economy.SustenanceEntity) that also holds the admin role, so the
// self-restore path can exercise both the HP and sustenance refill.
type adminCombatActor struct {
	*combatActor
}

func (a *adminCombatActor) HasRole(role string) bool { return strings.EqualFold(role, "admin") }

// restore tops off the sustenance pool (hunger/thirst), not just HP, for
// a target that carries one (a player). Mob restores stay HP-only.
func TestRestore_AlsoRefillsSustenance(t *testing.T) {
	f := newConsiderFixture(t)
	actor := &adminCombatActor{combatActor: newCombatActor("Maerys", "p-admin", nil)}
	actor.Vitals().ApplyDamage(20) // wound below max
	actor.SetSustenance(15)        // hungry

	dispatchRole(t, f.env(), actor, "restore") // no arg → self

	if cur, max := actor.Vitals().Snapshot(); cur != max {
		t.Errorf("HP = %d/%d, want restored to full", cur, max)
	}
	if got := actor.Sustenance(); got != economy.MaxSustenance {
		t.Errorf("sustenance = %d, want %d (fully fed)", got, economy.MaxSustenance)
	}
	if !strings.Contains(actor.lastLine(), "fully fed") {
		t.Errorf("confirmation = %q, want it to mention 'fully fed'", actor.lastLine())
	}
}

// An admin restores a wounded mob: vitals return to full and one
// admin.action fires with the mob as target.
func TestRestore_RoomMobToFullAndAudits(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus

	f.guard.Vitals().ApplyDamage(25) // 40 → 15

	dispatchRole(t, env, admin, "restore guard")

	if cur, max := f.guard.Vitals().Snapshot(); cur != max {
		t.Errorf("guard HP = %d/%d, want restored to full", cur, max)
	}
	if !strings.Contains(admin.lastLine(), "restored to 40/40") {
		t.Errorf("confirmation = %q, want 'restored to 40/40'", admin.lastLine())
	}
	if len(*got) != 1 {
		t.Fatalf("admin.action count = %d, want 1", len(*got))
	}
	if ev := (*got)[0].(eventbus.AdminAction); ev.Verb != "restore" || ev.Target != f.guard.EntityID() {
		t.Errorf("event = %+v, want verb=restore target=%s", ev, f.guard.EntityID())
	}
}

// An unknown restore target reports the miss and audits nothing.
func TestRestore_NotFoundNoAudit(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus

	dispatchRole(t, env, admin, "restore ghost")

	if !strings.Contains(admin.lastLine(), "don't see") {
		t.Errorf("message = %q, want a not-found message", admin.lastLine())
	}
	if len(*got) != 0 {
		t.Errorf("a missed restore must not audit, got %d", len(*got))
	}
}

// restore is admin-gated (§2): a non-admin gets "Huh?", no heal, no audit.
func TestRestore_RefusedForNonAdmin(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	bob := newRoleActor("Bob", "p-bob") // no admin role
	bob.SetRoom(f.room)
	env := f.env()
	env.Bus = bus

	f.guard.Vitals().ApplyDamage(25)

	dispatchRole(t, env, bob, "restore guard")

	if bob.lastLine() != "Huh?" {
		t.Errorf("refusal = %q, want 'Huh?'", bob.lastLine())
	}
	if cur, _ := f.guard.Vitals().Snapshot(); cur != 15 {
		t.Errorf("guard HP = %d, want 15 (non-admin must not heal)", cur)
	}
	if len(*got) != 0 {
		t.Errorf("a refused restore must not audit, got %d", len(*got))
	}
}
