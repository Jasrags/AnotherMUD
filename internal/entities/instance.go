package entities

import (
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/srckey"
)

// Reserved property keys with engine-defined semantics. Listed here
// because they participate in §2.3 instantiation rules.
const (
	// PropTemplateID is the property key that records which template
	// an instance was built from (spec §2.3 step 5).
	PropTemplateID = "template_id"
	// PropModifiers is the transient property storing the per-instance
	// stat modifiers tagged by entity id (spec §2.3 step 6). Rebuilt
	// from the template on reload, not persisted (spec §2.4).
	PropModifiers = "modifiers"
	// PropRoomID is filtered from templates at instantiation time
	// (spec §2.3 step 4) — templates do not get to dictate where their
	// instances are created.
	PropRoomID = "room_id"
)

// SourceKey is the modifier-source convention from §2.3 step 6 and
// §3.3 step 6: every modifier the equipment subsystem applies must
// carry a source that uniquely identifies the item instance, so unequip
// can reverse exactly the right set.
//
// The type now lives in the leaf package internal/srckey so that stats
// and progression can depend on it without importing entities (which
// would block entities from importing progression). This alias keeps
// every existing entities.SourceKey caller working unchanged.
type SourceKey = srckey.SourceKey

// EquipmentSourceKey returns the source key used when equipment
// applies an item's stat modifiers to its holder. Centralized so the
// equip and unequip paths cannot drift apart. Thin wrapper over
// srckey.Equipment so the EntityID-typed call sites stay ergonomic.
func EquipmentSourceKey(id EntityID) SourceKey {
	return srckey.Equipment(string(id))
}

// InstanceModifier is one source-tagged stat modifier carried on an
// ItemInstance until equip time. The Source field is set at
// instantiation so equip applies it under a stable key.
type InstanceModifier struct {
	Stat   string
	Value  int
	Source SourceKey
}

// ItemInstance is a live item built from an item.Template. The
// Properties bag is per-instance: it starts as a normalized copy of the
// template's properties (with PropRoomID filtered, PropTemplateID set)
// and may be mutated by gameplay (e.g. fill amount, condition).
//
// Tags and Keywords are likewise per-instance copies of the template's
// lists so per-instance retag does not bleed into the template.
type ItemInstance struct {
	id       EntityID
	typ      string
	name     string
	tags     []string
	keywords []string
	// propsMu guards properties against concurrent access. Like
	// MobInstance (see its propsMu doc), the map is read on the command
	// goroutine (consume/fill/shop/quest reads) and written there too,
	// but a tick-goroutine reader/writer is now plausible (an item DoT,
	// decay sweep, or charge regen) — and Go maps are not safe under
	// concurrent access even for disjoint keys. All property access goes
	// through Properties / Property / SetProperty, each holding the
	// appropriate lock. Covers properties ONLY (tags/keywords are
	// write-once at construction).
	propsMu    sync.RWMutex
	properties map[string]any
	modifiers  []InstanceModifier
	templateID item.TemplateID
}

// ID implements Entity.
func (it *ItemInstance) ID() EntityID { return it.id }

// Type implements Entity.
func (it *ItemInstance) Type() string { return it.typ }

// Tags implements Entity. Returns a fresh slice so callers cannot
// alias the backing storage; required for safe coexistence with the
// Store's tag index (see Entity.Tags doc).
func (it *ItemInstance) Tags() []string {
	return append([]string(nil), it.tags...)
}

// Name returns the display name. Per §2.3, instantiated entities take
// their name from the template at construction time.
func (it *ItemInstance) Name() string { return it.name }

// Keywords returns the per-instance keyword list (used by the keyword
// resolver, §6). Returns a fresh slice so callers cannot alias the
// backing storage — mirrors Tags() on the same type for consistency.
func (it *ItemInstance) Keywords() []string {
	return append([]string(nil), it.keywords...)
}

// Properties returns a SNAPSHOT of the per-instance property bag, not
// the live map. Callers that need to mutate MUST use SetProperty — the
// returned map is detached and writes to it do not flow back. Returning
// a copy under RLock (rather than the live map) closes the m11-5 race:
// a concurrent goroutine touching the same item's properties no longer
// corrupts the underlying hashmap. Mirrors MobInstance.Properties().
//
// Snapshot cost is O(n) per call; hot single-key readers should use
// Property(key), which avoids the copy.
//
// Reserved keys remain off-limits to mutation: PropTemplateID is set at
// instantiation (§2.3 step 5) and identifies the recipe for stacking,
// persistence, and loot listeners; PropRoomID is filtered at
// instantiation and must never be re-added.
func (it *ItemInstance) Properties() map[string]any {
	it.propsMu.RLock()
	defer it.propsMu.RUnlock()
	if len(it.properties) == 0 {
		return nil
	}
	out := make(map[string]any, len(it.properties))
	for k, v := range it.properties {
		out[k] = v
	}
	return out
}

// Property reads a single property by key under RLock. Returns
// (zero, false) on miss. Use on hot paths where the Properties()
// snapshot allocation is wasteful (e.g. the consume / fill / shop
// readers that pull one key at a time).
func (it *ItemInstance) Property(key string) (any, bool) {
	it.propsMu.RLock()
	defer it.propsMu.RUnlock()
	v, ok := it.properties[key]
	return v, ok
}

// SetProperty writes a property under Lock, replacing any prior value.
// The map is lazy-initialized. Callers MUST NOT write the reserved keys
// (PropTemplateID / PropRoomID) — doing so is a programming error.
func (it *ItemInstance) SetProperty(key string, value any) {
	it.propsMu.Lock()
	defer it.propsMu.Unlock()
	if it.properties == nil {
		it.properties = make(map[string]any)
	}
	it.properties[key] = value
}

// ClaimIntProperty atomically reads an int property and resets it to
// zero under the property write lock, returning the prior value (0 when
// absent or not an int). It is the single-winner primitive for a
// resource two goroutines may try to claim at once — e.g. a corpse's
// coin pile looted concurrently after its ownership window opens: only
// one caller can observe a non-zero amount.
func (it *ItemInstance) ClaimIntProperty(key string) int {
	it.propsMu.Lock()
	defer it.propsMu.Unlock()
	if it.properties == nil {
		return 0
	}
	v, _ := it.properties[key].(int)
	if v != 0 {
		it.properties[key] = 0
	}
	return v
}

// Modifiers returns the transient per-instance stat modifiers (§2.3
// step 6). Equip-time application reads this list; nothing else writes
// to it post-Spawn.
func (it *ItemInstance) Modifiers() []InstanceModifier { return it.modifiers }

// TemplateID returns the source template id (§2.3 step 5).
func (it *ItemInstance) TemplateID() item.TemplateID { return it.templateID }

// normalizeProperties recursively coerces any nested map[any]any (the
// yaml.v3 default for inner maps) to map[string]any so downstream code
// only ever sees typed dictionaries. Spec §2.3 step 4.
func normalizeProperties(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = normalizeValue(v)
	}
	return out
}

func normalizeValue(v any) any {
	switch m := v.(type) {
	case map[string]any:
		return normalizeProperties(m)
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, vv := range m {
			ks, ok := k.(string)
			if !ok {
				// Non-string keys are dropped: spec §2.3 step 4 talks
				// about "typed string-keyed dictionaries." A non-string
				// key has no place in a property bag downstream code
				// expects to treat as a string-keyed map.
				continue
			}
			out[ks] = normalizeValue(vv)
		}
		return out
	case []any:
		out := make([]any, len(m))
		for i, vv := range m {
			out[i] = normalizeValue(vv)
		}
		return out
	default:
		return v
	}
}

// buildInstanceFromTemplate is the §2.3 instantiation algorithm without
// the id assignment or tracking — those belong to the Store so id
// generation stays under the store's lock.
func buildInstanceFromTemplate(tpl *item.Template, id EntityID) *ItemInstance {
	props := normalizeProperties(tpl.Properties)
	delete(props, PropRoomID)              // §2.3 step 4: never honor a template-supplied room_id.
	props[PropTemplateID] = string(tpl.ID) // §2.3 step 5.

	// §2.3 step 2: tags from the template, minus the implicit tag that
	// matches the entity's own type (which is implied and never
	// re-applied).
	tags := make([]string, 0, len(tpl.Tags))
	for _, t := range tpl.Tags {
		if t == tpl.Type {
			continue
		}
		tags = append(tags, t)
	}

	// §2.3 step 3: copy keywords.
	keywords := append([]string(nil), tpl.Keywords...)

	// §2.3 step 6: build modifier list tagged by the fresh entity id.
	src := SourceKey("entity:" + string(id))
	mods := make([]InstanceModifier, 0, len(tpl.Modifiers))
	for _, m := range tpl.Modifiers {
		mods = append(mods, InstanceModifier{Stat: m.Stat, Value: m.Value, Source: src})
	}

	return &ItemInstance{
		id:         id,
		typ:        tpl.Type,
		name:       tpl.Name,
		tags:       tags,
		keywords:   keywords,
		properties: props,
		modifiers:  mods,
		templateID: tpl.ID,
	}
}
