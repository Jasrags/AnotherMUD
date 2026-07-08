package feat

import "slices"

import "strings"

// CharacterView is the read-only character snapshot the prereq evaluator reads
// (EPIC S4 Phase 1 — docs/proposals/wot-feats.md §2.1). It is declared here, in
// the leaf feat package, and implemented by the session/progression layer in a
// later phase — so the evaluator depends on nothing and stays unit-testable in
// isolation. Lookups are by the lowercased ids the registry normalizes to.
type CharacterView interface {
	// AbilityScore returns the character's current score for a stat name
	// (e.g. "str"), or 0 if unknown — an ability_score prereq then fails.
	AbilityScore(stat string) int
	// SkillProficiency returns the character's proficiency in a skill ability
	// id, or 0 if untrained — a skill prereq then fails.
	SkillProficiency(abilityID string) int
	// HasFeat reports whether the character already holds a feat id (the feat
	// prereq — a "doorway" feat such as Latent Dreamer or Power Attack).
	HasFeat(featID string) bool
	// CharacterLevel returns the total character level (across class-tracks for
	// a multiclass character) — the level prereq compares against it.
	CharacterLevel() int
	// ClassIDs returns the class ids the character holds — the AllowedClasses
	// gate is satisfied if any of them is allowed.
	ClassIDs() []string
}

// Eligibility is the result of an Eligible check (Phase 1). It is structured,
// not pre-formatted, so the presentation layer (the `feats` verb, Phase 4)
// owns the wording of "why you can't take this". OK is true iff no prereq is
// unmet AND the class gate passes.
//
// Phase 1 covers ONLY prerequisites + the class gate. The "already taken /
// multi-take allowance" and "a feat credit is available" checks belong to
// Phase 4 (they need the known-feats save from Phase 2 + the slot pool).
type Eligibility struct {
	OK bool
	// UnmetPrereqs lists the specific prerequisites the character does not
	// satisfy (in declaration order); empty when all are met.
	UnmetPrereqs []Prerequisite
	// ClassExcluded is true when the feat declares AllowedClasses and the
	// character holds none of them.
	ClassExcluded bool
}

// Eligible reports whether view's character may take feat f, evaluating each
// prerequisite (§2.1) and the optional class gate. f must be a registry-owned
// (normalized) feat and view must be non-nil. A nil feat is not eligible.
func Eligible(f *Feat, view CharacterView) Eligibility {
	if f == nil || view == nil {
		return Eligibility{OK: false}
	}
	var unmet []Prerequisite
	for _, p := range f.Prerequisites {
		if !prereqMet(p, view) {
			unmet = append(unmet, p)
		}
	}
	excluded := !classAllowed(f.AllowedClasses, view.ClassIDs())
	return Eligibility{
		OK:            len(unmet) == 0 && !excluded,
		UnmetPrereqs:  unmet,
		ClassExcluded: excluded,
	}
}

// prereqMet evaluates one prerequisite against the character view. An unknown
// kind (which decode rejects, so it should not occur) is treated as unmet —
// fail closed.
func prereqMet(p Prerequisite, view CharacterView) bool {
	switch p.Kind {
	case PrereqAbilityScore:
		return view.AbilityScore(p.Target) >= p.Min
	case PrereqSkill:
		return view.SkillProficiency(p.Target) >= p.Min
	case PrereqFeat:
		return view.HasFeat(p.Target)
	case PrereqLevel:
		return view.CharacterLevel() >= p.Min
	default:
		return false
	}
}

// classAllowed reports whether the character (holding heldClasses) satisfies an
// allowed-classes gate. An empty allowed list means unrestricted. Comparison is
// case-insensitive; allowed entries are already lowercased on Register, held
// ids are lowercased here defensively.
func classAllowed(allowed, heldClasses []string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, held := range heldClasses {
		h := strings.ToLower(strings.TrimSpace(held))
		if slices.Contains(allowed, h) {
			return true
		}
	}
	return false
}
