package command

import (
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/decoration"
	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// Item-decoration display helpers (M20.5, spec item-decorations §4/§5).
// An item carries its rarity / essence as reserved string properties; the
// value is the marker KEY, resolved through the Context's decoration
// registries. The property lives on the instance bag (a template-set value
// is copied there at spawn, an instance-set value via `set property`), so a
// single Property read covers both the template and instance cases (§5).
//
// Layout: rarity tag and essence glyph render INLINE, trailing the item
// name ("a short sword [RARE] (✦)"). Names stay flush-left, so an
// undecorated item renders exactly as its bare name (§1.1) and decorations
// grow rightward. (The padded-column form — decoration.Tier.PaddedMarkup /
// RarityRegistry.MaxVisibleWidth — is retained for a future column-aligned
// surface such as a shop list; inventory does not use it.)
//
// All helpers degrade to nothing when a registry is unwired, the property
// is absent/empty, or the key is unregistered — never an error (§6).

// Reserved item-property keys holding the marker key.
const (
	propRarity  = "rarity"
	propEssence = "essence"
)

// itemRarity resolves an item's rarity tier, or (zero, false) when there is
// no rarity registry, no `rarity` property, or the key is unregistered.
func itemRarity(c *Context, it *entities.ItemInstance) (decoration.Tier, bool) {
	if c.Rarity == nil {
		return decoration.Tier{}, false
	}
	key, ok := stringProp(it, propRarity)
	if !ok || key == "" {
		return decoration.Tier{}, false
	}
	return c.Rarity.Get(key)
}

// itemEssence resolves an item's essence, or (zero, false) when there is no
// essence registry, no `essence` property, or the key is unregistered.
func itemEssence(c *Context, it *entities.ItemInstance) (decoration.Essence, bool) {
	if c.Essence == nil {
		return decoration.Essence{}, false
	}
	key, ok := stringProp(it, propEssence)
	if !ok || key == "" {
		return decoration.Essence{}, false
	}
	return c.Essence.Get(key)
}

// decoratedName returns the item's display name with trailing inline
// decorations: the rarity tag then the essence glyph, each in its themed-tag
// markup, appended after the name ("a short sword [RARE] (✦)"). Blank or
// absent markers contribute nothing, so an undecorated item returns exactly
// it.Name(). The result is markup — the caller's line goes through the color
// renderer like any other output.
func decoratedName(c *Context, it *entities.ItemInstance) string {
	parts := make([]string, 0, 3)
	parts = append(parts, it.Name())
	if t, ok := itemRarity(c, it); ok {
		if m := t.InlineMarkup(); m != "" {
			parts = append(parts, m)
		}
	}
	if e, ok := itemEssence(c, it); ok {
		if m := e.Markup(); m != "" {
			parts = append(parts, m)
		}
	}
	return strings.Join(parts, " ")
}
