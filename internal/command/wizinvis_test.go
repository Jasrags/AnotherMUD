package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
)

// `wizinvis` toggles admin invisibility on, emitting entity.concealed
// (source = admin-invis), then off, emitting entity.revealed (visibility §3.4).
func TestWizinvis_TogglesWithEvents(t *testing.T) {
	f := newInvFixture(t)
	admin := newRoleActor("Maerys", "p-adm", "admin")
	admin.testActor.room = f.room
	bus := eventbus.New()
	concealed := captureEvents(t, bus, eventbus.EventEntityConcealed)
	revealed := captureEvents(t, bus, eventbus.EventEntityRevealed)
	env := f.env()
	env.Bus = bus
	env.AdminRole = "admin"
	r := newRegistry(t)

	// On.
	if err := r.Dispatch(context.Background(), env, admin, "wizinvis"); err != nil {
		t.Fatalf("dispatch wizinvis: %v", err)
	}
	if !admin.IsAdminInvisible() {
		t.Error("admin should be invisible after `wizinvis`")
	}
	if len(*concealed) != 1 {
		t.Fatalf("EntityConcealed published %d times, want 1", len(*concealed))
	}
	if ev := (*concealed)[0].(eventbus.EntityConcealed); ev.SourceType != "admin-invis" || ev.EntityID != "p-adm" {
		t.Errorf("EntityConcealed = %+v, want {p-adm, admin-invis}", ev)
	}

	// Off.
	if err := r.Dispatch(context.Background(), env, admin, "wizinvis"); err != nil {
		t.Fatalf("dispatch wizinvis (off): %v", err)
	}
	if admin.IsAdminInvisible() {
		t.Error("admin should be visible after a second `wizinvis`")
	}
	if len(*revealed) != 1 {
		t.Fatalf("EntityRevealed published %d times, want 1", len(*revealed))
	}
	if ev := (*revealed)[0].(eventbus.EntityRevealed); ev.SourceType != "admin-invis" || ev.Reason != "emerged" {
		t.Errorf("EntityRevealed = %+v, want source=admin-invis reason=emerged", ev)
	}
}

// A non-admin cannot use `wizinvis` — the admin gate refuses it with the same
// "Huh?" an unknown verb gets (admin-verbs §2), and no state changes.
func TestWizinvis_RefusedForNonAdmin(t *testing.T) {
	f := newInvFixture(t)
	pleb := newRoleActor("Pleb", "p-pleb") // no roles
	pleb.testActor.room = f.room
	env := f.env()
	env.AdminRole = "admin"

	if err := newRegistry(t).Dispatch(context.Background(), env, pleb, "wizinvis"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if pleb.IsAdminInvisible() {
		t.Error("a non-admin must not become invisible")
	}
	if got := pleb.lastLine(); !strings.Contains(got, "Huh?") {
		t.Errorf("non-admin wizinvis = %q, want the unknown-verb refusal", got)
	}
}

// wizinvis is flag-gated: a breaks_concealment command does NOT drop it
// (visibility §3.4 / §4.5 — only roll-based hide/sneak break on action).
func TestWizinvis_DoesNotBreakOnAction(t *testing.T) {
	f := newInvFixture(t)
	admin := newRoleActor("Maerys", "p-adm", "admin")
	admin.testActor.room = f.room
	admin.SetAdminInvisible(true)
	bus := eventbus.New()
	revealed := captureEvents(t, bus, eventbus.EventEntityRevealed)
	env := f.env()
	env.Bus = bus

	if err := breaksRegistry(t, true).Dispatch(context.Background(), env, admin, "act"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !admin.IsAdminInvisible() {
		t.Error("a loud action must NOT drop admin invisibility (flag-gated, §3.4)")
	}
	for _, ev := range *revealed {
		if ev.(eventbus.EntityRevealed).SourceType == "admin-invis" {
			t.Error("admin-invis must not emit revealed on action")
		}
	}
}

// End-to-end through the visibility predicate (visibility §3.4 / §5.4): a
// wizinvis occupant is concealed from a non-admin viewer's target resolution
// but visible to an admin viewer (equal rank pierces). Exercises
// visibilityPredicate → adminInvisibleOccupants → CanSee.
func TestWizinvis_PredicateHidesFromNonAdminVisibleToAdmin(t *testing.T) {
	f := newInvFixture(t)
	ghost := &namedActor{testActor: newTestActor(f.room), name: "Ghost", playerID: "p-ghost"}
	ghost.SetAdminInvisible(true)

	// Non-admin viewer cannot see the wizinvis occupant.
	pleb := newNamedTestActor("Pleb", "p-pleb", f.room)
	locP := &stubLocator{}
	locP.add(pleb)
	locP.add(ghost)
	rcP := (&command.Context{Actor: pleb, World: f.world, Items: f.store, Placement: f.place, Locator: locP}).BuildResolveContext()
	if rcP.CanSee == nil {
		t.Fatal("predicate should be built when a wizinvis occupant is present")
	}
	if rcP.CanSee("p-ghost") {
		t.Error("a non-admin must not see a wizinvis occupant")
	}
	if !rcP.CanSee("p-pleb") {
		t.Error("the viewer must see themselves")
	}

	// Admin viewer pierces it.
	watcher := newRoleActor("Watcher", "p-watch", "admin")
	watcher.testActor.room = f.room
	locA := &stubLocator{}
	locA.add(watcher)
	locA.add(ghost)
	rcA := (&command.Context{Actor: watcher, AdminRole: "admin", World: f.world, Items: f.store, Placement: f.place, Locator: locA}).BuildResolveContext()
	// rcA.CanSee may be nil only if nothing is concealed; the admin pierces the
	// admin-invis layer, so either nil (treated permissive) or true is correct.
	if rcA.CanSee != nil && !rcA.CanSee("p-ghost") {
		t.Error("an admin must see a wizinvis occupant")
	}
}

// helper: roleActor for admin-invis tests needs no extra surface — testActor
// already implements adminInvisible (IsAdminInvisible/SetAdminInvisible) and
// roleActor adds HasRole. This compile-time assertion documents that.
var _ command.Actor = (*roleActor)(nil)
