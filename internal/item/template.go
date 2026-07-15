// Package item owns content-side item data: the template type and the
// registry the pack loader populates at boot. Instances and runtime
// tracking live in internal/entities (M5.2).
//
// Spec: inventory-equipment-items §2.
package item

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// TemplateID is a namespace-qualified template identifier
// (e.g. "tapestry-core:short-sword"). Spec §5.2 namespace rule.
type TemplateID string

// Modifier is one stat modification a template grants to its equipper.
// Applied at equip time, not at instantiation (§2.3 step 6).
type Modifier struct {
	Stat  string
	Value int
}

// Template is the recipe an item instance is built from. Fields mirror
// spec §2.2: id, name, type are required; tags, keywords, properties,
// modifiers are optional.
//
// The Properties bag is intentionally untyped: pack content may carry
// arbitrary scalars/maps. Per §2.3 step 4, instantiation normalizes
// nested untyped maps; templates themselves store whatever the decoder
// produced.
type Template struct {
	ID   TemplateID
	Name string
	Type string
	// Description is the optional flavor prose shown by `look <item>`
	// (ui-rendering-help — the appearance lens). Empty means the look
	// handler falls back to a generic "nothing special" line; authoring
	// it is never required.
	Description string
	Tags        []string
	Keywords    []string
	Properties  map[string]any
	Modifiers   []Modifier
	// Grade is the item's quality-grade key (masterwork §2) from the
	// pack-loaded grade vocabulary (internal/grade) — masterwork /
	// masterpiece / power-wrought. Empty means an ordinary item (no grade
	// bonus). Validated against the grade registry at pack load; normalized
	// lowercase. A runtime-produced item (a craft) may instead set the grade
	// as an instance property, which overrides the template value.
	Grade string
	// WeaponDamage is the wielded-weapon damage dice (combat §4.5) as a
	// raw NdM±K string, e.g. "1d6+1". Empty means the item is not a
	// weapon — a holder wielding only non-weapon items rolls the engine's
	// unarmed default. The string is validated at pack load (a malformed
	// expression fails the load, naming the file) and parsed to a typed
	// dice expression when the instance is built; the template stays
	// combat-package-free by holding the canonical string. See
	// entities.ItemInstance.WeaponDamage.
	WeaponDamage string
	// EligibleSlots is the set of equipment slots this item MAY be
	// equipped into (inventory-equipment-items §2.2, §3.3). One slot is
	// the common case; several express interchangeable positions (main
	// or off hand). A nil/empty set means the item is not equippable.
	// Names are lowercased snake_case base slot names (no `:index`); a
	// single legacy `properties.slot` string is decoded into the
	// one-element form so existing content keeps working (§3.2 bridge).
	// Validated against the slot registry in a boot post-pass (§3.3).
	EligibleSlots []string
	// CompanionSlots are the additional slots this item occupies while
	// equipped — its footprint beyond the target slot (§2.2, §3.3), e.g.
	// a two-handed weapon that also ties up the off hand. nil means the
	// footprint is just the target slot. Same name rules as EligibleSlots.
	CompanionSlots []string
	// WeaponCategory is the weapon's kind (weapon-identity §2 — an opaque
	// label the engine matches for proficiency, e.g. a longsword kind).
	// Empty means the weapon is gated by tier alone. Normalized lowercase.
	WeaponCategory string
	// ProficiencyTier is the weapon's proficiency tier (weapon-identity §2)
	// from the engine tier vocabulary (simple/martial/exotic). Empty means
	// "untiered" — treated as the lowest tier (universally proficient, §3).
	// Validated at pack load; normalized lowercase.
	ProficiencyTier string
	// DamageTypes are the weapon's damage type(s) (weapon-identity §2) from
	// the fixed bludgeoning/piercing/slashing set. nil means untyped.
	// Recorded only — inert until armor depth. Validated at pack load;
	// normalized lowercase.
	DamageTypes []string
	// TargetPool is the pool.Kind this weapon's damage fills — a Shadowrun stun
	// baton routes to the Stun monitor, a bullet to Physical (shadowrun-mvp
	// SR-M2/M3b). Empty ⇒ the canonical hp path (every non-Shadowrun weapon).
	// Normalized lowercase at pack load so it matches the entity's lowercase
	// pool.Set keys; flowed into combat.Stats.TargetPool by the holder's Stats()
	// builder.
	TargetPool string
	// CritThreatLow is the lowest d20 face that threatens a critical
	// (weapon-identity §4). Zero means unset — the engine defaults to the
	// natural maximum (only a 20 crits). Validated at load to be 0 or in
	// [2,20] (a 1 is always a fumble).
	CritThreatLow int
	// CritMultiplier is the weapon's critical damage-dice multiplier
	// (weapon-identity §4). Zero means unset — the engine uses the
	// configured global default. Validated at load to be non-negative.
	CritMultiplier int
	// Size is the weapon's size category (size-and-wielding §2) from the
	// engine size vocabulary (tiny … huge). Empty ⇒ the baseline size, so a
	// weapon authored before this feature is one-handed for a baseline-size
	// wielder. The wield mode (light / one-handed / two-handed / too-large) is
	// DERIVED from (weapon size − wielder size), never declared. Validated at
	// pack load; normalized lowercase.
	Size string
	// Ranged weapon metadata (ranged-combat §2). All optional; a weapon
	// that declares no RangedClass is melee, unchanged. RECORDED in this
	// data slice — inert until the combat consumer (ammo + Strength rules +
	// the round-loop ammo hook) lands, mirroring how weapon-identity and
	// armor-depth shipped their data ahead of the consumer.
	//
	// RangedClass is "thrown" or "projectile" (item.RangedThrown /
	// RangedProjectile); empty means melee. Validated at load; lowercased.
	RangedClass string
	// AmmoKind matches a projectile weapon to the ammunition it fires and
	// an ammo item to what it is (ranged-combat §3) — compared verbatim. On
	// a projectile weapon it names the kind it consumes (e.g. "arrow"); on
	// an ammunition item it names the kind it supplies. Empty on a
	// thrown/melee weapon and on a non-ammo item. Normalized lowercase. A
	// projectile weapon must declare one (validated at load).
	AmmoKind string
	// RangedStyle names the weapon's flavor voice for ranged moments — running
	// dry, chambering, loosing a shot (rangedflavor). Content-defined (e.g.
	// "bow" / "crossbow" / "thrown" / a future "firearm"); a pack keys its
	// ranged_flavor vocabulary off it. Empty resolves to the default style, then
	// the engine floor, so it never breaks. Purely presentational — mechanics
	// ride RangedClass / AmmoKind / ReloadTicks. Normalized lowercase.
	RangedStyle string
	// RangeIncrement is the distance unit over which accuracy falls off
	// (ranged-combat §2, §5.3). Zero means unset. Inert until Slice B's
	// range bands consume it. Validated non-negative at load.
	RangeIncrement int
	// FireModes lists the selectable firing modes a ranged weapon supports
	// (ranged-combat §5.5) — a subset of {single, burst, auto}. A burst/auto
	// mode trades ammo + accuracy (recoil) for damage; the player picks the
	// active mode with `firemode`, clamped to this set. Empty (the default, and
	// all melee/thrown) means single-fire only — the pre-fire-modes behavior.
	// Normalized lowercase + validated against the known set at load.
	FireModes []string
	// RecoilComp is the weapon's inherent recoil compensation (SR5 "RC"): it
	// reduces the to-hit penalty a burst/full-auto firing mode imposes
	// (ranged-combat §5.5/§5.6), floored at zero. 0 (the default) = no
	// compensation — the full recoil bites. Validated non-negative at load.
	RecoilComp int
	// ArmorPen is the weapon's armor penetration (SR5 "AP", authored as a
	// positive magnitude). It reduces the defender's armor-derived soak
	// (Mitigation) at combat time, CAPPED at the worn-armor rating — so it
	// bypasses armor, never the creature's innate toughness or a typed
	// resistance. 0 (the default) = no penetration. Inert in AC-based rulesets
	// (WoT), where armor is AC not soak. Validated non-negative at load.
	ArmorPen int
	// ReloadTicks marks a projectile weapon that must be reloaded between shots
	// (a crossbow) and is its load time in engine ticks (action-economy.md §7.1).
	// 0 = fires freely (a bow). Validated non-negative at load.
	ReloadTicks int
	// Magazine is a firearm's magazine capacity (SR5 "Ammo"). > 0 marks a
	// MAGAZINE weapon: firing draws from a loaded-round count on the item
	// instance and `reload` refills it. 0 = per-shot loose rounds (bow) or the
	// single-chamber ReloadTicks model (crossbow). Validated non-negative.
	Magazine int
	// ReloadMethod names how a magazine weapon reloads (SR5 reloading table).
	// Empty on a magazine weapon defaults to "clip". Normalized lowercase.
	ReloadMethod string
	// HolderFits marks an ammunition HOLDER (clip/magazine/belt) and names the
	// weapon family it fits (ammo-and-reloading §2). A holder also has Magazine
	// (capacity) + AmmoKind. Empty = not a holder. Normalized lowercase.
	HolderFits string
	// Preload seeds a holder's loaded-round count at spawn so a pre-loaded holder
	// starts full (ammo-and-reloading §6). 0 = spawns empty. Clamped to Magazine.
	Preload int
	// AcceptsHolder marks a HOLDER-FED weapon and names the holder family it takes
	// (ammo-and-reloading §5): it fires from an inserted holder rather than its
	// own Magazine. Mutually exclusive with Magazine. Empty = not holder-fed.
	AcceptsHolder string
	// StrRating caps the positive Strength damage bonus a projectile weapon
	// grants (ranged-combat §4 — a composite / Strength-rated bow). nil is
	// the default projectile rule (no positive Strength bonus; a negative
	// modifier still applies). Ignored for thrown/melee weapons. A non-nil
	// value is the cap, validated non-negative at load.
	StrRating *int
	// Armor depth (armor-depth §2). All optional and, in this slice,
	// RECORDED ONLY — inert until the AC / mitigation / proficiency /
	// check-penalty slices consume them (mirrors how DamageTypes shipped
	// inert ahead of armor depth). A non-armor item declares none.
	//
	// ArmorBonus is the structured AC contribution (armor-depth §3) — the
	// additive armor term in the decomposed defense value. Zero means none.
	// Distinct from a legacy `{stat: ac}` modifier, which remains a "misc"
	// AC contributor; armor_bonus is the term the max-Dex cap composes with.
	ArmorBonus int
	// ArmorMaxDex caps how much of the wearer's Dex modifier counts toward
	// AC while worn (armor-depth §3). A pointer because zero is a valid cap
	// (heavy armor): nil means "no cap" (the full Dex modifier applies),
	// a non-nil value is the cap. Validated non-negative at load.
	ArmorMaxDex *int
	// ArmorCheckPenalty is the magnitude (non-negative) of the penalty this
	// armor imposes on Strength/Dexterity skill checks while worn
	// (armor-depth §6). Zero means none.
	ArmorCheckPenalty int
	// ArmorTier is the armor's proficiency tier (armor-depth §5) from the
	// engine armor-tier vocabulary (light/medium/heavy). Empty means
	// "untiered". Validated at pack load; normalized lowercase.
	ArmorTier string
	// Resistances is the per-damage-type damage reduction this item grants
	// while worn (armor-depth §4) — keyed by damage type (the weapon.go
	// vocabulary), value the amount soaked. nil/empty means none. Keys are
	// validated against the damage-type vocabulary and values to be
	// non-negative at pack load; keys normalized lowercase.
	Resistances map[string]int
	// Angreal (wot-the-one-power.md — S2 angreal/sa'angreal). AngrealPower is
	// the One Power amplification rating of a channeling device (1–3 angreal,
	// 4–10 sa'angreal — the same scale, the name is flavor): while a
	// same-gender channeler has the device equipped, it multiplies the woven
	// damage/heal payload upward (the engine analog of the d20 "extra effective
	// casting level"). Zero ⇒ the item is not an angreal. AngrealGender gates
	// which channeler the device serves ("male" / "female" — the saidin/saidar
	// split); a cross-gender device is inert (invisible/dead to the wrong
	// channeler). WoT-specific content; inert outside the WoT pack (no item
	// carries it). Validated at pack load: a non-zero rating requires a
	// positive value and a male/female gender. Normalized lowercase.
	AngrealPower  int
	AngrealGender string
	// Special is the set of special-maneuver tags this weapon enables
	// (special-weapons.md §2 — the increment J starter set: trip / disarm).
	// nil/empty means an ordinary weapon (every weapon today). Validated against
	// the engine vocabulary at pack load; normalized lowercase. (reach is NOT
	// here — it is the numeric Reach field below.)
	Special []string
	// Reach is the weapon's reach rating (special-weapons.md §3) — a numeric
	// weapon stat shared across rulesets, not a maneuver tag. 0 means an
	// ordinary close weapon (every weapon today). WoT reads `Reach > 0` as
	// "strikes at the near range band as well as melee"; a Shadowrun pack reads
	// the NET reach (attacker − defender) as a defense-roll modifier. Validated
	// non-negative at pack load.
	Reach int
	// TripBonus / DisarmBonus are the magnitudes (non-negative) by which a
	// weapon carrying the matching tag raises that maneuver's save DC
	// (special-weapons.md §4/§5): the tag says WHETHER, the scalar says HOW
	// MUCH. A bonus with no matching tag is an authoring error (load fails) so
	// a typo cannot ship an inert magnitude. Zero with the tag present means
	// "the engine default bonus".
	TripBonus   int
	DisarmBonus int
	// The fields below are RECORDED-ONLY equipment-depth metadata
	// (special-weapons.md / the equipment.md authoring surface). Each is
	// validated at load but has NO engine consumer yet — they let the WoT
	// equipment table be authored once at full data, each lighting up for free
	// when its mechanic's slice lands (the pattern damage_types / reach used).
	// Authoring rule: record the real value; the mechanic stays inert, never
	// wrong (a `Subdual` weapon still deals normal damage until subdual ships).
	//
	// Subdual marks a nonlethal weapon (the source's `§` — sap, whip, unarmed):
	// it should deal subdual/knock-out damage. Inert until a subdual damage mode
	// exists; a subdual weapon deals ordinary damage today.
	Subdual bool
	// DoubleDamage is the SECOND damage figure of a double weapon (the source's
	// `1d6/1d6` quarterstaff, `1d6/1d8` ashandarei) — the dice of the extra
	// off-hand attack a double weapon grants when wielded two-handed. An NdM±K
	// string, validated at load like WeaponDamage. Empty ⇒ not a double weapon.
	// Inert until double-weapon support (a two-weapon-fighting extension) reads
	// it; a double weapon swings only its main WeaponDamage today.
	DoubleDamage string
	// ArmorSpeed is the worn-speed value of a piece of armor (the source's Speed
	// column — heavy armor slows the wearer). 0 ⇒ unset (no speed effect).
	// Validated non-negative. Inert until the armor speed-penalty consumer
	// (armor §7 / encumbrance I tail) reads it.
	ArmorSpeed int
	// Reputation is the SIGNED reputation delta a piece of visible gear confers
	// while equipped (the source's masterwork +1 / Trolloc scythesword −2). 0 ⇒
	// none. Inert until S8 reputation reads it.
	Reputation int
	// EssenceCost is the Essence a piece of installed cyberware/bioware spends
	// while equipped, in TENTHS (Shadowrun SR-M4 — wired reflexes 2.0 → 20,
	// cybereyes 0.2 → 2). Stored as an integer of tenths because the essence
	// pool (like every pool.Pool) is integer-valued; content authors the SR
	// decimal and the loader multiplies by ten. 0 ⇒ costs no Essence (ordinary
	// gear). The essence pool's current is derived as max − Σ installed cost.
	EssenceCost int
	// Item modification (item-modification.md — Slice A: capacity). Two roles on
	// one item family:
	//
	// Capacity is a modifiable HOST's total modification budget (§2). > 0 marks
	// the item as accepting installed mods whose costs sum to at most this;
	// 0/absent ⇒ unmodifiable (unchanged from today). Validated non-negative.
	Capacity int
	// ModHost marks the item as a MODIFICATION and names the host class it fits
	// (§3 host-compatibility key) — this slice: "armor". A mod installs only into
	// a host carrying a matching tag (§4). Empty ⇒ not a mod. Normalized lowercase.
	ModHost string
	// ModCapacityCost is a modification's capacity cost — how much of a host's
	// budget it consumes (§3/§4). v1 is a flat cost (each rating authored as its
	// own template, §3). Meaningful only when ModHost is set; validated
	// non-negative.
	ModCapacityCost int
	// Mounts is a modifiable WEAPON host's set of exposed mount points
	// (weapon-accessories.md §2 — barrel/under-barrel/side/top/stock/internal).
	// Each mount holds at most one accessory. Empty ⇒ the weapon accepts no
	// accessories. Normalized lowercase at load. A host uses the mount-slot rule
	// (Mounts) OR the capacity rule (Capacity), not both.
	Mounts []string
	// AccessoryMounts is a MODIFICATION's compatible mount points
	// (weapon-accessories.md §3): the mounts it may occupy on a weapon host.
	// Non-empty ⇒ the mod is a mount ACCESSORY (installed by the mount rule, not
	// capacity). Requires ModHost set; mutually exclusive with ModCapacityCost.
	// Normalized lowercase.
	AccessoryMounts []string
	// Protection is the set of environmental protection keys a MODIFICATION grants
	// its host while the host is worn (item-modification §6 — a new contribution
	// channel; the first consumer is the biome-hazard immunity gate, area-effects
	// §4.6). A worn host with an installed mod granting a hazard's protection_key
	// makes the wearer immune, exactly as an intrinsic protection tag does. Keys
	// are content-declared (matched verbatim against a biome's protection_key);
	// normalized lowercase. Meaningful only on a mod (ModHost set).
	Protection []string
	// Grants is the set of general CAPABILITY keys a MODIFICATION confers on its
	// host while equipped (item-modification §6). Unlike Protection (a hazard-
	// immunity consumer), these are opaque content-declared flags a cross-item
	// runtime consumer keys off — the first is the smartlink↔smartgun pairing: a
	// smartlink cybereye enhancement grants "smartlink", a smartgun weapon
	// accessory grants "smartgun", and combat adds a to-hit bonus when the attacker
	// has both. Normalized lowercase. Meaningful only on a mod (ModHost set).
	Grants []string
}

// Errors callers may distinguish at the boundary.
var (
	ErrTemplateNotFound = errors.New("item template not found")
	ErrDuplicateID      = errors.New("duplicate item template id")
)

// LegacySlotName extracts the historical single `slot` string from an
// item property bag — the §3.2 backward-compat bridge used when a
// template declares no explicit EligibleSlots. Returns the
// lowercased/trimmed name and true when present and non-empty. The
// property is left in the bag; it is merely also surfaced as eligibility.
// Shared by the pack loader (template decode) and the instance builder so
// the bridge has a single definition.
func LegacySlotName(props map[string]any) (string, bool) {
	if props == nil {
		return "", false
	}
	raw, ok := props["slot"]
	if !ok {
		return "", false
	}
	s, ok := raw.(string)
	if !ok {
		return "", false
	}
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "", false
	}
	return s, true
}

// Templates is the boot-time registry of item templates. Safe for
// concurrent reads; mutations (Add, TryAdd) MUST happen at boot before
// serving — same invariant as world.World.
type Templates struct {
	mu  sync.RWMutex
	all map[TemplateID]*Template
}

// NewTemplates returns an empty registry.
func NewTemplates() *Templates {
	return &Templates{all: make(map[TemplateID]*Template)}
}

// Add registers t, replacing any existing template with the same id
// (spec §2.1: later registrations replace earlier ones).
func (r *Templates) Add(t *Template) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.all[t.ID] = t
}

// TryAdd registers t and returns ErrDuplicateID if a template with
// that id is already present. Used by the pack loader to catch
// cross-pack id collisions before they silently overwrite.
func (r *Templates) TryAdd(t *Template) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.all[t.ID]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateID, t.ID)
	}
	r.all[t.ID] = t
	return nil
}

// Get returns the template with id and ErrTemplateNotFound if absent.
func (r *Templates) Get(id TemplateID) (*Template, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.all[id]
	if !ok {
		return nil, fmt.Errorf("item.Templates.Get(%q): %w", id, ErrTemplateNotFound)
	}
	return t, nil
}

// Has reports whether id is registered.
func (r *Templates) Has(id TemplateID) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.all[id]
	return ok
}

// Count returns the number of registered templates.
func (r *Templates) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.all)
}

// All returns a snapshot of every registered template. Order is
// unspecified; callers that need determinism must sort.
func (r *Templates) All() []*Template {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Template, 0, len(r.all))
	for _, t := range r.all {
		out = append(out, t)
	}
	return out
}
