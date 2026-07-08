// Package stacking groups an entity's contents into stack entries for
// display ("3 healing potions" instead of three identical lines), per
// docs/specs/inventory-equipment-items.md §5.
//
// Stacking is PRESENTATION ONLY. The underlying items are never merged:
// each keeps its own id, its position in contents, and any per-instance
// state. The service is a read-only grouping over a contents slice — it
// mutates nothing — and the caller (a display handler) formats each entry.
//
// A StackEntry carries the rarity/essence KEY strings (not formatted
// markup), so this package stays decoupled from the decoration registries:
// the display layer resolves those keys through render/decoration when it
// builds the line.
package stacking

import (
	"slices"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// Reserved item-property names that participate in stack identity or the
// entry payload (item-decorations §5). Essence is part of the stack KEY
// (§5.1); rarity is carried on the entry but does NOT split stacks (the
// §5/§9 conservative default — rarity is cosmetic). Kept in sync with the
// reserved keys the decoration display + property registry use.
const (
	essenceProperty = "essence"
	rarityProperty  = "rarity"
)

// keyDelim separates the components of a composite stack key. It is the
// ASCII Unit Separator (0x1F), NOT the "|" the spec uses illustratively:
// the key is internal (never shown or persisted), and a printable delimiter
// could collide when a key/value component itself contains it (a pack
// essence key, or an admin-set property value, may contain "|" — neither is
// validated against it). A control char can't appear in those, so distinct
// component tuples always map to distinct keys.
const keyDelim = "\x1f"

// StackEntry is one group of stack-identical items (§5.2). Quantity equals
// len(ItemIDs); the slice is in stack (first-seen) order. DisplayName,
// RarityKey, and EssenceKey are taken from the FIRST item of the stack.
type StackEntry struct {
	TemplateID  string // shared template id; empty for template-less items
	DisplayName string // the first item's name
	Quantity    int    // count of items in the stack (== len(ItemIDs))
	RarityKey   string // the first item's rarity property (empty if absent)
	EssenceKey  string // the first item's essence property (empty if absent)
	ItemIDs     []entities.EntityID
}

// Service groups contents into stacks. It carries the pack-registered extra
// stack keys (§5.1 add-key hook): property names whose values must ALSO
// match before two items stack. The zero value is unusable — use NewService.
type Service struct {
	extraKeys []string // property names, in registration order
}

// NewService returns a stacking service with no extra keys (the engine
// default: stacks key on template id + essence only).
func NewService() *Service {
	return &Service{}
}

// AddKey registers propertyName as an additional stack-key component
// (§5.1): two items then stack only if this property's value also matches.
// Each registered key contributes `|<value>` to the stack key in
// registration order (empty string when the property is absent). A blank
// name, or a name already registered, is ignored — so the key set is
// stable and a property never contributes twice.
func (s *Service) AddKey(propertyName string) {
	name := strings.TrimSpace(propertyName)
	if name == "" {
		return
	}
	if slices.Contains(s.extraKeys, name) {
		return
	}
	s.extraKeys = append(s.extraKeys, name)
}

// Stack groups items into stack entries (§5.1/§5.2). Order is preserved:
// the first item of each unique stack key fixes that stack's position among
// the returned entries. Read-only — items are not mutated. A nil/empty
// input returns nil.
func (s *Service) Stack(items []*entities.ItemInstance) []StackEntry {
	if len(items) == 0 {
		return nil
	}
	entries := make([]StackEntry, 0, len(items))
	index := make(map[string]int, len(items)) // stack key → entries index
	for _, it := range items {
		if it == nil {
			continue
		}
		key := s.stackKey(it)
		if i, ok := index[key]; ok {
			entries[i].Quantity++
			entries[i].ItemIDs = append(entries[i].ItemIDs, it.ID())
			continue
		}
		index[key] = len(entries)
		entries = append(entries, StackEntry{
			TemplateID:  string(it.TemplateID()),
			DisplayName: it.Name(),
			Quantity:    1,
			RarityKey:   stringProperty(it, rarityProperty),
			EssenceKey:  stringProperty(it, essenceProperty),
			ItemIDs:     []entities.EntityID{it.ID()},
		})
	}
	return entries
}

// stackKey computes an item's stack key (§5.1). A template-less item is a
// singleton keyed by its entity id, so it never stacks with anything. A
// templated item keys on `<templateId>|<essence>|<extra…>`, with an empty
// string standing in for any absent property.
func (s *Service) stackKey(it *entities.ItemInstance) string {
	tid := string(it.TemplateID())
	if tid == "" {
		return "notemplate:" + string(it.ID())
	}
	parts := make([]string, 0, 2+len(s.extraKeys))
	parts = append(parts, tid, stringProperty(it, essenceProperty))
	for _, k := range s.extraKeys {
		parts = append(parts, stringProperty(it, k))
	}
	return strings.Join(parts, keyDelim)
}

// stringProperty reads a string-valued property, returning "" when the key
// is absent or the value is not a string.
func stringProperty(it *entities.ItemInstance, key string) string {
	v, ok := it.Property(key)
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}
