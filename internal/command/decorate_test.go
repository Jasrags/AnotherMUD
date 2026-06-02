package command

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/decoration"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/render"
)

// decoRig builds a Context wired with a rarity registry (a visible "rare"
// tier) and an essence registry (a "fire" glyph), plus a store to spawn
// items into.
func decoRig() (*Context, *entities.Store) {
	rarity := decoration.NewRarityRegistry()
	rarity.Register(decoration.Tier{Key: "rare", Order: 30, Display: "RARE", Left: "[", Right: "]", Color: render.ThemeEntry{FG: "blue"}, Visible: true})
	ess := decoration.NewEssenceRegistry()
	ess.Register(decoration.Essence{Key: "fire", Glyph: "✦", Color: render.ThemeEntry{FG: "red"}})
	return &Context{Rarity: rarity, Essence: ess}, entities.NewStore()
}

func spawnItem(t *testing.T, store *entities.Store, props map[string]any) *entities.ItemInstance {
	t.Helper()
	tpl := &item.Template{ID: "sword", Name: "a short sword", Type: "weapon", Keywords: []string{"sword"}, Properties: props}
	it, err := store.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	return it
}

// A template-set rarity/essence property is readable on the spawned
// instance (copied to the instance bag at spawn) and resolves to its tier /
// essence (item-decorations §5 "template" case).
func TestItemDecoration_TemplateSet(t *testing.T) {
	c, store := decoRig()
	it := spawnItem(t, store, map[string]any{"rarity": "rare", "essence": "fire"})

	tier, ok := itemRarity(c, it)
	if !ok || tier.Key != "rare" {
		t.Errorf("itemRarity = %+v, %v; want rare", tier, ok)
	}
	e, ok := itemEssence(c, it)
	if !ok || e.Key != "fire" {
		t.Errorf("itemEssence = %+v, %v; want fire", e, ok)
	}
}

// An instance-set rarity (e.g. via `set property`) is read the same way —
// the §5 "instance" case.
func TestItemDecoration_InstanceSet(t *testing.T) {
	c, store := decoRig()
	it := spawnItem(t, store, nil) // no template rarity
	if _, ok := itemRarity(c, it); ok {
		t.Fatal("itemRarity hit before any property set")
	}
	it.SetProperty("rarity", "rare")
	if tier, ok := itemRarity(c, it); !ok || tier.Key != "rare" {
		t.Errorf("after SetProperty: itemRarity = %+v, %v; want rare", tier, ok)
	}
}

// decoratedName trails the name with inline rarity then essence markup.
func TestDecoratedName_Full(t *testing.T) {
	c, store := decoRig()
	it := spawnItem(t, store, map[string]any{"rarity": "rare", "essence": "fire"})
	got := decoratedName(c, it)
	want := "a short sword <item.rare>[RARE]</item.rare> <essence.fire>(✦)</essence.fire>"
	if got != want {
		t.Errorf("decoratedName = %q, want %q", got, want)
	}
}

// An undecorated item renders exactly its bare name (§1.1).
func TestDecoratedName_Bare(t *testing.T) {
	c, store := decoRig()
	it := spawnItem(t, store, nil)
	if got := decoratedName(c, it); got != "a short sword" {
		t.Errorf("decoratedName (bare) = %q, want bare name", got)
	}
}

// An unregistered key renders unset — bare name, never an error (§6).
func TestDecoratedName_UnknownKey(t *testing.T) {
	c, store := decoRig()
	it := spawnItem(t, store, map[string]any{"rarity": "mythic", "essence": "void"})
	if got := decoratedName(c, it); got != "a short sword" {
		t.Errorf("unknown keys = %q, want bare name", got)
	}
}

// nil registries disable decoration entirely (bare name).
func TestDecoratedName_NilRegistries(t *testing.T) {
	c := &Context{} // no Rarity/Essence
	store := entities.NewStore()
	it := spawnItem(t, store, map[string]any{"rarity": "rare", "essence": "fire"})
	if got := decoratedName(c, it); got != "a short sword" {
		t.Errorf("nil-registry decoratedName = %q, want bare name", got)
	}
}

// Rarity-only and essence-only items trail just that one marker.
func TestDecoratedName_SingleMarker(t *testing.T) {
	c, store := decoRig()
	rare := spawnItem(t, store, map[string]any{"rarity": "rare"})
	if got := decoratedName(c, rare); got != "a short sword <item.rare>[RARE]</item.rare>" {
		t.Errorf("rarity-only = %q", got)
	}
	ess := spawnItem(t, store, map[string]any{"essence": "fire"})
	if got := decoratedName(c, ess); got != "a short sword <essence.fire>(✦)</essence.fire>" {
		t.Errorf("essence-only = %q", got)
	}
}
