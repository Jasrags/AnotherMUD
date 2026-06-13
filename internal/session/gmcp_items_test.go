package session

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// newItemsGmcpActor builds a connActor wired with a fresh entity
// store so the flusher can resolve EntityID → Name. The Vitals/
// Sustenance plumbing is inherited via the existing helpers; this
// extends with a real items store.
func newItemsGmcpActor(t *testing.T, playerID string) (*connActor, *gmcpFakeConn, *entities.Store) {
	t.Helper()
	fc := &gmcpFakeConn{fakeConn: fakeConn{id: "test-" + playerID}}
	store := entities.NewStore()
	room := &world.Room{ID: "test-room", Name: "Test"}
	a := &connActor{
		id:         fc.id,
		conn:       fc,
		playerID:   playerID,
		room:       room,
		items:      store,
		equipment:  make(map[string]entities.EntityID),
		footprints: make(map[entities.EntityID][]string),
		vitals:     combat.NewVitalsAt(50, 100),
		save:       &player.Save{ID: playerID, Name: playerID, Sustenance: 100},
	}
	a.sustenance = 100
	return a, fc, store
}

// spawnItem creates an item in the store and returns its instance.
func spawnItem(t *testing.T, store *entities.Store, id, name string) *entities.ItemInstance {
	t.Helper()
	inst, err := store.Spawn(&item.Template{
		ID:   item.TemplateID(id),
		Name: name,
		Type: "trinket",
	})
	if err != nil {
		t.Fatalf("Spawn(%s): %v", id, err)
	}
	return inst
}

// itemListFrames returns the subset of the fake conn's frames that
// match the Char.Items.List package, decoded into typed values.
func itemListFrames(t *testing.T, fc *gmcpFakeConn) []gmcp.CharItemsList {
	t.Helper()
	raw := fc.framesSnapshot()
	out := make([]gmcp.CharItemsList, 0, len(raw))
	for _, f := range raw {
		if f.pkg != gmcp.PackageCharItemsList {
			continue
		}
		var lst gmcp.CharItemsList
		if err := json.Unmarshal(f.payload, &lst); err != nil {
			t.Fatalf("payload unmarshal: %v (raw %s)", err, f.payload)
		}
		out = append(out, lst)
	}
	return out
}

func TestFlushGmcpItems_NoSendBeforeActivation(t *testing.T) {
	a, fc, _ := newItemsGmcpActor(t, "p-1")
	// active=false
	a.flushGmcpItems(context.Background())
	if got := len(fc.framesSnapshot()); got != 0 {
		t.Errorf("pre-activation emitted %d frames, want 0", got)
	}
}

func TestFlushGmcpItems_FirstFlushSendsBothLocationsEvenWhenEmpty(t *testing.T) {
	// Empty inv + empty equipment: the first active flush must
	// still emit one frame per location with an empty Items array
	// so the panel knows to render empty tiles.
	a, fc, _ := newItemsGmcpActor(t, "p-1")
	fc.setActive(true)

	a.flushGmcpItems(context.Background())

	frames := itemListFrames(t, fc)
	if len(frames) != 2 {
		t.Fatalf("first flush sent %d list frames, want 2", len(frames))
	}
	locs := map[string]bool{}
	for _, f := range frames {
		locs[f.Location] = true
		if len(f.Items) != 0 {
			t.Errorf("frame for %q had %d items, want 0", f.Location, len(f.Items))
		}
	}
	if !locs[gmcp.LocationInventory] || !locs[gmcp.LocationWear] {
		t.Errorf("missing location in first-flush set: %v", locs)
	}
}

func TestFlushGmcpItems_InventoryAddSendsOnlyInvFrame(t *testing.T) {
	a, fc, store := newItemsGmcpActor(t, "p-1")
	fc.setActive(true)
	a.flushGmcpItems(context.Background()) // baseline (two empty frames)

	inst := spawnItem(t, store, "tpl:sword", "a short sword")
	a.AddToInventory(inst.ID())
	a.flushGmcpItems(context.Background())

	frames := itemListFrames(t, fc)
	if len(frames) != 3 {
		t.Fatalf("post-add total frames = %d, want 3 (2 baseline + 1 inv update)", len(frames))
	}
	// The third frame must be inventory and contain the new item.
	last := frames[len(frames)-1]
	if last.Location != gmcp.LocationInventory {
		t.Errorf("delta location = %q, want %q", last.Location, gmcp.LocationInventory)
	}
	if len(last.Items) != 1 || last.Items[0].Name != "a short sword" {
		t.Errorf("delta items = %+v", last.Items)
	}
}

func TestFlushGmcpItems_EquipSendsOnlyWearFrame(t *testing.T) {
	a, fc, store := newItemsGmcpActor(t, "p-1")
	fc.setActive(true)
	a.flushGmcpItems(context.Background()) // baseline

	inst := spawnItem(t, store, "tpl:cap", "a leather cap")
	a.AddToInventory(inst.ID())
	a.flushGmcpItems(context.Background()) // inv-only update

	// Now equip — moves the item from inv to wear. Mutate the
	// underlying fields directly (we're in the session package);
	// going through a.Equip would drag in stat-block plumbing
	// that's not relevant to the GMCP behavior under test.
	a.mu.Lock()
	a.inventory = a.inventory[:0]
	a.equipment["head"] = inst.ID()
	a.mu.Unlock()
	a.flushGmcpItems(context.Background())

	frames := itemListFrames(t, fc)
	// 2 baseline + 1 inv-add + 2 (inv-shrink + wear-grow) = 5
	if len(frames) != 5 {
		t.Fatalf("total frames = %d, want 5", len(frames))
	}
	last2 := frames[len(frames)-2:]
	locs := map[string][]gmcp.CharItem{}
	for _, f := range last2 {
		locs[f.Location] = f.Items
	}
	if invItems, ok := locs[gmcp.LocationInventory]; !ok || len(invItems) != 0 {
		t.Errorf("post-equip inv frame missing/non-empty: %+v", locs)
	}
	if wearItems, ok := locs[gmcp.LocationWear]; !ok || len(wearItems) != 1 || wearItems[0].Name != "a leather cap" {
		t.Errorf("post-equip wear frame wrong: %+v", locs)
	}
}

func TestFlushGmcpItems_NoRedundantSendWhenUnchanged(t *testing.T) {
	a, fc, store := newItemsGmcpActor(t, "p-1")
	fc.setActive(true)
	inst := spawnItem(t, store, "tpl:x", "a thing")
	a.AddToInventory(inst.ID())

	a.flushGmcpItems(context.Background()) // baseline (both locations)
	preCount := len(fc.framesSnapshot())

	a.flushGmcpItems(context.Background()) // nothing changed
	a.flushGmcpItems(context.Background())

	if got := len(fc.framesSnapshot()); got != preCount {
		t.Errorf("redundant flushes added %d frames, want 0", got-preCount)
	}
}

func TestFlushGmcpItems_ShadowResetForcesResend(t *testing.T) {
	a, fc, _ := newItemsGmcpActor(t, "p-1")
	fc.setActive(true)
	a.flushGmcpItems(context.Background()) // baseline
	preCount := len(fc.framesSnapshot())

	a.resetGmcpItemsShadow()
	a.flushGmcpItems(context.Background())

	if got := len(fc.framesSnapshot()) - preCount; got != 2 {
		t.Errorf("post-reset added %d frames, want 2 (both locations)", got)
	}
}

func TestFlushGmcpItems_NonGmcpConnIsSilent(t *testing.T) {
	store := entities.NewStore()
	room := &world.Room{ID: "r", Name: "R"}
	a := &connActor{
		id:         "x",
		conn:       &fakeConn{id: "x"},
		playerID:   "p-x",
		room:       room,
		items:      store,
		equipment:  make(map[string]entities.EntityID),
		footprints: make(map[entities.EntityID][]string),
		vitals:     combat.NewVitalsAt(50, 100),
		save:       &player.Save{ID: "p-x", Sustenance: 100},
	}
	a.sustenance = 100
	// No panic, no writes.
	a.flushGmcpItems(context.Background())
}

func TestSnapshotItemsSortedByID(t *testing.T) {
	// Stable iteration: equipment map ranges in random order, but
	// the snapshot must come out sorted so the diff compare is
	// deterministic. Spawn three items, equip in arbitrary order,
	// inspect the snapshot.
	a, _, store := newItemsGmcpActor(t, "p-1")
	insts := make([]*entities.ItemInstance, 3)
	insts[0] = spawnItem(t, store, "t:a", "alpha")
	insts[1] = spawnItem(t, store, "t:b", "beta")
	insts[2] = spawnItem(t, store, "t:c", "gamma")
	a.mu.Lock()
	for _, inst := range insts {
		a.equipment["slot-"+string(inst.ID())] = inst.ID()
	}
	a.mu.Unlock()

	wear := a.snapshotItemsForEquipment()
	if len(wear) != 3 {
		t.Fatalf("snapshot len = %d, want 3", len(wear))
	}
	for i := 1; i < len(wear); i++ {
		if wear[i-1].ID >= wear[i].ID {
			t.Errorf("snapshot not sorted: %+v", wear)
			break
		}
	}
}

func TestCharItemsEqual(t *testing.T) {
	a := []gmcp.CharItem{{ID: "1", Name: "x"}, {ID: "2", Name: "y"}}
	b := []gmcp.CharItem{{ID: "1", Name: "x"}, {ID: "2", Name: "y"}}
	c := []gmcp.CharItem{{ID: "1", Name: "x"}, {ID: "2", Name: "DIFFERENT"}}
	d := []gmcp.CharItem{{ID: "1", Name: "x"}}

	if !charItemsEqual(a, b) {
		t.Error("identical slices should be equal")
	}
	if charItemsEqual(a, c) {
		t.Error("name diff should not be equal")
	}
	if charItemsEqual(a, d) {
		t.Error("length diff should not be equal")
	}
	if !charItemsEqual(nil, nil) || !charItemsEqual(nil, []gmcp.CharItem{}) {
		t.Error("nil/empty should be equal")
	}
}

func TestManagerFlushGmcpItems_FansOutToLiveActors(t *testing.T) {
	mgr := NewManager()
	a1, fc1, _ := newItemsGmcpActor(t, "p-1")
	a2, fc2, _ := newItemsGmcpActor(t, "p-2")
	fc1.setActive(true)
	fc2.setActive(true)
	mgr.Add(a1)
	mgr.Add(a2)

	mgr.FlushGmcpItems(context.Background())

	if got := len(itemListFrames(t, fc1)); got != 2 {
		t.Errorf("a1 received %d list frames, want 2", got)
	}
	if got := len(itemListFrames(t, fc2)); got != 2 {
		t.Errorf("a2 received %d list frames, want 2", got)
	}
}

// Pin: a swap-then-send-via-flush works the same way as the
// vitals path. The link-dead reattach hook calls
// resetGmcpItemsShadow + then the gmcp-items-flush tick handler
// picks up the change. Mirrors the M16.4b reattach test shape.
func TestFlushGmcpItems_LandsOnSwappedConnAfterReset(t *testing.T) {
	a, oldFC, _ := newItemsGmcpActor(t, "p-1")
	oldFC.setActive(true)
	a.flushGmcpItems(context.Background()) // baseline on old conn

	// Swap (what reattach does internally) + reset shadow (what
	// the reattach hook calls).
	newFC := &gmcpFakeConn{fakeConn: fakeConn{id: "test-new"}}
	newFC.setActive(true)
	a.mu.Lock()
	a.conn = newFC
	a.mu.Unlock()
	a.resetGmcpItemsShadow()

	a.flushGmcpItems(context.Background())

	if got := len(itemListFrames(t, newFC)); got != 2 {
		t.Errorf("new conn received %d frames after reset, want 2", got)
	}
}

func TestFlushGmcpItems_PayloadContainsExpectedShape(t *testing.T) {
	// End-to-end shape pin: build inv with one named item, flush,
	// decode, verify the JSON the client sees.
	a, fc, store := newItemsGmcpActor(t, "p-1")
	fc.setActive(true)
	inst := spawnItem(t, store, "tpl:ration", "a trail ration")
	a.AddToInventory(inst.ID())
	a.flushGmcpItems(context.Background())

	frames := fc.framesSnapshot()
	for _, f := range frames {
		if f.pkg == gmcp.PackageCharItemsList {
			if strings.Contains(string(f.payload), `"location":"inv"`) &&
				strings.Contains(string(f.payload), `"name":"a trail ration"`) {
				return
			}
		}
	}
	t.Errorf("no inv list frame matching expected shape; frames=%v", frames)
}
