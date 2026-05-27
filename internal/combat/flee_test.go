package combat

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// recordingFleeBus collects every flee event for assertion. All
// three event kinds land in separate slices so tests can assert
// "exactly one prevented, zero others" without slice-of-any
// gymnastics.
type recordingFleeBus struct {
	flees     []fleeEv
	prevented []fleePreventedEv
	failed    []fleeFailedEv
}

type fleeEv struct {
	id        CombatantID
	name      string
	from, to  world.RoomID
	direction string
}

type fleePreventedEv struct {
	id   CombatantID
	name string
	room world.RoomID
}

type fleeFailedEv struct {
	id     CombatantID
	name   string
	room   world.RoomID
	reason string
}

func (b *recordingFleeBus) EmitFlee(_ context.Context, id CombatantID, name string, from, to world.RoomID, dir string) {
	b.flees = append(b.flees, fleeEv{id, name, from, to, dir})
}

func (b *recordingFleeBus) EmitFleePrevented(_ context.Context, id CombatantID, name string, room world.RoomID) {
	b.prevented = append(b.prevented, fleePreventedEv{id, name, room})
}

func (b *recordingFleeBus) EmitFleeFailed(_ context.Context, id CombatantID, name string, room world.RoomID, reason string) {
	b.failed = append(b.failed, fleeFailedEv{id, name, room, reason})
}

// mapRooms is the test-side RoomSource. Keys are room ids.
type mapRooms map[world.RoomID]*world.Room

func (m mapRooms) Room(id world.RoomID) (*world.Room, error) {
	if r, ok := m[id]; ok {
		return r, nil
	}
	return nil, world.ErrRoomNotFound
}

// mapRoomLocator is the test-side RoomLocator: combatant id → room.
type mapRoomLocator map[CombatantID]world.RoomID

func (m mapRoomLocator) RoomOf(id CombatantID) (world.RoomID, bool) {
	r, ok := m[id]
	return r, ok
}

// recordingMover captures every Move call so tests can assert
// "moved to the right room" without needing a real placement store.
type recordingMover struct {
	moves []moveCall
	fail  bool
}

type moveCall struct {
	id  CombatantID
	dst world.RoomID
}

func (m *recordingMover) Move(_ context.Context, id CombatantID, dst *world.Room) bool {
	if m.fail {
		return false
	}
	m.moves = append(m.moves, moveCall{id, dst.ID})
	return true
}

const fromRoom world.RoomID = "tapestry-core:town-square"
const toRoom world.RoomID = "tapestry-core:market"

func buildFleeRig(t *testing.T) (FleeConfig, *recordingFleeBus, *recordingMover, mapRoomLocator) {
	t.Helper()
	rooms := mapRooms{
		fromRoom: {
			ID: fromRoom,
			Exits: map[world.Direction]world.Exit{
				world.DirNorth: {Target: toRoom},
			},
		},
		toRoom: {ID: toRoom},
	}
	roomLoc := mapRoomLocator{}
	locator := MapLocator{}
	bus := &recordingFleeBus{}
	mover := &recordingMover{}
	mgr := NewManagerWith(ManagerConfig{Locator: locator})

	return FleeConfig{
		Mgr:           mgr,
		Locator:       locator,
		RoomLocator:   roomLoc,
		Rooms:         rooms,
		Mover:         mover,
		Bus:           bus,
		Cooldowns:     NewFleeCooldowns(),
		CooldownTicks: 30,
	}, bus, mover, roomLoc
}

func TestFleeSuccessPath(t *testing.T) {
	cfg, bus, mover, roomLoc := buildFleeRig(t)
	id := NewMobCombatantID("rat")
	cfg.Locator.(MapLocator)[id] = staticCombatant{id: id, name: "a rat"}
	roomLoc[id] = fromRoom

	outcome := Flee(context.Background(), id, cfg)
	if outcome != FleeOutcomeSuccess {
		t.Fatalf("outcome = %v, want FleeOutcomeSuccess", outcome)
	}
	if len(mover.moves) != 1 || mover.moves[0].dst != toRoom {
		t.Errorf("Move calls = %+v, want one to %s", mover.moves, toRoom)
	}
	if len(bus.flees) != 1 {
		t.Fatalf("flee events = %d, want 1", len(bus.flees))
	}
	if bus.flees[0].from != fromRoom || bus.flees[0].to != toRoom || bus.flees[0].direction != "north" {
		t.Errorf("flee event = %+v", bus.flees[0])
	}
	if !cfg.Cooldowns.Active(id) {
		t.Error("cooldown not set after successful flee")
	}
}

func TestFleeNoFleeTagPrevents(t *testing.T) {
	cfg, bus, mover, roomLoc := buildFleeRig(t)
	id := NewMobCombatantID("guard")
	cfg.Locator.(MapLocator)[id] = staticCombatant{id: id, name: "a stoic guard"}
	roomLoc[id] = fromRoom
	tags := newMapTagSource()
	tags.tagEntity(id, TagNoFlee)
	cfg.Tags = tags

	outcome := Flee(context.Background(), id, cfg)
	if outcome != FleeOutcomePrevented {
		t.Fatalf("outcome = %v, want FleeOutcomePrevented", outcome)
	}
	if len(bus.prevented) != 1 {
		t.Errorf("flee_prevented events = %d, want 1", len(bus.prevented))
	}
	if len(mover.moves) != 0 {
		t.Error("Move called despite no-flee tag")
	}
	if cfg.Cooldowns.Active(id) {
		t.Error("cooldown set on prevented flee")
	}
}

func TestFleeFailsNoExits(t *testing.T) {
	cfg, bus, mover, roomLoc := buildFleeRig(t)
	// Replace fromRoom with an exit-less variant.
	cfg.Rooms = mapRooms{
		fromRoom: {ID: fromRoom},
	}
	id := NewMobCombatantID("rat")
	cfg.Locator.(MapLocator)[id] = staticCombatant{id: id, name: "a rat"}
	roomLoc[id] = fromRoom

	outcome := Flee(context.Background(), id, cfg)
	if outcome != FleeOutcomeFailedNoExits {
		t.Fatalf("outcome = %v, want FleeOutcomeFailedNoExits", outcome)
	}
	if len(bus.failed) != 1 || bus.failed[0].reason != FleeReasonNoExits {
		t.Errorf("flee_failed events = %+v, want one no-exits", bus.failed)
	}
	if len(mover.moves) != 0 {
		t.Error("Move called despite no exits")
	}
}

func TestFleeFailsUnknownRoom(t *testing.T) {
	cfg, bus, _, roomLoc := buildFleeRig(t)
	id := NewMobCombatantID("rat")
	cfg.Locator.(MapLocator)[id] = staticCombatant{id: id, name: "a rat"}
	// Combatant's RoomOf reports a room the RoomSource doesn't know.
	roomLoc[id] = world.RoomID("nowhere")

	outcome := Flee(context.Background(), id, cfg)
	if outcome != FleeOutcomeFailedUnknownRoom {
		t.Fatalf("outcome = %v, want FleeOutcomeFailedUnknownRoom", outcome)
	}
	if len(bus.failed) != 1 || bus.failed[0].reason != FleeReasonUnknownRoom {
		t.Errorf("flee_failed events = %+v, want one unknown-room", bus.failed)
	}
}

// Multi-exit rooms must pick uniformly via the supplied Roller. We
// verify the call passes len(dirs) to IntN and picks the returned
// index.
func TestFleeMultiExitUsesRoller(t *testing.T) {
	cfg, bus, mover, roomLoc := buildFleeRig(t)
	cfg.Rooms = mapRooms{
		fromRoom: {
			ID: fromRoom,
			Exits: map[world.Direction]world.Exit{
				world.DirNorth: {Target: toRoom},
				world.DirEast:  {Target: "tapestry-core:forge"},
				world.DirWest:  {Target: "tapestry-core:gate"},
			},
		},
		toRoom:                       {ID: toRoom},
		"tapestry-core:forge":        {ID: "tapestry-core:forge"},
		"tapestry-core:gate":         {ID: "tapestry-core:gate"},
	}
	id := NewMobCombatantID("rat")
	cfg.Locator.(MapLocator)[id] = staticCombatant{id: id, name: "rat"}
	roomLoc[id] = fromRoom
	// Sorted by direction name: east, north, west → idx 1 = "north".
	cfg.Rand = &fixedRoller{t: t, values: []int{1}}

	if outcome := Flee(context.Background(), id, cfg); outcome != FleeOutcomeSuccess {
		t.Fatalf("outcome = %v, want success", outcome)
	}
	if len(mover.moves) != 1 || mover.moves[0].dst != toRoom {
		t.Errorf("Move = %+v, want to %s", mover.moves, toRoom)
	}
	if bus.flees[0].direction != "north" {
		t.Errorf("direction = %s, want north", bus.flees[0].direction)
	}
}

// DisengageAll must run before Move so CombatEnded events carry the
// from-room, not the post-move to-room.
func TestFleeDisengagesBeforeMove(t *testing.T) {
	cfg, _, _, roomLoc := buildFleeRig(t)
	sink := &recordingSink{}
	cfg.Mgr = NewManagerWith(ManagerConfig{Locator: cfg.Locator, Sink: sink})

	fleer := NewMobCombatantID("rat")
	other := NewPlayerCombatantID("hero")
	cfg.Locator.(MapLocator)[fleer] = staticCombatant{id: fleer, name: "rat"}
	cfg.Locator.(MapLocator)[other] = staticCombatant{id: other, name: "Hero"}
	roomLoc[fleer] = fromRoom
	roomLoc[other] = fromRoom
	cfg.Mgr.Engage(context.Background(), fleer, other, fromRoom)
	// Reset the sink's ended slice so we only see CombatEnded from
	// the flee path.
	sink.mu.Lock()
	sink.ended = nil
	sink.mu.Unlock()

	if Flee(context.Background(), fleer, cfg) != FleeOutcomeSuccess {
		t.Fatal("flee did not succeed")
	}
	if cfg.Mgr.InCombat(fleer) || cfg.Mgr.InCombat(other) {
		t.Error("DisengageAll did not clear both sides")
	}
	if sink.endedCount() < 2 {
		t.Errorf("CombatEnded count = %d, want at least 2 (fleer + opponent)", sink.endedCount())
	}
}
