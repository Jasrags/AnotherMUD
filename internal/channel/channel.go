package channel

// Channel is one fixed, engine-consumed derived input. The vocabulary is
// CURATED, not open-ended: a channel exists only because some kernel path
// reads it. Content may declare arbitrary *attributes* and map them into
// these channels, but it cannot invent a channel the engine has no
// consumer for (design: docs/themes/channel-vocabulary.md §3).
//
// Kept a typed string (like pool.Kind / progression.StatType) so YAML
// round-trips and an unmapped channel reads its configured default rather
// than erroring.
type Channel string

const (
	// Attack feeds the to-hit roll's modifier side (combat.Stats.HitMod).
	Attack Channel = "attack"
	// Defense feeds the to-hit roll's difficulty side (combat.Stats.AC).
	Defense Channel = "defense"
	// DamageBonus is added to rolled weapon damage (today STRBonus(STR)).
	DamageBonus Channel = "damage_bonus"

	// --- vocabulary reserved ahead of consumers (design §3) ---
	// These are defined so mappings can declare them, but the engine does
	// not read them yet; their consumers land with later slices (the §6
	// damage/mitigation struct, initiative ordering, channeling).
	//
	// Mitigation subtracts from incoming damage (soak / armor); wired with
	// the §6 damage-struct slice.
	Mitigation Channel = "mitigation"
	// Initiative orders action cadence; wired if/when turn order exists.
	Initiative Channel = "initiative"
	// Potency scales ability/weave/spell effect magnitude.
	Potency Channel = "potency"
	// ResistBacklash resists drain / overchannel / madness (WoT S2).
	ResistBacklash Channel = "resist.backlash"
)

// DefaultValue is the value a channel reads when a Mapping declares no
// formula for it (config surface, design §11). Defense defaults to 10 —
// the engine's "no armor, no defensive stats" AC, so an empty mapping
// reproduces the pre-channel baseline. Every other channel defaults to 0
// (no contribution).
func DefaultValue(ch Channel) int {
	if ch == Defense {
		return DefaultDefense
	}
	return 0
}

// DefaultDefense is the unmapped-Defense baseline (the legacy AC 10).
const DefaultDefense = 10

// Mapping is one pack/ruleset's derivation: a channel → formula table.
// Built once at pack load (Parse each formula then), evaluated per entity
// against a stat-lookup. A nil/empty Mapping is valid — every channel
// then reads its DefaultValue, which is exactly the pre-channel engine
// behavior, so an unmapped pack is behavior-neutral.
//
// Immutable after construction; safe for concurrent Value calls.
type Mapping struct {
	formulas map[Channel]Expr
}

// NewMapping compiles a channel → formula-source table into a Mapping.
// Returns the first parse error encountered (keyed by channel) so a pack
// with a malformed formula fails loudly at load rather than silently
// reading a default at runtime. A nil/empty input yields an all-defaults
// Mapping (not an error).
func NewMapping(formulas map[Channel]string) (*Mapping, error) {
	m := &Mapping{formulas: make(map[Channel]Expr, len(formulas))}
	for ch, src := range formulas {
		expr, err := Parse(src)
		if err != nil {
			return nil, &MappingError{Channel: ch, Source: src, Err: err}
		}
		m.formulas[ch] = expr
	}
	return m, nil
}

// MappingError wraps a per-channel formula parse failure with the channel
// and source for a load-time diagnostic.
type MappingError struct {
	Channel Channel
	Source  string
	Err     error
}

func (e *MappingError) Error() string {
	return "channel mapping [" + string(e.Channel) + "] (" + e.Source + "): " + e.Err.Error()
}

func (e *MappingError) Unwrap() error { return e.Err }

// Has reports whether the mapping declares a formula for ch (vs. relying
// on the default).
func (m *Mapping) Has(ch Channel) bool {
	if m == nil {
		return false
	}
	_, ok := m.formulas[ch]
	return ok
}

// BaselineMapping returns the engine's behavior-preserving default
// mapping: the channels that have live consumers today, defined to
// reproduce the pre-channel-layer derivation exactly — attack reads the
// `hit_mod` stat, defense reads the `ac` stat. A boot using this mapping
// behaves identically to before the channel layer existed; content packs
// override per ruleset. It grows as more channels gain kernel consumers
// (damage_bonus once the §6 damage struct lands, etc.).
//
// Engine-defined, so a malformed formula here is a programmer bug — panics
// rather than returning an error.
func BaselineMapping() *Mapping {
	m, err := NewMapping(map[Channel]string{
		Attack:  "hit_mod",
		Defense: "ac",
		// trunc (round toward zero), NOT mod (floor), to match combat.STRBonus
		// = (str-10)/2 under Go integer division — exact for all int str. The
		// `+ damage_mod` term composes a flat weapon-damage modifier (the
		// sibling of `hit_mod` on the attack channel): 0 for an ordinary
		// fighter — byte-identical to before — and the grade step for a
		// power-wrought weapon (masterwork §3).
		DamageBonus: "trunc((str - 10) / 2) + damage_mod",
		// Mitigation is intentionally unmapped → defaults to 0: fantasy folds
		// armor into Defense (AC), so the §6 soak step is inert until a
		// setting (e.g. Shadowrun) maps it.
	})
	if err != nil {
		panic("channel.BaselineMapping: " + err.Error())
	}
	return m
}

// Value computes the channel's value for an entity. lookup resolves an
// attribute name to its effective value (typically StatBlock.Effective,
// 0 for unknown). A channel with no declared formula — or a nil Mapping —
// reads DefaultValue(ch), so callers always get a usable number.
func (m *Mapping) Value(ch Channel, lookup func(name string) int) int {
	if m != nil {
		if expr, ok := m.formulas[ch]; ok {
			return expr.Eval(lookup)
		}
	}
	return DefaultValue(ch)
}
