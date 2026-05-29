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
