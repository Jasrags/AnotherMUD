package session

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/gameclock"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// roomSourceStub satisfies RoomSource over a fixed room set.
type roomSourceStub map[world.RoomID]*world.Room

func (s roomSourceStub) Room(id world.RoomID) (*world.Room, error) {
	if r, ok := s[id]; ok {
		return r, nil
	}
	return nil, world.ErrRoomNotFound
}

func lightTransitionFixture(t *testing.T, room *world.Room) (*Manager, *connActor, *fakeConn, *light.Resolver, roomSourceStub) {
	t.Helper()
	mgr := NewManager()
	a, fc := newFakeActor("c1", "p1", "acc1", "Alice", room)
	mgr.Add(a)
	res := light.NewResolver(light.DefaultConfig(), nil) // period passed explicitly
	return mgr, a, fc, res, roomSourceStub{room.ID: room}
}

func TestLightTransitions_DarkenOnNightfall(t *testing.T) {
	room := &world.Room{ID: "x:road", Name: "Road", Terrain: world.TerrainOutdoors}
	mgr, _, fc, res, rooms := lightTransitionFixture(t, room)

	// day (lit) → night (gloom): a crossing → darken.
	mgr.LightTransitions(context.Background(), res, rooms, entities.NewStore(), entities.NewPlacement(),
		gameclock.PeriodDay, gameclock.PeriodNight)

	got := fc.writes()
	if len(got) != 1 || !strings.Contains(got[0], "shadows close in") {
		t.Fatalf("expected one darken message, got %v", got)
	}
}

func TestLightTransitions_BrightenAtDawn(t *testing.T) {
	room := &world.Room{ID: "x:road", Name: "Road", Terrain: world.TerrainOutdoors}
	mgr, _, fc, res, rooms := lightTransitionFixture(t, room)

	mgr.LightTransitions(context.Background(), res, rooms, entities.NewStore(), entities.NewPlacement(),
		gameclock.PeriodNight, gameclock.PeriodDay)

	got := fc.writes()
	if len(got) != 1 || !strings.Contains(got[0], "world brightens") {
		t.Fatalf("expected one brighten message, got %v", got)
	}
}

func TestLightTransitions_NoCrossIsSilent(t *testing.T) {
	// A room pinned `lit` never changes level across a period boundary.
	room := &world.Room{ID: "x:hall", Name: "Glowing Hall", Terrain: world.TerrainOutdoors,
		Properties: map[string]any{light.PropRoomLight: "lit"}}
	mgr, _, fc, res, rooms := lightTransitionFixture(t, room)

	mgr.LightTransitions(context.Background(), res, rooms, entities.NewStore(), entities.NewPlacement(),
		gameclock.PeriodDay, gameclock.PeriodNight)

	if got := fc.writes(); len(got) != 0 {
		t.Fatalf("pinned-lit room should emit no transition, got %v", got)
	}
}

func TestLightTransitions_UndergroundSilent(t *testing.T) {
	// Underground is black at every period — no period change crosses.
	room := &world.Room{ID: "x:cave", Name: "Cave", Terrain: world.TerrainUnderground}
	mgr, _, fc, res, rooms := lightTransitionFixture(t, room)

	mgr.LightTransitions(context.Background(), res, rooms, entities.NewStore(), entities.NewPlacement(),
		gameclock.PeriodDay, gameclock.PeriodNight)

	if got := fc.writes(); len(got) != 0 {
		t.Fatalf("underground room should emit no transition, got %v", got)
	}
}

func TestLightTransitions_SamePeriodNoop(t *testing.T) {
	room := &world.Room{ID: "x:road", Name: "Road", Terrain: world.TerrainOutdoors}
	mgr, _, fc, res, rooms := lightTransitionFixture(t, room)

	mgr.LightTransitions(context.Background(), res, rooms, entities.NewStore(), entities.NewPlacement(),
		gameclock.PeriodDay, gameclock.PeriodDay)

	if got := fc.writes(); len(got) != 0 {
		t.Fatalf("same-period call should be a no-op, got %v", got)
	}
}

func TestLightTransitions_NilResolverNoop(t *testing.T) {
	room := &world.Room{ID: "x:road", Name: "Road", Terrain: world.TerrainOutdoors}
	mgr, _, fc, _, rooms := lightTransitionFixture(t, room)

	mgr.LightTransitions(context.Background(), nil, rooms, entities.NewStore(), entities.NewPlacement(),
		gameclock.PeriodDay, gameclock.PeriodNight)

	if got := fc.writes(); len(got) != 0 {
		t.Fatalf("nil resolver should be a no-op, got %v", got)
	}
}
