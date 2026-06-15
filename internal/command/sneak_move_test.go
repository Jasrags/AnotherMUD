package command_test

import (
	"context"
	"slices"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/gameclock"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// sneakBroadcaster captures the room id, text, and exclusion set of each
// SendToRoom so a test can assert exactly who the §3.2 movement filter
// suppressed the enter/leave line for.
type sneakBroadcaster struct {
	calls []sneakCall
}

type sneakCall struct {
	roomID  world.RoomID
	text    string
	exclude []string
}

func (b *sneakBroadcaster) SendToRoom(_ context.Context, roomID world.RoomID, text string, exclude ...string) {
	b.calls = append(b.calls, sneakCall{roomID: roomID, text: text, exclude: append([]string(nil), exclude...)})
}

func (b *sneakBroadcaster) callFor(roomID world.RoomID) (sneakCall, bool) {
	for _, c := range b.calls {
		if c.roomID == roomID {
			return c, true
		}
	}
	return sneakCall{}, false
}

// sneakMoveWorld builds a lit room A with a north exit into B (and B back south).
func sneakMoveWorld() (*world.World, *world.Room, *world.Room) {
	a := &world.Room{ID: "a", Name: "Plaza", Description: "A lit plaza.", Terrain: world.TerrainOutdoors,
		Exits: map[world.Direction]world.Exit{world.DirNorth: {Target: "b"}}}
	b := &world.Room{ID: "b", Name: "Lane", Description: "A quiet lane.", Terrain: world.TerrainOutdoors,
		Exits: map[world.Direction]world.Exit{world.DirSouth: {Target: "a"}}}
	w := world.New()
	w.AddRoom(a)
	w.AddRoom(b)
	return w, a, b
}

// A sneaking mover's departure line reaches an occupant who pierces the sneak
// (high perception) but is suppressed for one who does not (visibility §3.2).
func TestSneakMove_FiltersDepartureLinePerObserver(t *testing.T) {
	w, a, _ := sneakMoveWorld()
	store, place := entities.NewStore(), entities.NewPlacement()

	mover := &namedActor{testActor: newTestActor(a), name: "Rogue", playerID: "p-mover"}
	mover.Sneak(12) // moving concealment, contest difficulty 12

	sharp := &namedActor{testActor: newTestActor(a), name: "Sharp", playerID: "p-sharp"}
	sharp.perceptionBonus = 10 // 5 (roll) + 10 = 15 >= 12 → pierces
	dull := &namedActor{testActor: newTestActor(a), name: "Dull", playerID: "p-dull"}
	dull.perceptionBonus = 0 // 5 + 0 = 5 < 12 → fails

	loc := &stubLocator{}
	loc.add(mover)
	loc.add(sharp)
	loc.add(dull)

	rec := &sneakBroadcaster{}
	reg := command.New()
	if err := command.RegisterBuiltins(reg); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	env := command.Env{
		World:       w,
		Items:       store,
		Placement:   place,
		Light:       newLightResolver(gameclock.PeriodDay),
		Locator:     loc,
		Broadcaster: rec,
		SkillRoller: pickRoller{raw: 4}, // d20 face = 5 for every contest
	}

	if err := reg.Dispatch(context.Background(), env, mover, "n"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if mover.Room().ID != "b" {
		t.Fatalf("mover did not move; room = %q, want b", mover.Room().ID)
	}

	dep, ok := rec.callFor("a")
	if !ok {
		t.Fatal("no departure broadcast to room a")
	}
	// The mover and the failing observer are excluded; the piercing observer is not.
	if !slices.Contains(dep.exclude, "p-mover") {
		t.Errorf("departure exclude %v should contain the mover", dep.exclude)
	}
	if !slices.Contains(dep.exclude, "p-dull") {
		t.Errorf("departure exclude %v should suppress the line for the failing observer", dep.exclude)
	}
	if slices.Contains(dep.exclude, "p-sharp") {
		t.Errorf("departure exclude %v must NOT suppress the line for the piercing observer", dep.exclude)
	}
}

// A non-sneaking mover's movement is unfiltered: only the mover is excluded,
// preserving the legacy "everyone sees the line" path (visibility §3.2).
func TestSneakMove_NonSneakingMoverUnfiltered(t *testing.T) {
	w, a, _ := sneakMoveWorld()
	store, place := entities.NewStore(), entities.NewPlacement()

	mover := &namedActor{testActor: newTestActor(a), name: "Walker", playerID: "p-walker"}
	// NOT sneaking.
	bystander := &namedActor{testActor: newTestActor(a), name: "Bystander", playerID: "p-by"}

	loc := &stubLocator{}
	loc.add(mover)
	loc.add(bystander)

	rec := &sneakBroadcaster{}
	reg := command.New()
	if err := command.RegisterBuiltins(reg); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	env := command.Env{
		World:       w,
		Items:       store,
		Placement:   place,
		Light:       newLightResolver(gameclock.PeriodDay),
		Locator:     loc,
		Broadcaster: rec,
		SkillRoller: pickRoller{raw: 0},
	}

	if err := reg.Dispatch(context.Background(), env, mover, "n"); err != nil {
		t.Fatalf("move: %v", err)
	}
	dep, ok := rec.callFor("a")
	if !ok {
		t.Fatal("no departure broadcast to room a")
	}
	if len(dep.exclude) != 1 || dep.exclude[0] != "p-walker" {
		t.Errorf("non-sneaking departure exclude = %v, want only the mover", dep.exclude)
	}
}
