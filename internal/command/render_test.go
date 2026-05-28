package command_test

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// renderFixture builds a tiny world + entity store + placement so
// renderer tests can drive the new RenderRoom contract end-to-end.
type renderFixture struct {
	room  *world.Room
	store *entities.Store
	place *entities.Placement
}

func newRenderFixture() *renderFixture {
	return &renderFixture{
		room: &world.Room{
			ID:          "tapestry-core:square",
			Name:        "Town Square",
			Description: "A worn cobblestone plaza.",
			Exits: map[world.Direction]world.Exit{
				world.DirNorth: {Target: "tapestry-core:forge"},
			},
		},
		store: entities.NewStore(),
		place: entities.NewPlacement(),
	}
}

func (f *renderFixture) placeItem(t *testing.T, tpl *item.Template) *entities.ItemInstance {
	t.Helper()
	inst, err := f.store.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	f.place.Place(inst.ID(), f.room.ID)
	return inst
}

func (f *renderFixture) placeMob(t *testing.T, tpl *mob.Template) *entities.MobInstance {
	t.Helper()
	inst, err := f.store.SpawnMob(tpl)
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	f.place.Place(inst.ID(), f.room.ID)
	return inst
}

func TestRenderRoom_NilPlacementAndStoreSkipsEntityLine(t *testing.T) {
	// Old contract: RenderRoom(r, nil, nil) renders geography only.
	// Pins backward-compat: tests / call sites that don't care about
	// placement can pass nil for both args without any "you see" line.
	f := newRenderFixture()
	out := command.RenderRoom(f.room, nil, nil, nil)
	if strings.Contains(out, "You see here") {
		t.Errorf("nil placement+store produced entity line:\n%s", out)
	}
	if !strings.Contains(out, "Town Square") {
		t.Errorf("missing room name:\n%s", out)
	}
	if !strings.Contains(out, "Exits: north") {
		t.Errorf("missing exits:\n%s", out)
	}
}

func TestRenderRoom_EmptyPlacementSkipsEntityLine(t *testing.T) {
	// Placement + store supplied but no entities in the room — same
	// shape as the nil case (no "You see here" line).
	f := newRenderFixture()
	out := command.RenderRoom(f.room, f.place, f.store, nil)
	if strings.Contains(out, "You see here") {
		t.Errorf("empty room produced entity line:\n%s", out)
	}
}

func TestRenderRoom_ListsPlacedItem(t *testing.T) {
	f := newRenderFixture()
	f.placeItem(t, &item.Template{
		ID:   "tapestry-core:well",
		Name: "a stone well",
		Type: "fixture",
	})
	out := command.RenderRoom(f.room, f.place, f.store, nil)
	if !strings.Contains(out, "You see here: a stone well.") {
		t.Errorf("missing item in render:\n%s", out)
	}
}

func TestRenderRoom_ListsPlacedMob(t *testing.T) {
	f := newRenderFixture()
	f.placeMob(t, &mob.Template{
		ID:       "tapestry-core:guard",
		Name:     "a village guard",
		Type:     "npc",
		Behavior: "idle",
	})
	out := command.RenderRoom(f.room, f.place, f.store, nil)
	if !strings.Contains(out, "You see here: a village guard.") {
		t.Errorf("missing mob in render:\n%s", out)
	}
}

func TestRenderRoom_PreservesInsertionOrderAcrossMixedEntities(t *testing.T) {
	// Spec: Placement order is preservation-of-arrival. The renderer
	// must not reorder by entity kind — items and mobs render in the
	// order they were placed. This fixture places the well first,
	// the guard second; cross-pack loader ordering is a separate
	// concern (the loader places items before mobs today, but that's
	// not what's under test here).
	f := newRenderFixture()
	f.placeItem(t, &item.Template{ID: "tapestry-core:well", Name: "a stone well", Type: "fixture"})
	f.placeMob(t, &mob.Template{ID: "tapestry-core:guard", Name: "a village guard", Type: "npc", Behavior: "idle"})
	out := command.RenderRoom(f.room, f.place, f.store, nil)
	idxWell := strings.Index(out, "a stone well")
	idxGuard := strings.Index(out, "a village guard")
	if idxWell == -1 || idxGuard == -1 {
		t.Fatalf("missing entities in render:\n%s", out)
	}
	if idxWell > idxGuard {
		t.Errorf("rendering reordered placement-insertion sequence:\n%s", out)
	}
}

// TestRenderRoom_EmptyNameEntitySilentlySkipped exercises the
// defensive branch in renderRoomEntities that drops entities whose
// Name() returns "". Production templates always have a non-empty
// name, but the guard exists so a future tooling bug (corrupted
// instance, partial load) doesn't produce "You see here: , a guard."
// type output. Pinning it here prevents the guard from being
// accidentally removed in a future refactor.
func TestRenderRoom_EmptyNameEntitySilentlySkipped(t *testing.T) {
	f := newRenderFixture()
	// One real item to keep the "You see here:" line present so we
	// can verify the empty-name entity is omitted from it (rather
	// than the whole line being absent for some other reason).
	f.placeItem(t, &item.Template{ID: "tapestry-core:well", Name: "a stone well", Type: "fixture"})
	f.placeItem(t, &item.Template{ID: "tapestry-core:nameless", Name: "", Type: "fixture"})
	out := command.RenderRoom(f.room, f.place, f.store, nil)
	if !strings.Contains(out, "You see here: a stone well.") {
		t.Errorf("expected named entity intact, empty-name entity omitted:\n%s", out)
	}
	// Belt-and-braces: no stray comma from a blank-name list entry.
	if strings.Contains(out, ", .") || strings.Contains(out, ",  ") {
		t.Errorf("entity line contains a stray empty-name slot:\n%s", out)
	}
}

func TestRenderRoom_EntityLinePlacedBetweenDescriptionAndExits(t *testing.T) {
	// Reading-order pins: description, then entities, then exits.
	// Future renderer rewrites must not invert this without updating
	// the test.
	f := newRenderFixture()
	f.placeItem(t, &item.Template{ID: "tapestry-core:well", Name: "a stone well", Type: "fixture"})
	out := command.RenderRoom(f.room, f.place, f.store, nil)
	idxDesc := strings.Index(out, "cobblestone")
	idxWell := strings.Index(out, "a stone well")
	idxExits := strings.Index(out, "Exits:")
	if idxDesc == -1 || idxWell == -1 || idxExits == -1 {
		t.Fatalf("missing sections in render:\n%s", out)
	}
	if !(idxDesc < idxWell && idxWell < idxExits) {
		t.Errorf("section order wrong (desc=%d well=%d exits=%d):\n%s", idxDesc, idxWell, idxExits, out)
	}
}

// TestRenderRoom_UnresolvedPlacementIDSilentlySkipped covers the
// defensive branch in renderRoomEntities: an id in Placement that's
// missing from the store does not panic or surface — it's quietly
// omitted. (Pre-condition shouldn't happen in production, but the
// render path is player-visible and must not break on stale state.)
func TestRenderRoom_UnresolvedPlacementIDSilentlySkipped(t *testing.T) {
	f := newRenderFixture()
	f.place.Place(entities.EntityID("ghost-id"), f.room.ID)
	out := command.RenderRoom(f.room, f.place, f.store, nil)
	// Ghost id resolves to nothing; line should be absent OR not
	// mention any entity name.
	if strings.Contains(out, "You see here") {
		t.Errorf("ghost id produced a visible entity line:\n%s", out)
	}
}

func TestRenderRoom_MarkerDecoratesEntity(t *testing.T) {
	f := newRenderFixture()
	f.placeItem(t, &item.Template{ID: "tapestry-core:gem", Name: "a quest gem", Type: "treasure"})
	f.placeItem(t, &item.Template{ID: "tapestry-core:rock", Name: "a plain rock", Type: "junk"})

	// marker fires only for the gem template.
	marker := func(tid string) bool { return tid == "tapestry-core:gem" }
	out := command.RenderRoom(f.room, f.place, f.store, marker)

	if !strings.Contains(out, "(!)</good> a quest gem") {
		t.Errorf("quest item not marked:\n%s", out)
	}
	if strings.Contains(out, "(!)</good> a plain rock") {
		t.Errorf("non-quest item should not be marked:\n%s", out)
	}
}
