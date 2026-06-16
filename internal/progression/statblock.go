// Package progression owns the progression substrate: stat block,
// races, classes, tracks, alignment, and training (docs/specs/
// progression.md).
//
// M8.1 ships the StatBlock — the cached, modifier-aware container
// holding an entity's base attributes plus a sourced modifier list.
// Later slices fill in tracks (M8.2), races (M8.3), classes (M8.4),
// alignment (M8.5), and training (M8.6).
//
// StatBlock subsumes the surface today's combat.Stats + combat.Vitals
// expose for derivation purposes. Vitals (HP) remain the
// hit-point-runtime truth — the combat round writes them every swing
// through a tight mutex — so StatBlock holds only the maxima
// (hp_max, resource_max, movement_max). When a max-affecting modifier
// changes, callers read the new Effective max and apply it to
// combat.Vitals.SetMax, which already clamps current HP.
//
// The modifier set is the same shape as the M5.6 stats.Block: keyed
// by sources like `equipment:<entity id>` / `effect:<effect id>`,
// with remove-by-source as the canonical lifecycle. StatBlock wraps
// a *stats.Block internally so the persistence shape (stats.Snapshot)
// continues to round-trip unchanged.
package progression

import (
	"sort"
	"strings"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/srckey"
	"github.com/Jasrags/AnotherMUD/internal/stats"
)

// StatType is the string identity of a stat. The package documents
// the canonical attribute set (§2.1) — six classics plus three vital
// maxima — but the surface accepts any stat name so callers that
// haven't yet been folded into the canonical set (combat's hit_mod
// and ac today) can keep working until later slices narrow them.
//
// Stat names are lowercase by convention; the StatDisplayNames
// registry maps them to display strings.
type StatType string

// Canonical attribute names (§2.1). The six classic attributes plus
// three vital-pool maxima. Stored as a typed-string constant rather
// than an enum so YAML round-trips cleanly and unknown stat names
// (`hit_mod`, `ac`, future scripted stats) coexist with the canonical
// set without a registry roundtrip.
const (
	StatSTR  StatType = "str"
	StatINT  StatType = "int"
	StatWIS  StatType = "wis"
	StatDEX  StatType = "dex"
	StatCON  StatType = "con"
	StatLUCK StatType = "luck"

	StatHPMax       StatType = "hp_max"
	StatResourceMax StatType = "resource_max"
	StatMovementMax StatType = "movement_max"
	// StatCarryMax is the personal carry-weight ceiling: the summed
	// weight of carried items may not exceed it on pickup
	// (inventory-equipment-items §4.2 step 2). A non-positive value means
	// "no limit", so content opts in by declaring it (the Strength-derived
	// capacity WoT encumbrance will set).
	StatCarryMax StatType = "carry_max"
)

// Combat-derived stat names. The progression spec §2.1 keeps the
// canonical base attribute set tight (the six classics + three
// vital maxima). HitMod and AC are *derived* values that the spec
// expects abilities + equipment + class hooks to compute downstream.
// M8.1 carries them as additional stat keys on the StatBlock so the
// player default builder + every consumer keep working under the
// unified surface; later slices (M8.4 stat growth, M8.6 training)
// will move the derivation into proper hooks.
const (
	StatHitMod StatType = "hit_mod"
	StatAC     StatType = "ac"
)

// DefaultPlayerBase returns the engine-default base attribute map
// applied to a brand-new player. The numbers are the M7.1
// hardcoded defaults (HitMod=0, AC=10, STR=10, hp_max=20) lifted
// into the StatBlock-shaped surface; M8.3 (races) and M8.4 (class
// stat growth) will replace them with derivation from race +
// class + level.
//
// The six classic attributes are seeded at 10 even though only STR
// is read by combat today, so the M8.4 stat-growth handler has a
// non-zero baseline to apply growth dice to without first having to
// roll a character-creation initial roll. The resource (mana / One
// Power) max stays zero — only a channeler class grants that. The
// movement max is a flat baseline every character carries: travel now
// spends it (the movement-cost gate in the move command), so a zero
// pool would strand a character. Balance (final size, regen rate,
// biome-weighted cost) is deliberately rough at this stage.
func DefaultPlayerBase() map[StatType]int {
	return map[StatType]int{
		StatSTR:         10,
		StatINT:         10,
		StatWIS:         10,
		StatDEX:         10,
		StatCON:         10,
		StatLUCK:        10,
		StatHPMax:       20,
		StatMovementMax: DefaultMovementMax,
		StatHitMod:      0,
		StatAC:          10,
	}
}

// DefaultMovementMax is the baseline movement pool every new character
// starts with. With a flat per-step cost of 1 it buys a stretch of
// uninterrupted travel before the mover is winded; out-of-combat regen
// (economy.RegenConfig.BaseMovement) refills it between bouts. A starter
// figure, not a balanced one.
const DefaultMovementMax = 30

// ModifierType discriminates modifier application kinds. M8.1 supports
// only Flat (additive). The discriminator is present so percent /
// multiplicative / "set to" kinds can be added later without breaking
// the existing surface (spec §2.4). Today every modifier is treated
// as Flat regardless of the field value — but stored faithfully so a
// future upgrade can interpret older saves correctly.
type ModifierType uint8

const (
	// ModFlat is an additive (base + value) modifier. Default zero
	// value so legacy modifiers persisted under stats.Modifier (which
	// has no type field) are interpreted as Flat without migration.
	ModFlat ModifierType = 0
)

// StatBlock holds base attributes plus a sourced modifier set, and
// caches effective (base + sum-of-modifiers) reads behind a dirty
// flag. Safe for concurrent use; the RWMutex is internal so combat
// can call Effective from a tick goroutine while a session goroutine
// equip/unequips without coordinating an outer lock.
//
// The lock NESTS INSIDE the actor lock on the player side: session
// code holds a.mu while calling AddModifiers / RemoveBySource. No
// method on StatBlock calls back out into the actor.
type StatBlock struct {
	mu sync.RWMutex

	// base holds the entity's intrinsic attribute values before any
	// modifier is applied. Created lazily — an absent key reads as
	// zero, so the canonical attribute set does not have to be
	// pre-populated at construction.
	base map[StatType]int

	// mods is the sourced modifier set (the M5.6 stats.Block surface).
	// Composed rather than re-implemented so persistence shape
	// (stats.Snapshot) and existing equip/unequip plumbing keep
	// working through StatBlock's accessors.
	mods *stats.Block

	// effective is the lazy cache populated by Effective when dirty
	// is true. Recomputed in full on every miss — keeps the
	// invalidation rule simple ("any mutation invalidates") at the
	// cost of a per-mutation O(modifier count) recompute that's well
	// inside the noise of a tick at M8.1 scale.
	effective map[StatType]int

	// dirty signals that effective needs a rebuild. Set by every
	// base setter and every modifier mutation. Cleared in Effective
	// after a successful recompute.
	dirty bool

	// maxListeners holds OnMaxChange subscribers keyed by stat. Used
	// by combat.Vitals to keep `current/max HP` consistent when a
	// max-affecting effect or stat change recomputes (M14.1, spec
	// progression "Vital re-clamp under max-affecting stat
	// recompute"). nil when no consumer has registered — keeps the
	// notification path a fast no-op for paths that don't care.
	maxListeners map[StatType][]MaxChangeListener

	// lastWatched caches the most recent effective value for every
	// stat that has at least one OnMaxChange listener. Lets the
	// notifier compare new vs. old without re-running the diff
	// against an external source of truth.
	lastWatched map[StatType]int
}

// MaxChangeListener is called when a stat's effective value changes
// AND the stat has been registered via StatBlock.OnMaxChange. Fired
// outside the StatBlock lock so the listener may safely acquire
// other locks (e.g., combat.Vitals.SetMax which takes Vitals.mu);
// listeners MUST NOT call back into the same StatBlock or take any
// lock held by the StatBlock caller — the notification fires on the
// mutating goroutine after the mutation lands.
//
// Spec: progression "Vital re-clamp" (M14.1).
type MaxChangeListener func(oldMax, newMax int)

// New returns an empty StatBlock with no base attributes and no
// modifiers. Callers populate base via SetBase or the WithBase
// constructor.
func New() *StatBlock {
	return &StatBlock{
		base: make(map[StatType]int),
		mods: stats.New(),
	}
}

// NewWithBase returns a StatBlock seeded with the given base
// attributes. The input map is copied — callers can mutate the
// original without affecting the block.
func NewWithBase(base map[StatType]int) *StatBlock {
	b := New()
	for k, v := range base {
		b.base[k] = v
	}
	return b
}

// Base returns the intrinsic value of stat before any modifier is
// applied. Returns zero for an unset stat — the canonical set is not
// required to be populated for unrelated stats to read as zero.
func (b *StatBlock) Base(stat StatType) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.base[stat]
}

// SetBase writes the intrinsic value of stat. Invalidates the
// effective cache.
func (b *StatBlock) SetBase(stat StatType, value int) {
	b.mu.Lock()
	b.base[stat] = value
	b.dirty = true
	deferred := b.notifyMaxLocked()
	b.mu.Unlock()
	fireDeferred(deferred)
}

// AdjustBase increments the intrinsic value of stat by delta (which
// may be negative). Used by the M8.4 stat-growth handler at level-up
// and by the M8.6 train command. Invalidates the effective cache.
// Returns the new base value.
func (b *StatBlock) AdjustBase(stat StatType, delta int) int {
	b.mu.Lock()
	b.base[stat] += delta
	b.dirty = true
	newBase := b.base[stat]
	deferred := b.notifyMaxLocked()
	b.mu.Unlock()
	fireDeferred(deferred)
	return newBase
}

// Effective returns the cached effective value (`base + sum of
// modifiers`) for stat. Recomputes the cache on demand when the
// dirty flag is set; otherwise reads from the cache under a read
// lock so concurrent callers do not serialize on a hot path.
//
// The read-then-upgrade pattern (drop RLock, take Lock if dirty)
// has a benign race: another goroutine may invalidate between the
// RUnlock and the Lock. That's intentional — recomputeLocked
// re-checks dirty under the write lock, so the worst case is one
// thread does the recompute and another finds it already clean.
// No correctness hazard.
func (b *StatBlock) Effective(stat StatType) int {
	b.mu.RLock()
	if !b.dirty && b.effective != nil {
		v := b.effective[stat]
		b.mu.RUnlock()
		return v
	}
	b.mu.RUnlock()

	b.mu.Lock()
	defer b.mu.Unlock()
	b.recomputeLocked()
	return b.effective[stat]
}

// AllEffective returns a copy of every (stat → effective value) the
// block knows about. Useful for renderers (`score`, GMCP) that want
// to draw the whole block in one pass; cheaper than calling
// Effective per stat under the lock churn.
func (b *StatBlock) AllEffective() map[StatType]int {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.recomputeLocked()
	out := make(map[StatType]int, len(b.effective))
	for k, v := range b.effective {
		out[k] = v
	}
	return out
}

// AddModifier installs a single modifier under src. Replaces any
// existing entry under the same source (consistent with stats.Block
// semantics: a fresh equip is a full replacement, not an append).
// Invalidates the effective cache.
func (b *StatBlock) AddModifier(src srckey.SourceKey, stat StatType, value int) {
	b.AddModifiers(src, []stats.Modifier{{Stat: string(stat), Value: value}})
}

// AddModifiers installs the given modifier list under src. Replaces
// any existing entry under the same source. An empty list removes
// the entry (mirrors stats.Block.Apply). Invalidates the effective
// cache.
//
// Stat names are normalized to lowercase on ingress: an item
// template that declares `{stat: STR, value: 2}` and one that
// declares `{stat: str, value: 2}` are semantically identical and
// both contribute to Effective(StatSTR). Without normalization a
// case-typo would silently produce a zero-contribution modifier —
// the kind of bug that surfaces only when a content author wonders
// "why doesn't my +2 STR sword work."
func (b *StatBlock) AddModifiers(src srckey.SourceKey, mods []stats.Modifier) {
	normalized := mods
	if len(mods) > 0 {
		normalized = make([]stats.Modifier, len(mods))
		for i, m := range mods {
			normalized[i] = stats.Modifier{
				Stat:  strings.ToLower(m.Stat),
				Value: m.Value,
			}
		}
	}
	b.mu.Lock()
	b.mods.Apply(src, normalized)
	b.dirty = true
	deferred := b.notifyMaxLocked()
	b.mu.Unlock()
	fireDeferred(deferred)
}

// RemoveBySource drops every modifier installed under src in one
// operation (spec §2.4: "remove-by-source is the canonical lifecycle
// pattern"). Returns true if anything was removed. Invalidates the
// effective cache only when something actually changed.
func (b *StatBlock) RemoveBySource(src srckey.SourceKey) bool {
	b.mu.Lock()
	if !b.mods.Remove(src) {
		b.mu.Unlock()
		return false
	}
	b.dirty = true
	deferred := b.notifyMaxLocked()
	b.mu.Unlock()
	fireDeferred(deferred)
	return true
}

// HasSource reports whether any modifiers are installed under src.
func (b *StatBlock) HasSource(src srckey.SourceKey) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.mods.Has(src)
}

// RebindSource moves the modifier set from old to new in place,
// invalidating the cache. Used at login to reattach a persisted set
// to a freshly-spawned item's new runtime id (inventory-equipment-
// items §3.5). Returns false on the same conditions as
// stats.Block.RebindSource: missing old set, or new already
// occupied.
func (b *StatBlock) RebindSource(old, new srckey.SourceKey) bool {
	b.mu.Lock()
	if !b.mods.RebindSource(old, new) {
		b.mu.Unlock()
		return false
	}
	b.dirty = true
	deferred := b.notifyMaxLocked()
	b.mu.Unlock()
	fireDeferred(deferred)
	return true
}

// Invalidate forces the effective cache to be rebuilt on the next
// read. Exposed (per spec §2.2) so callers that touch the block
// through alternate paths — e.g. the M8.6 train command directly
// adjusting a base attribute through an external mutator — can
// guarantee subsequent reads see the new value. Most paths don't
// need it: SetBase / AdjustBase / AddModifier* / RemoveBySource /
// RebindSource all invalidate automatically.
func (b *StatBlock) Invalidate() {
	b.mu.Lock()
	b.dirty = true
	deferred := b.notifyMaxLocked()
	b.mu.Unlock()
	fireDeferred(deferred)
}

// ModifiersSnapshot returns the persisted shape of the sourced
// modifier set — the same stats.Snapshot the M5.6 save loop
// already writes. Used by player.Save to keep the on-disk shape
// stable across the M8.1 surface change.
func (b *StatBlock) ModifiersSnapshot() stats.Snapshot {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.mods.Snapshot()
}

// RestoreModifiers replaces the sourced modifier set from a
// stats.Snapshot. Used at login to install the persisted set before
// equipment respawn rebinds source keys onto fresh entity ids.
// Stat names are lowercased on ingress to match the AddModifiers
// normalization — pre-M8.1 saves that round-tripped mixed-case
// modifier names are corrected on first load. Invalidates the
// effective cache.
func (b *StatBlock) RestoreModifiers(snap stats.Snapshot) {
	normalized := snap
	if len(snap) > 0 {
		normalized = make(stats.Snapshot, len(snap))
		for i, e := range snap {
			mods := e.Modifiers
			if len(mods) > 0 {
				mods = make([]stats.Modifier, len(e.Modifiers))
				for j, m := range e.Modifiers {
					mods[j] = stats.Modifier{
						Stat:  strings.ToLower(m.Stat),
						Value: m.Value,
					}
				}
			}
			normalized[i] = stats.Entry{Source: e.Source, Modifiers: mods}
		}
	}
	b.mu.Lock()
	b.mods.Restore(normalized)
	b.dirty = true
	deferred := b.notifyMaxLocked()
	b.mu.Unlock()
	fireDeferred(deferred)
}

// BaseSnapshot returns a deterministically-ordered serialization of
// the base attributes. Used by player.Save to persist the base block
// (M8.1 save v6).
func (b *StatBlock) BaseSnapshot() BaseSnapshot {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if len(b.base) == 0 {
		return nil
	}
	keys := make([]string, 0, len(b.base))
	for k := range b.base {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	out := make(BaseSnapshot, 0, len(keys))
	for _, k := range keys {
		out = append(out, BaseEntry{Stat: StatType(k), Value: b.base[StatType(k)]})
	}
	return out
}

// RestoreBase merges the snapshot into the base attributes. Used at
// login to apply the persisted base block over whatever defaults
// the constructor (NewWithBase) seeded. Keys in the snapshot
// overwrite existing entries; keys absent from the snapshot are
// preserved.
//
// Merge rather than replace is deliberate: a v6 save written before
// a future slice adds a new base stat (say a Vth vital pool) loads
// without that key — replace semantics would silently zero it,
// whereas merge keeps the engine default seeded at construction.
// Forward-compatible by default; a future migration that genuinely
// wants to clear a stat must do so explicitly via SetBase.
//
// Invalidates the effective cache.
func (b *StatBlock) RestoreBase(snap BaseSnapshot) {
	b.mu.Lock()
	if b.base == nil {
		b.base = make(map[StatType]int, len(snap))
	}
	for _, e := range snap {
		b.base[e.Stat] = e.Value
	}
	b.dirty = true
	deferred := b.notifyMaxLocked()
	b.mu.Unlock()
	fireDeferred(deferred)
}

// OnMaxChange registers a listener to fire whenever the effective
// value of stat changes from the previously-observed value. The
// initial effective value is snapshotted into lastWatched at
// registration time; the listener fires on the NEXT change, not
// for the registration itself.
//
// Callers register one listener per (stat, consumer). Multiple
// listeners on the same stat fire in registration order, all
// outside the StatBlock lock.
//
// Spec: progression "Vital re-clamp under max-affecting stat
// recompute" (M14.1).
func (b *StatBlock) OnMaxChange(stat StatType, fn MaxChangeListener) {
	if fn == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.maxListeners == nil {
		b.maxListeners = make(map[StatType][]MaxChangeListener)
	}
	if b.lastWatched == nil {
		b.lastWatched = make(map[StatType]int)
	}
	b.maxListeners[stat] = append(b.maxListeners[stat], fn)
	// Snapshot the current effective so the first real change has
	// a baseline. Force a recompute if dirty so the baseline is
	// not stale.
	b.recomputeLocked()
	b.lastWatched[stat] = b.effective[stat]
}

// notifyMaxLocked walks every watched stat, compares the freshly
// recomputed effective value against lastWatched, and returns a
// slice of zero-arg functions the caller invokes after releasing
// b.mu. Returns nil when no listeners are registered — keeps the
// notification path a no-op for the common case.
//
// Callers MUST hold b.mu write-locked.
func (b *StatBlock) notifyMaxLocked() []func() {
	if len(b.maxListeners) == 0 {
		return nil
	}
	b.recomputeLocked()
	var out []func()
	for stat, listeners := range b.maxListeners {
		newVal := b.effective[stat]
		oldVal := b.lastWatched[stat]
		if newVal == oldVal {
			continue
		}
		b.lastWatched[stat] = newVal
		// go 1.22+ gives each iteration its own l; no shadow
		// needed. oldVal/newVal are int and captured by value.
		for _, l := range listeners {
			out = append(out, func() { l(oldVal, newVal) })
		}
	}
	return out
}

// fireDeferred runs every callback in the slice. Safe to call with
// a nil slice. Always called AFTER releasing b.mu.
func fireDeferred(out []func()) {
	for _, fn := range out {
		fn()
	}
}

// recomputeLocked rebuilds the effective cache from scratch.
// Callers MUST hold b.mu write-locked.
func (b *StatBlock) recomputeLocked() {
	if !b.dirty && b.effective != nil {
		return
	}
	out := make(map[StatType]int, len(b.base))
	for k, v := range b.base {
		out[k] = v
	}
	// stats.Block.Snapshot copies the modifier set under its own
	// internal lock — no nested-lock hazard, but it's a deep copy on
	// the hot path. Acceptable at M8.1 scale (modifiers per holder
	// are O(equipped items + active effects), typically <20).
	for _, e := range b.mods.Snapshot() {
		for _, m := range e.Modifiers {
			out[StatType(m.Stat)] += m.Value
		}
	}
	b.effective = out
	b.dirty = false
}

// BaseEntry is one (stat, value) pair in the persisted base block.
type BaseEntry struct {
	Stat  StatType `yaml:"stat"`
	Value int      `yaml:"value"`
}

// BaseSnapshot is the persisted shape of a StatBlock's base
// attributes — an ordered list of (stat, value) entries. A list
// (not a map) so YAML round-trips preserve order and small base
// blocks read deterministically in diffs.
type BaseSnapshot []BaseEntry
