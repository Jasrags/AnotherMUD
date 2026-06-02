package property

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// ValueType enumerates the closed set of typed property shapes the
// engine knows how to coerce on load. Unknown properties at save
// time round-trip via the tagged-value envelope (§4.4 / §4.5);
// the envelope only carries primitives, not these container shapes.
//
// Spec: docs/specs/persistence.md §2.2, §2.5.
type ValueType int

const (
	// TypeString is a plain string value.
	TypeString ValueType = iota + 1
	// TypeInt is a Go int. Distinct from TypeInt64 so platforms
	// where the two diverge can round-trip exactly.
	TypeInt
	// TypeInt64 is an int64. Useful for timestamp-like fields
	// where the value can exceed 32 bits.
	TypeInt64
	// TypeFloat64 is a float64.
	TypeFloat64
	// TypeBool is a bool.
	TypeBool
	// TypeMapInt is a map[string]int.
	TypeMapInt
	// TypeMapString is a map[string]string.
	TypeMapString
	// TypeListString is a []string.
	TypeListString
)

// String returns the canonical name for the type. Stable across
// versions so the tagged-value envelope (which embeds this as the
// `kind:` field) round-trips identically.
func (v ValueType) String() string {
	switch v {
	case TypeString:
		return "string"
	case TypeInt:
		return "int"
	case TypeInt64:
		return "int64"
	case TypeFloat64:
		return "float64"
	case TypeBool:
		return "bool"
	case TypeMapInt:
		return "map_int"
	case TypeMapString:
		return "map_string"
	case TypeListString:
		return "list_string"
	default:
		return "unknown"
	}
}

// IsValid reports whether v is one of the enumerated types.
func (v ValueType) IsValid() bool {
	return v >= TypeString && v <= TypeListString
}

// Entry is a single registered property's metadata. Created by
// callers and handed to Registry.Register{Engine,Pack}.
//
// Spec: docs/specs/persistence.md §2.2.
type Entry struct {
	// Name is the snake_case property identifier. Must NOT contain
	// the colon (`:`) — the `pack:name` separator is reserved for
	// the full-key form.
	Name string
	// Pack is the owning pack name. Empty == engine-owned.
	Pack string
	// Description is shown by diagnostics tooling.
	Description string
	// Type is the closed-set value type. Required; an invalid
	// Type is a registration error.
	Type ValueType
	// AppliesTo is an optional set of entity-type strings the
	// property is meaningful on (diagnostic only — not enforced
	// at write time, per §2.2).
	AppliesTo []string
	// Transient marks a property the serializer MUST skip. Useful
	// for runtime-only state (e.g. mid-combat flags) that lives on
	// the same property bag but should never persist.
	Transient bool
	// AdminSettable opts the property into the admin `set` surface
	// (admin-verbs §4). Defaults false: a property is NOT settable via
	// `set` unless its registration explicitly flags it, so the generic
	// admin write can't poke arbitrary engine state. The `set property`
	// handler validates the typed value against Type before writing.
	AdminSettable bool
}

// Key returns the canonical lookup key for the entry. Engine
// entries use the bare name; pack entries use `pack:name`.
func (e Entry) Key() string {
	if e.Pack == "" {
		return e.Name
	}
	return e.Pack + ":" + e.Name
}

// DependencyResolver is the function the pack loader installs so
// shorthand lookups (a name with no pack prefix) can fall through
// the current pack's declared dependencies in order.
//
// Spec: docs/specs/persistence.md §2.4 step 3.
type DependencyResolver func(pack string) []string

// snakeCaseRE matches snake_case identifiers: lowercase letters,
// digits, and underscores; must start with a letter; no double
// underscores; no trailing underscore. Hyphens are explicitly
// rejected per spec §2.2.
var snakeCaseRE = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

// Registry is the engine-wide property catalog. Safe for concurrent
// reads (Get, IsTransient, ValueType, All); Register* are the only
// writers and are expected to be called at boot.
type Registry struct {
	mu        sync.RWMutex
	entries   map[string]Entry // keyed by Entry.Key()
	depResolv DependencyResolver
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{entries: make(map[string]Entry)}
}

// RegisterEngine installs e as an engine-scoped property. e.Pack
// MUST be empty (this method enforces it). Snake-case validation
// runs on e.Name; an invalid name or a duplicate (case-sensitive
// on Name) is an error.
func (r *Registry) RegisterEngine(e Entry) error {
	if e.Pack != "" {
		return fmt.Errorf("property.RegisterEngine %q: Pack must be empty (use RegisterPack)", e.Name)
	}
	return r.registerLocked(e)
}

// RegisterPack installs e as a pack-scoped property under packName.
// Snake-case validation runs on e.Name; an invalid name, a
// duplicate `pack:name` key, or a shadowing of an existing engine
// property is an error.
//
// Spec: §2.3 "A pack property MUST NOT shadow an engine property
// — attempting to do so MUST raise." Shadow check happens under the
// same write lock as the duplicate-key check so concurrent
// registrations cannot both pass the gate in a TOCTOU window.
func (r *Registry) RegisterPack(packName string, e Entry) error {
	if packName == "" {
		return fmt.Errorf("property.RegisterPack %q: empty packName", e.Name)
	}
	e.Pack = packName
	return r.registerLocked(e)
}

// registerLocked validates and installs e under e.Key(). Acquires
// r.mu for the write. Pack-scoped entries additionally check for
// shadowing an engine property under the same lock so the spec's
// "MUST raise" contract holds even under (today hypothetical)
// concurrent registration.
func (r *Registry) registerLocked(e Entry) error {
	if e.Name == "" {
		return fmt.Errorf("property.Register: empty Name")
	}
	if !snakeCaseRE.MatchString(e.Name) {
		return fmt.Errorf("property.Register %q: name must be snake_case (no hyphens, no leading digits)", e.Name)
	}
	if !e.Type.IsValid() {
		return fmt.Errorf("property.Register %q: invalid Type %d", e.Name, e.Type)
	}
	key := e.Key()
	r.mu.Lock()
	defer r.mu.Unlock()
	if e.Pack != "" {
		if _, shadows := r.entries[e.Name]; shadows {
			return fmt.Errorf("property.RegisterPack %q: shadows engine property", e.Name)
		}
	}
	if _, dup := r.entries[key]; dup {
		return fmt.Errorf("property.Register %q: duplicate key", key)
	}
	r.entries[key] = e
	return nil
}

// SetDependencyResolver installs the pack-dependency hook. The
// resolver MUST return the dependencies of the supplied pack in
// declaration order (the spec calls for "first hit wins" walking
// down that list at §2.4 step 3). nil-safe; passing nil disables
// dependency resolution.
func (r *Registry) SetDependencyResolver(fn DependencyResolver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.depResolv = fn
}

// Get resolves name (against currentPack as the shorthand context)
// per spec §2.4:
//
//  1. Direct match on the registered key.
//  2. If currentPack is non-empty and name has no `:`, try
//     `currentPack:name`.
//  3. If still unresolved AND a DependencyResolver is installed,
//     walk currentPack's dependencies and try `<dep>:name` for
//     each.
//
// currentPack may be empty when the caller has no pack context
// (e.g. lookups from engine code); in that case only step 1 runs.
func (r *Registry) Get(name, currentPack string) (Entry, bool) {
	r.mu.RLock()
	if e, ok := r.entries[name]; ok {
		r.mu.RUnlock()
		return e, true
	}
	if currentPack != "" && !strings.Contains(name, ":") {
		key := currentPack + ":" + name
		if e, ok := r.entries[key]; ok {
			r.mu.RUnlock()
			return e, true
		}
		dep := r.depResolv
		r.mu.RUnlock()
		if dep != nil {
			for _, d := range dep(currentPack) {
				r.mu.RLock()
				e, ok := r.entries[d+":"+name]
				r.mu.RUnlock()
				if ok {
					return e, true
				}
			}
		}
		return Entry{}, false
	}
	r.mu.RUnlock()
	return Entry{}, false
}

// IsTransient reports whether the resolved entry is transient.
// Returns false (i.e. "serialize it") when the name is unknown —
// matches §4.4 step 3, where unknown names go through the
// tagged-value envelope rather than being silently dropped.
func (r *Registry) IsTransient(name, currentPack string) bool {
	e, ok := r.Get(name, currentPack)
	return ok && e.Transient
}

// ValueType returns the registered type. Returns (_, false) for an
// unknown name.
func (r *Registry) ValueType(name, currentPack string) (ValueType, bool) {
	e, ok := r.Get(name, currentPack)
	if !ok {
		return 0, false
	}
	return e.Type, true
}

// All returns every registered entry. Fresh slice; callers may
// mutate it. Order is unstable (map iteration); sort at the
// rendering layer.
func (r *Registry) All() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Entry, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e)
	}
	return out
}

// Len returns the number of registered entries.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries)
}
