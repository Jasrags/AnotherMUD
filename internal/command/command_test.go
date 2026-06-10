package command_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/crafting"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/help"
	"github.com/Jasrags/AnotherMUD/internal/progression"
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
		{"LOOK", "look", false}, // case-insensitive exact
		{"loo", "look", false},  // prefix
		{"n", "n", false},       // exact wins over prefix to "north"
		{"nor", "north", false}, // prefix
		{"q", "quit", false},    // prefix unique
		{"xyz", "", true},       // no match
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

func TestRegisterCommand_AliasesAndMetadata(t *testing.T) {
	t.Parallel()
	r := command.New()
	noop := func(ctx context.Context, c *command.Context) error { return nil }

	if err := r.RegisterCommand(command.Command{
		Keyword: "equipment",
		Aliases: []string{"eq"},
		Brief:   "Show equipped items.",
		Syntax:  []string{"equipment"},
		Handler: noop,
	}); err != nil {
		t.Fatalf("RegisterCommand: %v", err)
	}
	// Both the primary and the alias resolve.
	if r.Resolve("equipment") == nil || r.Resolve("eq") == nil {
		t.Fatal("primary or alias did not resolve")
	}

	cmds := r.Commands()
	if len(cmds) != 1 {
		t.Fatalf("Commands() = %d entries, want 1 (alias excluded)", len(cmds))
	}
	got := cmds[0]
	if got.Keyword != "equipment" || got.Category != "commands" || got.Brief != "Show equipped items." {
		t.Errorf("metadata = %+v", got)
	}
	if len(got.Aliases) != 1 || got.Aliases[0] != "eq" {
		t.Errorf("aliases = %v", got.Aliases)
	}
}

func TestRegisterCommand_AliasCollisionLeavesNothingRegistered(t *testing.T) {
	t.Parallel()
	r := command.New()
	noop := func(ctx context.Context, c *command.Context) error { return nil }

	if err := r.Register("look", noop); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// "consider" is free but its alias "look" collides → whole command rejected.
	if err := r.RegisterCommand(command.Command{
		Keyword: "consider",
		Aliases: []string{"look"},
		Brief:   "x",
		Handler: noop,
	}); err == nil {
		t.Fatal("expected alias-collision error")
	}
	if r.Resolve("consider") != nil {
		t.Fatal("primary registered despite alias collision")
	}
}

func TestRegister_BareCommandNotListed(t *testing.T) {
	t.Parallel()
	r := command.New()
	noop := func(ctx context.Context, c *command.Context) error { return nil }
	if err := r.Register("xp", noop); err != nil {
		t.Fatalf("register: %v", err)
	}
	if len(r.Commands()) != 0 {
		t.Fatalf("bare Register surfaced in Commands(): %v", r.Commands())
	}
}

func TestGenerateHelpTopics_SkipsAuthored(t *testing.T) {
	t.Parallel()
	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	svc := help.NewService()
	// Authored topic for `look` must win over the generated one.
	svc.AddTopic(&help.Topic{PackName: "core", ID: "look", Title: "Look", Category: "commands", Brief: "Authored."}, 1)

	command.GenerateHelpTopics(r, svc)

	// A verb with no authored topic gets generated and is listed.
	if !svc.HasTopic("kill") {
		t.Error("expected generated topic for kill")
	}
	res := svc.Query("p1", "kill")
	if res.Topic == nil || !strings.Contains(res.Topic.Brief, "Attack") {
		t.Errorf("kill topic = %+v", res.Topic)
	}
	// Authored look brief survives (generation skipped it).
	look := svc.Query("p1", "look")
	if look.Topic == nil || look.Topic.Brief != "Authored." {
		t.Errorf("authored look was overwritten: %+v", look.Topic)
	}
	// Movement directions are not generated (registered bare).
	if svc.HasTopic("north") {
		t.Error("direction verb should not be generated")
	}
	// The admin xp probe (M19.3) now generates an admin-tier topic, hidden
	// from a non-admin (player-tier) viewer — not absent, but invisible.
	if res := svc.Query("p1", "xp"); res.Topic != nil {
		t.Error("admin xp verb should be hidden from a non-admin in help")
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
	mu         sync.Mutex
	id         string
	name       string
	playerID   string
	room       *world.Room
	lines      []string
	color      bool
	inventory  []entities.EntityID
	equipment  map[string]entities.EntityID
	footprints map[entities.EntityID][]string
	mods       map[entities.SourceKey][]stats.Modifier
	gold       int

	restState      string
	restTarget     string
	sleepStartTick uint64
	sust           int
	autoloot       bool

	craftPending crafting.PendingCraft
	hasCraft     bool

	lastArea  world.AreaID
	seenAreas map[world.AreaID]struct{}

	carryMax int // StatValue(StatCarryMax); 0 ⇒ no carry-weight limit
}

// StatValue exposes the actor's stat ceilings to handlers that read them
// (carry weight §4.2.2; the score sheet's StatValue surface). Only
// carry_max is modeled by the fake; everything else reads 0.
func (a *testActor) StatValue(s progression.StatType) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if s == progression.StatCarryMax {
		return a.carryMax
	}
	return 0
}

// PendingCraft / SetPendingCraft / ClearPendingCraft make testActor satisfy
// crafting.CraftBusy so the timed-craft path (B3) can be exercised.
func (a *testActor) PendingCraft() (crafting.PendingCraft, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.craftPending, a.hasCraft
}

func (a *testActor) SetPendingCraft(p crafting.PendingCraft) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.hasCraft {
		return false
	}
	a.craftPending, a.hasCraft = p, true
	return true
}

func (a *testActor) ClearPendingCraft() (crafting.PendingCraft, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	p, had := a.craftPending, a.hasCraft
	a.craftPending, a.hasCraft = crafting.PendingCraft{}, false
	return p, had
}

// Autoloot / SetAutoloot make testActor satisfy the inline preference
// interface AutolootHandler asserts (loot-and-corpses §6).
func (a *testActor) Autoloot() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.autoloot
}

func (a *testActor) SetAutoloot(on bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.autoloot = on
}

// Sustenance / SetSustenance make testActor satisfy
// economy.SustenanceEntity (and thus economy.Consumer) so the
// eat/drink/use verbs can be exercised.
func (a *testActor) Sustenance() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sust
}

func (a *testActor) SetSustenance(v int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sust = v
}

// RestState / SetRestState / SetRestTarget / SetSleepStart make
// testActor satisfy economy.RestEntity so the rest/sleep/wake verbs can
// be exercised.
func (a *testActor) RestState() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.restState
}

func (a *testActor) SetRestState(s string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.restState = s
}

func (a *testActor) SetRestTarget(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.restTarget = id
}

func (a *testActor) SetSleepStart(tick uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sleepStartTick = tick
}

// Gold / SetGold make testActor satisfy economy.Entity so the
// currency verb and the get/give auto-convert hook can be exercised.
func (a *testActor) Gold() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.gold
}

func (a *testActor) SetGold(v int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.gold = v
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

// AreaTransition makes testActor satisfy command.AreaTracker (the single
// atomic method the banner now calls), mirroring connActor's semantics
// over the fake's own fields. The concrete LastAreaSeen/SetLastAreaSeen/
// HasSeenArea/MarkAreaSeen below remain as test setup/assertion helpers.
func (a *testActor) AreaTransition(newArea world.AreaID) (prev world.AreaID, changed, firstEntry bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	prev = a.lastArea
	if prev == newArea {
		return prev, false, false
	}
	a.lastArea = newArea
	if a.seenAreas == nil {
		a.seenAreas = make(map[world.AreaID]struct{})
	}
	if _, seen := a.seenAreas[newArea]; seen {
		return prev, true, false
	}
	a.seenAreas[newArea] = struct{}{}
	return prev, true, true
}

// LastAreaSeen / SetLastAreaSeen make testActor satisfy
// command.AreaTracker so the area-transition zone-line (player-maps §4)
// can be exercised through the movement handlers.
func (a *testActor) LastAreaSeen() world.AreaID {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastArea
}

func (a *testActor) SetLastAreaSeen(id world.AreaID) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastArea = id
}

func (a *testActor) HasSeenArea(id world.AreaID) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.seenAreas[id]
	return ok
}

func (a *testActor) MarkAreaSeen(id world.AreaID) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.seenAreas == nil {
		a.seenAreas = make(map[world.AreaID]struct{})
	}
	a.seenAreas[id] = struct{}{}
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

func (a *testActor) Equip(footprint []string, id entities.EntityID, mods []stats.Modifier) bool {
	if len(footprint) == 0 {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	// Mirror connActor.Equip: reject if any footprint key is already taken.
	for _, k := range footprint {
		if _, taken := a.equipment[k]; taken {
			return false
		}
	}
	for i, e := range a.inventory {
		if e == id {
			a.inventory = append(a.inventory[:i], a.inventory[i+1:]...)
			if a.equipment == nil {
				a.equipment = make(map[string]entities.EntityID)
			}
			if a.footprints == nil {
				a.footprints = make(map[entities.EntityID][]string)
			}
			keys := append([]string(nil), footprint...)
			for _, k := range keys {
				a.equipment[k] = id
			}
			a.footprints[id] = keys
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
	keys := a.footprints[id]
	if len(keys) == 0 {
		keys = []string{slotKey}
	}
	for _, k := range keys {
		delete(a.equipment, k)
	}
	delete(a.footprints, id)
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

func (a *testActor) allLines() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.lines...)
}

func (a *testActor) clearLines() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lines = nil
}
