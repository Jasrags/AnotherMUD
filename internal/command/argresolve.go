package command

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// ArgResolveError is the resolver's contract failure type. Carries
// the argument NAME so the dispatcher can format the spec's
// "What <name>?" / "<resolver error>" diagnostics.
//
// Wrapped chain: errors.Is + errors.As both pass through the
// underlying Cause, so a handler can match on a specific resolver
// failure (e.g. ErrNotANumber) without losing the arg-name
// context.
type ArgResolveError struct {
	// ArgName is the ArgDefinition.Name that failed to resolve.
	// Empty for resolver-internal errors that don't tie back to
	// a single argument (rare; only happens when the registry
	// can't find a resolver at all).
	ArgName string

	// Cause is the resolver-supplied error. For required-missing
	// it is ErrMissingRequired; for known type failures the
	// resolver returns one of the package sentinels (see below).
	Cause error
}

// Error implements error. Spec §5.4: the first required-arg
// failure short-circuits and the error string is what gets sent
// back to the player. For ErrMissingRequired the dispatcher
// formats the spec "What <name>?" string; otherwise it surfaces
// the underlying Cause text verbatim.
func (e *ArgResolveError) Error() string {
	if errors.Is(e.Cause, ErrMissingRequired) {
		return fmt.Sprintf("What %s?", e.ArgName)
	}
	return e.Cause.Error()
}

// Unwrap exposes Cause so errors.Is / errors.As traverse to it.
func (e *ArgResolveError) Unwrap() error { return e.Cause }

// Standard resolver-failure sentinels. The engine-baseline
// resolvers (keyword, text, number) raise these; pack resolvers
// MAY raise their own typed errors that bubble through
// ArgResolveError.Cause unchanged.
var (
	// ErrMissingRequired is the resolver's "this required arg
	// had no token" verdict. Spec §5.4 step 3 / step 2 (when
	// text type has no remainder). The dispatcher converts this
	// to "What <name>?" for the player; tests match on it
	// directly to assert short-circuit behavior.
	ErrMissingRequired = errors.New("argres: required argument missing")

	// ErrNotANumber is raised by the number resolver when the
	// token does not parse as an integer (spec §5.2 row).
	ErrNotANumber = errors.New("That's not a number.")

	// ErrUnknownArgType is the registry's "this name was never
	// registered" verdict, surfaced when the resolver falls back
	// to keyword passthrough so callers can log a warning.
	ErrUnknownArgType = errors.New("argres: unknown arg type")
)

// ResolverInput is the per-argument call shape an ArgResolver
// receives. The resolver decides whether to consume zero or one
// tokens via the returned Consumed field — the driver advances
// the cursor by that many tokens.
//
// Tokens is a view into the remaining input AFTER preposition
// skipping. Resolvers should NOT mutate it; mutation is the
// driver's responsibility. The slice is positionally indexed by
// resolver semantics (most engine resolvers consume Tokens[0]).
type ResolverInput struct {
	// Def is the ArgDefinition currently being resolved. Resolvers
	// read flags like Bulk / BypassVisibility through it.
	Def ArgDefinition

	// Tokens is the remaining input after any preposition skip.
	// Empty when the user gave fewer tokens than required.
	Tokens []string

	// Context carries the per-resolve scope (actor inventory,
	// room items, room entities, etc.) the entity-flavored
	// resolvers consult. Zero value works fine for the
	// keyword/text/number resolvers, which ignore it.
	Context ResolveContext
}

// ResolverOutput is the per-argument resolution result. Value is
// what the dispatcher stores under Def.Name in the final map;
// Consumed is the number of tokens the resolver claimed (0 or
// more). A resolver that fails returns (zero, error) and Consumed
// is ignored.
type ResolverOutput struct {
	// Value is the resolved typed value the handler will read.
	// Shape depends on the resolver: a number resolver returns
	// int; keyword/text return string; entity resolvers
	// (M17.2b+) return structured records.
	Value any

	// Consumed is the number of tokens the resolver took from
	// Tokens. For keyword/number this is 1; for text it equals
	// len(Tokens) since text slurps the remainder.
	Consumed int
}

// ArgResolver is the function-shape resolver implementation
// registered under an ArgType name. Spec §5.3 says packs may
// register their own; the engine baseline registers the §5.2
// types up front via NewArgResolverRegistry.
type ArgResolver func(ResolverInput) (ResolverOutput, error)

// ArgResolverRegistry maps ArgType names to their resolver
// implementations. Safe for concurrent reads after construction;
// concurrent Register calls during boot are serialized by the
// internal mutex.
//
// Engine-baseline types are seeded by NewArgResolverRegistry and
// CANNOT be overridden via Register (spec §5.3). Pack registrations
// of an existing pack name (last-wins) emit a warning via the
// returned error so the loader can decide whether to log or fail.
type ArgResolverRegistry struct {
	mu        sync.RWMutex
	resolvers map[ArgType]ArgResolver
}

// NewArgResolverRegistry returns a registry pre-loaded with the
// M17.2a engine-baseline resolvers (keyword, text, number). The
// remaining §5.2 types are seeded as placeholders that fall
// through to keyword passthrough until their M17.2b/c
// implementations land — the placeholder logs through
// ErrUnknownArgType so a too-eager use surfaces as a warning
// rather than a silent succeed.
func NewArgResolverRegistry() *ArgResolverRegistry {
	r := &ArgResolverRegistry{resolvers: make(map[ArgType]ArgResolver)}
	r.resolvers[ArgKeyword] = resolveKeyword
	r.resolvers[ArgText] = resolveText
	r.resolvers[ArgNumber] = resolveNumber
	// M17.2b entity / inventory / room family. These consult
	// ResolverInput.Context for actor + room scope.
	r.resolvers[ArgInventory] = resolveInventory
	r.resolvers[ArgRoomItem] = resolveRoomItem
	r.resolvers[ArgEntity] = resolveEntity
	r.resolvers[ArgPlayer] = resolvePlayer
	r.resolvers[ArgNPC] = resolveNPC
	r.resolvers[ArgGiveTarget] = resolveGiveTarget
	r.resolvers[ArgContainer] = resolveContainer
	r.resolvers[ArgVisible] = resolveVisible
	r.resolvers[ArgFindable] = resolveFindable
	// M17.2c door resolver. Consults ResolverInput.Context.Doors.
	r.resolvers[ArgDoor] = resolveDoor
	return r
}

// Register installs resolver under name. Rejects engine-type
// collisions with a non-nil error AND leaves the existing engine
// resolver intact (immutability per §5.3). Returns nil on success.
// A pack-type collision overwrites the prior registration and
// returns nil — the caller may choose to warn via the loader.
//
// The error is a sentinel (ErrEngineTypeImmutable) so the loader
// can distinguish from a malformed-name error.
func (r *ArgResolverRegistry) Register(name ArgType, resolver ArgResolver) error {
	if resolver == nil {
		return fmt.Errorf("argres: nil resolver for %q", name)
	}
	if IsEngineArgType(name) {
		return fmt.Errorf("argres: %w: %s", ErrEngineTypeImmutable, name)
	}
	r.mu.Lock()
	r.resolvers[name] = resolver
	r.mu.Unlock()
	return nil
}

// ErrEngineTypeImmutable is the sentinel Register raises when a
// pack tries to override an engine-baseline type name.
var ErrEngineTypeImmutable = errors.New("argres: engine arg type cannot be overridden")

// Lookup returns the registered resolver for name. (nil, false)
// for an unknown name — the driver falls back to keyword
// passthrough with a warning.
func (r *ArgResolverRegistry) Lookup(name ArgType) (ArgResolver, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res, ok := r.resolvers[name]
	return res, ok
}

// ResolveArgs is the spec §5.4 driver. Walks defs in declaration
// order, consuming tokens left-to-right with preposition skipping
// and text early-out, and returns the resolved {name → value} map
// or the first required-arg failure.
//
// The third return value is the unconsumed-token tail — useful
// when an admin wants to pass the rest verbatim to a sub-command,
// but most handlers ignore it. The driver returns it even on
// success so a future "trailing tokens are an error" check can
// surface at the dispatcher layer rather than re-tokenizing.
//
// Warning behavior: unknown ArgTypes fall through to keyword per
// §5.3 and the per-arg ResolverOutput records the fallback in the
// returned warnings slice. Callers may log these or ignore them.
func (r *ArgResolverRegistry) ResolveArgs(defs []ArgDefinition, tokens []string) (map[string]any, []string, []string, error) {
	return r.ResolveArgsWithContext(defs, tokens, ResolveContext{})
}

// ResolveArgsWithContext is the M17.2b context-aware form of
// ResolveArgs. Entity / inventory / room resolvers consult ctx
// to scan the actor's contents and the current room. The driver
// behavior is otherwise identical to ResolveArgs.
func (r *ArgResolverRegistry) ResolveArgsWithContext(defs []ArgDefinition, tokens []string, ctx ResolveContext) (map[string]any, []string, []string, error) {
	out := make(map[string]any, len(defs))
	var warnings []string
	cursor := 0

	for _, def := range defs {
		// Step 1: preposition skip (case-insensitive on a single
		// look-ahead).
		if cursor < len(tokens) && hasPrep(def.Prepositions, tokens[cursor]) {
			cursor++
		}

		remaining := tokens[cursor:]

		// Step 2: text early-out. Text slurps the remainder
		// regardless of preposition; if no tokens remain and
		// required, fail with the canonical "What <name>?".
		if def.Type == ArgText {
			if len(remaining) == 0 {
				if !def.Optional {
					return nil, warnings, tokens[cursor:], &ArgResolveError{
						ArgName: def.Name,
						Cause:   ErrMissingRequired,
					}
				}
				out[def.Name] = nil
				continue
			}
			out[def.Name] = strings.Join(remaining, " ")
			cursor = len(tokens)
			continue
		}

		// Step 3: missing-required short-circuit.
		if len(remaining) == 0 {
			if !def.Optional {
				return nil, warnings, tokens[cursor:], &ArgResolveError{
					ArgName: def.Name,
					Cause:   ErrMissingRequired,
				}
			}
			out[def.Name] = nil
			continue
		}

		// Step 4: dispatch to the registered resolver.
		resolver, ok := r.Lookup(def.Type)
		if !ok {
			warnings = append(warnings,
				fmt.Sprintf("argres: unknown type %q for %q; falling back to keyword",
					def.Type, def.Name))
			resolver = resolveKeyword
		}
		result, err := resolver(ResolverInput{Def: def, Tokens: remaining, Context: ctx})
		if err != nil {
			if def.Optional && errors.Is(err, ErrMissingRequired) {
				out[def.Name] = nil
				continue
			}
			return nil, warnings, tokens[cursor:], &ArgResolveError{
				ArgName: def.Name,
				Cause:   err,
			}
		}
		// Step 5: store the resolved value and advance the cursor
		// by however many tokens the resolver claimed.
		out[def.Name] = result.Value
		cursor += result.Consumed
	}

	return out, warnings, tokens[cursor:], nil
}

// hasPrep reports whether token matches any preposition in list
// (case-insensitive). Empty list = no skip.
func hasPrep(list []string, token string) bool {
	if len(list) == 0 {
		return false
	}
	lower := strings.ToLower(token)
	for _, p := range list {
		if strings.ToLower(p) == lower {
			return true
		}
	}
	return false
}

// --- Engine-baseline resolvers ---

// resolveKeyword returns the raw token verbatim. Consumes one
// token. Spec §5.2 keyword row.
func resolveKeyword(in ResolverInput) (ResolverOutput, error) {
	return ResolverOutput{Value: in.Tokens[0], Consumed: 1}, nil
}

// resolveText is the §5.2 text row — slurps the remainder of the
// tokens joined by single spaces. The driver special-cases this
// (text never reaches resolveText because the early-out in
// ResolveArgs handles it), but we register a working resolver
// here for symmetry / pack reuse.
func resolveText(in ResolverInput) (ResolverOutput, error) {
	return ResolverOutput{
		Value:    strings.Join(in.Tokens, " "),
		Consumed: len(in.Tokens),
	}, nil
}

// resolveNumber returns the parsed int OR ErrNotANumber. Spec
// §5.2 number row. Negative ints are accepted (a content author
// who wants positive-only ranges can validate at the handler
// layer).
func resolveNumber(in ResolverInput) (ResolverOutput, error) {
	v, err := strconv.Atoi(in.Tokens[0])
	if err != nil {
		return ResolverOutput{}, ErrNotANumber
	}
	return ResolverOutput{Value: v, Consumed: 1}, nil
}
