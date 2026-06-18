package entities

import (
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/combat"
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
	// PropGrade is the item's quality-grade key (masterwork §2). Seeded
	// from the template's Grade at instantiation; a runtime producer (a
	// craft) may overwrite it on the instance, where it persists with the
	// item's other instance properties (masterwork §6).
	PropGrade = "grade"
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
	// Ranged weapon metadata (ranged-combat §2), lifted onto the instance at
	// build (mirrors weaponCategory). rangedClass empty = melee; ammoKind is
	// what a projectile fires / an ammo item supplies; rangeIncrement is the
	// distance-falloff unit; strRating caps a Strength-rated bow's positive
	// Strength bonus (nil = the default no-positive-Strength projectile rule).
	// Write-once at construction — no mutex needed.
	rangedClass    string
	ammoKind       string
	rangeIncrement int
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
	// special / tripBonus / disarmBonus are the special-weapon tags and their
	// maneuver DC magnitudes (special-weapons.md §2, increment J). nil special
	// ⇒ an ordinary weapon. Carried verbatim from the template (validated at
	// pack load); read by the combat maneuvers (reach / trip / disarm).
	special     []string
	tripBonus   int
	disarmBonus int
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

// Modifiers returns the transient per-instance stat modifiers (§2.3
// step 6). Equip-time application reads this list; nothing else writes
// to it post-Spawn.
func (it *ItemInstance) Modifiers() []InstanceModifier { return it.modifiers }

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

// RangedClass returns the weapon's ranged class (ranged-combat §2):
// "thrown", "projectile", or "" for a melee weapon (or non-weapon).
func (it *ItemInstance) RangedClass() string { return it.rangedClass }

// AmmoKind returns the ammunition kind a projectile weapon consumes, or an
// ammo item supplies (ranged-combat §3); empty for thrown/melee/non-ammo.
func (it *ItemInstance) AmmoKind() string { return it.ammoKind }

// RangeIncrement returns the weapon's distance-falloff unit (ranged-combat
// §2); zero when unset. Inert until Slice B's range bands.
func (it *ItemInstance) RangeIncrement() int { return it.rangeIncrement }

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

// Resistances returns the armor's per-damage-type damage reduction
// (armor-depth §4) as a fresh map; nil when the item grants none.
func (it *ItemInstance) Resistances() map[string]int {
	if len(it.resistances) == 0 {
		return nil
	}
	out := make(map[string]int, len(it.resistances))
	for k, v := range it.resistances {
		out[k] = v
	}
	return out
}

// HasSpecial reports whether the weapon carries the given special-weapon tag
// (special-weapons.md §2 — reach / trip / disarm). Tags are normalized lowercase
// at load, so the caller passes the bare tag constant.
func (it *ItemInstance) HasSpecial(tag string) bool {
	for _, t := range it.special {
		if t == tag {
			return true
		}
	}
	return false
}

// TripBonus / DisarmBonus return the DC magnitude this weapon adds to the
// matching maneuver (special-weapons.md §4/§5); 0 when the weapon lacks the tag
// or declares no explicit bonus (the engine default applies). Read alongside
// HasSpecial so a 0 with the tag present still means "amplify by the default".
func (it *ItemInstance) TripBonus() int   { return it.tripBonus }
func (it *ItemInstance) DisarmBonus() int { return it.disarmBonus }

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

// ArmorCheckPenalty returns the magnitude (non-negative) of the penalty
// this armor imposes on Str/Dex skill checks while worn (armor-depth §6);
// 0 for an item that imposes none.
func (it *ItemInstance) ArmorCheckPenalty() int { return it.armorCheckPenalty }

// ArmorBonus returns the item's structured additive AC contribution
// (armor-depth §3); 0 when it grants none. The equip path applies it as an
// `ac` stat modifier, stacking across distinct worn pieces.
func (it *ItemInstance) ArmorBonus() int { return it.armorBonus }

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
		for k, v := range tpl.Resistances {
			resistances[k] = v
		}
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
		rangedClass:       tpl.RangedClass,
		ammoKind:          tpl.AmmoKind,
		rangeIncrement:    tpl.RangeIncrement,
		strRating:         strRating,
		resistances:       resistances,
		armorCheckPenalty: tpl.ArmorCheckPenalty,
		armorBonus:        tpl.ArmorBonus,
		armorMaxDex:       tpl.ArmorMaxDex,
		armorTier:         tpl.ArmorTier,
		angrealPower:      tpl.AngrealPower,
		angrealGender:     tpl.AngrealGender,
		special:           append([]string(nil), tpl.Special...), // copy: never alias the shared template slice
		tripBonus:         tpl.TripBonus,
		disarmBonus:       tpl.DisarmBonus,
	}
}
