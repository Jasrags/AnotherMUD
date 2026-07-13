package command_test

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/questspawn"
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

// A description's single (soft) newlines are re-flowed to spaces so the
// prose wraps to the column instead of inheriting authored line breaks;
// a blank line is kept as a real paragraph break.
func TestRenderRoom_ReflowsSoftNewlinesKeepsParagraphs(t *testing.T) {
	r := &world.Room{ID: "ar:o", Name: "Glade",
		Description: "alpha beta\ngamma delta\n\nsecond paragraph here."}
	out := command.RenderRoom(r, nil, nil, nil, nil, nil, light.Lit, nil, nil)
	if !strings.Contains(out, "alpha beta gamma delta") {
		t.Errorf("soft newlines should join into a flowing line:\n%s", out)
	}
	if !strings.Contains(out, "\n\nsecond paragraph here.") {
		t.Errorf("a blank line should survive as a paragraph break:\n%s", out)
	}
}

// With a long authored multi-line description beside a large minimap, no
// left-column line is a lone orphan word — the reflow lets it wrap full.
func TestAppendMinimap_ReflowAvoidsOrphanWords(t *testing.T) {
	w := world.New()
	w.AddArea(&world.Area{ID: "wild", Name: "The Westwood"})
	w.AddArea(&world.Area{ID: "nb", Name: "X"})
	r := &world.Room{ID: "wild:o", AreaID: "wild", Name: "Deep Wood",
		// Authored ~76-column hard newlines, the shape that orphaned words.
		Description: "Here the wood closes in for true, the oaks giving way to\n" +
			"dark stands of pine and the light failing to a green dimness. Mushrooms cluster\n" +
			"in the leaf-mould, deadfall lies thick enough to cut, and the only sounds are\n" +
			"the wind in the high branches and something moving off through the undergrowth.",
		Terrain: "forest",
		Exits:   map[world.Direction]world.Exit{world.DirWest: {Target: "nb:x"}}}
	w.AddRoom(r)
	w.AddRoom(&world.Room{ID: "nb:x", AreaID: "nb"})

	a := &mapActor{testActor: newTestActor(r), visited: map[string]bool{"wild:o": true}, minimap: true, minimapSize: "large", termWidth: 100}
	base := command.RenderRoom(r, nil, nil, nil, nil, nil, light.Lit, nil, nil)
	out := command.AppendMinimap(base, r, a, w)

	// None of the words that previously orphaned should appear alone on a
	// left-column line (the column is everything left of the {x} reset, or
	// the whole line on a room-only row).
	for line := range strings.SplitSeq(out, "\n") {
		left := line
		if before, _, ok := strings.Cut(line, "{x}"); ok {
			left = before
		}
		if w := strings.TrimSpace(left); w == "cluster" || w == "are" {
			t.Errorf("orphan word on its own line (%q):\n%s", w, out)
		}
	}
}

func TestRenderRoom_NilPlacementAndStoreSkipsEntityLine(t *testing.T) {
	// Old contract: RenderRoom(r, nil, nil) renders geography only.
	// Pins backward-compat: tests / call sites that don't care about
	// placement can pass nil for both args without any "you see" line.
	f := newRenderFixture()
	out := command.RenderRoom(f.room, nil, nil, nil, nil, nil, light.Lit, nil, nil)
	if strings.Contains(out, "You see here") {
		t.Errorf("nil placement+store produced entity line:\n%s", out)
	}
	if !strings.Contains(out, "Town Square") {
		t.Errorf("missing room name:\n%s", out)
	}
	if !strings.Contains(out, "<subtle>Exits:</subtle> <exit>north</exit>") {
		t.Errorf("missing exits:\n%s", out)
	}
}

func TestRenderRoom_EmptyPlacementSkipsEntityLine(t *testing.T) {
	// Placement + store supplied but no entities in the room — same
	// shape as the nil case (no "You see here" line).
	f := newRenderFixture()
	out := command.RenderRoom(f.room, f.place, f.store, nil, nil, nil, light.Lit, nil, nil)
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
	out := command.RenderRoom(f.room, f.place, f.store, nil, nil, nil, light.Lit, nil, nil)
	if !strings.Contains(out, "<subtle>You see here:</subtle> <item.common>a stone well</item.common>.") {
		t.Errorf("missing item in render:\n%s", out)
	}
}

func TestRenderRoom_ColorsItemByRarity(t *testing.T) {
	// A rare item takes the item.rare tag; an unset rarity falls back
	// to item.common (covered by TestRenderRoom_ListsPlacedItem).
	// Rarity is the reserved "rarity" instance property (the same
	// source item-decorations reads), copied from the template's
	// property bag at spawn.
	f := newRenderFixture()
	f.placeItem(t, &item.Template{
		ID:         "tapestry-core:blade",
		Name:       "a glowing blade",
		Type:       "weapon",
		Properties: map[string]any{"rarity": "rare"},
	})
	out := command.RenderRoom(f.room, f.place, f.store, nil, nil, nil, light.Lit, nil, nil)
	if !strings.Contains(out, "<item.rare>a glowing blade</item.rare>") {
		t.Errorf("rare item not colored by rarity:\n%s", out)
	}
}

func TestRenderRoom_UnknownRarityFallsBackToCommon(t *testing.T) {
	// A mis-authored / custom-tier rarity must not emit an unregistered
	// tag — it falls back to item.common so the renderer's unknown-tag
	// passthrough never fires on item names.
	f := newRenderFixture()
	f.placeItem(t, &item.Template{
		ID:         "tapestry-core:odd",
		Name:       "an odd trinket",
		Type:       "trinket",
		Properties: map[string]any{"rarity": "mythic"}, // not a theme-colored tier
	})
	out := command.RenderRoom(f.room, f.place, f.store, nil, nil, nil, light.Lit, nil, nil)
	if !strings.Contains(out, "<item.common>an odd trinket</item.common>") {
		t.Errorf("unknown rarity did not fall back to common:\n%s", out)
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
	out := command.RenderRoom(f.room, f.place, f.store, nil, nil, nil, light.Lit, nil, nil)
	if !strings.Contains(out, "<subtle>You see here:</subtle> <present.mob>a village guard</present.mob>.") {
		t.Errorf("missing mob in render:\n%s", out)
	}
}

func TestRenderRoom_RedensHostileMob(t *testing.T) {
	// A hostile predicate reddens the mob (<present.hostile>); the
	// neutral default (nil predicate) is covered by ListsPlacedMob.
	f := newRenderFixture()
	f.placeMob(t, &mob.Template{
		ID: "tapestry-core:goblin", Name: "a snarling goblin", Type: "npc", Behavior: "idle",
	})
	hostile := func(*entities.MobInstance) bool { return true }
	out := command.RenderRoom(f.room, f.place, f.store, nil, nil, hostile, light.Lit, nil, nil)
	if !strings.Contains(out, "<present.hostile>a snarling goblin</present.hostile>") {
		t.Errorf("hostile mob not reddened:\n%s", out)
	}
	if strings.Contains(out, "<present.mob>a snarling goblin") {
		t.Errorf("hostile mob also rendered neutral:\n%s", out)
	}
}

func TestRenderRoom_ListsOtherPlayers(t *testing.T) {
	// Players passed via the variadic param appear in "You see here:",
	// before placed mobs/items.
	f := newRenderFixture()
	f.placeMob(t, &mob.Template{
		ID: "tapestry-core:guard", Name: "a village guard", Type: "npc", Behavior: "idle",
	})
	out := command.RenderRoom(f.room, f.place, f.store, nil, nil, nil, light.Lit, nil, nil, "Bob")
	if !strings.Contains(out, "<subtle>You see here:</subtle> <present.player>Bob</present.player>, <present.mob>a village guard</present.mob>.") {
		t.Errorf("player not listed with mob:\n%s", out)
	}
}

func TestRenderRoom_PlayersOnlyNoPlacement(t *testing.T) {
	// A player present in an otherwise-empty room (no placement/store)
	// still produces the line.
	f := newRenderFixture()
	out := command.RenderRoom(f.room, nil, nil, nil, nil, nil, light.Lit, nil, nil, "Bob", "Carol")
	if !strings.Contains(out, "<subtle>You see here:</subtle> <present.player>Bob</present.player>, <present.player>Carol</present.player>.") {
		t.Errorf("players-only render wrong:\n%s", out)
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
	out := command.RenderRoom(f.room, f.place, f.store, nil, nil, nil, light.Lit, nil, nil)
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
	out := command.RenderRoom(f.room, f.place, f.store, nil, nil, nil, light.Lit, nil, nil)
	if !strings.Contains(out, "<subtle>You see here:</subtle> <item.common>a stone well</item.common>.") {
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
	out := command.RenderRoom(f.room, f.place, f.store, nil, nil, nil, light.Lit, nil, nil)
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
	out := command.RenderRoom(f.room, f.place, f.store, nil, nil, nil, light.Lit, nil, nil)
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
	out := command.RenderRoom(f.room, f.place, f.store, marker, nil, nil, light.Lit, nil, nil)

	// The marker prepends OUTSIDE the rarity tag (sequential, not
	// nested): "<good>(!)</good> <item.common>a quest gem</item.common>".
	if !strings.Contains(out, "(!)</good> <item.common>a quest gem</item.common>") {
		t.Errorf("quest item not marked:\n%s", out)
	}
	if strings.Contains(out, "(!)</good> <item.common>a plain rock") {
		t.Errorf("non-quest item should not be marked:\n%s", out)
	}
}

// quest-spawns.md Phase 2: a quest spawn owned by another player is withheld
// from the "You see here" line (the entityVisible filter), while the viewer's
// own spawn and ordinary items still show. A nil filter (legacy) shows all.
func TestRenderRoom_QuestSpawnFilterHidesForeignSpawns(t *testing.T) {
	f := newRenderFixture()
	f.placeItem(t, &item.Template{ID: "tapestry-core:rock", Name: "a plain rock", Type: "junk"})
	mine := f.placeItem(t, &item.Template{ID: "sr:chip", Name: "a paydata chip", Type: "treasure"})
	mine.SetProperty(questspawn.OwnerProperty, "me")
	theirs := f.placeItem(t, &item.Template{ID: "sr:chip", Name: "a paydata chip", Type: "treasure"})
	theirs.SetProperty(questspawn.OwnerProperty, "them")

	// Viewer "me": sees the ordinary rock and their own chip, but exactly one
	// chip (the foreign one is gone).
	mineView := command.RenderRoom(f.room, f.place, f.store, nil, nil, nil, light.Lit, nil, command.QuestSpawnVisible("me"))
	if !strings.Contains(mineView, "a plain rock") {
		t.Errorf("ordinary item must always show:\n%s", mineView)
	}
	if got := strings.Count(mineView, "a paydata chip"); got != 1 {
		t.Errorf("owner should see exactly their own chip, got %d:\n%s", got, mineView)
	}

	// A nil filter (legacy / headless) shows both chips.
	allView := command.RenderRoom(f.room, f.place, f.store, nil, nil, nil, light.Lit, nil, nil)
	if got := strings.Count(allView, "a paydata chip"); got != 2 {
		t.Errorf("nil filter should show both chips, got %d:\n%s", got, allView)
	}
}

func TestRenderRoom_AmbienceCallbackAppendsLine(t *testing.T) {
	// The ambience callback fires once per render. A non-empty
	// return is appended on its own line BETWEEN the description
	// and the entity / exits lines so the weather line reads as
	// part of the room's atmosphere, not its inventory.
	f := newRenderFixture()
	called := 0
	ambience := func(r *world.Room) string {
		called++
		if r != f.room {
			t.Errorf("ambience called with %p, want %p", r, f.room)
		}
		return "A steady rain falls around you."
	}
	out := command.RenderRoom(f.room, f.place, f.store, nil, ambience, nil, light.Lit, nil, nil)
	if called != 1 {
		t.Errorf("ambience called %d times, want 1", called)
	}
	if !strings.Contains(out, "A steady rain falls around you.") {
		t.Errorf("ambience line missing:\n%s", out)
	}
	// Ordering: description then ambience then exits (no entities
	// in this fixture's room → no entity line). The ambience line
	// must come AFTER the description and BEFORE the exits.
	descIdx := strings.Index(out, "cobblestone")
	ambIdx := strings.Index(out, "A steady rain")
	exitsIdx := strings.Index(out, "Exits:")
	if descIdx < 0 || ambIdx < 0 || exitsIdx < 0 {
		t.Fatalf("missing landmarks: desc=%d amb=%d exits=%d\n%s",
			descIdx, ambIdx, exitsIdx, out)
	}
	if !(descIdx < ambIdx && ambIdx < exitsIdx) {
		t.Errorf("wrong order: desc=%d < amb=%d < exits=%d\n%s",
			descIdx, ambIdx, exitsIdx, out)
	}
}

func TestRenderRoom_NilAmbienceSkipsLine(t *testing.T) {
	// Backward-compat: nil ambience must produce the same output
	// as the pre-M15.4b₂b render path.
	f := newRenderFixture()
	out := command.RenderRoom(f.room, f.place, f.store, nil, nil, nil, light.Lit, nil, nil)
	for _, marker := range []string{"weather", "rain", "wind"} {
		if strings.Contains(out, marker) {
			t.Errorf("nil ambience produced %q in output:\n%s", marker, out)
		}
	}
}

func TestRenderRoom_EmptyAmbienceReturnSkipsLine(t *testing.T) {
	// A non-nil callback that returns "" is treated the same as nil:
	// no extra blank line, no marker.
	f := newRenderFixture()
	ambience := func(*world.Room) string { return "" }
	out := command.RenderRoom(f.room, f.place, f.store, nil, ambience, nil, light.Lit, nil, nil)
	// The render output joins with "\n"; an empty ambience must not
	// inject a stray blank line between description and exits. The
	// "Exits:" label now renders dimmed (<subtle>).
	if strings.Contains(out, "\n\n<subtle>Exits:") {
		t.Errorf("empty ambience produced blank line before exits:\n%q", out)
	}
}
