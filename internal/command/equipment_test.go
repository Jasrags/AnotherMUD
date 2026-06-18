package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// --- P3 fixtures: spanning + multi-eligible items ---

// greatswordTpl is a two-handed weapon: eligible for wield, but its
// footprint also occupies the off hand (companion slot).
func greatswordTpl() *item.Template {
	return &item.Template{
		ID:             "tapestry-core:greatsword",
		Name:           "a greatsword",
		Type:           "weapon",
		Keywords:       []string{"greatsword", "great"},
		Modifiers:      []item.Modifier{{Stat: "str", Value: 2}},
		EligibleSlots:  []string{"wield"},
		CompanionSlots: []string{"offhand"},
	}
}

func shieldTpl() *item.Template {
	return &item.Template{
		ID:            "tapestry-core:shield",
		Name:          "a wooden shield",
		Type:          "armor",
		Keywords:      []string{"shield"},
		EligibleSlots: []string{"offhand"},
	}
}

// daggerTpl is multi-eligible: it fits wield OR offhand.
func daggerTpl() *item.Template {
	return &item.Template{
		ID:            "tapestry-core:dagger",
		Name:          "a dagger",
		Type:          "weapon",
		Keywords:      []string{"dagger"},
		EligibleSlots: []string{"wield", "offhand"},
	}
}

func containsID(ids []entities.EntityID, want entities.EntityID) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// eqFixture extends invFixture with a slot registry holding the M5.3
// engine baseline (wield cap 1, head cap 1, finger cap 2).
type eqFixture struct {
	*invFixture
	slots *slot.Registry
}

func newEqFixture(t *testing.T) *eqFixture {
	t.Helper()
	f := newInvFixture(t)
	reg := slot.NewRegistry()
	if err := slot.RegisterEngineBaseline(reg); err != nil {
		t.Fatalf("RegisterEngineBaseline: %v", err)
	}
	return &eqFixture{invFixture: f, slots: reg}
}

func (f *eqFixture) env() command.Env {
	e := f.invFixture.env()
	e.Slots = f.slots
	return e
}

// spawnInInventory bypasses Placement so the item is owned but not in
// a room — equivalent to the actor having picked it up off the floor.
func (f *eqFixture) spawnInInventory(t *testing.T, tpl *item.Template, a *testActor) *entities.ItemInstance {
	t.Helper()
	inst, err := f.store.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	a.AddToInventory(inst.ID())
	return inst
}

func swordWithMods() *item.Template {
	return &item.Template{
		ID:            "tapestry-core:short-sword",
		Name:          "a short sword",
		Type:          "weapon",
		Keywords:      []string{"sword", "short"},
		Modifiers:     []item.Modifier{{Stat: "str", Value: 1}},
		EligibleSlots: []string{"wield"},
	}
}

func ringTpl(id string) *item.Template {
	return &item.Template{
		ID:            item.TemplateID(id),
		Name:          "a plain ring",
		Type:          "ring",
		Keywords:      []string{"ring"},
		EligibleSlots: []string{"finger"},
	}
}

func dispatch(t *testing.T, r *command.Registry, env command.Env, a *testActor, line string) {
	t.Helper()
	if err := r.Dispatch(context.Background(), env, a, line); err != nil {
		t.Fatalf("dispatch %q: %v", line, err)
	}
}

func newRegistry(t *testing.T) *command.Registry {
	t.Helper()
	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	return r
}

func TestEquip_MovesFromInventoryToSlotAndAppliesMods(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	inst := f.spawnInInventory(t, swordWithMods(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip sword wield")

	if len(a.Inventory()) != 0 {
		t.Errorf("inventory after equip = %v, want empty", a.Inventory())
	}
	eq := a.Equipment()
	if got := eq["wield"]; got != inst.ID() {
		t.Errorf("equipment[wield] = %q, want %q", got, inst.ID())
	}
	mods := a.mods[entities.EquipmentSourceKey(inst.ID())]
	if len(mods) != 1 || mods[0].Stat != "str" || mods[0].Value != 1 {
		t.Errorf("modifiers applied = %+v", mods)
	}
}

func TestEquip_WearWieldHoldAliases(t *testing.T) {
	// wear/wield/hold all alias `equip`; a sole-eligible item equips
	// without naming the slot.
	for _, verb := range []string{"wear", "wield", "hold"} {
		t.Run(verb, func(t *testing.T) {
			f := newEqFixture(t)
			a := newTestActor(f.room)
			inst := f.spawnInInventory(t, swordWithMods(), a)
			r := newRegistry(t)

			dispatch(t, r, f.env(), a, verb+" sword")

			if got := a.Equipment()["wield"]; got != inst.ID() {
				t.Errorf("%q sword: equipment[wield] = %q, want %q", verb, got, inst.ID())
			}
		})
	}
}

func TestEquip_AutoSwapDisplacesOccupant(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	first := f.spawnInInventory(t, swordWithMods(), a)
	second := f.spawnInInventory(t, swordWithMods(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip 1.sword wield")
	dispatch(t, r, f.env(), a, "equip sword wield")

	// Second equip should have displaced the first.
	if got := a.Equipment()["wield"]; got != second.ID() {
		t.Errorf("wield = %q, want second %q", got, second.ID())
	}
	// First sword returned to inventory.
	inv := a.Inventory()
	found := false
	for _, id := range inv {
		if id == first.ID() {
			found = true
		}
	}
	if !found {
		t.Errorf("displaced item not returned to inventory; inv = %v", inv)
	}
	// Mods from first item gone; from second present.
	if _, ok := a.mods[entities.EquipmentSourceKey(first.ID())]; ok {
		t.Error("first item's mods still applied after displacement")
	}
	if _, ok := a.mods[entities.EquipmentSourceKey(second.ID())]; !ok {
		t.Error("second item's mods not applied")
	}
}

func TestEquip_MultiCapFingerPacksFromZero(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	first := f.spawnInInventory(t, ringTpl("tapestry-core:ring-a"), a)
	second := f.spawnInInventory(t, ringTpl("tapestry-core:ring-b"), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip 1.ring finger")
	dispatch(t, r, f.env(), a, "equip ring finger")

	eq := a.Equipment()
	if eq["finger:0"] != first.ID() {
		t.Errorf("finger:0 = %q, want %q", eq["finger:0"], first.ID())
	}
	if eq["finger:1"] != second.ID() {
		t.Errorf("finger:1 = %q, want %q", eq["finger:1"], second.ID())
	}
}

func TestEquip_UnknownSlotFails(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	f.spawnInInventory(t, swordWithMods(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip sword nostril")

	last := a.lastLine()
	if !strings.Contains(last, "No such slot") {
		t.Errorf("expected slot error, got %q", last)
	}
	// Item stayed in inventory.
	if len(a.Inventory()) != 1 {
		t.Errorf("inventory disturbed by failed equip: %v", a.Inventory())
	}
}

// TestEquip_WrongSlotRejected covers Gap 1 (§3.4 step 3): an item is
// eligible only for the slots it declares. Equipping a wield-only sword
// into the head slot fails with a message distinct from "No such slot",
// and the item stays in inventory (no mutation, no mods applied).
func TestEquip_WrongSlotRejected(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	inst := f.spawnInInventory(t, swordWithMods(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip sword head")

	last := a.lastLine()
	if strings.Contains(last, "No such slot") {
		t.Errorf("wrong-slot error must differ from unknown-slot error, got %q", last)
	}
	if !strings.Contains(last, "can't equip") {
		t.Errorf("expected eligibility error, got %q", last)
	}
	if len(a.Inventory()) != 1 {
		t.Errorf("inventory disturbed by rejected equip: %v", a.Inventory())
	}
	if _, ok := a.Equipment()["head"]; ok {
		t.Error("head slot occupied after rejected equip")
	}
	if _, ok := a.mods[entities.EquipmentSourceKey(inst.ID())]; ok {
		t.Error("modifiers applied despite rejected equip")
	}
}

// TestEquip_NotEquippableRejected: an item declaring no eligible slots
// (a quest token) can never be equipped, with a distinct reason.
func TestEquip_NotEquippableRejected(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	token := &item.Template{
		ID:       "tapestry-core:quest-token",
		Name:     "a wax seal",
		Type:     "item",
		Keywords: []string{"seal", "token"},
	}
	f.spawnInInventory(t, token, a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip seal wield")

	last := a.lastLine()
	if !strings.Contains(last, "can't equip") {
		t.Errorf("expected not-equippable error, got %q", last)
	}
	if len(a.Inventory()) != 1 {
		t.Errorf("inventory disturbed: %v", a.Inventory())
	}
}

// TestEquip_LegacySlotPropertyStillWorks: a hand-built template carrying
// only the legacy `properties.slot` string (no EligibleSlots) remains
// equippable via the §3.2 bridge applied at instance build.
func TestEquip_LegacySlotPropertyStillWorks(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	legacy := &item.Template{
		ID:         "tapestry-core:legacy-cap",
		Name:       "a worn cap",
		Type:       "item",
		Keywords:   []string{"cap"},
		Properties: map[string]any{"slot": "head"},
	}
	inst := f.spawnInInventory(t, legacy, a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip cap head")

	if got := a.Equipment()["head"]; got != inst.ID() {
		t.Errorf("equipment[head] = %q, want %q (legacy slot bridge)", got, inst.ID())
	}
}

// --- P3: footprint / contention (gaps 2 & 3) ---

// TestEquip_SpanningOccupiesWholeFootprint: a two-handed weapon occupies
// both wield and offhand, and applies its modifiers exactly once (§3.4
// steps 5/8/9).
func TestEquip_SpanningOccupiesWholeFootprint(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	gs := f.spawnInInventory(t, greatswordTpl(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip greatsword wield")

	eq := a.Equipment()
	if eq["wield"] != gs.ID() || eq["offhand"] != gs.ID() {
		t.Errorf("greatsword footprint = %v, want wield+offhand → %s", eq, gs.ID())
	}
	if mods := a.mods[entities.EquipmentSourceKey(gs.ID())]; len(mods) != 1 {
		t.Errorf("spanning item mods applied %d times, want exactly 1", len(mods))
	}
}

// TestEquip_SoleEligibleAutoTargets: with no slot named, a sole-eligible
// item equips into its only slot (Decision A).
func TestEquip_SoleEligibleAutoTargets(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	inst := f.spawnInInventory(t, swordWithMods(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip sword")

	if a.Equipment()["wield"] != inst.ID() {
		t.Errorf("sole-eligible auto-target failed; eq = %v", a.Equipment())
	}
}

// TestEquip_AmbiguousSlotAsksWhich: a multi-eligible item with no slot
// named is asked which, with no mutation (Decision A).
func TestEquip_AmbiguousSlotAsksWhich(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	f.spawnInInventory(t, daggerTpl(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip dagger")

	if last := a.lastLine(); !strings.Contains(last, "Which slot") {
		t.Errorf("expected ask-which, got %q", last)
	}
	if len(a.Inventory()) != 1 {
		t.Errorf("inventory disturbed by ambiguous equip: %v", a.Inventory())
	}
}

// TestEquip_SpanningDisplacesOccupant: equipping a two-hander into wield
// while a shield holds the offhand displaces the shield (§3.6).
func TestEquip_SpanningDisplacesOccupant(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	shield := f.spawnInInventory(t, shieldTpl(), a)
	gs := f.spawnInInventory(t, greatswordTpl(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip shield offhand")
	dispatch(t, r, f.env(), a, "equip greatsword wield")

	eq := a.Equipment()
	if eq["wield"] != gs.ID() || eq["offhand"] != gs.ID() {
		t.Errorf("greatsword not spanning after displace: %v", eq)
	}
	if !containsID(a.Inventory(), shield.ID()) {
		t.Errorf("displaced shield not returned to inventory; inv = %v", a.Inventory())
	}
}

// TestEquip_OneHandDisplacesSpanningInFull: equipping into any slot of a
// worn spanning item's footprint displaces that item in FULL (§3.6).
func TestEquip_OneHandDisplacesSpanningInFull(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	gs := f.spawnInInventory(t, greatswordTpl(), a)
	sword := f.spawnInInventory(t, swordWithMods(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip greatsword wield")
	dispatch(t, r, f.env(), a, "equip sword wield")

	eq := a.Equipment()
	if eq["wield"] != sword.ID() {
		t.Errorf("wield = %v, want sword", eq["wield"])
	}
	if _, ok := eq["offhand"]; ok {
		t.Errorf("offhand still occupied after displacing spanning item: %v", eq)
	}
	if !containsID(a.Inventory(), gs.ID()) {
		t.Errorf("displaced greatsword not returned; inv = %v", a.Inventory())
	}
	if _, ok := a.mods[entities.EquipmentSourceKey(gs.ID())]; ok {
		t.Error("greatsword mods still applied after full displacement")
	}
}

// TestEquip_DisplacesMultipleOccupants: a companion-bearing item can evict
// more than one item in a single equip (§3.4 step 6).
func TestEquip_DisplacesMultipleOccupants(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	sword := f.spawnInInventory(t, swordWithMods(), a)
	shield := f.spawnInInventory(t, shieldTpl(), a)
	gs := f.spawnInInventory(t, greatswordTpl(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip sword wield")
	dispatch(t, r, f.env(), a, "equip shield offhand")
	dispatch(t, r, f.env(), a, "equip greatsword wield")

	eq := a.Equipment()
	if eq["wield"] != gs.ID() || eq["offhand"] != gs.ID() {
		t.Errorf("greatsword not spanning after multi-displace: %v", eq)
	}
	if !containsID(a.Inventory(), sword.ID()) || !containsID(a.Inventory(), shield.ID()) {
		t.Errorf("both displaced items should be back in inventory; inv = %v", a.Inventory())
	}
}

// TestUnequip_SpanningFreesWholeFootprint: unequipping a two-hander frees
// every slot it held (§3.5 step 2).
func TestUnequip_SpanningFreesWholeFootprint(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	f.spawnInInventory(t, greatswordTpl(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip greatsword wield")
	dispatch(t, r, f.env(), a, "unequip greatsword")

	if eq := a.Equipment(); len(eq) != 0 {
		t.Errorf("equipment not empty after unequipping spanning item: %v", eq)
	}
}

// TestEquip_CancellableVetoBlocks: a listener flipping the cancel flag on
// entity.equipping aborts the equip with no mutation (§3.4 step 7).
func TestEquip_CancellableVetoBlocks(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	inst := f.spawnInInventory(t, swordWithMods(), a)

	bus := eventbus.New()
	bus.Subscribe(eventbus.EventEntityEquipping, func(ctx context.Context, e eventbus.Event) {
		if ev, ok := e.(*eventbus.EntityEquipping); ok {
			ev.Cancel()
		}
	})
	env := f.env()
	env.Bus = bus
	r := newRegistry(t)

	dispatch(t, r, env, a, "equip sword wield")

	if _, ok := a.Equipment()["wield"]; ok {
		t.Error("veto did not prevent the equip")
	}
	if !containsID(a.Inventory(), inst.ID()) {
		t.Error("item left inventory despite veto")
	}
	if _, ok := a.mods[entities.EquipmentSourceKey(inst.ID())]; ok {
		t.Error("modifiers applied despite veto")
	}
}

// TestEquip_NoRemoveBlocksAutoSwap: Decision B — auto-swap must not force
// a no-remove item off; the equip fails with no mutation.
func TestEquip_NoRemoveBlocksAutoSwap(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	cursed := &item.Template{
		ID:            "tapestry-core:cursed-blade",
		Name:          "a cursed blade",
		Type:          "weapon",
		Keywords:      []string{"cursed", "blade"},
		Tags:          []string{"no_remove"},
		EligibleSlots: []string{"wield"},
	}
	cursedInst := f.spawnInInventory(t, cursed, a)
	sword := f.spawnInInventory(t, swordWithMods(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip blade wield")
	dispatch(t, r, f.env(), a, "equip sword wield") // would displace the cursed blade

	if last := a.lastLine(); !strings.Contains(last, "can't remove") {
		t.Errorf("expected no-remove block, got %q", last)
	}
	if a.Equipment()["wield"] != cursedInst.ID() {
		t.Error("no-remove item was force-displaced")
	}
	if !containsID(a.Inventory(), sword.ID()) {
		t.Error("blocked sword should remain in inventory")
	}
}

func TestEquip_ItemNotInInventoryFails(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip sword wield")

	last := a.lastLine()
	if !strings.Contains(last, "carrying") {
		t.Errorf("expected carrying-error, got %q", last)
	}
}

func TestUnequip_ReturnsItemAndReversesMods(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	inst := f.spawnInInventory(t, swordWithMods(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip sword wield")
	dispatch(t, r, f.env(), a, "unequip sword")

	if _, ok := a.Equipment()["wield"]; ok {
		t.Error("wield slot still occupied after unequip")
	}
	if inv := a.Inventory(); len(inv) != 1 || inv[0] != inst.ID() {
		t.Errorf("inventory after unequip = %v, want [%q]", inv, inst.ID())
	}
	if _, ok := a.mods[entities.EquipmentSourceKey(inst.ID())]; ok {
		t.Error("modifiers still present after unequip")
	}
}

func TestUnequip_NotEquippedFails(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	r := newRegistry(t)
	dispatch(t, r, f.env(), a, "unequip sword")
	last := a.lastLine()
	if !strings.Contains(last, "wearing") {
		t.Errorf("expected wearing-error, got %q", last)
	}
}

// recordingBroadcaster captures SendToRoom calls for assertion.
type recordingBroadcaster struct {
	calls []recordedCall
}

type recordedCall struct {
	roomID world.RoomID
	text   string
}

func (b *recordingBroadcaster) SendToRoom(ctx context.Context, roomID world.RoomID, text string, exclude ...string) {
	b.calls = append(b.calls, recordedCall{roomID: roomID, text: text})
}

// namedActor is a testActor variant with a fixed name + player id so
// broadcast guards fire.
type namedActor struct {
	*testActor
	name     string
	playerID string
}

func (n *namedActor) Name() string     { return n.name }
func (n *namedActor) PlayerID() string { return n.playerID }

func TestEquip_BroadcastFiresWithItemName(t *testing.T) {
	// Verifies the §3.3 step 7 broadcast: when a named actor with a
	// non-empty player id equips, the room receives a notification
	// referencing the item name (the base slot name is implicit in the
	// message, not the slot key — slot keys never reach players).
	f := newEqFixture(t)
	rec := &recordingBroadcaster{}
	env := f.env()
	env.Broadcaster = rec

	inner := newTestActor(f.room)
	a := &namedActor{testActor: inner, name: "Alice", playerID: "p-1"}

	inst, err := f.store.Spawn(swordWithMods())
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	a.AddToInventory(inst.ID())

	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), env, a, "equip sword wield"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if len(rec.calls) != 1 {
		t.Fatalf("broadcast count = %d, want 1", len(rec.calls))
	}
	if got := rec.calls[0].text; !strings.Contains(got, "Alice") || !strings.Contains(got, "short sword") {
		t.Errorf("broadcast text = %q, expected Alice + short sword", got)
	}
	if strings.Contains(rec.calls[0].text, ":") {
		t.Errorf("broadcast leaked a slot-key colon: %q", rec.calls[0].text)
	}
}

// heavyHelmTpl is a heavy-tier armor on the head slot (like the great helm) —
// "slow" armor for the §7 don/doff combat gate.
func heavyHelmTpl() *item.Template {
	return &item.Template{
		ID: "tapestry-core:great-helm", Name: "a great helm", Type: "item",
		Keywords: []string{"helm"}, EligibleSlots: []string{"head"},
		ArmorTier: "heavy", ArmorBonus: 4,
	}
}

// lightCapTpl is light-tier head armor — NOT gated in combat.
func lightCapTpl() *item.Template {
	return &item.Template{
		ID: "tapestry-core:padded-cap", Name: "a padded cap", Type: "item",
		Keywords: []string{"cap"}, EligibleSlots: []string{"head"},
		ArmorTier: "light", ArmorBonus: 1,
	}
}

// Armor §7: bulky (medium/heavy) armor can't be donned or removed mid-combat;
// light armor and out-of-combat changes are unaffected.
func TestEquip_SlowArmorBlockedInCombat(t *testing.T) {
	r := newRegistry(t)

	// In combat: a heavy helm is refused, and stays in inventory.
	f := newEqFixture(t)
	a := newTestActor(f.room)
	a.inCombat = true
	helm := f.spawnInInventory(t, heavyHelmTpl(), a)
	dispatch(t, r, f.env(), a, "equip helm")
	if _, worn := a.Equipment()["head"]; worn {
		t.Error("heavy helm equipped in combat; the §7 gate should refuse it")
	}
	if !containsID(a.Inventory(), helm.ID()) {
		t.Error("refused helm should stay in inventory")
	}
	if joined := strings.Join(actorLines(a), "\n"); !strings.Contains(joined, "no time to buckle on") {
		t.Errorf("expected the don-in-combat refusal:\n%s", joined)
	}

	// In combat: light armor is quick — it equips fine.
	f2 := newEqFixture(t)
	a2 := newTestActor(f2.room)
	a2.inCombat = true
	f2.spawnInInventory(t, lightCapTpl(), a2)
	dispatch(t, r, f2.env(), a2, "equip cap")
	if _, worn := a2.Equipment()["head"]; !worn {
		t.Error("light cap should equip even in combat")
	}

	// Out of combat: the heavy helm equips; then entering combat, it can't be shed.
	f3 := newEqFixture(t)
	a3 := newTestActor(f3.room)
	f3.spawnInInventory(t, heavyHelmTpl(), a3)
	dispatch(t, r, f3.env(), a3, "equip helm")
	if _, worn := a3.Equipment()["head"]; !worn {
		t.Fatal("heavy helm should equip out of combat")
	}
	a3.inCombat = true
	a3.lines = nil
	dispatch(t, r, f3.env(), a3, "unequip helm")
	if _, worn := a3.Equipment()["head"]; !worn {
		t.Error("heavy helm removed in combat; the §7 gate should refuse the doff")
	}
	if joined := strings.Join(actorLines(a3), "\n"); !strings.Contains(joined, "can't shed") {
		t.Errorf("expected the doff-in-combat refusal:\n%s", joined)
	}
}
