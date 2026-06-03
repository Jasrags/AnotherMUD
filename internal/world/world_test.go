package world_test

import (
	"errors"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

func TestRoomHasTag(t *testing.T) {
	r := &world.Room{ID: "x:1", Tags: []string{"safe-room", "safe"}}
	if !r.HasTag("safe-room") {
		t.Error("HasTag(safe-room) = false, want true")
	}
	if !r.HasTag("safe") {
		t.Error("HasTag(safe) = false, want true")
	}
	if r.HasTag("indoors") {
		t.Error("HasTag(indoors) = true, want false")
	}
	if (&world.Room{ID: "x:2"}).HasTag("safe") {
		t.Error("untagged room should report no tags")
	}
}

func TestParseDirection(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want world.Direction
		ok   bool
	}{
		{"n", world.DirNorth, true},
		{"NORTH", world.DirNorth, true},
		{"  south ", world.DirSouth, true},
		{"e", world.DirEast, true},
		{"w", world.DirWest, true},
		{"u", world.DirUp, true},
		{"d", world.DirDown, true},
		{"northeast", world.DirInvalid, false},
		{"", world.DirInvalid, false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			t.Parallel()
			got, ok := world.ParseDirection(c.in)
			if got != c.want || ok != c.ok {
				t.Fatalf("ParseDirection(%q) = (%v, %v), want (%v, %v)",
					c.in, got, ok, c.want, c.ok)
			}
		})
	}
}

func TestDirection_Opposite(t *testing.T) {
	t.Parallel()
	pairs := [][2]world.Direction{
		{world.DirNorth, world.DirSouth},
		{world.DirSouth, world.DirNorth},
		{world.DirEast, world.DirWest},
		{world.DirWest, world.DirEast},
		{world.DirUp, world.DirDown},
		{world.DirDown, world.DirUp},
	}
	for _, p := range pairs {
		if got := p[0].Opposite(); got != p[1] {
			t.Errorf("%v.Opposite() = %v, want %v", p[0], got, p[1])
		}
	}
}

func TestWorld_Move(t *testing.T) {
	t.Parallel()
	w := world.New()
	a := &world.Room{ID: "a", Name: "A"}
	b := &world.Room{ID: "b", Name: "B"}
	a.Exits = map[world.Direction]world.Exit{world.DirNorth: {Target: b.ID}}
	b.Exits = map[world.Direction]world.Exit{world.DirSouth: {Target: a.ID}}
	w.AddRoom(a)
	w.AddRoom(b)

	t.Run("success", func(t *testing.T) {
		got, err := w.Move("a", world.DirNorth)
		if err != nil {
			t.Fatalf("Move: %v", err)
		}
		if got.ID != "b" {
			t.Fatalf("Move target = %q, want b", got.ID)
		}
	})

	t.Run("no exit in direction", func(t *testing.T) {
		_, err := w.Move("a", world.DirSouth)
		if !errors.Is(err, world.ErrNoExit) {
			t.Fatalf("err = %v, want ErrNoExit", err)
		}
	})

	t.Run("source room unknown", func(t *testing.T) {
		_, err := w.Move("ghost", world.DirNorth)
		if !errors.Is(err, world.ErrRoomNotFound) {
			t.Fatalf("err = %v, want ErrRoomNotFound", err)
		}
	})

	t.Run("target room unknown", func(t *testing.T) {
		w2 := world.New()
		dangling := &world.Room{
			ID:    "x",
			Exits: map[world.Direction]world.Exit{world.DirNorth: {Target: "missing"}},
		}
		w2.AddRoom(dangling)
		_, err := w2.Move("x", world.DirNorth)
		if !errors.Is(err, world.ErrRoomNotFound) {
			t.Fatalf("err = %v, want ErrRoomNotFound", err)
		}
	})
}

// TestRoomPropertyAccessors pins the M14.5 property bag surface:
// typed accessors return (value, true) when the property is set
// AND of the right type, otherwise (zero, false).
func TestRoomPropertyAccessors(t *testing.T) {
	r := &world.Room{Properties: map[string]any{
		"quest_grant": "tapestry-core:village-welcome",
		"some_count":  3,
		"is_special":  true,
		"wrong_type":  "not an int",
	}}

	// Raw Property always returns the stored value.
	if v, ok := r.Property("quest_grant"); !ok || v != "tapestry-core:village-welcome" {
		t.Errorf("Property = %v ok=%v", v, ok)
	}

	// PropertyString
	if s, ok := r.PropertyString("quest_grant"); !ok || s != "tapestry-core:village-welcome" {
		t.Errorf("PropertyString quest_grant = %q ok=%v", s, ok)
	}
	if _, ok := r.PropertyString("some_count"); ok {
		t.Error("PropertyString on int: want miss")
	}

	// PropertyInt
	if n, ok := r.PropertyInt("some_count"); !ok || n != 3 {
		t.Errorf("PropertyInt some_count = %d ok=%v", n, ok)
	}
	if _, ok := r.PropertyInt("wrong_type"); ok {
		t.Error("PropertyInt on string: want miss")
	}

	// PropertyBool
	if b, ok := r.PropertyBool("is_special"); !ok || !b {
		t.Errorf("PropertyBool is_special = %v ok=%v", b, ok)
	}

	// Absent key
	if _, ok := r.Property("nope"); ok {
		t.Error("Property absent: want miss")
	}
}

// TestRoomPropertyNilSafety: a nil Room or a Room with nil
// Properties returns zero/false on every accessor without
// panicking.
func TestRoomPropertyNilSafety(t *testing.T) {
	var r *world.Room
	if _, ok := r.Property("anything"); ok {
		t.Error("nil receiver: want miss")
	}

	empty := &world.Room{}
	if _, ok := empty.Property("anything"); ok {
		t.Error("nil Properties: want miss")
	}
	if _, ok := empty.PropertyString("x"); ok {
		t.Error("nil Properties PropertyString: want miss")
	}
	if _, ok := empty.PropertyInt("x"); ok {
		t.Error("nil Properties PropertyInt: want miss")
	}
	if _, ok := empty.PropertyBool("x"); ok {
		t.Error("nil Properties PropertyBool: want miss")
	}
}

// twoRoomWorld returns a world with two rooms ("a" and "b") and
// paired north/south exits between them. The exits may carry doors
// per the supplied closures.
func twoRoomWorld(t *testing.T, doorA, doorB *world.DoorState) *world.World {
	t.Helper()
	w := world.New()
	w.AddRoom(&world.Room{
		ID:   "a",
		Name: "Room A",
		Exits: map[world.Direction]world.Exit{
			world.DirNorth: {Target: "b", Door: doorA},
		},
	})
	w.AddRoom(&world.Room{
		ID:   "b",
		Name: "Room B",
		Exits: map[world.Direction]world.Exit{
			world.DirSouth: {Target: "a", Door: doorB},
		},
	})
	return w
}

func newDoor() *world.DoorState {
	return &world.DoorState{
		Name:          "iron gate",
		Keywords:      []string{"iron", "gate"},
		Closed:        true,
		Locked:        false,
		DefaultClosed: true,
	}
}

// TestMoveBlockedByClosedDoor pins spec §3.3 step 4.
func TestMoveBlockedByClosedDoor(t *testing.T) {
	w := twoRoomWorld(t, newDoor(), newDoor())
	_, err := w.Move("a", world.DirNorth)
	if !errors.Is(err, world.ErrDoorClosed) {
		t.Errorf("Move through closed door: err = %v, want ErrDoorClosed", err)
	}
}

func TestMoveAllowedWhenDoorOpen(t *testing.T) {
	d := newDoor()
	d.Closed = false
	w := twoRoomWorld(t, d, nil)
	if _, err := w.Move("a", world.DirNorth); err != nil {
		t.Errorf("Move through open door: %v", err)
	}
}

func TestMoveDoorless(t *testing.T) {
	w := twoRoomWorld(t, nil, nil)
	if _, err := w.Move("a", world.DirNorth); err != nil {
		t.Errorf("doorless move: %v", err)
	}
}

func TestCanPass(t *testing.T) {
	w := twoRoomWorld(t, newDoor(), nil)
	if w.CanPass("a", world.DirNorth) {
		t.Error("CanPass through closed door: want false")
	}
	if !w.CanPass("b", world.DirSouth) {
		t.Error("CanPass through doorless exit: want true")
	}
	if w.CanPass("a", world.DirEast) {
		t.Error("CanPass through no-exit: want false")
	}
	if w.CanPass("nowhere", world.DirNorth) {
		t.Error("CanPass from unknown room: want false")
	}
}

func TestGetDoor(t *testing.T) {
	d := newDoor()
	w := twoRoomWorld(t, d, nil)
	got, ok := w.GetDoor("a", world.DirNorth)
	if !ok || got.Name != "iron gate" {
		t.Errorf("GetDoor = %+v ok=%v", got, ok)
	}
	if _, ok := w.GetDoor("b", world.DirSouth); ok {
		t.Error("GetDoor on doorless exit: want false")
	}
}

func TestOpenCloseRoundTripWithReverseSync(t *testing.T) {
	w := twoRoomWorld(t, newDoor(), newDoor())

	if !w.OpenDoor("a", world.DirNorth) {
		t.Fatal("OpenDoor returned false")
	}
	// Both sides should now be open.
	if d, _ := w.GetDoor("a", world.DirNorth); d.Closed {
		t.Error("a-side door still closed after OpenDoor")
	}
	if d, _ := w.GetDoor("b", world.DirSouth); d.Closed {
		t.Error("b-side door not synchronized")
	}

	// Re-opening an already-open door returns false (silent no-op).
	if w.OpenDoor("a", world.DirNorth) {
		t.Error("OpenDoor on already-open door returned true")
	}

	if !w.CloseDoor("a", world.DirNorth) {
		t.Fatal("CloseDoor returned false")
	}
	if d, _ := w.GetDoor("b", world.DirSouth); !d.Closed {
		t.Error("b-side door not synchronized on close")
	}
}

func TestLockRequiresClosed(t *testing.T) {
	d := newDoor()
	d.Closed = false
	w := twoRoomWorld(t, d, nil)
	if w.LockDoor("a", world.DirNorth) {
		t.Error("LockDoor on open door returned true")
	}
}

func TestLockUnlockRoundTrip(t *testing.T) {
	w := twoRoomWorld(t, newDoor(), newDoor()) // closed, unlocked
	if !w.LockDoor("a", world.DirNorth) {
		t.Fatal("LockDoor returned false")
	}
	if d, _ := w.GetDoor("b", world.DirSouth); !d.Locked {
		t.Error("b-side not synchronized on lock")
	}
	// Already locked → no-op
	if w.LockDoor("a", world.DirNorth) {
		t.Error("LockDoor on already-locked door returned true")
	}

	if !w.UnlockDoor("a", world.DirNorth) {
		t.Fatal("UnlockDoor returned false")
	}
	if d, _ := w.GetDoor("b", world.DirSouth); d.Locked {
		t.Error("b-side not synchronized on unlock")
	}
}

// TestOneWayDoorReverseAbsentIsOK pins spec §5.2 step 4: a
// reverse-side absent door (one-way door, or asymmetric mapping)
// is allowed, not an error.
func TestOneWayDoorReverseAbsentIsOK(t *testing.T) {
	w := twoRoomWorld(t, newDoor(), nil) // only the a-side has a door
	if !w.OpenDoor("a", world.DirNorth) {
		t.Error("OpenDoor one-way: want true")
	}
	if d, _ := w.GetDoor("a", world.DirNorth); d.Closed {
		t.Error("a-side door still closed")
	}
}

// RoomDoors snapshots each door's state by value under the read lock, so
// enumerating a room's doors (tab-completion runs this off the live
// world) never races a concurrent open/close/lock/unlock. Run under
// -race: a direct room.Exits read would flag here.
func TestRoomDoors_RaceWithMutation(t *testing.T) {
	w := twoRoomWorld(t, newDoor(), newDoor())
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 2000; i++ {
			w.OpenDoor("a", world.DirNorth)
			w.CloseDoor("a", world.DirNorth)
		}
	}()
	for i := 0; i < 2000; i++ {
		for _, d := range w.RoomDoors("a") {
			_ = d.Door.Closed // read a field the mutators write
		}
	}
	<-done
}

func TestDoorOpsOnUnknownRoomNoop(t *testing.T) {
	w := twoRoomWorld(t, newDoor(), newDoor())
	if w.OpenDoor("nowhere", world.DirNorth) {
		t.Error("OpenDoor on unknown room: want false")
	}
	if w.CloseDoor("nowhere", world.DirNorth) {
		t.Error("CloseDoor on unknown room: want false")
	}
}

func TestDoorOpsOnDoorlessExitNoop(t *testing.T) {
	w := twoRoomWorld(t, nil, nil)
	if w.OpenDoor("a", world.DirNorth) {
		t.Error("OpenDoor on doorless exit: want false")
	}
}

// crossroads returns a room with three doors: north (iron gate),
// east (oak gate), and south (no door — just an exit).
func crossroads() *world.World {
	w := world.New()
	w.AddRoom(&world.Room{
		ID:   "x",
		Name: "Crossroads",
		Exits: map[world.Direction]world.Exit{
			world.DirNorth: {Target: "n", Door: &world.DoorState{Name: "iron gate", Keywords: []string{"iron", "gate"}, Closed: true}},
			world.DirEast:  {Target: "e", Door: &world.DoorState{Name: "oak gate", Keywords: []string{"oak", "gate"}, Closed: true}},
			world.DirSouth: {Target: "s"},
		},
	})
	w.AddRoom(&world.Room{ID: "n"})
	w.AddRoom(&world.Room{ID: "e"})
	w.AddRoom(&world.Room{ID: "s"})
	return w
}

func TestResolveDoorTarget_DirectionWins(t *testing.T) {
	w := crossroads()
	res := w.ResolveDoorTarget("x", "north")
	if !res.Ok || res.Direction != world.DirNorth {
		t.Errorf("direction north: %+v", res)
	}
	// Direction parse works even for a doorless exit (south).
	res = w.ResolveDoorTarget("x", "south")
	if !res.Ok || res.Direction != world.DirSouth {
		t.Errorf("doorless direction: %+v", res)
	}
}

func TestResolveDoorTarget_KeywordUnique(t *testing.T) {
	w := crossroads()
	res := w.ResolveDoorTarget("x", "iron")
	if !res.Ok || res.Direction != world.DirNorth {
		t.Errorf("iron: %+v", res)
	}
}

func TestResolveDoorTarget_KeywordAmbiguous(t *testing.T) {
	w := crossroads()
	res := w.ResolveDoorTarget("x", "gate")
	if !res.Ambiguous {
		t.Errorf("gate (ambiguous): %+v", res)
	}
}

func TestResolveDoorTarget_OrdinalDisambiguates(t *testing.T) {
	w := crossroads()
	if res := w.ResolveDoorTarget("x", "1.gate"); !res.Ok || res.Direction != world.DirNorth {
		t.Errorf("1.gate: %+v", res)
	}
	if res := w.ResolveDoorTarget("x", "2.gate"); !res.Ok || res.Direction != world.DirEast {
		t.Errorf("2.gate: %+v", res)
	}
	// Out-of-range ordinal returns miss.
	if res := w.ResolveDoorTarget("x", "5.gate"); res.Ok {
		t.Errorf("5.gate: want miss, got %+v", res)
	}
}

func TestResolveDoorTarget_UnknownKeyword(t *testing.T) {
	w := crossroads()
	if res := w.ResolveDoorTarget("x", "trapdoor"); res.Ok || res.Ambiguous {
		t.Errorf("trapdoor: want plain miss, got %+v", res)
	}
}

func TestResolveDoorTarget_EmptyAndUnknownRoom(t *testing.T) {
	w := crossroads()
	if res := w.ResolveDoorTarget("x", ""); res.Ok {
		t.Errorf("empty arg: %+v", res)
	}
	if res := w.ResolveDoorTarget("nowhere", "gate"); res.Ok {
		t.Errorf("unknown room: %+v", res)
	}
}

// TestResetDoorsInArea_RestoresDefaults pins spec §5.4 — area
// reset restores doors to their DefaultClosed / DefaultLocked.
func TestResetDoorsInArea_RestoresDefaults(t *testing.T) {
	d := newDoor()           // closed, unlocked, DefaultClosed=true
	d.DefaultLocked = true   // pretend the boot state was locked too
	d.Locked = true          // and lock it now to test re-lock from unlocked
	d.Closed = true
	w := twoRoomWorld(t, d, nil)
	w.AddArea(&world.Area{ID: "town"})

	// Open + unlock — diverges from defaults.
	w.UnlockDoor("a", world.DirNorth)
	w.OpenDoor("a", world.DirNorth)
	if got, _ := w.GetDoor("a", world.DirNorth); !got.Closed != true || got.Locked != false {
		// expect now Closed=false, Locked=false
	}

	// Reset — should restore Closed=true, Locked=true.
	// But: roomA's id is "a", not "town:a", so area=town won't
	// match. Skip prefix-keyed test and pass area="" which matches
	// the bare-id form... actually the test rooms use bare ids
	// ("a", "b") so we need to query by area="a" to match room "a".
	if n := w.ResetDoorsInArea("a"); n == 0 {
		t.Errorf("ResetDoorsInArea returned 0 transitions; want at least 1")
	}
	got, _ := w.GetDoor("a", world.DirNorth)
	if !got.Closed || !got.Locked {
		t.Errorf("after reset: Closed=%v Locked=%v, want both true", got.Closed, got.Locked)
	}
}

// TestResetDoorsInArea_MatchesPrefix pins the area-prefix match
// rule (rooms named "<area>:<sub>" reset under area).
func TestResetDoorsInArea_MatchesPrefix(t *testing.T) {
	w := world.New()
	d := &world.DoorState{
		Name: "iron gate", Keywords: []string{"gate"},
		Closed:        false, // diverges from default below
		Locked:        false,
		DefaultClosed: true,
		DefaultLocked: false,
	}
	w.AddRoom(&world.Room{
		ID:   "town:square",
		Name: "Town Square",
		Exits: map[world.Direction]world.Exit{
			world.DirNorth: {Target: "town:gate", Door: d},
		},
	})
	w.AddRoom(&world.Room{
		ID:   "town:gate",
		Name: "Gate",
	})

	if n := w.ResetDoorsInArea("town"); n == 0 {
		t.Error("prefix match reset: want at least 1 transition")
	}
	got, _ := w.GetDoor("town:square", world.DirNorth)
	if !got.Closed {
		t.Error("door not restored to closed after area reset")
	}
}

// TestResetDoorsInArea_NoChangeWhenAlreadyDefault is a no-op when
// doors are already at their defaults.
func TestResetDoorsInArea_NoChangeWhenAlreadyDefault(t *testing.T) {
	w := twoRoomWorld(t, newDoor(), newDoor())
	if n := w.ResetDoorsInArea("a"); n != 0 {
		t.Errorf("already-at-default: transitions = %d, want 0", n)
	}
}

// TestKeywordExit_Lifecycle pins the M15.2 keyword exit substrate:
// add/has/remove, plus MoveByKeyword with case-insensitive lookup.
func TestKeywordExit_Lifecycle(t *testing.T) {
	w := world.New()
	w.AddRoom(&world.Room{ID: "a", Name: "A"})
	w.AddRoom(&world.Room{ID: "b", Name: "B"})

	if !w.AddKeywordExit("a", "Portal", "b") {
		t.Fatal("AddKeywordExit failed")
	}
	if !w.HasKeywordExit("a", "PORTAL") {
		t.Error("HasKeywordExit case-insensitive miss")
	}

	dst, err := w.MoveByKeyword("a", "portal")
	if err != nil {
		t.Fatalf("MoveByKeyword: %v", err)
	}
	if dst.ID != "b" {
		t.Errorf("dst = %q, want b", dst.ID)
	}

	if !w.RemoveKeywordExit("a", "portal") {
		t.Error("RemoveKeywordExit returned false")
	}
	if w.HasKeywordExit("a", "portal") {
		t.Error("HasKeywordExit after remove: want false")
	}
	if _, err := w.MoveByKeyword("a", "portal"); !errors.Is(err, world.ErrNoExit) {
		t.Errorf("MoveByKeyword after remove: err = %v, want ErrNoExit", err)
	}
}

func TestKeywordExit_RejectsCollision(t *testing.T) {
	w := world.New()
	w.AddRoom(&world.Room{ID: "a"})
	w.AddRoom(&world.Room{ID: "b"})
	w.AddRoom(&world.Room{ID: "c"})
	w.AddKeywordExit("a", "gate", "b")
	if w.AddKeywordExit("a", "GATE", "c") {
		t.Error("AddKeywordExit collision: want false (case-insensitive)")
	}
}

func TestKeywordExit_RejectsMissingRoom(t *testing.T) {
	w := world.New()
	w.AddRoom(&world.Room{ID: "a"})
	if w.AddKeywordExit("a", "void", "no:such") {
		t.Error("missing target: want false")
	}
	if w.AddKeywordExit("ghost", "gate", "a") {
		t.Error("missing source: want false")
	}
}

func TestKeywordExit_RejectsEmpty(t *testing.T) {
	w := world.New()
	w.AddRoom(&world.Room{ID: "a"})
	w.AddRoom(&world.Room{ID: "b"})
	if w.AddKeywordExit("a", "   ", "b") {
		t.Error("empty keyword: want false")
	}
}
