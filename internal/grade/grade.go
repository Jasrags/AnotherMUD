// Package grade implements the item quality-grade ladder of
// docs/specs/masterwork.md: an ordered, content-registered vocabulary of
// quality grades (the WoT pack: masterwork < masterpiece < power-wrought)
// that confer small, grade-scaled mechanical bonuses through the combat /
// skill seams already in play.
//
// The grade is MECHANICAL and stays independent of the cosmetic
// item-decorations marker (masterwork §5): an item's bonus is read from its
// grade, never from a rarity/essence key. A grade's bonus by item kind
// (masterwork §3) rides existing seams — a weapon's to-hit adjustment, the
// damage_bonus channel (power-wrought), the armor check penalty, a tool's
// skill check — it never invents a new resolution path.
//
// The package is a leaf: it does not import the engine's entity, combat,
// session, or command layers, so item/equipment/combat code can depend on
// it without a cycle. Mirrors the decoration.RarityRegistry idiom.
package grade

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// ErrInvalidKey is the sentinel wrapped by ValidateKey for an unusable grade
// key. The pack loader surfaces it with pack + path attribution so a
// malformed grade fails the boot loudly.
var ErrInvalidKey = errors.New("grade: invalid grade key")

// ValidateKey reports why a grade key is unusable, or nil if valid. A key
// must be non-empty after trimming and free of internal whitespace (it is a
// content identifier and may be surfaced in displays).
func ValidateKey(raw string) error {
	k := strings.TrimSpace(raw)
	if k == "" {
		return fmt.Errorf("%w: empty", ErrInvalidKey)
	}
	if strings.ContainsAny(k, " \t\n") {
		return fmt.Errorf("%w: %q contains whitespace", ErrInvalidKey, raw)
	}
	return nil
}

// Grade is one quality-grade definition (masterwork §2). Grades form an
// ordered ladder via Order (low → high) so "finer than" comparisons and
// bonus scaling are well-defined. Every bonus magnitude is content policy
// (masterwork §8); the engine reads them but never hardcodes them.
//
// Each bonus field targets the seam its item kind already uses (masterwork
// §3). A field left zero contributes nothing on that axis, so a grade need
// only set the axes relevant to the kinds it grades.
type Grade struct {
	// Key is the short identifier (e.g. "masterwork"). Compared
	// case-insensitively; unique within the registry.
	Key string
	// Order is the ladder rank, low → high — establishes "is this finer
	// than that" and the sort order of All.
	Order int
	// WeaponToHit is the to-hit bonus a graded WEAPON adds while wielded,
	// contributing to the same per-attacker hit-modifier seam weapon
	// proficiency feeds (masterwork §3, weapon-identity §3).
	WeaponToHit int
	// WeaponDamage is the damage_bonus a POWER-WROUGHT weapon adds while
	// wielded (masterwork §3, combat §4.5) — added post-crit-multiply.
	// Ordinary grades leave this zero (only power-wrought buffs damage).
	WeaponDamage int
	// WeaponAP is the armor penetration a graded AMMUNITION round confers to
	// the shot it feeds (SR5 APDS, combat §4.5): it adds to the attacker's AP
	// for that swing, bypassing armor soak. Zero for ordinary grades. The
	// round-fed analogue of a weapon's intrinsic `ap`.
	WeaponAP int
	// ArmorCheckImprove is the amount a graded ARMOR/SHIELD reduces the
	// magnitude of its armor check penalty while worn (masterwork §3,
	// armor-depth §6), floored at zero. Does not change armor bonus / max-Dex.
	ArmorCheckImprove int
	// ToolSkill is the bonus a graded TOOL adds to the skill check it assists
	// (masterwork §3, skills.md). Multiple graded tools toward one check do
	// not stack — best applies (the consumer enforces that).
	ToolSkill int
	// Unbreakable records the power-wrought "never breaks / no maintenance"
	// flag (masterwork §4). The engine has no durability system, so this is
	// an inert forward hook today.
	Unbreakable bool
}

// Registry holds the ordered set of grade definitions. Safe for concurrent
// reads (Get, All, IsHigher); Register is the only writer and runs at boot /
// pack load. Mirrors decoration.RarityRegistry.
type Registry struct {
	mu     sync.RWMutex
	grades map[string]Grade // keyed by normalized Key
}

// NewRegistry returns an empty registry. An empty registry resolves no
// grades — every item is ordinary (masterwork §2).
func NewRegistry() *Registry {
	return &Registry{grades: make(map[string]Grade)}
}

// normalizeKey lowercases + trims a grade key so "Masterwork", "MASTERWORK",
// and " masterwork " denote the same grade (case-insensitive keys, §2).
func normalizeKey(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// Register installs g under its normalized key, replacing any prior
// definition (idempotent, later-wins — the pack convention). A grade with an
// invalid key is ignored (returns false); success returns true. The stored
// Grade carries the normalized key so lookups and the stored value agree.
func (r *Registry) Register(g Grade) bool {
	if ValidateKey(g.Key) != nil {
		return false
	}
	g.Key = normalizeKey(g.Key)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.grades[g.Key] = g
	return true
}

// Get resolves a grade by key (case-insensitive). Returns (zero, false) on
// an unknown key — callers treat an unknown grade as ungraded, never an
// error (an item with a stale grade loads cleanly and confers no bonus).
func (r *Registry) Get(key string) (Grade, bool) {
	k := normalizeKey(key)
	if k == "" {
		return Grade{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.grades[k]
	return g, ok
}

// Has reports whether a grade is registered under key (case-insensitive) —
// the load-boundary check for validating an item template's grade.
func (r *Registry) Has(key string) bool {
	_, ok := r.Get(key)
	return ok
}

// IsHigher reports whether grade a is finer than grade b by ladder Order.
// Unknown keys rank below every registered grade (an ungraded item is the
// floor).
func (r *Registry) IsHigher(a, b string) bool {
	ga, aok := r.Get(a)
	gb, bok := r.Get(b)
	switch {
	case !aok:
		return false
	case !bok:
		return true
	default:
		return ga.Order > gb.Order
	}
}

// All returns every grade sorted by Order ascending (the ladder, low →
// high). Ties on Order break by key for a stable order. Fresh slice.
func (r *Registry) All() []Grade {
	r.mu.RLock()
	out := make([]Grade, 0, len(r.grades))
	for _, g := range r.grades {
		out = append(out, g)
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// Len reports the number of registered grades.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.grades)
}
