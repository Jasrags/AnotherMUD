package command_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/stats"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

func TestRegistry_Resolve(t *testing.T) {
	t.Parallel()
	r := command.New()
	mark := func(s string) command.Handler {
		return func(ctx context.Context, c *command.Context) error {
			return c.Actor.Write(ctx, s)
		}
	}
	for _, k := range []string{"look", "north", "n", "quit"} {
		if err := r.Register(k, mark(k)); err != nil {
			t.Fatalf("register %q: %v", k, err)
		}
	}

	cases := []struct {
		verb    string
		want    string
		nilWant bool
	}{
		{"look", "look", false},
		{"LOOK", "look", false},   // case-insensitive exact
		{"loo", "look", false},    // prefix
		{"n", "n", false},         // exact wins over prefix to "north"
		{"nor", "north", false},   // prefix
		{"q", "quit", false},      // prefix unique
		{"xyz", "", true},         // no match
	}
	for _, c := range cases {
		c := c
		t.Run(c.verb, func(t *testing.T) {
			t.Parallel()
			h := r.Resolve(c.verb)
			if c.nilWant {
				if h != nil {
					t.Fatalf("Resolve(%q) = handler, want nil", c.verb)
				}
				return
			}
			if h == nil {
				t.Fatalf("Resolve(%q) = nil, want %q", c.verb, c.want)
			}
			a := newTestActor(nil)
			_ = h(context.Background(), &command.Context{Actor: a})
			if got := a.lastLine(); got != c.want {
				t.Fatalf("handler wrote %q, want %q", got, c.want)
			}
		})
	}
}

func TestRegistry_RejectsDuplicateAndEmpty(t *testing.T) {
	t.Parallel()
	r := command.New()
	noop := func(ctx context.Context, c *command.Context) error { return nil }

	if err := r.Register("", noop); err == nil {
		t.Fatal("expected error on empty keyword")
	}
	if err := r.Register("k", nil); err == nil {
		t.Fatal("expected error on nil handler")
	}
	if err := r.Register("k", noop); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := r.Register("K", noop); err == nil {
		t.Fatal("expected error on duplicate (case-insensitive)")
	}
}

func TestDispatch_HuhOnUnknown(t *testing.T) {
	t.Parallel()
	r := command.New()
	a := newTestActor(nil)
	if err := r.Dispatch(context.Background(), command.Env{}, a, "wibble"); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got := a.lastLine(); got != "Huh?" {
		t.Fatalf("wrote %q, want Huh?", got)
	}
}

func TestDispatch_EmptyInputIsNoOp(t *testing.T) {
	t.Parallel()
	r := command.New()
	a := newTestActor(nil)
	if err := r.Dispatch(context.Background(), command.Env{}, a, "   "); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if a.lastLine() != "" {
		t.Fatalf("empty input produced output: %q", a.lastLine())
	}
}

func TestBuiltins_LookAndMove(t *testing.T) {
	t.Parallel()
	w := world.New()
	a := &world.Room{ID: "a", Name: "Room A", Description: "first"}
	b := &world.Room{ID: "b", Name: "Room B", Description: "second"}
	a.Exits = map[world.Direction]world.Exit{world.DirNorth: {Target: b.ID}}
	b.Exits = map[world.Direction]world.Exit{world.DirSouth: {Target: a.ID}}
	w.AddRoom(a)
	w.AddRoom(b)

	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}

	actor := newTestActor(a)

	if err := r.Dispatch(context.Background(), command.Env{World: w}, actor, "look"); err != nil {
		t.Fatalf("look: %v", err)
	}
	if !strings.Contains(actor.lastLine(), "Room A") {
		t.Fatalf("look did not render room: %q", actor.lastLine())
	}

	if err := r.Dispatch(context.Background(), command.Env{World: w}, actor, "n"); err != nil {
		t.Fatalf("n: %v", err)
	}
	if actor.Room().ID != "b" {
		t.Fatalf("after n, room = %q, want b", actor.Room().ID)
	}
	if !strings.Contains(actor.lastLine(), "Room B") {
		t.Fatalf("move did not render destination: %q", actor.lastLine())
	}

	if err := r.Dispatch(context.Background(), command.Env{World: w}, actor, "n"); err != nil {
		t.Fatalf("n with no exit: %v", err)
	}
	if !strings.Contains(actor.lastLine(), "cannot go that way") {
		t.Fatalf("expected no-exit reply, got %q", actor.lastLine())
	}
}

func TestBuiltins_Quit(t *testing.T) {
	t.Parallel()
	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	a := newTestActor(&world.Room{ID: "void"})
	err := r.Dispatch(context.Background(), command.Env{World: world.New()}, a, "quit")
	if !errors.Is(err, command.ErrQuit) {
		t.Fatalf("quit returned %v, want ErrQuit", err)
	}
}

// testActor is a command.Actor used by these tests; it captures every
// Write so assertions can inspect output.
type testActor struct {
	mu        sync.Mutex
	id        string
	name      string
	playerID  string
	room      *world.Room
	lines     []string
	color     bool
	inventory []entities.EntityID
	equipment map[string]entities.EntityID
	mods      map[entities.SourceKey][]stats.Modifier
}

func newTestActor(start *world.Room) *testActor {
	return &testActor{room: start}
}

// newNamedTestActor builds an actor with a stable identity so tests
// that exercise multi-actor flows (give, broadcast) can distinguish
// sender from recipient.
func newNamedTestActor(name, playerID string, start *world.Room) *testActor {
	return &testActor{
		id:       "test-" + playerID,
		name:     name,
		playerID: playerID,
		room:     start,
	}
}

func (a *testActor) ID() string {
	if a.id != "" {
		return a.id
	}
	return "test"
}
func (a *testActor) Name() string     { return a.name }
func (a *testActor) PlayerID() string { return a.playerID }

func (a *testActor) Room() *world.Room {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.room
}

func (a *testActor) SetRoom(r *world.Room) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.room = r
}

func (a *testActor) Write(ctx context.Context, msg string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lines = append(a.lines, msg)
	return nil
}

func (a *testActor) ColorEnabled() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.color
}

func (a *testActor) SetColorEnabled(v bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.color = v
}

func (a *testActor) Inventory() []entities.EntityID {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]entities.EntityID, len(a.inventory))
	copy(out, a.inventory)
	return out
}

func (a *testActor) AddToInventory(id entities.EntityID) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.inventory = append(a.inventory, id)
}

func (a *testActor) RemoveFromInventory(id entities.EntityID) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, e := range a.inventory {
		if e == id {
			a.inventory = append(a.inventory[:i], a.inventory[i+1:]...)
			return true
		}
	}
	return false
}

func (a *testActor) Equipment() map[string]entities.EntityID {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[string]entities.EntityID, len(a.equipment))
	for k, v := range a.equipment {
		out[k] = v
	}
	return out
}

func (a *testActor) Equip(slotKey string, id entities.EntityID, mods []stats.Modifier) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, e := range a.inventory {
		if e == id {
			a.inventory = append(a.inventory[:i], a.inventory[i+1:]...)
			if a.equipment == nil {
				a.equipment = make(map[string]entities.EntityID)
			}
			a.equipment[slotKey] = id
			if a.mods == nil {
				a.mods = make(map[entities.SourceKey][]stats.Modifier)
			}
			dup := make([]stats.Modifier, len(mods))
			copy(dup, mods)
			a.mods[entities.EquipmentSourceKey(id)] = dup
			return true
		}
	}
	return false
}

// MarkContentsDirty is a no-op for the test actor — the testActor
// does not persist, so there is no save tree to re-sync. The
// connActor implementation is what production exercises (see
// session.go).
func (a *testActor) MarkContentsDirty() {}

func (a *testActor) Unequip(slotKey string) (entities.EntityID, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	id, ok := a.equipment[slotKey]
	if !ok {
		return "", false
	}
	delete(a.equipment, slotKey)
	a.inventory = append(a.inventory, id)
	delete(a.mods, entities.EquipmentSourceKey(id))
	return id, true
}

func (a *testActor) lastLine() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.lines) == 0 {
		return ""
	}
	return a.lines[len(a.lines)-1]
}
