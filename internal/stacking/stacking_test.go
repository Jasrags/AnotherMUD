package stacking

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// mk spawns one item from a template id + name into the store, applying any
// instance properties.
func mk(t *testing.T, store *entities.Store, tplID, name string, props map[string]any) *entities.ItemInstance {
	t.Helper()
	it, err := store.Spawn(&item.Template{ID: item.TemplateID(tplID), Name: name, Type: "item"})
	if err != nil {
		t.Fatalf("Spawn(%q): %v", tplID, err)
	}
	for k, v := range props {
		it.SetProperty(k, v)
	}
	return it
}

// Same template, no essence → one stack with the full quantity.
func TestStack_SameTemplateStacks(t *testing.T) {
	store := entities.NewStore()
	items := []*entities.ItemInstance{
		mk(t, store, "potion", "a healing potion", nil),
		mk(t, store, "potion", "a healing potion", nil),
		mk(t, store, "potion", "a healing potion", nil),
	}
	got := NewService().Stack(items)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 stack", len(got))
	}
	if got[0].Quantity != 3 || len(got[0].ItemIDs) != 3 {
		t.Errorf("stack = %+v, want quantity 3", got[0])
	}
	if got[0].TemplateID != "potion" || got[0].DisplayName != "a healing potion" {
		t.Errorf("stack fields = %+v", got[0])
	}
}

// Same template, different essence → separate stacks (closes M20.6).
func TestStack_DifferentEssenceSplits(t *testing.T) {
	store := entities.NewStore()
	items := []*entities.ItemInstance{
		mk(t, store, "blade", "a blade", map[string]any{"essence": "fire"}),
		mk(t, store, "blade", "a blade", map[string]any{"essence": "frost"}),
		mk(t, store, "blade", "a blade", map[string]any{"essence": "fire"}),
	}
	got := NewService().Stack(items)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 stacks (fire / frost)", len(got))
	}
	// First-seen order: fire stack first (qty 2), then frost (qty 1).
	if got[0].EssenceKey != "fire" || got[0].Quantity != 2 {
		t.Errorf("stack[0] = %+v, want fire x2", got[0])
	}
	if got[1].EssenceKey != "frost" || got[1].Quantity != 1 {
		t.Errorf("stack[1] = %+v, want frost x1", got[1])
	}
}

// Same template + same essence stack together.
func TestStack_SameEssenceStacks(t *testing.T) {
	store := entities.NewStore()
	items := []*entities.ItemInstance{
		mk(t, store, "blade", "a blade", map[string]any{"essence": "fire"}),
		mk(t, store, "blade", "a blade", map[string]any{"essence": "fire"}),
	}
	got := NewService().Stack(items)
	if len(got) != 1 || got[0].Quantity != 2 {
		t.Fatalf("got %d stacks, want 1 of qty 2: %+v", len(got), got)
	}
}

// Template-less items never stack — each is its own singleton.
func TestStack_TemplatelessAreSingletons(t *testing.T) {
	store := entities.NewStore()
	items := []*entities.ItemInstance{
		mk(t, store, "", "a mysterious orb", nil),
		mk(t, store, "", "a mysterious orb", nil),
	}
	got := NewService().Stack(items)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 singletons", len(got))
	}
	for i, e := range got {
		if e.Quantity != 1 || e.TemplateID != "" {
			t.Errorf("singleton[%d] = %+v", i, e)
		}
	}
}

// Rarity does NOT split stacks (cosmetic, §5/§9 default); the entry carries
// the first item's rarity key.
func TestStack_RarityDoesNotSplit(t *testing.T) {
	store := entities.NewStore()
	items := []*entities.ItemInstance{
		mk(t, store, "gem", "a gem", map[string]any{"rarity": "rare"}),
		mk(t, store, "gem", "a gem", map[string]any{"rarity": "legendary"}),
	}
	got := NewService().Stack(items)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 stack (rarity must not split)", len(got))
	}
	if got[0].RarityKey != "rare" {
		t.Errorf("RarityKey = %q, want the first item's 'rare'", got[0].RarityKey)
	}
}

// First-seen position fixes a stack's place in the output.
func TestStack_PreservesFirstSeenOrder(t *testing.T) {
	store := entities.NewStore()
	items := []*entities.ItemInstance{
		mk(t, store, "torch", "a torch", nil),
		mk(t, store, "potion", "a potion", nil),
		mk(t, store, "torch", "a torch", nil), // stacks with the first torch
	}
	got := NewService().Stack(items)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].TemplateID != "torch" || got[0].Quantity != 2 {
		t.Errorf("stack[0] = %+v, want torch x2 (first seen)", got[0])
	}
	if got[1].TemplateID != "potion" {
		t.Errorf("stack[1] = %+v, want potion", got[1])
	}
}

// An extra registered key (§5.1) splits stacks by that property too; extras
// only participate after AddKey.
func TestStack_ExtraKeySplits(t *testing.T) {
	store := entities.NewStore()
	items := []*entities.ItemInstance{
		mk(t, store, "flask", "a flask", map[string]any{"fill": "water"}),
		mk(t, store, "flask", "a flask", map[string]any{"fill": "wine"}),
	}
	// Without the extra key, both flasks stack (template + empty essence).
	if got := NewService().Stack(items); len(got) != 1 {
		t.Fatalf("without AddKey: %d stacks, want 1", len(got))
	}
	// With "fill" registered, they split.
	s := NewService()
	s.AddKey("fill")
	if got := s.Stack(items); len(got) != 2 {
		t.Fatalf("with AddKey(fill): %d stacks, want 2", len(got))
	}
}

// AddKey ignores blank names and de-duplicates re-registration.
func TestAddKey_BlankAndDedup(t *testing.T) {
	s := NewService()
	s.AddKey("  ")   // ignored
	s.AddKey("fill") // added
	s.AddKey("fill") // dedup
	if len(s.extraKeys) != 1 || s.extraKeys[0] != "fill" {
		t.Errorf("extraKeys = %v, want [fill]", s.extraKeys)
	}
}

// Empty input returns nil; nil items in the slice are skipped.
func TestStack_EmptyAndNil(t *testing.T) {
	s := NewService()
	if got := s.Stack(nil); got != nil {
		t.Errorf("Stack(nil) = %v, want nil", got)
	}
	store := entities.NewStore()
	items := []*entities.ItemInstance{nil, mk(t, store, "potion", "a potion", nil), nil}
	got := s.Stack(items)
	if len(got) != 1 || got[0].Quantity != 1 {
		t.Errorf("got %+v, want one stack skipping nils", got)
	}
}

// A "|" inside a property value must not cause a stack-key collision: an
// essence of "a|b" with no extra key must NOT merge with essence "a" + an
// extra key "b". (Guards the internal delimiter choice.)
func TestStack_DelimiterCollisionResistant(t *testing.T) {
	store := entities.NewStore()
	pipe := mk(t, store, "rune", "a rune", map[string]any{"essence": "a|b"})

	withExtra := mk(t, store, "rune", "a rune", map[string]any{"essence": "a", "tier": "b"})
	s := NewService()
	s.AddKey("tier")

	got := s.Stack([]*entities.ItemInstance{pipe, withExtra})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 distinct stacks (no delimiter collision)", len(got))
	}
}

// Stacking is read-only: an item's observable state is unchanged after
// Stack (inventory-equipment-items §5 acceptance).
func TestStack_ReadOnly(t *testing.T) {
	store := entities.NewStore()
	it := mk(t, store, "potion", "a healing potion", map[string]any{"essence": "fire", "rarity": "rare"})
	nameBefore := it.Name()
	essBefore, _ := it.Property("essence")
	rarBefore, _ := it.Property("rarity")

	_ = NewService().Stack([]*entities.ItemInstance{it, it})

	if it.Name() != nameBefore {
		t.Errorf("Name mutated: %q -> %q", nameBefore, it.Name())
	}
	if e, _ := it.Property("essence"); e != essBefore {
		t.Errorf("essence mutated: %v -> %v", essBefore, e)
	}
	if r, _ := it.Property("rarity"); r != rarBefore {
		t.Errorf("rarity mutated: %v -> %v", rarBefore, r)
	}
}
