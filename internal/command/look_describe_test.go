package command_test

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/mob"
)

func TestLook_AtItemWithDescription(t *testing.T) {
	f := newInvFixture(t)
	a := newNamedTestActor("Alice", "p-1", f.room)
	tpl := sword()
	tpl.Description = "A plain soldier's short sword, nicked from use."
	f.spawnInRoom(t, tpl)
	r := newRegistry(t)
	dispatchActor(t, r, f.env(), a, "look sword")
	if got := a.lastLine(); !strings.Contains(got, "soldier's short sword") {
		t.Errorf("look at described item = %q, want the authored prose", got)
	}
}

func TestLook_AtItemNoDescriptionFallback(t *testing.T) {
	f := newInvFixture(t)
	a := newNamedTestActor("Alice", "p-1", f.room)
	f.spawnInRoom(t, sword()) // sword() carries no Description
	r := newRegistry(t)
	dispatchActor(t, r, f.env(), a, "look sword")
	if got := a.lastLine(); !strings.Contains(got, "nothing special") {
		t.Errorf("look at undescribed item = %q, want the generic fallback", got)
	}
}

func placeMob(t *testing.T, f *invFixture, tpl *mob.Template) *entities.MobInstance {
	t.Helper()
	mb, err := f.store.SpawnMob(tpl)
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	f.place.Place(mb.ID(), f.room.ID)
	return mb
}

// TestLook_AtMobWithDescription pins the reported bug fix: `look maerys`
// resolved "You don't see that here." even though the mob was in the
// room, because look only searched items. Creatures are now look targets.
func TestLook_AtMobWithDescription(t *testing.T) {
	f := newInvFixture(t)
	a := newNamedTestActor("Alice", "p-1", f.room)
	placeMob(t, f, &mob.Template{
		ID: "tapestry-core:trainer", Name: "Maerys the Training Master",
		Type: "npc", Behavior: "stationary",
		Keywords:    []string{"maerys", "master"},
		Description: "A broad-shouldered woman with scarred forearms.",
	})
	r := newRegistry(t)
	dispatchActor(t, r, f.env(), a, "look maerys")
	if got := a.lastLine(); !strings.Contains(got, "scarred forearms") {
		t.Errorf("look at described mob = %q, want the authored prose", got)
	}
}

func TestLook_AtMobNoDescriptionFallback(t *testing.T) {
	f := newInvFixture(t)
	a := newNamedTestActor("Alice", "p-1", f.room)
	placeMob(t, f, &mob.Template{
		ID: "tapestry-core:rat", Name: "a small rat", Type: "npc",
		Behavior: "wander", Keywords: []string{"rat"},
	})
	r := newRegistry(t)
	dispatchActor(t, r, f.env(), a, "look rat")
	if got := a.lastLine(); !strings.Contains(got, "nothing special about a small rat") {
		t.Errorf("look at undescribed mob = %q, want the generic fallback", got)
	}
}

// `consider` must NOT resolve items (it's the creature/tactical lens):
// looking at a creature and considering an item are kept distinct.
func TestLook_AtMissingCreatureStillNotFound(t *testing.T) {
	f := newInvFixture(t)
	a := newNamedTestActor("Alice", "p-1", f.room)
	r := newRegistry(t)
	dispatchActor(t, r, f.env(), a, "look dragon")
	if got := a.lastLine(); !strings.Contains(got, "don't see that") {
		t.Errorf("look at absent target = %q, want not-found", got)
	}
}
