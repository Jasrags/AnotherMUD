// Package stats owns the holder-side modifier set per
// inventory-equipment-items §3.3 step 6 and §3.5: a sourced modifier
// map that equip writes into and unequip reverses by source.
//
// M5.6 scope is deliberately minimal — there is no derived attribute
// system here, no rules layer, no "current value of strength." The
// Block is a bag of (source → [(stat, value)...]) entries. Equip adds
// one entry per item under EquipmentSourceKey(item.ID()); unequip
// removes the entry by the same key. The real stat system, deriving
// attributes from class/race/level/effects, lands with M8 progression.
//
// Source keys cross the save/load boundary (§3.5). On reload, an item
// instance is respawned with a fresh entity id; the Block carries the
// modifier set persisted from the previous session under the *old*
// entity id. RebindSource closes that gap by reassigning a source key
// in place — see session.respawnEquipment for the call site.
package stats

import (
	"sort"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// Modifier is one (stat, value) pair applied under some source. The
// source is the map key in Block, not a field, so a single source can
// own many modifiers (a weapon that boosts both str and dex) and one
// removal call reverses the whole set atomically.
type Modifier struct {
	Stat  string `yaml:"stat"`
	Value int    `yaml:"value"`
}

// Block is the per-holder sourced modifier set. Safe for concurrent
// use; the mutex is internal so callers (session.connActor) don't have
// to coordinate with their own lock for stat mutations.
//
// Block locks NEST INSIDE the actor lock: the session actor holds its
// own mutex while calling Apply/Remove/RebindSource during equip and
// unequip. No method on Block calls back out into the actor.
type Block struct {
	mu   sync.Mutex
	bySrc map[entities.SourceKey][]Modifier
}

// New returns an empty Block.
func New() *Block {
	return &Block{bySrc: make(map[entities.SourceKey][]Modifier)}
}

// Apply installs mods under src. Idempotent in the sense that
// repeating the call with the same src replaces (does not append) —
// equip is supposed to be a fresh application, so a stale set under
// the same key is overwritten rather than doubled.
func (b *Block) Apply(src entities.SourceKey, mods []Modifier) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(mods) == 0 {
		delete(b.bySrc, src)
		return
	}
	out := make([]Modifier, len(mods))
	copy(out, mods)
	b.bySrc[src] = out
}

// Remove drops the modifier set under src. Returns true if anything
// was removed (lets unequip distinguish "actually had modifiers" from
// "item carried none" for diagnostics).
func (b *Block) Remove(src entities.SourceKey) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.bySrc[src]; !ok {
		return false
	}
	delete(b.bySrc, src)
	return true
}

// RebindSource moves the modifier set from old to new in place. Used
// at login to reattach a persisted set to a freshly-spawned item's new
// runtime id (§3.5). No-op if old has no entries. If new already has
// entries, the call FAILS by returning false rather than overwriting
// — a collision would mean two items are claiming the same source,
// which is a programming error worth surfacing rather than silently
// merging.
func (b *Block) RebindSource(old, new entities.SourceKey) bool {
	if old == new {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	mods, ok := b.bySrc[old]
	if !ok {
		return false
	}
	if _, exists := b.bySrc[new]; exists {
		return false
	}
	b.bySrc[new] = mods
	delete(b.bySrc, old)
	return true
}

// Has reports whether any modifiers are installed under src.
func (b *Block) Has(src entities.SourceKey) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.bySrc[src]
	return ok
}

// Snapshot returns a deterministically-ordered serialization of the
// block, suitable for persistence. Source keys are sorted so the YAML
// output is stable across saves — diffs of saves on disk should be
// driven by gameplay changes, not map-iteration order.
func (b *Block) Snapshot() Snapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.bySrc) == 0 {
		return nil
	}
	keys := make([]string, 0, len(b.bySrc))
	for k := range b.bySrc {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	out := make(Snapshot, 0, len(keys))
	for _, k := range keys {
		src := entities.SourceKey(k)
		mods := b.bySrc[src]
		dup := make([]Modifier, len(mods))
		copy(dup, mods)
		out = append(out, Entry{Source: src, Modifiers: dup})
	}
	return out
}

// Restore replaces the block's contents with snap. Used at login to
// install the persisted set before equipment respawn rebinds source
// keys onto fresh entity ids.
func (b *Block) Restore(snap Snapshot) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.bySrc = make(map[entities.SourceKey][]Modifier, len(snap))
	for _, e := range snap {
		if len(e.Modifiers) == 0 {
			continue
		}
		dup := make([]Modifier, len(e.Modifiers))
		copy(dup, e.Modifiers)
		b.bySrc[e.Source] = dup
	}
}

// Entry is one source's modifier set in serialized form.
type Entry struct {
	Source    entities.SourceKey `yaml:"source"`
	Modifiers []Modifier         `yaml:"modifiers"`
}

// Snapshot is the persisted shape of a Block — an ordered list of
// (source, modifiers) entries. A list (not a map) so YAML round-trips
// preserve order and source keys with colons (e.g. "equipment:entity-1")
// don't tangle with YAML's key-parsing quirks.
type Snapshot []Entry
