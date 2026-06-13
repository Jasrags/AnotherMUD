package channel

// known is the curated channel vocabulary (design §3, §8): the engine
// only consumes these, so a mapping declaring any other channel is a
// content error caught at load. Reserved channels (no consumer yet) are
// included so a pack may declare them ahead of their wiring.
var known = map[Channel]struct{}{
	Attack:         {},
	Defense:        {},
	DamageBonus:    {},
	Mitigation:     {},
	Initiative:     {},
	Potency:        {},
	ResistBacklash: {},
}

// IsKnown reports whether ch is part of the curated channel vocabulary.
// The pack loader rejects an unknown channel name so content cannot
// declare a channel the engine has no consumer for (design §8).
func IsKnown(ch Channel) bool {
	_, ok := known[ch]
	return ok
}

// Registry accumulates channel→formula-source declarations across packs
// (later-wins per channel, mirroring the theme registry's "downstream
// pack re-themes" rule) and builds them into a Mapping. Populated
// single-threaded at pack load, then Build()-t once at composition; not
// safe for concurrent mutation.
type Registry struct {
	formulas map[Channel]string
}

// NewRegistry returns an empty channel-formula registry.
func NewRegistry() *Registry {
	return &Registry{formulas: make(map[Channel]string)}
}

// Register sets the formula source for ch, overriding any earlier value
// (later-wins). The caller is responsible for validating IsKnown(ch) and
// that src parses (the loader does both for attribution); Register itself
// is a plain store.
func (r *Registry) Register(ch Channel, src string) {
	r.formulas[ch] = src
}

// Len returns the number of registered channel formulas. Composition uses
// it to decide whether any content mapping was loaded (0 ⇒ fall back to
// the Go baseline).
func (r *Registry) Len() int { return len(r.formulas) }

// Build compiles the accumulated formulas into a Mapping, parsing each.
// Returns the first parse error (as a *MappingError). An empty registry
// yields an all-defaults Mapping.
func (r *Registry) Build() (*Mapping, error) {
	return NewMapping(r.formulas)
}
