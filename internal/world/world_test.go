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
