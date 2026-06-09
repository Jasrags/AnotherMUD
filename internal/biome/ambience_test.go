package biome

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

type fakeRooms struct{ rooms []*world.Room }

func (f fakeRooms) Rooms() []*world.Room { return f.rooms }

type recBroadcast struct{ sent map[world.RoomID]string }

func (r *recBroadcast) SendToRoom(_ context.Context, id world.RoomID, text string, _ ...string) {
	if r.sent == nil {
		r.sent = map[world.RoomID]string{}
	}
	r.sent[id] = text
}

// fixedRoller returns a constant index (clamped) so a chosen line is
// deterministic.
type fixedRoller struct{ n int }

func (r fixedRoller) IntN(max int) int {
	if r.n < max {
		return r.n
	}
	return 0
}

func ambienceFixture(t *testing.T) (*Registry, *recBroadcast) {
	t.Helper()
	r := NewRegistry()
	if err := RegisterEngineBaseline(r); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	// A forest biome with an ambience pool; outdoors (baseline) has none.
	if err := r.RegisterEngine(&Biome{ID: "forest", Ambience: []string{"A bird trills.", "Leaves rustle."}}); err != nil {
		t.Fatalf("register forest: %v", err)
	}
	return r, &recBroadcast{}
}

func TestAmbience_DeliversToOccupiedBiomeRoom(t *testing.T) {
	reg, bc := ambienceFixture(t)
	rooms := fakeRooms{rooms: []*world.Room{
		{ID: "a", Terrain: "forest"},  // forest + occupied → line
		{ID: "b", Terrain: "forest"},  // forest + EMPTY → skipped
		{ID: "c", Terrain: "outdoors"}, // baseline, no pool → skipped
		{ID: "d", Terrain: "void"},     // unregistered → skipped
	}}
	occupied := func(id world.RoomID) bool { return id == "a" }
	svc := NewAmbienceService(reg, rooms, occupied, bc, fixedRoller{n: 1})
	svc.Tick(context.Background())

	if got := bc.sent["a"]; got != "Leaves rustle." {
		t.Errorf("room a got %q, want the index-1 forest line", got)
	}
	if _, ok := bc.sent["b"]; ok {
		t.Error("empty room b should receive no ambience")
	}
	if _, ok := bc.sent["c"]; ok {
		t.Error("outdoors (no pool) should receive no ambience")
	}
	if _, ok := bc.sent["d"]; ok {
		t.Error("unregistered terrain should receive no ambience")
	}
}

func TestAmbience_NilOccupiedFuncFlavorsAll(t *testing.T) {
	reg, bc := ambienceFixture(t)
	rooms := fakeRooms{rooms: []*world.Room{{ID: "a", Terrain: "forest"}}}
	// nil occupied func → treat every room as occupied (delivery still
	// no-ops on truly empty rooms at the real broadcaster).
	svc := NewAmbienceService(reg, rooms, nil, bc, fixedRoller{n: 0})
	svc.Tick(context.Background())
	if bc.sent["a"] != "A bird trills." {
		t.Errorf("room a got %q, want the index-0 line", bc.sent["a"])
	}
}

func TestAmbience_NilDepsNoPanic(t *testing.T) {
	// A service missing any required dep is a silent no-op.
	(&AmbienceService{}).Tick(context.Background())
	NewAmbienceService(nil, nil, nil, nil, nil).Tick(context.Background())
}
