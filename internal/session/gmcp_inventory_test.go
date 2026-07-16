package session

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/stacking"
)

// spawnEquip creates an equippable, keyworded item in the store — the shape the
// affordance logic keys on (EligibleSlots non-empty ⇒ `equip` action; the first
// keyword is the resolvable token the client sends).
func spawnEquip(t *testing.T, store *entities.Store, id, name, keyword, slot string) *entities.ItemInstance {
	t.Helper()
	inst, err := store.Spawn(&item.Template{
		ID:            item.TemplateID(id),
		Name:          name,
		Type:          "weapon",
		Keywords:      []string{keyword},
		EligibleSlots: []string{slot},
	})
	if err != nil {
		t.Fatalf("Spawn(%s): %v", id, err)
	}
	return inst
}

// inventoryFrames decodes the fake conn's Char.Inventory frames.
func inventoryFrames(t *testing.T, fc *gmcpFakeConn) []gmcp.CharInventory {
	t.Helper()
	raw := fc.framesSnapshot()
	out := make([]gmcp.CharInventory, 0, len(raw))
	for _, f := range raw {
		if f.pkg != gmcp.PackageCharInventory {
			continue
		}
		var inv gmcp.CharInventory
		if err := json.Unmarshal(f.payload, &inv); err != nil {
			t.Fatalf("payload unmarshal: %v (raw %s)", err, f.payload)
		}
		out = append(out, inv)
	}
	return out
}

func TestBuildCarriedItems_AffordancesAndKeyword(t *testing.T) {
	a, _, store := newItemsGmcpActor(t, "p-1")
	// An equippable item (equip+drop) and a plain trinket (drop only, no keyword).
	sword := spawnEquip(t, store, "tpl:sword", "a short sword", "sword", "wield")
	ration := spawnItem(t, store, "tpl:ration", "a ration") // trinket: no slots, no keyword
	a.AddToInventory(sword.ID())
	a.AddToInventory(ration.ID())

	carried := a.buildCarriedItems()
	if len(carried) != 2 {
		t.Fatalf("carried len = %d, want 2", len(carried))
	}
	byName := map[string]gmcp.InventoryItem{}
	for _, it := range carried {
		byName[it.Name] = it
	}
	sw := byName["a short sword"]
	// Equippable → equip (cmd carries the keyword) then drop.
	if got := actionLabels(sw.Actions); len(got) != 2 || got[0] != actionEquip || got[1] != actionDrop {
		t.Errorf("sword action labels = %v, want [equip drop]", got)
	}
	if cmd := actionCmd(sw.Actions, actionEquip); cmd != "equip sword" {
		t.Errorf("sword equip cmd = %q, want %q", cmd, "equip sword")
	}
	rat := byName["a ration"]
	if got := actionLabels(rat.Actions); len(got) != 1 || got[0] != actionDrop {
		t.Errorf("ration action labels = %v, want [drop]", got)
	}
	// No keyword on a trinket → the command falls back to the name's noun.
	if cmd := actionCmd(rat.Actions, actionDrop); cmd != "drop ration" {
		t.Errorf("ration drop cmd = %q, want %q", cmd, "drop ration")
	}
}

// actionLabels/actionCmd are small readers over an item's affordance list.
func actionLabels(acts []gmcp.InvAction) []string {
	out := make([]string, len(acts))
	for i, a := range acts {
		out[i] = a.Label
	}
	return out
}

func actionCmd(acts []gmcp.InvAction, label string) string {
	for _, a := range acts {
		if a.Label == label {
			return a.Cmd
		}
	}
	return ""
}

func TestBuildCarriedItems_StacksIdenticalItems(t *testing.T) {
	// 12 identical crossbow bolts must collapse to ONE row with qty=12 (the M21
	// stacking the CLI inventory uses) — not 12 rows.
	a, _, store := newItemsGmcpActor(t, "p-1")
	a.stacking = stacking.NewService()
	for i := 0; i < 12; i++ {
		bolt := spawnItem(t, store, "tpl:bolt", "a crossbow bolt")
		a.AddToInventory(bolt.ID())
	}

	carried := a.buildCarriedItems()
	if len(carried) != 1 {
		t.Fatalf("carried rows = %d, want 1 (a stacked bolt)", len(carried))
	}
	if carried[0].Qty != 12 {
		t.Errorf("stacked qty = %d, want 12", carried[0].Qty)
	}
	if carried[0].Name != "a crossbow bolt" {
		t.Errorf("stacked name = %q, want a crossbow bolt", carried[0].Name)
	}
	if got := actionLabels(carried[0].Actions); len(got) != 1 || got[0] != actionDrop {
		t.Errorf("bolt actions = %v, want [drop]", got)
	}
}

func TestBuildCarriedItems_SingleItemOmitsQty(t *testing.T) {
	// A lone item leaves Qty at 0 so the wire omits it (client reads absent as 1).
	a, _, store := newItemsGmcpActor(t, "p-1")
	a.stacking = stacking.NewService()
	bolt := spawnItem(t, store, "tpl:bolt", "a crossbow bolt")
	a.AddToInventory(bolt.ID())

	carried := a.buildCarriedItems()
	if len(carried) != 1 || carried[0].Qty != 0 {
		t.Fatalf("single item = %+v, want one row with Qty 0", carried)
	}
}

func TestBuildWornItems_SlotAndUnequipAffordance(t *testing.T) {
	a, _, store := newItemsGmcpActor(t, "p-1")
	jerkin := spawnEquip(t, store, "tpl:jerkin", "a leather jerkin", "jerkin", "torso")
	a.mu.Lock()
	a.equipment["torso"] = jerkin.ID()
	a.mu.Unlock()

	worn := a.buildWornItems()
	if len(worn) != 1 {
		t.Fatalf("worn len = %d, want 1", len(worn))
	}
	w := worn[0]
	if w.Slot != "torso" || w.Name != "a leather jerkin" {
		t.Errorf("worn item = %+v, want torso/jerkin", w)
	}
	if got := actionLabels(w.Actions); len(got) != 1 || got[0] != actionUnequip {
		t.Errorf("worn actions = %v, want [unequip]", got)
	}
	if cmd := actionCmd(w.Actions, actionUnequip); cmd != "unequip jerkin" {
		t.Errorf("unequip cmd = %q, want %q", cmd, "unequip jerkin")
	}
}

func TestBuildWornItems_SpanningItemListedOnceUnderPrimarySlot(t *testing.T) {
	// A two-handed weapon occupies several slots. It must appear ONCE, under the
	// lexicographically smallest slot ("main-hand" < "off-hand"), so the panel
	// doesn't double-render it and the shadow stays stable across map order.
	a, _, store := newItemsGmcpActor(t, "p-1")
	greatsword := spawnEquip(t, store, "tpl:greatsword", "a greatsword", "greatsword", "main-hand")
	a.mu.Lock()
	a.equipment["off-hand"] = greatsword.ID()
	a.equipment["main-hand"] = greatsword.ID()
	a.mu.Unlock()

	worn := a.buildWornItems()
	if len(worn) != 1 {
		t.Fatalf("spanning item produced %d worn entries, want 1: %+v", len(worn), worn)
	}
	if worn[0].Slot != "main-hand" {
		t.Errorf("primary slot = %q, want main-hand (smallest)", worn[0].Slot)
	}
}

func TestHolderDetail_LoadedAndEmpty(t *testing.T) {
	_, _, store := newItemsGmcpActor(t, "p-1")
	loaded, err := store.Spawn(&item.Template{
		ID: "tpl:clip", Name: "an Ares Predator V clip", Type: "trinket",
		Keywords: []string{"clip"}, HolderFits: "predator", Magazine: 15, Preload: 15,
	})
	if err != nil {
		t.Fatalf("spawn loaded clip: %v", err)
	}
	loaded.SetHolderAmmoGrade("apds")
	if got := holderDetail(loaded); got != "15/15 APDS" {
		t.Errorf("loaded clip detail = %q, want 15/15 APDS", got)
	}

	empty, err := store.Spawn(&item.Template{
		ID: "tpl:clip2", Name: "an Ares Predator V clip", Type: "trinket",
		Keywords: []string{"clip"}, HolderFits: "predator", Magazine: 15,
	})
	if err != nil {
		t.Fatalf("spawn empty clip: %v", err)
	}
	if got := holderDetail(empty); got != "empty" {
		t.Errorf("empty clip detail = %q, want empty", got)
	}
}

func TestEffectDetail_ModifiersAndArmor(t *testing.T) {
	_, _, store := newItemsGmcpActor(t, "p-1")
	vest, err := store.Spawn(&item.Template{
		ID: "tpl:vest", Name: "an armored vest", Type: "armor",
		Modifiers: []item.Modifier{{Stat: "body", Value: 1}}, ArmorBonus: 4,
	})
	if err != nil {
		t.Fatalf("spawn vest: %v", err)
	}
	// nil attribute set → humanized stat key ("body" → "Body").
	if got := effectDetail(nil, vest); got != "+1 Body, Armor 4" {
		t.Errorf("effect detail = %q, want %q", got, "+1 Body, Armor 4")
	}
}

func TestWornActions_ReloadAndLoad(t *testing.T) {
	_, _, store := newItemsGmcpActor(t, "p-1")
	// A holder-fed firearm → reload (bare command, targets the wielded weapon).
	gun, err := store.Spawn(&item.Template{
		ID: "tpl:gun", Name: "an Ares Predator V", Type: "weapon",
		Keywords: []string{"predator"}, AcceptsHolder: "predator", EligibleSlots: []string{"wield"},
	})
	if err != nil {
		t.Fatalf("spawn gun: %v", err)
	}
	acts := wornActions(gun)
	if got := actionLabels(acts); len(got) != 2 || got[0] != actionReload || got[1] != actionUnequip {
		t.Fatalf("gun worn actions = %v, want [reload unequip]", got)
	}
	if cmd := actionCmd(acts, actionReload); cmd != "reload" {
		t.Errorf("gun reload cmd = %q, want bare %q", cmd, "reload")
	}

	// An internally-fed magazine firearm (Magazine>0, no holder) → reload too
	// (mirrors the CLI ReloadHandler's middle feed model).
	revolver, err := store.Spawn(&item.Template{
		ID: "tpl:revolver", Name: "a heavy revolver", Type: "weapon",
		Keywords: []string{"revolver"}, Magazine: 6, EligibleSlots: []string{"wield"},
	})
	if err != nil {
		t.Fatalf("spawn revolver: %v", err)
	}
	if cmd := actionCmd(wornActions(revolver), actionReload); cmd != "reload" {
		t.Errorf("magazine revolver reload cmd = %q, want bare %q", cmd, "reload")
	}

	// A reload-gated projectile (crossbow) → load (bare command).
	bow, err := store.Spawn(&item.Template{
		ID: "tpl:xbow", Name: "a medium crossbow", Type: "weapon",
		Keywords: []string{"crossbow"}, ReloadTicks: 20, EligibleSlots: []string{"wield"},
	})
	if err != nil {
		t.Fatalf("spawn crossbow: %v", err)
	}
	if cmd := actionCmd(wornActions(bow), actionLoad); cmd != "load" {
		t.Errorf("crossbow load cmd = %q, want bare %q", cmd, "load")
	}
}

func TestBuildCarriedItems_ClipsListedIndividuallyWithReload(t *testing.T) {
	// Ammunition holders never collapse into a stack — each shows its own load
	// state — and carry a `reload <clip>` affordance (fill from loose rounds).
	a, _, store := newItemsGmcpActor(t, "p-1")
	a.stacking = stacking.NewService()
	for i := 0; i < 3; i++ {
		clip, err := store.Spawn(&item.Template{
			ID: "tpl:clip", Name: "an Ares Predator V clip", Type: "trinket",
			Keywords: []string{"clip"}, HolderFits: "predator", Magazine: 15, Preload: 15,
		})
		if err != nil {
			t.Fatalf("spawn clip %d: %v", i, err)
		}
		a.AddToInventory(clip.ID())
	}
	carried := a.buildCarriedItems()
	if len(carried) != 3 {
		t.Fatalf("clips collapsed to %d rows, want 3 (individual)", len(carried))
	}
	got := actionLabels(carried[0].Actions)
	if len(got) != 2 || got[0] != actionReload || got[1] != actionDrop {
		t.Errorf("clip actions = %v, want [reload drop]", got)
	}
	if cmd := actionCmd(carried[0].Actions, actionReload); cmd != "reload clip" {
		t.Errorf("clip reload cmd = %q, want %q", cmd, "reload clip")
	}
}

func TestBuildWornItems_EnumeratesEmptySlotsInOrder(t *testing.T) {
	// With a slot registry wired, every slot appears in registration order —
	// including empties — mirroring the `equipment` verb.
	a, _, store := newItemsGmcpActor(t, "p-1")
	reg := slot.NewRegistry()
	for _, name := range []string{"wield", "head", "body"} {
		if err := reg.Register(slot.Def{Name: name, Max: 1}); err != nil {
			t.Fatalf("register slot %q: %v", name, err)
		}
	}
	a.slots = reg
	vest := spawnEquip(t, store, "tpl:vest", "an armored vest", "vest", "body")
	a.mu.Lock()
	a.equipment["body"] = vest.ID()
	a.mu.Unlock()

	worn := a.buildWornItems()
	if len(worn) != 3 {
		t.Fatalf("worn rows = %d, want 3 (all slots)", len(worn))
	}
	// Registration order: wield (empty), head (empty), body (vest).
	if worn[0].Slot != "wield" || !worn[0].Empty {
		t.Errorf("row 0 = %+v, want empty wield", worn[0])
	}
	if worn[1].Slot != "head" || !worn[1].Empty {
		t.Errorf("row 1 = %+v, want empty head", worn[1])
	}
	if worn[2].Slot != "body" || worn[2].Empty || worn[2].Name != "an armored vest" {
		t.Errorf("row 2 = %+v, want body vest", worn[2])
	}
}

func TestFlushGmcpInventory_NoSendBeforeActivation(t *testing.T) {
	a, fc, _ := newItemsGmcpActor(t, "p-1")
	a.flushGmcpInventory(context.Background()) // active=false
	if got := len(inventoryFrames(t, fc)); got != 0 {
		t.Errorf("pre-activation emitted %d inventory frames, want 0", got)
	}
}

func TestFlushGmcpInventory_FirstFlushSendsEvenWhenEmpty(t *testing.T) {
	a, fc, _ := newItemsGmcpActor(t, "p-1")
	fc.setActive(true)
	a.flushGmcpInventory(context.Background())

	frames := inventoryFrames(t, fc)
	if len(frames) != 1 {
		t.Fatalf("first flush sent %d inventory frames, want 1", len(frames))
	}
	if len(frames[0].Carried) != 0 || len(frames[0].Worn) != 0 {
		t.Errorf("empty actor produced non-empty frame: %+v", frames[0])
	}
}

func TestFlushGmcpInventory_NoRedundantSendWhenUnchanged(t *testing.T) {
	a, fc, store := newItemsGmcpActor(t, "p-1")
	fc.setActive(true)
	sword := spawnEquip(t, store, "tpl:sword", "a short sword", "sword", "wield")
	a.AddToInventory(sword.ID())

	a.flushGmcpInventory(context.Background()) // baseline
	pre := len(inventoryFrames(t, fc))
	a.flushGmcpInventory(context.Background())
	a.flushGmcpInventory(context.Background())
	if got := len(inventoryFrames(t, fc)); got != pre {
		t.Errorf("redundant flushes added %d frames, want 0", got-pre)
	}
}

func TestFlushGmcpInventory_ChangeSendsNewFrame(t *testing.T) {
	a, fc, store := newItemsGmcpActor(t, "p-1")
	fc.setActive(true)
	a.flushGmcpInventory(context.Background()) // baseline (empty)

	sword := spawnEquip(t, store, "tpl:sword", "a short sword", "sword", "wield")
	a.AddToInventory(sword.ID())
	a.flushGmcpInventory(context.Background())

	frames := inventoryFrames(t, fc)
	if len(frames) != 2 {
		t.Fatalf("total inventory frames = %d, want 2", len(frames))
	}
	last := frames[len(frames)-1]
	if len(last.Carried) != 1 || last.Carried[0].Name != "a short sword" {
		t.Errorf("post-add frame = %+v, want one carried short sword", last)
	}
}

func TestFlushGmcpInventory_ShadowResetForcesResend(t *testing.T) {
	a, fc, _ := newItemsGmcpActor(t, "p-1")
	fc.setActive(true)
	a.flushGmcpInventory(context.Background()) // baseline
	pre := len(inventoryFrames(t, fc))

	a.resetGmcpItemsShadow() // clears the inventory shadow too
	a.flushGmcpInventory(context.Background())
	if got := len(inventoryFrames(t, fc)) - pre; got != 1 {
		t.Errorf("post-reset added %d frames, want 1", got)
	}
}
