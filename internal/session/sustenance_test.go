package session

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// TestDrainSustenance_DecrementsAndReminds verifies the world-tick drain
// body: every logged-in actor's pool drops by DrainAmount, and a player
// below the Full tier gets one throttled reminder.
func TestDrainSustenance_DecrementsAndReminds(t *testing.T) {
	mgr := NewManager()
	svc := economy.NewSustenanceService(economy.DefaultSustenanceConfig()) // DrainAmount 1, ReminderInterval 3000
	r := &world.Room{ID: "x:1", Name: "X"}
	a, fc := newFakeActor("c1", "p1", "acc1", "Alice", r)
	a.sustenance = 35 // hungry band
	mgr.Add(a)

	// First drain at tick 100: 35 -> 34 (still hungry), reminder fires
	// (never reminded before).
	mgr.DrainSustenance(context.Background(), svc, "", 100)
	if a.Sustenance() != 34 {
		t.Fatalf("after first drain sustenance = %d, want 34", a.Sustenance())
	}
	if got := fc.writes(); len(got) != 1 || !strings.Contains(got[0], "hungry") {
		t.Fatalf("expected one hungry reminder, got %v", got)
	}

	// Second drain shortly after (tick 130, within ReminderInterval):
	// value drops again but the reminder is throttled.
	mgr.DrainSustenance(context.Background(), svc, "", 130)
	if a.Sustenance() != 33 {
		t.Fatalf("after second drain sustenance = %d, want 33", a.Sustenance())
	}
	if got := fc.writes(); len(got) != 1 {
		t.Fatalf("reminder should be throttled; got %v", got)
	}

	// Drain past the interval (tick 100 + 3000 + 1): a fresh reminder
	// fires, now famished.
	mgr.DrainSustenance(context.Background(), svc, "", 3101)
	got := fc.writes()
	if len(got) != 2 || !strings.Contains(got[1], "famished") {
		t.Fatalf("expected a second (famished) reminder past the interval, got %v", got)
	}
}

// A Full-tier player is drained but never reminded.
func TestDrainSustenance_NoReminderWhenFull(t *testing.T) {
	mgr := NewManager()
	svc := economy.NewSustenanceService(economy.DefaultSustenanceConfig())
	r := &world.Room{ID: "x:1", Name: "X"}
	a, fc := newFakeActor("c1", "p1", "acc1", "Alice", r)
	a.sustenance = 100
	mgr.Add(a)

	mgr.DrainSustenance(context.Background(), svc, "", 50)
	if a.Sustenance() != 99 {
		t.Fatalf("sustenance = %d, want 99", a.Sustenance())
	}
	if got := fc.writes(); len(got) != 0 {
		t.Fatalf("full-tier player should get no reminder, got %v", got)
	}
}

// A nil service makes the drain a no-op (the unwired test default).
func TestDrainSustenance_NilServiceNoop(t *testing.T) {
	mgr := NewManager()
	r := &world.Room{ID: "x:1", Name: "X"}
	a, _ := newFakeActor("c1", "p1", "acc1", "Alice", r)
	a.sustenance = 50
	mgr.Add(a)

	mgr.DrainSustenance(context.Background(), nil, "", 1)
	if a.Sustenance() != 50 {
		t.Fatalf("nil-service drain mutated sustenance to %d, want 50", a.Sustenance())
	}
}

// An actor holding the admin role is skipped entirely: their sustenance
// never drains and they get no hunger reminder, so an admin never has to
// deal with famine while administering.
func TestDrainSustenance_AdminExempt(t *testing.T) {
	mgr := NewManager()
	svc := economy.NewSustenanceService(economy.DefaultSustenanceConfig())
	r := &world.Room{ID: "x:1", Name: "X"}
	admin, ac := newFakeActor("c1", "p1", "acc1", "Admin", r)
	admin.sustenance = 35 // hungry band — would normally drain + remind
	admin.Grant("admin")
	plain, _ := newFakeActor("c2", "p2", "acc2", "Bob", r)
	plain.sustenance = 35
	mgr.Add(admin)
	mgr.Add(plain)

	mgr.DrainSustenance(context.Background(), svc, "admin", 100)

	if admin.Sustenance() != 35 {
		t.Fatalf("admin sustenance = %d, want 35 (untouched)", admin.Sustenance())
	}
	if got := ac.writes(); len(got) != 0 {
		t.Fatalf("admin should get no hunger reminder, got %v", got)
	}
	if plain.Sustenance() != 34 {
		t.Fatalf("non-admin sustenance = %d, want 34 (drained)", plain.Sustenance())
	}
}

// An empty adminRole disables the exemption: even an admin-tagged actor
// drains normally.
func TestDrainSustenance_EmptyAdminRoleDrainsEveryone(t *testing.T) {
	mgr := NewManager()
	svc := economy.NewSustenanceService(economy.DefaultSustenanceConfig())
	r := &world.Room{ID: "x:1", Name: "X"}
	a, _ := newFakeActor("c1", "p1", "acc1", "Admin", r)
	a.sustenance = 50
	a.Grant("admin")
	mgr.Add(a)

	mgr.DrainSustenance(context.Background(), svc, "", 1)
	if a.Sustenance() != 49 {
		t.Fatalf("empty adminRole should drain admin too; sustenance = %d, want 49", a.Sustenance())
	}
}
