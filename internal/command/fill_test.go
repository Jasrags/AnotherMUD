package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// Local mirrors of the package-private property keys used by fill.go.
// Keeping them as constants pinned alongside their definitions: a
// change in fill.go shows up as a test failure here, not a silent
// drift.
const (
	propMaxCharges = "max_charges"
	propCharges    = "charges"
	propFillSource = "fill_source"
	propFillSupply = "fill_supply"
	propFillType   = "fill_type"
)

// waterskinTpl is a fillable target: declares max_charges but starts
// empty (no charges / fill_type properties — fill should populate both
// on success).
func waterskinTpl() *item.Template {
	return &item.Template{
		ID:       "tapestry-core:waterskin",
		Name:     "a waterskin",
		Type:     "vessel",
		Keywords: []string{"waterskin", "skin"},
		Properties: map[string]any{
			propMaxCharges: 5,
		},
	}
}

// fountainTpl is an infinite source: declares fill_source (the
// liquid) but no fill_supply. Filling from it never depletes.
func fountainTpl() *item.Template {
	return &item.Template{
		ID:       "tapestry-core:fountain",
		Name:     "a stone fountain",
		Type:     "fixture",
		Tags:     []string{"fixture"},
		Keywords: []string{"fountain"},
		Properties: map[string]any{
			propFillSource: "water",
		},
	}
}

// finiteSpringTpl declares both fill_source and a finite fill_supply.
// Tests use this to verify the supply decrement and source_empty
// failure.
func finiteSpringTpl() *item.Template {
	return &item.Template{
		ID:       "tapestry-core:spring",
		Name:     "a holy spring",
		Type:     "fixture",
		Tags:     []string{"fixture"},
		Keywords: []string{"spring"},
		Properties: map[string]any{
			propFillSource: "holy water",
			propFillSupply: 2,
		},
	}
}

// taggedSourceTpl exercises the §4.6 step 2 fallback: a source with
// the `fill_source` *tag* but no `fill_source` *property* defaults to
// "water".
func taggedSourceTpl() *item.Template {
	return &item.Template{
		ID:       "tapestry-core:well",
		Name:     "an old well",
		Type:     "fixture",
		Tags:     []string{"fixture", "fill_source"},
		Keywords: []string{"well"},
	}
}

// wineFountainTpl produces "wine" — used to set up the mixed_liquids
// guard against a water-filled target.
func wineFountainTpl() *item.Template {
	return &item.Template{
		ID:       "tapestry-core:wine-fountain",
		Name:     "a wine fountain",
		Type:     "fixture",
		Tags:     []string{"fixture"},
		Keywords: []string{"fountain", "wine"},
		Properties: map[string]any{
			propFillSource: "wine",
		},
	}
}

// unfillableTpl is a non-vessel without max_charges — the not_fillable
// failure path.
func unfillableTpl() *item.Template {
	return &item.Template{
		ID:       "tapestry-core:rock",
		Name:     "a small rock",
		Type:     "treasure",
		Keywords: []string{"rock"},
	}
}

func dispatchFill(t *testing.T, f *putFixture, a *testActor, input string) {
	t.Helper()
	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	if err := r.Dispatch(context.Background(), f.env(), a, input); err != nil {
		t.Fatalf("dispatch %q: %v", input, err)
	}
}

func TestFill_HappyPath_InfiniteFountain(t *testing.T) {
	f := newPutFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	skin := f.spawnInActorInventory(t, a, waterskinTpl())
	fountain := f.spawnInRoom(t, fountainTpl())

	var filled []eventbus.ItemFilled
	f.bus.Subscribe(eventbus.EventItemFilled, func(_ context.Context, e eventbus.Event) {
		filled = append(filled, e.(eventbus.ItemFilled))
	})

	dispatchFill(t, f, a, "fill skin from fountain")

	if got := intProperty(skin, propCharges); got != 5 {
		t.Errorf("charges = %d, want 5 (max)", got)
	}
	if got := stringProperty(skin, propFillType); got != "water" {
		t.Errorf("fill_type = %q, want %q", got, "water")
	}
	if len(filled) != 1 {
		t.Fatalf("ItemFilled events = %d, want 1", len(filled))
	}
	if filled[0].TargetID != skin.ID() || filled[0].SourceID != fountain.ID() {
		t.Errorf("event payload wrong: %+v", filled[0])
	}
	if filled[0].FillType != "water" {
		t.Errorf("event FillType = %q, want water", filled[0].FillType)
	}
}

func TestFill_AcceptsImplicitAndExplicitFrom(t *testing.T) {
	for _, input := range []string{"fill skin fountain", "fill skin from fountain"} {
		input := input
		t.Run(input, func(t *testing.T) {
			f := newPutFixture(t)
			a := newNamedTestActor("Alice", "p-alice", f.room)
			skin := f.spawnInActorInventory(t, a, waterskinTpl())
			f.spawnInRoom(t, fountainTpl())
			dispatchFill(t, f, a, input)
			if got := intProperty(skin, propCharges); got != 5 {
				t.Errorf("charges after %q = %d, want 5", input, got)
			}
		})
	}
}

func TestFill_NotFillable(t *testing.T) {
	f := newPutFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	f.spawnInActorInventory(t, a, unfillableTpl())
	f.spawnInRoom(t, fountainTpl())

	dispatchFill(t, f, a, "fill rock fountain")

	if !strings.Contains(a.lastLine(), "can fill") && !strings.Contains(a.lastLine(), "you can fill") {
		t.Errorf("reply = %q, want not_fillable message", a.lastLine())
	}
}

// TestFill_NotFillable_BeforeSourceLookup pins the §4.6 validation
// order: a non-fillable target produces the target-side error even
// when the named source does not exist in the room. Without this, the
// player would see "nothing here" / "you don't see that here" when
// the real problem is the target.
func TestFill_NotFillable_BeforeSourceLookup(t *testing.T) {
	f := newPutFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	f.spawnInActorInventory(t, a, unfillableTpl())
	// Intentionally no source in the room.

	dispatchFill(t, f, a, "fill rock fountain")

	if !strings.Contains(a.lastLine(), "can fill") {
		t.Errorf("reply = %q, want not_fillable message even without source present", a.lastLine())
	}
}

func TestFill_NoFillSource(t *testing.T) {
	f := newPutFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	f.spawnInActorInventory(t, a, waterskinTpl())
	// Not a source — a statue placed in the room. No fill_source
	// property and no fill_source tag.
	f.spawnInRoom(t, fixtureStatue())

	dispatchFill(t, f, a, "fill skin statue")

	if !strings.Contains(a.lastLine(), "isn't a source") {
		t.Errorf("reply = %q, want no_fill_source message", a.lastLine())
	}
}

func TestFill_SourceEmpty(t *testing.T) {
	f := newPutFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	skin := f.spawnInActorInventory(t, a, waterskinTpl())
	// Drain a finite source by setting fill_supply to 0 directly.
	spring := f.spawnInRoom(t, finiteSpringTpl())
	spring.SetProperty(propFillSupply, 0)

	dispatchFill(t, f, a, "fill skin spring")

	if !strings.Contains(a.lastLine(), "empty") {
		t.Errorf("reply = %q, want source_empty message", a.lastLine())
	}
	if got := intProperty(skin, propCharges); got != 0 {
		t.Errorf("target charges = %d, want 0 (untouched)", got)
	}
}

func TestFill_MixedLiquids(t *testing.T) {
	f := newPutFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	skin := f.spawnInActorInventory(t, a, waterskinTpl())
	// Pre-fill with water so the mixed_liquids guard kicks in when we
	// try to top up from a wine fountain.
	skin.SetProperty(propCharges, 3)
	skin.SetProperty(propFillType, "water")
	f.spawnInRoom(t, wineFountainTpl())

	dispatchFill(t, f, a, "fill skin wine")

	if !strings.Contains(a.lastLine(), "mix") {
		t.Errorf("reply = %q, want mixed_liquids message", a.lastLine())
	}
	if got := intProperty(skin, propCharges); got != 3 {
		t.Errorf("charges = %d, want 3 (untouched after mix rejection)", got)
	}
	if got := stringProperty(skin, propFillType); got != "water" {
		t.Errorf("fill_type = %q, want water (untouched)", got)
	}
}

func TestFill_EmptyTargetAcceptsAnyLiquid(t *testing.T) {
	// Mixed-liquid guard MUST NOT fire when target charges == 0, even
	// if a stale fill_type is sitting on the target. This is the
	// §4.6 "empty targets accept any liquid" rule.
	f := newPutFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	skin := f.spawnInActorInventory(t, a, waterskinTpl())
	skin.SetProperty(propCharges, 0)
	skin.SetProperty(propFillType, "water") // stale label
	f.spawnInRoom(t, wineFountainTpl())

	dispatchFill(t, f, a, "fill skin wine")

	if got := stringProperty(skin, propFillType); got != "wine" {
		t.Errorf("fill_type = %q, want wine (re-typed from empty)", got)
	}
	if got := intProperty(skin, propCharges); got != 5 {
		t.Errorf("charges = %d, want 5 (max)", got)
	}
}

func TestFill_MatchingLiquidTopsUp(t *testing.T) {
	// Same liquid: the mixed_liquid guard MUST NOT fire and charges
	// should jump to max regardless of starting value.
	f := newPutFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	skin := f.spawnInActorInventory(t, a, waterskinTpl())
	skin.SetProperty(propCharges, 2)
	skin.SetProperty(propFillType, "water")
	f.spawnInRoom(t, fountainTpl())

	dispatchFill(t, f, a, "fill skin fountain")

	if got := intProperty(skin, propCharges); got != 5 {
		t.Errorf("charges = %d, want 5 (max)", got)
	}
}

func TestFill_FiniteSourceDecrements(t *testing.T) {
	f := newPutFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	f.spawnInActorInventory(t, a, waterskinTpl())
	spring := f.spawnInRoom(t, finiteSpringTpl()) // supply=2

	dispatchFill(t, f, a, "fill skin spring")

	if got := intProperty(spring, propFillSupply); got != 1 {
		t.Errorf("fill_supply = %d, want 1 (decremented from 2)", got)
	}
}

func TestFill_TaggedSourceDefaultsToWater(t *testing.T) {
	f := newPutFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	skin := f.spawnInActorInventory(t, a, waterskinTpl())
	f.spawnInRoom(t, taggedSourceTpl())

	dispatchFill(t, f, a, "fill skin well")

	if got := stringProperty(skin, propFillType); got != "water" {
		t.Errorf("fill_type = %q, want water (fill_source tag fallback)", got)
	}
}

func TestFill_MissingArgs(t *testing.T) {
	f := newPutFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	f.spawnInActorInventory(t, a, waterskinTpl())
	f.spawnInRoom(t, fountainTpl())

	for _, input := range []string{"fill", "fill skin", "fill skin from"} {
		input := input
		t.Run(input, func(t *testing.T) {
			a.lines = nil
			dispatchFill(t, f, a, input)
			if !strings.Contains(a.lastLine(), "Fill what") {
				t.Errorf("%q reply = %q, want usage message", input, a.lastLine())
			}
		})
	}
}

func TestFill_NoSourcesInRoom(t *testing.T) {
	f := newPutFixture(t)
	a := newNamedTestActor("Alice", "p-alice", f.room)
	f.spawnInActorInventory(t, a, waterskinTpl())

	dispatchFill(t, f, a, "fill skin fountain")

	if !strings.Contains(a.lastLine(), "nothing here") {
		t.Errorf("reply = %q, want empty-room message", a.lastLine())
	}
}

// intProperty reads an int property off an instance for assertions.
// Uses the same int-shape coercion as fill.go's intProp.
func intProperty(it interface{ Properties() map[string]any }, key string) int {
	v, ok := it.Properties()[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func stringProperty(it interface{ Properties() map[string]any }, key string) string {
	v, ok := it.Properties()[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
