// Package rangedflavor holds the per-pack, per-weapon-style flavor text for
// ranged-weapon moments — running dry, firing on an empty chamber, loading,
// and loosing a shot. It exists so the *voice* of these lines is content, not
// code: a "*click*" reads right for a crossbow or a firearm but wrong for a
// bow, so each weapon declares a `ranged_style` (bow / crossbow / thrown / …)
// and a pack supplies the matching phrasing. The engine keeps a neutral,
// non-firearm floor so a pack that defines nothing still produces sensible
// text and a fight never breaks for want of flavor.
//
// Scope is deliberately ranged-only (ranged-combat §3 / action-economy.md §7.1);
// the resolver's keyed-lookup + {token} substitution + default-fallback shape
// is a natural seed for a broader message catalog if one is ever wanted, but
// that generalization is explicitly out of scope here.
package rangedflavor

import (
	"strings"
	"sync"
)

// Message keys — the ranged moments that carry style-specific flavor. Failure
// moments plus the two discrete-verb successes (load, fire); the sustained
// auto-attack round keeps its own hit/miss render, so there is no per-swing
// success key here (that would spam every round).
const (
	// KeyDry — a freely-firing weapon (a bow) has no matching ammunition.
	KeyDry = "dry"
	// KeyUnloaded — a reload-gated weapon (a crossbow) isn't chambered.
	KeyUnloaded = "unloaded"
	// KeyLoadEmpty — a `load` attempt finds no ammunition to chamber.
	KeyLoadEmpty = "load_empty"
	// KeyLoad — success: a reload-gated weapon is chambered and ready.
	KeyLoad = "load"
	// KeyFire — success: the cross-room `shoot` opener looses a shot.
	KeyFire = "fire"
)

// DefaultStyleID is the style a weapon with no `ranged_style` resolves to. A
// pack (core) may register a style under this id to set the shared baseline
// voice; absent that, the engine floor applies.
const DefaultStyleID = "default"

// Line is one moment's two-audience templates: Self is the second-person line
// shown to the actor; Room is the third-person line shown to onlookers. Either
// may be empty (an empty Self means "this style doesn't override this moment",
// so resolution falls through to the default style, then the engine floor).
type Line struct {
	Self string
	Room string
}

// Style is one named ranged voice (bow, crossbow, firearm, …) and its per-key
// lines. Missing keys fall through in Resolve; a style need only declare what
// differs from the baseline.
type Style struct {
	ID   string
	Msgs map[string]Line
}

// floor is the engine's neutral, non-firearm default for every key — the
// guaranteed floor when neither the weapon's style nor the default style
// supplies a line. Intentionally plain (no "*click*", no quiver): packs enrich
// it, this only guarantees sensible, style-agnostic output.
var floor = map[string]Line{
	KeyDry:       {Self: "You are out of {ammo}.", Room: "{actor} is out of {ammo}."},
	KeyUnloaded:  {Self: "{weapon} isn't loaded. (load it first)", Room: "{actor} fumbles with an unloaded weapon."},
	KeyLoadEmpty: {Self: "You have no {ammo} to load.", Room: "{actor} reaches for ammunition that isn't there."},
	KeyLoad:      {Self: "You load {weapon}. It's ready to fire.", Room: "{actor} loads {weapon}."},
	KeyFire:      {Self: "You loose a shot to the {dir} at {target}!", Room: "{actor} looses a shot to the {dir}."},
}

// Registry holds the loaded ranged-flavor styles, keyed by id.
type Registry struct {
	mu      sync.RWMutex
	byID    map[string]Style
	ordered []string
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{byID: make(map[string]Style)}
}

// Register inserts s under its id. A later Register for an existing id replaces
// it (last-writer-wins), matching the priority-override convention for
// baseline-plus-pack content; the loader still guards cross-pack collisions
// upstream where it wants a hard error.
func (r *Registry) Register(s Style) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byID[s.ID]; !exists {
		r.ordered = append(r.ordered, s.ID)
	}
	r.byID[s.ID] = s
}

// IDs returns the registered style ids in registration order (for logging).
func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.ordered))
	copy(out, r.ordered)
	return out
}

// Resolve returns the (self, room) lines for a ranged moment, substituting
// params. It tries, in order: the weapon's own style, the default style, then
// the engine floor — the first that supplies a non-empty Self for key wins,
// per audience-line, so a style may override just the Self and inherit the Room
// (or vice-versa). A nil receiver resolves entirely off the floor, so callers
// need not nil-guard.
func (r *Registry) Resolve(styleID, key string, params map[string]string) (self, room string) {
	var line Line
	if r != nil {
		r.mu.RLock()
		if styleID != "" {
			if s, ok := r.byID[styleID]; ok {
				line = merge(line, s.Msgs[key])
			}
		}
		if def, ok := r.byID[DefaultStyleID]; ok {
			line = merge(line, def.Msgs[key])
		}
		r.mu.RUnlock()
	}
	line = merge(line, floor[key])
	return substitute(line.Self, params), substitute(line.Room, params)
}

// merge fills any empty field of base from fallback (base wins). Used to layer
// weapon-style over default-style over floor per audience line.
func merge(base, fallback Line) Line {
	if base.Self == "" {
		base.Self = fallback.Self
	}
	if base.Room == "" {
		base.Room = fallback.Room
	}
	return base
}

// substitute expands {token} placeholders from params, left to right. Unknown
// tokens pass through unchanged so templates stay forward-compatible; a token
// the caller didn't supply is simply left as literal text (callers provide the
// tokens their key uses — see the message-key docs).
func substitute(tmpl string, params map[string]string) string {
	if tmpl == "" || len(params) == 0 || !strings.ContainsRune(tmpl, '{') {
		return tmpl
	}
	var b strings.Builder
	b.Grow(len(tmpl))
	for i := 0; i < len(tmpl); {
		if tmpl[i] != '{' {
			b.WriteByte(tmpl[i])
			i++
			continue
		}
		end := strings.IndexByte(tmpl[i:], '}')
		if end < 0 {
			b.WriteString(tmpl[i:]) // no closing brace — emit the rest verbatim
			break
		}
		name := tmpl[i+1 : i+end]
		if v, ok := params[name]; ok {
			b.WriteString(v)
		} else {
			b.WriteString(tmpl[i : i+end+1]) // unknown token: pass through with braces
		}
		i += end + 1
	}
	return b.String()
}
