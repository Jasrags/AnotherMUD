package entities

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/srckey"
)

// Item-modification errors (item-modification.md §4). Callers distinguish these
// at the command boundary to emit a distinct refusal cue.
var (
	// ErrNotModifiable — the host declares no capacity budget (§2).
	ErrNotModifiable = errors.New("item is not modifiable")
	// ErrNotAModification — the item is not a modification (no host-compat key, §3).
	ErrNotAModification = errors.New("item is not a modification")
	// ErrModIncompatible — the mod's host class does not match the host (§4 step 1).
	ErrModIncompatible = errors.New("modification does not fit this host")
	// ErrModNoCapacity — the mod's cost exceeds the host's free capacity (§4 step 2).
	ErrModNoCapacity = errors.New("not enough free capacity")
	// ErrMountOccupied — no compatible mount is free (weapon-accessories.md §4).
	ErrMountOccupied = errors.New("no compatible mount is free")
)

// InstalledMod is one modification installed into a host item
// (item-modification.md §3/§7). Its effect is SNAPSHOTTED from the mod item at
// install so the equip/recompute aggregation seams need no template registry
// (they read the host's effective Modifiers/ArmorBonus/ArmorCheckPenalty/
// Resistances, §6). Recorded as durable host-instance state; the mod's template
// id is what persists (§7), the rest is re-derived from the template on load.
type InstalledMod struct {
	TemplateID   item.TemplateID
	Name         string
	CapacityCost int
	ArmorBonus   int
	ArmorCheck   int
	Resistances  map[string]int
	Modifiers    []InstanceModifier
	// Mount is the weapon mount point this accessory occupies
	// (weapon-accessories.md §4); "" for a capacity-rule armor mod.
	Mount string
	// Protection is the environmental protection keys this mod grants its host
	// while worn (item-modification §6 → biome-hazard immunity, area-effects §4.6).
	Protection []string
	// Grants is the general capability keys this mod confers on its host while
	// equipped (item-modification §6 → the smartlink↔smartgun pairing).
	Grants []string
}

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
	// PropGrade is the item's quality-grade key (masterwork §2). Seeded
	// from the template's Grade at instantiation; a runtime producer (a
	// craft) may overwrite it on the instance, where it persists with the
	// item's other instance properties (masterwork §6).
	PropGrade = "grade"
	// propLoadedRounds is the mutable instance property holding a magazine
	// weapon's current loaded-round count. Absent = a full magazine (lazy-full,
	// see MagazineLoaded); written on fire/reload and persisted with the item.
	propLoadedRounds = "loaded"
	// propInsertedHolderTpl / propInsertedHolderLoaded / propInsertedHolderGrade
	// record the ammunition holder inserted in a holder-fed weapon
	// (ammo-and-reloading §5): the holder's template id, its current round count,
	// and the grade of its rounds (grade-through-holder §8, "" if ungraded).
	// Absent = no holder inserted. Persisted with the weapon via EquippedItem.Holder.
	propInsertedHolderTpl    = "inserted_holder"
	propInsertedHolderLoaded = "inserted_holder_loaded"
	propInsertedHolderGrade  = "inserted_holder_grade"
	// propHolderAmmoGrade is the grade of the rounds loaded in a HOLDER item
	// (ammo-and-reloading §8) — homogeneous, captured at fill. "" = ungraded.
	propHolderAmmoGrade = "holder_ammo_grade"
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
	desc     string
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
	// weaponDamage is the parsed wielded-weapon dice (combat §4.5),
	// derived once from the template's WeaponDamage string at build.
	// The zero value (IsZero) means the item is not a weapon. Write-once
	// at construction like tags/keywords — no mutex needed.
	weaponDamage combat.DiceExpr
	// eligibleSlots / companionSlots are the equipment-slot eligibility
	// and footprint (inventory-equipment-items §3.3), lifted onto the
	// instance at build so the equip path reads them without a template
	// registry (R5). Empty eligibleSlots means the item is not
	// equippable. Write-once at construction — no mutex needed.
	eligibleSlots  []string
	companionSlots []string
	// weaponCategory / proficiencyTier are the weapon-identity §2 labels
	// lifted onto the instance at build so the equip path can decide
	// proficiency without a template registry (mirrors weaponDamage).
	// Empty proficiencyTier means "untiered" (treated as the lowest tier).
	// Write-once at construction — no mutex needed.
	weaponCategory  string
	proficiencyTier string
	// critThreatLow / critMultiplier are the weapon-identity §4 critical
	// parameters lifted onto the instance at build (mirrors weaponDamage).
	// Zero means unset — the combat resolver applies the engine defaults.
	critThreatLow  int
	critMultiplier int
	// weaponSize is the weapon's size category (size-and-wielding §2), lifted
	// onto the instance at build. "" = undeclared (the equip/combat paths
	// resolve it to the baseline, and the footprint falls back to the static
	// companion slots). Read to derive the wield mode against a wielder's size.
	weaponSize string
	// damageTypes are the weapon's damage type(s) (weapon-identity §2),
	// lifted onto the instance at build. nil = untyped. Read by the combat
	// path to select a defender's per-type resistance (armor-depth §4).
	damageTypes []string
	// targetPool is the pool.Kind this weapon's damage fills (shadowrun-mvp
	// SR-M3b), lifted from the template (already lowercased at load). Empty ⇒
	// the hp path. Read by the holder's Stats() builder into
	// combat.Stats.TargetPool.
	targetPool string
	// Ranged weapon metadata (ranged-combat §2), lifted onto the instance at
	// build (mirrors weaponCategory). rangedClass empty = melee; ammoKind is
	// what a projectile fires / an ammo item supplies; rangeIncrement is the
	// distance-falloff unit; strRating caps a Strength-rated bow's positive
	// Strength bonus (nil = the default no-positive-Strength projectile rule).
	// Write-once at construction — no mutex needed.
	rangedClass    string
	ammoKind       string
	rangedStyle    string
	rangeIncrement int
	fireModes      []string
	recoilComp     int
	reloadTicks    int
	magazine       int
	reloadMethod   string
	holderFits     string
	acceptsHolder  string
	strRating      *int
	// resistances are the armor's per-damage-type damage reduction
	// (armor-depth §4), keyed by damage type. nil = none. Aggregated across
	// worn armor into the holder's combat.Stats.Resistances at Stats() time.
	resistances map[string]int
	// armorCheckPenalty is the magnitude (non-negative) of the penalty this
	// armor imposes on Str/Dex skill checks while worn (armor-depth §6). 0 =
	// none. The equip path applies the grade-reduced penalty as an armor_check
	// stat modifier the skill check subtracts.
	armorCheckPenalty int
	// armorBonus is the item's structured additive AC contribution
	// (armor-depth §3), applied as an `ac` stat modifier at equip. 0 = none.
	// Distinct from a legacy `{stat: ac}` modifier (a "misc" AC source): both
	// raise AC, but armor_bonus is the named armor term — kept structured so
	// future grade-scaling / typed-bonus rules have a handle. The max-Dex cap
	// composes with the Dex term (§3 dex_ac), never with this.
	armorBonus int
	// armorMaxDex caps how much of the wearer's Dex modifier counts toward AC
	// while this armor is worn (armor-depth §3). nil = no cap (full Dex). The
	// most restrictive (lowest) cap across worn pieces wins, snapshotted at
	// equip into the holder's armorDexCap. Aliases the template pointer (the
	// template is immutable, never mutated through this).
	armorMaxDex *int
	// armorTier is the item's armor proficiency tier (armor-depth §5) from the
	// engine armor-tier vocabulary (light/medium/heavy); "" = untiered. Matched
	// against the wearer's class-granted armor tiers to gate the non-proficient
	// attack penalty.
	armorTier string
	// angrealPower / angrealGender are the One Power amplification rating and
	// gender gate of an angreal/sa'angreal device (wot-the-one-power.md S2).
	// angrealPower 0 ⇒ not an angreal. While a same-gender channeler has the
	// device equipped, it multiplies woven damage/heal upward. Carried verbatim
	// from the template (validated at pack load); never overridden at runtime.
	angrealPower  int
	angrealGender string
	// special / tripBonus / disarmBonus are the special-maneuver tags and their
	// DC magnitudes (special-weapons.md §2, increment J). nil special ⇒ no
	// maneuver. reach is the numeric reach rating (§3, cross-ruleset). Carried
	// verbatim from the template (validated at pack load); read by the combat
	// maneuvers (trip / disarm) and the reach band-gate.
	special     []string
	reach       int
	tripBonus   int
	disarmBonus int
	// Recorded-only equipment-depth metadata (special-weapons §2) — no engine
	// consumer yet; carried so the WoT equipment table is authored once and each
	// mechanic lights up free when its slice lands. subdual = nonlethal weapon;
	// doubleDamage = a double weapon's parsed second dice; armorSpeed = worn
	// speed; reputation = a signed visible-gear delta.
	subdual      bool
	doubleDamage combat.DiceExpr
	armorSpeed   int
	reputation   int
	// essenceCost is the Essence (in tenths) this item spends while installed as
	// cyberware (Shadowrun SR-M4). 0 for ordinary gear. Read by the equip
	// recompute to derive the wearer's essence pool current (max − Σ installed).
	essenceCost int
	// Item modification (item-modification.md — Slice A). capacity is a HOST's
	// mod budget (0 ⇒ unmodifiable); modHost/modCapacityCost mark a MODIFICATION
	// (its host class + cost). installedMods is the host's durable set of
	// installed modifications — mutated at install/remove and read by the
	// effective Modifiers/ArmorBonus/ArmorCheckPenalty/Resistances accessors,
	// so it is guarded by propsMu like the property bag.
	capacity        int
	modHost         string
	modCapacityCost int
	installedMods   []InstalledMod
	// Weapon accessories (weapon-accessories.md — Slice B). mounts is a weapon
	// host's exposed mount points; accessoryMounts is a mount accessory's
	// compatible mounts. Both write-once at build (like capacity/modHost).
	mounts          []string
	accessoryMounts []string
	// protection is the set of environmental protection keys this MODIFICATION
	// grants its host while worn (item-modification §6 → biome-hazard immunity).
	// Write-once at build.
	protection []string
	// grants is the set of general capability keys this MODIFICATION confers on
	// its host while equipped (item-modification §6 → smartlink↔smartgun pairing).
	// Write-once at build.
	grants []string
}

// ID implements Entity.
func (it *ItemInstance) ID() EntityID { return it.id }

// Type implements Entity.
func (it *ItemInstance) Type() string { return it.typ }

// Grade returns the item's quality-grade key (masterwork §2), or "" for an
// ordinary item. Reads the PropGrade instance property — seeded from the
// template at build and overridable by a runtime producer (a craft).
func (it *ItemInstance) Grade() string {
	if v, ok := it.Property(PropGrade); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// Tags implements Entity. Returns a fresh slice so callers cannot
// alias the backing storage; required for safe coexistence with the
// Store's tag index (see Entity.Tags doc).
func (it *ItemInstance) Tags() []string {
	return append([]string(nil), it.tags...)
}

// HasTag reports whether the item carries a gameplay tag.
func (it *ItemInstance) HasTag(tag string) bool {
	for _, t := range it.tags {
		if t == tag {
			return true
		}
	}
	return false
}

// AddTag appends a gameplay tag if absent, mutating the item's tag set in place.
// The caller re-indexes via Store.Retag when the item is tracked (the store's
// tag index only refreshes at Track/Untrack/Retag). Reports whether it changed.
func (it *ItemInstance) AddTag(tag string) bool {
	if tag == "" || it.HasTag(tag) {
		return false
	}
	it.tags = append(it.tags, tag)
	return true
}

// Name returns the display name. Per §2.3, instantiated entities take
// their name from the template at construction time.
func (it *ItemInstance) Name() string { return it.name }

// Description returns the flavor prose snapshotted from the template at
// construction (alongside Name). Empty when the template authored none;
// the `look` handler renders a generic fallback in that case.
func (it *ItemInstance) Description() string { return it.desc }

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
	maps.Copy(out, it.properties)
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

// DecrementInt atomically subtracts amount from the int property at key
// under the property write lock, flooring at zero, and reports whether
// the value reached zero on this call. A missing or non-int value is
// treated as zero (so the result is (0, true) and the key is written as
// 0). It is the burn-down primitive for a fuel-carrying light source
// (light-and-darkness §3.2): the read-modify-write is a single critical
// section, so a tick-goroutine burn cannot lose against a concurrent
// property write. Callers that must not create the key (e.g. to leave a
// permanent, fuel-less source untouched) check Property(key) first.
func (it *ItemInstance) DecrementInt(key string, amount int) (remaining int, hitZero bool) {
	it.propsMu.Lock()
	defer it.propsMu.Unlock()
	if it.properties == nil {
		it.properties = make(map[string]any)
	}
	v, _ := it.properties[key].(int)
	v -= amount
	if v <= 0 {
		v = 0
		hitZero = true
	}
	it.properties[key] = v
	return v, hitZero
}

// TakeCharge atomically claims one unit from the integer property key,
// decrementing it by 1 ONLY if it is currently positive. It returns the
// remaining count and whether a charge was actually taken — so two
// concurrent callers racing for the last charge of a 1-charge resource node
// (gathering.md §3) cannot both succeed: exactly one sees taken=true, the
// other sees (0, false). taken=false also means "already empty". Mirrors
// DecrementInt's locking; the conditional decrement is what makes it a
// single-winner claim rather than a flooring subtract.
func (it *ItemInstance) TakeCharge(key string) (remaining int, taken bool) {
	it.propsMu.Lock()
	defer it.propsMu.Unlock()
	if it.properties == nil {
		return 0, false
	}
	v, _ := it.properties[key].(int)
	if v <= 0 {
		return 0, false
	}
	v--
	it.properties[key] = v
	return v, true
}

// Modifiers returns the EFFECTIVE per-instance stat modifiers: the item's own
// (§2.3 step 6) plus every installed modification's (item-modification §6). The
// equip path reads this to build the wearer's modifier group, so a mod's stat
// bonuses apply while the host is worn and reverse on unequip. Installed-mod
// modifiers carry no meaningful Source (equip re-groups everything under the
// host's EquipmentSourceKey).
func (it *ItemInstance) Modifiers() []InstanceModifier {
	it.propsMu.RLock()
	defer it.propsMu.RUnlock()
	if len(it.installedMods) == 0 {
		return it.modifiers
	}
	out := append([]InstanceModifier(nil), it.modifiers...)
	for _, m := range it.installedMods {
		out = append(out, m.Modifiers...)
	}
	return out
}

// TemplateID returns the source template id (§2.3 step 5).
func (it *ItemInstance) TemplateID() item.TemplateID { return it.templateID }

// WeaponDamage returns the item's wielded-weapon damage dice (combat
// §4.5) and whether the item is a weapon at all. A zero DiceExpr (ok
// false) means the wielder rolls the engine's unarmed default. Read by
// the equip paths that populate combat.Stats.Damage.
func (it *ItemInstance) WeaponDamage() (combat.DiceExpr, bool) {
	return it.weaponDamage, !it.weaponDamage.IsZero()
}

// WeaponCategory returns the weapon's kind label (weapon-identity §2),
// empty when the weapon declares none. Read by the equip path that
// computes proficiency.
func (it *ItemInstance) WeaponCategory() string { return it.weaponCategory }

// WeaponSize returns the weapon's declared size category (size-and-wielding
// §2); "" when undeclared (callers resolve to the baseline size, and the
// footprint falls back to the static companion slots).
func (it *ItemInstance) WeaponSize() string { return it.weaponSize }

// ProficiencyTier returns the weapon's proficiency tier (weapon-identity
// §2), empty for an untiered weapon (treated as the lowest tier). Read by
// the equip path that computes proficiency.
func (it *ItemInstance) ProficiencyTier() string { return it.proficiencyTier }

// CritThreatLow returns the weapon's critical threat-low (weapon-identity
// §4), zero when unset (the combat resolver then uses the natural max).
func (it *ItemInstance) CritThreatLow() int { return it.critThreatLow }

// CritMultiplier returns the weapon's critical damage multiplier
// (weapon-identity §4), zero when unset (the resolver uses the default).
func (it *ItemInstance) CritMultiplier() int { return it.critMultiplier }

// DamageTypes returns the weapon's damage type(s) (weapon-identity §2) as
// a fresh slice; nil when untyped. Read by the combat path to select a
// defender's per-type resistance (armor-depth §4).
func (it *ItemInstance) DamageTypes() []string {
	return append([]string(nil), it.damageTypes...)
}

// TargetPool returns the pool.Kind (as a lowercased string) this weapon's
// damage fills (shadowrun-mvp SR-M3b); "" for the hp path. Read by the holder's
// Stats() builder into combat.Stats.TargetPool.
func (it *ItemInstance) TargetPool() string { return it.targetPool }

// RangedClass returns the weapon's ranged class (ranged-combat §2):
// "thrown", "projectile", or "" for a melee weapon (or non-weapon).
func (it *ItemInstance) RangedClass() string { return it.rangedClass }

// AmmoKind returns the ammunition kind a projectile weapon consumes, or an
// ammo item supplies (ranged-combat §3); empty for thrown/melee/non-ammo.
func (it *ItemInstance) AmmoKind() string { return it.ammoKind }

// RangedStyle returns the weapon's ranged flavor-voice id (rangedflavor). Empty
// means "use the default style / engine floor".
func (it *ItemInstance) RangedStyle() string { return it.rangedStyle }

// RangeIncrement returns the weapon's distance-falloff unit (ranged-combat
// §2); zero when unset. Inert until Slice B's range bands.
func (it *ItemInstance) RangeIncrement() int { return it.rangeIncrement }

// FireModes returns a copy of the weapon's supported firing modes (ranged-combat
// §5.5) — a subset of {single, burst, auto}. Empty means single-fire only.
func (it *ItemInstance) FireModes() []string { return append([]string(nil), it.fireModes...) }

// RecoilComp returns the weapon's inherent recoil compensation (SR5 "RC"), which
// reduces a burst/full-auto firing mode's recoil to-hit penalty (ranged-combat
// §5.6). 0 for weapons without it.
func (it *ItemInstance) RecoilComp() int { return it.recoilComp }

// ReloadTicks is the load time of a reload-gated projectile (a crossbow), in
// engine ticks; 0 means the weapon fires freely (a bow). action-economy §7.1.
func (it *ItemInstance) ReloadTicks() int { return it.reloadTicks }

// Magazine is a magazine weapon's capacity (SR5 "Ammo"); 0 means the weapon is
// not magazine-fed (a bow's loose rounds, or a crossbow's single chamber).
func (it *ItemInstance) Magazine() int { return it.magazine }

// ReloadMethod names how a magazine weapon reloads (SR5 reloading table, e.g.
// "clip"); empty for a non-magazine weapon.
func (it *ItemInstance) ReloadMethod() string { return it.reloadMethod }

// HolderFits reports the weapon family this item (an ammunition holder) fits, or
// "" if the item is not a holder (ammo-and-reloading §2).
func (it *ItemInstance) HolderFits() string { return it.holderFits }

// AcceptsHolder reports the holder family a holder-fed weapon takes, or "" if the
// weapon is not holder-fed (ammo-and-reloading §5). A holder-fed weapon fires
// from its inserted holder (see InsertedHolder), not its own magazine.
func (it *ItemInstance) AcceptsHolder() string { return it.acceptsHolder }

// InsertedHolder reports the holder currently inserted in this (holder-fed)
// weapon: its template id, loaded-round count, and whether one is inserted at
// all. The inserted holder is recorded as instance state (template + count), not
// a live item — insertion consumes the holder item, ejection re-spawns one
// (ammo-and-reloading §5). Reads 0/"" /false when nothing is inserted.
func (it *ItemInstance) InsertedHolder() (template string, loaded int, grade string, has bool) {
	it.propsMu.RLock()
	defer it.propsMu.RUnlock()
	tv, ok := it.properties[propInsertedHolderTpl]
	if !ok {
		return "", 0, "", false
	}
	tpl, _ := tv.(string)
	if tpl == "" {
		return "", 0, "", false
	}
	n := 0
	if lv, ok := it.properties[propInsertedHolderLoaded]; ok {
		n, _ = lv.(int)
	}
	if n < 0 {
		n = 0
	}
	g, _ := it.properties[propInsertedHolderGrade].(string)
	return tpl, n, g, true
}

// SetInsertedHolder records the holder inserted in this weapon (its template,
// loaded-round count, and round grade). Overwrites any prior inserted holder —
// the caller is responsible for ejecting the old one first (ammo-and-reloading
// §5).
func (it *ItemInstance) SetInsertedHolder(template string, loaded int, grade string) {
	if loaded < 0 {
		loaded = 0
	}
	it.propsMu.Lock()
	defer it.propsMu.Unlock()
	if it.properties == nil {
		it.properties = make(map[string]any)
	}
	it.properties[propInsertedHolderTpl] = template
	it.properties[propInsertedHolderLoaded] = loaded
	it.properties[propInsertedHolderGrade] = grade
}

// HolderAmmoGrade / SetHolderAmmoGrade carry the grade of the rounds loaded in a
// HOLDER item (grade-through-holder §8) — set at fill, read at insertion so the
// grade travels with the clip to the shot. "" = ungraded.
func (it *ItemInstance) HolderAmmoGrade() string {
	if v, ok := it.Property(propHolderAmmoGrade); ok {
		if g, ok := v.(string); ok {
			return g
		}
	}
	return ""
}

func (it *ItemInstance) SetHolderAmmoGrade(grade string) {
	it.SetProperty(propHolderAmmoGrade, grade)
}

// SetInsertedHolderLoaded updates just the inserted holder's round count (firing
// decrements it). No-op if no holder is inserted.
func (it *ItemInstance) SetInsertedHolderLoaded(loaded int) {
	if loaded < 0 {
		loaded = 0
	}
	it.propsMu.Lock()
	defer it.propsMu.Unlock()
	if it.properties == nil {
		return
	}
	if tpl, ok := it.properties[propInsertedHolderTpl].(string); !ok || tpl == "" {
		return
	}
	it.properties[propInsertedHolderLoaded] = loaded
}

// ClearInsertedHolder removes the inserted-holder record (the weapon becomes
// empty of a holder — e.g. after ejecting without inserting a replacement).
func (it *ItemInstance) ClearInsertedHolder() {
	it.propsMu.Lock()
	defer it.propsMu.Unlock()
	delete(it.properties, propInsertedHolderTpl)
	delete(it.properties, propInsertedHolderLoaded)
	delete(it.properties, propInsertedHolderGrade)
}

// MagazineLoaded reports the rounds currently in a magazine weapon. A weapon
// carries no explicit `loaded` property until it is reloaded or fired, and reads
// EMPTY (0) until then — a freshly-spawned or freshly-picked-up firearm must be
// reloaded before it fires. Once reloaded/fired, an explicit count is stored
// (SetMagazineLoaded) and persists. Non-magazine weapons read 0.
func (it *ItemInstance) MagazineLoaded() int {
	if it.magazine <= 0 {
		return 0
	}
	if v, ok := it.Property(propLoadedRounds); ok {
		if n, ok := v.(int); ok {
			return n
		}
	}
	return 0 // lazy-empty: an untouched magazine weapon starts unloaded
}

// SetMagazineLoaded writes the current loaded-round count (clamped to
// [0, Magazine]) as an instance property so firing and reload persist. A no-op
// for a non-magazine weapon.
func (it *ItemInstance) SetMagazineLoaded(n int) {
	if it.magazine <= 0 {
		return
	}
	if n < 0 {
		n = 0
	}
	if n > it.magazine {
		n = it.magazine
	}
	it.SetProperty(propLoadedRounds, n)
}

// StrRating returns the cap on a Strength-rated projectile weapon's
// positive Strength damage bonus (ranged-combat §4), or nil for the default
// projectile rule (no positive Strength bonus). The returned pointer is a
// copy — mutating it does not affect the instance.
func (it *ItemInstance) StrRating() *int {
	if it.strRating == nil {
		return nil
	}
	v := *it.strRating
	return &v
}

// Resistances returns the EFFECTIVE per-damage-type damage reduction
// (armor-depth §4): the armor's own plus every installed modification's
// (item-modification §6 — the typed-field aggregation path a Fire Resistance mod
// rides). A fresh map; nil when neither the item nor its mods grant any.
func (it *ItemInstance) Resistances() map[string]int {
	it.propsMu.RLock()
	defer it.propsMu.RUnlock()
	if len(it.resistances) == 0 && len(it.installedMods) == 0 {
		return nil
	}
	out := make(map[string]int, len(it.resistances)+len(it.installedMods))
	maps.Copy(out, it.resistances)
	for _, m := range it.installedMods {
		for dt, amt := range m.Resistances {
			out[dt] += amt
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// HasSpecial reports whether the weapon carries the given special-weapon tag
// (special-weapons.md §2 — reach / trip / disarm). Tags are normalized lowercase
// at load, so the caller passes the bare tag constant.
func (it *ItemInstance) HasSpecial(tag string) bool {
	return slices.Contains(it.special, tag)
}

// TripBonus / DisarmBonus return the DC magnitude this weapon adds to the
// matching maneuver (special-weapons.md §4/§5); 0 when the weapon lacks the tag
// or declares no explicit bonus (the engine default applies). Read alongside
// HasSpecial so a 0 with the tag present still means "amplify by the default".
func (it *ItemInstance) TripBonus() int   { return it.tripBonus }
func (it *ItemInstance) DisarmBonus() int { return it.disarmBonus }

// Reach returns the weapon's reach rating (special-weapons.md §3) — a numeric,
// cross-ruleset weapon stat (0 = an ordinary close weapon). WoT reads `> 0` as
// "strikes at the near range band"; a Shadowrun pack reads net reach as a
// defense-roll modifier.
func (it *ItemInstance) Reach() int { return it.reach }

// Subdual / DoubleDamage / ArmorSpeed / Reputation expose the recorded-only
// equipment-depth metadata (special-weapons.md §2) — no engine consumer reads
// them yet; they exist so the equipment table is authored once and each mechanic
// lights up free when its slice lands. Subdual: a nonlethal weapon. DoubleDamage:
// a double weapon's second-attack dice (ok=false when not a double weapon).
// ArmorSpeed: worn speed (0 = unset). Reputation: a signed visible-gear delta.
func (it *ItemInstance) Subdual() bool { return it.subdual }
func (it *ItemInstance) DoubleDamage() (combat.DiceExpr, bool) {
	return it.doubleDamage, !it.doubleDamage.IsZero()
}
func (it *ItemInstance) ArmorSpeed() int { return it.armorSpeed }
func (it *ItemInstance) Reputation() int { return it.reputation }

// EssenceCost returns the Essence (in tenths) this item spends while installed
// as cyberware (Shadowrun SR-M4); 0 for ordinary gear. The equip recompute sums
// it across installed augmentations to derive the wearer's essence pool current.
func (it *ItemInstance) EssenceCost() int { return it.essenceCost }

// Angreal returns the item's One Power amplification rating and gender gate
// (wot-the-one-power.md S2). ok is false (power 0, gender "") for an item that
// is not an angreal. When ok, gender is "male"/"female" and power is positive
// (the pack loader guarantees the pairing). A same-gender channeler holding the
// device weaves a stronger damage/heal payload.
func (it *ItemInstance) Angreal() (power int, gender string, ok bool) {
	if it.angrealPower <= 0 {
		return 0, "", false
	}
	return it.angrealPower, it.angrealGender, true
}

// ArmorCheckPenalty returns the EFFECTIVE Str/Dex skill-check penalty magnitude
// (armor-depth §6): the armor's own plus every installed modification's
// (item-modification §6). 0 when neither imposes any.
func (it *ItemInstance) ArmorCheckPenalty() int {
	it.propsMu.RLock()
	defer it.propsMu.RUnlock()
	total := it.armorCheckPenalty
	for _, m := range it.installedMods {
		total += m.ArmorCheck
	}
	return total
}

// Capacity returns the host's modification budget (item-modification §2); 0 ⇒
// the item is unmodifiable. Write-once at build, so lock-free.
func (it *ItemInstance) Capacity() int { return it.capacity }

// IsModification reports whether this item is a modification (declares a
// host-compat key, §3). ModHost / ModCapacityCost are meaningful only then.
func (it *ItemInstance) IsModification() bool { return it.modHost != "" }

// ModHost returns the host class a modification fits (§3 host-compat key), or
// "" if the item is not a modification.
func (it *ItemInstance) ModHost() string { return it.modHost }

// ModCapacityCost returns a modification's capacity cost (§3); 0 for a non-mod.
func (it *ItemInstance) ModCapacityCost() int { return it.modCapacityCost }

// Protection returns the environmental protection keys THIS item (a modification)
// grants its host while worn (item-modification §6); nil for a non-mod. Write-once
// at build, so lock-free.
func (it *ItemInstance) Protection() []string { return append([]string(nil), it.protection...) }

// GrantedProtections returns the environmental protection keys conferred by the
// modifications installed in THIS host (the union across installed mods) — the
// biome-hazard immunity a worn modded item provides (area-effects §4.6). nil when
// no installed mod grants any.
func (it *ItemInstance) GrantedProtections() []string {
	it.propsMu.RLock()
	defer it.propsMu.RUnlock()
	if len(it.installedMods) == 0 {
		return nil
	}
	var out []string
	seen := make(map[string]bool)
	for _, m := range it.installedMods {
		for _, k := range m.Protection {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	return out
}

// GrantedCapabilities returns the general capability keys conferred by the
// modifications installed in THIS host (the union across installed mods) — e.g. a
// smartlink cybereye or a smartgun weapon, read by the cross-item pairing at
// combat time (item-modification §6). nil when no installed mod grants any.
func (it *ItemInstance) GrantedCapabilities() []string {
	it.propsMu.RLock()
	defer it.propsMu.RUnlock()
	if len(it.installedMods) == 0 {
		return nil
	}
	var out []string
	seen := make(map[string]bool)
	for _, m := range it.installedMods {
		for _, k := range m.Grants {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	return out
}

// ProvidesCapability reports whether this item confers capability key — either an
// installed mod grants it (GrantedCapabilities) or the item declares it
// intrinsically (a tag, for a monolithic capability item). Case-insensitive.
func (it *ItemInstance) ProvidesCapability(key string) bool {
	for _, c := range it.GrantedCapabilities() {
		if strings.EqualFold(c, key) {
			return true
		}
	}
	return it.HasTag(key)
}

// InstalledMods returns a snapshot of the modifications installed in this host
// (§7), for display and persistence. Fresh slice; callers cannot alias.
func (it *ItemInstance) InstalledMods() []InstalledMod {
	it.propsMu.RLock()
	defer it.propsMu.RUnlock()
	return append([]InstalledMod(nil), it.installedMods...)
}

// UsedCapacity is the sum of installed mods' costs (§2); FreeCapacity is
// budget − used, never negative (the install rule enforces it).
func (it *ItemInstance) UsedCapacity() int {
	it.propsMu.RLock()
	defer it.propsMu.RUnlock()
	return it.usedCapacityLocked()
}

func (it *ItemInstance) usedCapacityLocked() int {
	used := 0
	for _, m := range it.installedMods {
		used += m.CapacityCost
	}
	return used
}

// FreeCapacity returns the host's remaining modification budget (§2).
func (it *ItemInstance) FreeCapacity() int { return it.Capacity() - it.UsedCapacity() }

// --- Weapon accessories (mount slots — weapon-accessories.md, Slice B) ---

// Mounts returns the mount points this weapon host exposes (§2); nil for a
// non-mount host. Write-once at build, so lock-free.
func (it *ItemInstance) Mounts() []string { return append([]string(nil), it.mounts...) }

// AccessoryMounts returns the mounts a mount accessory may occupy (§3); nil for a
// non-accessory. Write-once at build, so lock-free.
func (it *ItemInstance) AccessoryMounts() []string {
	return append([]string(nil), it.accessoryMounts...)
}

// IsAccessory reports whether the item is a mount accessory (declares compatible
// mounts, §3). Distinct from a capacity mod (IsModification with a cost).
func (it *ItemInstance) IsAccessory() bool { return len(it.accessoryMounts) > 0 }

// IsModifiable reports whether the item accepts modifications at all — a capacity
// host (Capacity > 0) or a mount host (exposes mounts).
func (it *ItemInstance) IsModifiable() bool { return it.capacity > 0 || len(it.mounts) > 0 }

// OccupiedMounts returns the mounts currently taken by installed accessories, and
// FreeMounts the exposed mounts that are still open (weapon-accessories §2/§4).
func (it *ItemInstance) OccupiedMounts() []string {
	it.propsMu.RLock()
	defer it.propsMu.RUnlock()
	return it.occupiedMountsLocked()
}

func (it *ItemInstance) occupiedMountsLocked() []string {
	var out []string
	for _, m := range it.installedMods {
		if m.Mount != "" {
			out = append(out, m.Mount)
		}
	}
	return out
}

// FreeMounts returns this host's exposed mounts that hold no accessory.
func (it *ItemInstance) FreeMounts() []string {
	it.propsMu.RLock()
	defer it.propsMu.RUnlock()
	return it.freeMountsLocked()
}

func (it *ItemInstance) freeMountsLocked() []string {
	occupied := make(map[string]bool, len(it.installedMods))
	for _, m := range it.installedMods {
		if m.Mount != "" {
			occupied[m.Mount] = true
		}
	}
	var out []string
	for _, mount := range it.mounts {
		if !occupied[mount] {
			out = append(out, mount)
		}
	}
	return out
}

// AttachAccessory installs a mount accessory into this weapon host
// (weapon-accessories.md §4). It validates that the host exposes mounts, the item
// is an accessory, and a compatible mount is free — then records it on the chosen
// mount (the first free mount, in the host's declared order, that the accessory
// fits) and snapshots its contribution (§6). Returns the chosen mount, or a
// sentinel error on refusal. v1 occupies exactly ONE mount per accessory (the
// multi-mount `both` form is deferred).
func (it *ItemInstance) AttachAccessory(acc *ItemInstance) (string, error) {
	if len(it.mounts) == 0 {
		return "", ErrNotModifiable
	}
	if acc == nil || !acc.IsAccessory() {
		return "", ErrNotAModification
	}
	fits := make(map[string]bool, len(acc.accessoryMounts))
	for _, m := range acc.accessoryMounts {
		fits[m] = true
	}
	rec := InstalledMod{
		TemplateID:  acc.templateID,
		Name:        acc.name,
		ArmorBonus:  acc.armorBonus,
		ArmorCheck:  acc.armorCheckPenalty,
		Resistances: cloneIntMap(acc.resistances),
		Modifiers:   append([]InstanceModifier(nil), acc.modifiers...),
		Protection:  append([]string(nil), acc.protection...),
		Grants:      append([]string(nil), acc.grants...),
	}
	it.propsMu.Lock()
	defer it.propsMu.Unlock()
	// A mount the host exposes, the accessory fits, and nothing else occupies.
	compatible := false
	for _, mount := range it.freeMountsLocked() {
		if fits[mount] {
			rec.Mount = mount
			it.installedMods = append(it.installedMods, rec)
			return mount, nil
		}
	}
	// Distinguish "the accessory fits no mount this host has" (incompatible) from
	// "every compatible mount is taken" (occupied).
	for _, mount := range it.mounts {
		if fits[mount] {
			compatible = true
			break
		}
	}
	if !compatible {
		return "", ErrModIncompatible
	}
	return "", ErrMountOccupied
}

// InstallMod installs mod into this host (item-modification §4). It validates
// modifiability, host compatibility, and free capacity, then SNAPSHOTS the mod's
// contribution onto the host (§6) so the equip/recompute seams read it without a
// registry. Returns a sentinel error on refusal; the caller consumes the mod
// item only on success. The mod's cost is the mod template's ModCapacityCost.
func (it *ItemInstance) InstallMod(mod *ItemInstance) error {
	if it.Capacity() <= 0 {
		return ErrNotModifiable
	}
	// A mount accessory installs by the mount rule (AttachAccessory), never by
	// capacity — refuse it here so a capacity host can't absorb one for free
	// (mirror of AttachAccessory's !IsAccessory guard).
	if mod == nil || !mod.IsModification() || mod.IsAccessory() {
		return ErrNotAModification
	}
	if !it.HasTag(mod.ModHost()) {
		return ErrModIncompatible
	}
	cost := mod.ModCapacityCost()
	rec := InstalledMod{
		TemplateID:   mod.templateID,
		Name:         mod.name,
		CapacityCost: cost,
		ArmorBonus:   mod.armorBonus,        // intrinsic — a mod is never itself modded
		ArmorCheck:   mod.armorCheckPenalty, // (so field == effective accessor)
		Resistances:  cloneIntMap(mod.resistances),
		Modifiers:    append([]InstanceModifier(nil), mod.modifiers...),
		Protection:   append([]string(nil), mod.protection...),
		Grants:       append([]string(nil), mod.grants...),
	}
	it.propsMu.Lock()
	defer it.propsMu.Unlock()
	if it.usedCapacityLocked()+cost > it.capacity {
		return ErrModNoCapacity
	}
	it.installedMods = append(it.installedMods, rec)
	return nil
}

// RemoveMod removes the first installed mod matching token (a case-insensitive
// substring of its name, or its exact template id; empty token matches the
// first). Returns the removed record (so the caller can re-spawn the mod item
// from its TemplateID, §5) and whether one was removed.
func (it *ItemInstance) RemoveMod(token string) (InstalledMod, bool) {
	token = strings.ToLower(strings.TrimSpace(token))
	it.propsMu.Lock()
	defer it.propsMu.Unlock()
	for i, m := range it.installedMods {
		if token == "" || string(m.TemplateID) == token || strings.Contains(strings.ToLower(m.Name), token) {
			it.installedMods = slices.Delete(it.installedMods, i, i+1)
			return m, true
		}
	}
	return InstalledMod{}, false
}

// RestoreInstalledMod re-adds a modification recorded in a save (item-modification
// §7), re-deriving its contribution from the mod's template. Used only by the
// load/respawn path (which has the template registry); it does not re-validate
// capacity (saved state is trusted, and content that shrank a budget must not
// silently drop a persisted mod). Skips a nil/non-mod template defensively.
func (it *ItemInstance) RestoreInstalledMod(tpl *item.Template) {
	if tpl == nil || tpl.ModHost == "" {
		return
	}
	rec := InstalledMod{
		TemplateID:   tpl.ID,
		Name:         tpl.Name,
		CapacityCost: tpl.ModCapacityCost,
		ArmorBonus:   tpl.ArmorBonus,
		ArmorCheck:   tpl.ArmorCheckPenalty,
		Resistances:  cloneIntMap(tpl.Resistances),
		Modifiers:    modifiersFromTemplate(tpl),
		Protection:   append([]string(nil), tpl.Protection...),
		Grants:       append([]string(nil), tpl.Grants...),
	}
	it.propsMu.Lock()
	defer it.propsMu.Unlock()
	// A mount accessory on a mount host reclaims a mount deterministically — the
	// first free compatible mount, in the host's declared order. The persisted
	// install order reproduces a valid layout (weapon-accessories §4/§6). The
	// mount itself is derived, not persisted, so re-seating is stable.
	if len(tpl.AccessoryMounts) > 0 {
		if len(it.mounts) == 0 {
			// The host lost its mount capability entirely since the save — drop
			// the accessory rather than fall through and re-add it mountless
			// (which would apply its effect invisibly, forever).
			return
		}
		fits := make(map[string]bool, len(tpl.AccessoryMounts))
		for _, m := range tpl.AccessoryMounts {
			fits[strings.ToLower(strings.TrimSpace(m))] = true
		}
		for _, mount := range it.freeMountsLocked() {
			if fits[mount] {
				rec.Mount = mount
				break
			}
		}
		if rec.Mount == "" {
			// Content shrank the weapon's mounts since the save: no seat left.
			// Skip rather than double-occupy a mount.
			return
		}
	}
	it.installedMods = append(it.installedMods, rec)
}

// cloneIntMap returns a fresh copy of m, or nil when m is empty — so an
// InstalledMod never aliases a template's or another instance's map.
func cloneIntMap(m map[string]int) map[string]int {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]int, len(m))
	maps.Copy(out, m)
	return out
}

// modifiersFromTemplate builds the source-less InstanceModifier list a restored
// mod contributes (equip re-groups them under the host's EquipmentSourceKey, so
// the Source is intentionally empty here).
func modifiersFromTemplate(tpl *item.Template) []InstanceModifier {
	if len(tpl.Modifiers) == 0 {
		return nil
	}
	out := make([]InstanceModifier, 0, len(tpl.Modifiers))
	for _, m := range tpl.Modifiers {
		out = append(out, InstanceModifier{Stat: m.Stat, Value: m.Value})
	}
	return out
}

// ArmorBonus returns the EFFECTIVE structured additive AC contribution
// (armor-depth §3): the item's own plus every installed modification's
// (item-modification §6). 0 when neither grants any. The equip path applies it
// as an `ac` stat modifier, stacking across distinct worn pieces.
func (it *ItemInstance) ArmorBonus() int {
	it.propsMu.RLock()
	defer it.propsMu.RUnlock()
	total := it.armorBonus
	for _, m := range it.installedMods {
		total += m.ArmorBonus
	}
	return total
}

// ArmorMaxDex returns the cap this armor places on the wearer's Dex
// contribution to AC (armor-depth §3), or nil for no cap (full Dex applies).
// The returned pointer is a copy — mutating it does not affect the instance.
func (it *ItemInstance) ArmorMaxDex() *int {
	if it.armorMaxDex == nil {
		return nil
	}
	v := *it.armorMaxDex
	return &v
}

// ArmorTier returns the item's armor proficiency tier (armor-depth §5) from
// the light/medium/heavy vocabulary; "" when untiered.
func (it *ItemInstance) ArmorTier() string { return it.armorTier }

// EligibleSlots returns the slots this item may be equipped into
// (inventory-equipment-items §3.3) as a fresh slice so callers cannot
// alias instance state. Empty means the item is not equippable. Lifted
// from the template at construction: explicit EligibleSlots, else the
// legacy `properties.slot` one-element bridge (§3.2).
func (it *ItemInstance) EligibleSlots() []string {
	return append([]string(nil), it.eligibleSlots...)
}

// CompanionSlots returns the additional slots this item occupies while
// equipped — its footprint beyond the target slot (§3.3) — as a fresh
// slice. nil when the footprint is just the target slot.
func (it *ItemInstance) CompanionSlots() []string {
	return append([]string(nil), it.companionSlots...)
}

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
	if tpl.Grade != "" {                   // masterwork §6: authored grade rides the template.
		props[PropGrade] = tpl.Grade
	}
	// A pre-loaded holder spawns with rounds already in it (ammo-and-reloading
	// §6): seed the loaded-round count from the template's Preload (clamped to
	// capacity). Absent/0 leaves the holder lazy-empty.
	if tpl.HolderFits != "" && tpl.Preload > 0 {
		n := tpl.Preload
		if tpl.Magazine > 0 && n > tpl.Magazine {
			n = tpl.Magazine
		}
		props[propLoadedRounds] = n
	}

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

	// Parse the weapon-damage dice once (combat §4.5). The string was
	// validated at pack load, so a parse error here can only come from a
	// hand-built template (tests) — treat it as "not a weapon" (zero
	// DiceExpr → unarmed fallback) rather than panicking at spawn.
	var weaponDamage combat.DiceExpr
	if tpl.WeaponDamage != "" {
		if d, err := combat.ParseDice(tpl.WeaponDamage); err == nil {
			weaponDamage = d
		}
	}
	// Double weapon's second-attack dice (special-weapons §2, recorded-only) —
	// parsed once like weaponDamage; validated at load, so a parse miss here means
	// a hand-built test template (treat as "not a double weapon").
	var doubleDamage combat.DiceExpr
	if tpl.DoubleDamage != "" {
		if d, err := combat.ParseDice(tpl.DoubleDamage); err == nil {
			doubleDamage = d
		}
	}

	// Equipment slot eligibility + footprint (§3.3), lifted onto the
	// instance (R5). Explicit template fields win; an item declaring only
	// the legacy `properties.slot` string still becomes eligible for that
	// one slot (§3.2 bridge). For loader-built templates decodeItem
	// already lifted the legacy slot into EligibleSlots, so the fallback
	// here is a no-op; it covers hand-built templates (tests) that set
	// only properties.slot.
	eligible := append([]string(nil), tpl.EligibleSlots...)
	if len(eligible) == 0 {
		if s, ok := item.LegacySlotName(props); ok {
			eligible = []string{s}
		}
	}
	companion := append([]string(nil), tpl.CompanionSlots...)

	// Weapon damage types (weapon-identity §2) + armor resistances
	// (armor-depth §4), snapshotted onto the instance. Both copied so the
	// instance never aliases the shared template's slices/maps.
	damageTypes := append([]string(nil), tpl.DamageTypes...)
	var resistances map[string]int
	if len(tpl.Resistances) > 0 {
		resistances = make(map[string]int, len(tpl.Resistances))
		maps.Copy(resistances, tpl.Resistances)
	}
	// Ranged Strength rating (ranged-combat §4): copy the optional pointer so
	// the instance never aliases the shared template's pointer.
	var strRating *int
	if tpl.StrRating != nil {
		v := *tpl.StrRating
		strRating = &v
	}

	return &ItemInstance{
		id:                id,
		typ:               tpl.Type,
		name:              tpl.Name,
		desc:              tpl.Description, // §2.3: snapshot prose alongside name.
		tags:              tags,
		keywords:          keywords,
		properties:        props,
		modifiers:         mods,
		templateID:        tpl.ID,
		weaponDamage:      weaponDamage,
		eligibleSlots:     eligible,
		companionSlots:    companion,
		weaponCategory:    tpl.WeaponCategory,
		proficiencyTier:   tpl.ProficiencyTier,
		weaponSize:        tpl.Size,
		critThreatLow:     tpl.CritThreatLow,
		critMultiplier:    tpl.CritMultiplier,
		damageTypes:       damageTypes,
		targetPool:        tpl.TargetPool,
		rangedClass:       tpl.RangedClass,
		ammoKind:          tpl.AmmoKind,
		rangedStyle:       tpl.RangedStyle,
		rangeIncrement:    tpl.RangeIncrement,
		fireModes:         append([]string(nil), tpl.FireModes...),
		recoilComp:        tpl.RecoilComp,
		reloadTicks:       tpl.ReloadTicks,
		magazine:          tpl.Magazine,
		reloadMethod:      tpl.ReloadMethod,
		holderFits:        tpl.HolderFits,
		acceptsHolder:     tpl.AcceptsHolder,
		strRating:         strRating,
		resistances:       resistances,
		armorCheckPenalty: tpl.ArmorCheckPenalty,
		armorBonus:        tpl.ArmorBonus,
		armorMaxDex:       tpl.ArmorMaxDex,
		armorTier:         tpl.ArmorTier,
		angrealPower:      tpl.AngrealPower,
		angrealGender:     tpl.AngrealGender,
		special:           append([]string(nil), tpl.Special...), // copy: never alias the shared template slice
		reach:             tpl.Reach,
		tripBonus:         tpl.TripBonus,
		disarmBonus:       tpl.DisarmBonus,
		subdual:           tpl.Subdual,
		doubleDamage:      doubleDamage,
		armorSpeed:        tpl.ArmorSpeed,
		reputation:        tpl.Reputation,
		essenceCost:       tpl.EssenceCost,
		capacity:          tpl.Capacity,
		modHost:           tpl.ModHost,
		modCapacityCost:   tpl.ModCapacityCost,
		mounts:            append([]string(nil), tpl.Mounts...),
		accessoryMounts:   append([]string(nil), tpl.AccessoryMounts...),
		protection:        append([]string(nil), tpl.Protection...),
		grants:            append([]string(nil), tpl.Grants...),
		// installedMods starts empty — a freshly built item carries no mods;
		// the save-respawn path re-adds them (item-modification §7).
	}
}
