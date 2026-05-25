package mob

// Reaction is the keyword the disposition evaluator emits for a
// (mob, player) pair. Spec mobs-ai-spawning §5.1 lists the canonical
// set; unknown values are allowed (content may extend the set) and
// the engine treats them as no-op at dispatch time (no event fires).
type Reaction string

const (
	ReactionHostile  Reaction = "hostile"
	ReactionWary     Reaction = "wary"
	ReactionFriendly Reaction = "friendly"
	ReactionNeutral  Reaction = "neutral"
)

// Definition is the structured disposition policy a mob carries when
// its reaction depends on player state. Spec §5.1: "an ordered list
// of conditional rules" plus a default. Evaluation order is the slice
// order; first match wins (§5.3 step 4).
//
// Empty Default is treated as ReactionNeutral by the evaluator so a
// mob with rules but no explicit default still has a meaningful
// fallback. A mob with no Definition at all is handled at the
// evaluator boundary (no dispatch).
type Definition struct {
	Default Reaction
	Rules   []Rule
}

// Rule is one entry in a Definition. A rule with no conditions
// matches anything — typically used as the last entry so a "default"
// can sit at the end of the list rather than the Definition.Default
// field. Spec §5.3.
//
// Alignment fields land with M8 progression. Today the evaluator
// ignores them (treats their condition as "not present") because the
// player has no alignment integer to match against. Decoding them
// now keeps the YAML schema stable so content can be authored ahead
// of the runtime support.
type Rule struct {
	// HasTag, when non-empty, requires the player to carry the named
	// tag. Today this is the only matchable condition.
	HasTag string

	// Reaction is what the rule emits on match. Required.
	Reaction Reaction

	// MinAlignment / MaxAlignment / Buckets — declared by spec §5.3,
	// not yet honored by the evaluator (M8 dependency). HasAlignment*
	// flags distinguish "field omitted" from "field set to 0".
	MinAlignment    int
	MaxAlignment    int
	HasMinAlignment bool
	HasMaxAlignment bool
	Buckets         []string
}

// HasConditions reports whether r carries any condition. A rule with
// no conditions matches every player (spec §5.3 "A rule with no
// conditions matches anything").
func (r Rule) HasConditions() bool {
	return r.HasTag != "" || r.HasMinAlignment || r.HasMaxAlignment || len(r.Buckets) > 0
}
