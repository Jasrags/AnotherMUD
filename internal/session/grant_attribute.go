package session

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/recipe"
)

// Generalized admin grant/revoke (admin-verbs — `grant <kind> <value> to
// <player>`). GrantAttribute / RevokeAttribute dispatch a CANONICAL kind
// ("role"/"feat"/"ability"/"recipe"/"language") to the actor's backing store,
// returning whether the set actually changed (idempotency) and a user-facing
// error when the value doesn't name a real thing. The command layer owns the
// kind vocabulary + aliases (e.g. skill→ability) AND normalizes `value` to its
// canonical (lower-cased) form — the single source of truth — so these methods
// only trim it. connActor satisfies command.GrantTarget. Online-only in v1 (the
// target is a live connActor); offline grants are a later slice.

// grantDefaultAbilityProf is the proficiency a freshly admin-granted ability is
// learned at — novice (1), matching the class-path grant convention; the admin
// raises it afterward via the training path if wanted.
const grantDefaultAbilityProf = 1

// GrantAttribute adds value to the actor's `kind` set (see the file doc).
func (a *connActor) GrantAttribute(kind, value string) (bool, error) {
	switch kind {
	case "role":
		return a.Grant(strings.TrimSpace(value)), nil
	case "feat":
		return a.grantFeat(value)
	case "ability":
		return a.grantAbility(value)
	case "recipe":
		return a.grantRecipe(value)
	case "language":
		return a.grantLanguage(value)
	}
	return false, fmt.Errorf("unknown grant kind %q", kind)
}

// RevokeAttribute removes value from the actor's `kind` set.
func (a *connActor) RevokeAttribute(kind, value string) (bool, error) {
	switch kind {
	case "role":
		return a.Revoke(strings.TrimSpace(value)), nil
	case "feat":
		return a.revokeFeat(value)
	case "ability":
		return a.revokeAbility(value)
	case "recipe":
		return a.revokeRecipe(value)
	case "language":
		return a.revokeLanguage(value)
	}
	return false, fmt.Errorf("unknown grant kind %q", kind)
}

// --- feat ---

func (a *connActor) grantFeat(id string) (bool, error) {
	id = strings.TrimSpace(id)
	a.mu.Lock()
	reg := a.feats
	a.mu.Unlock()
	if reg == nil {
		return false, errors.New("feats aren't enabled on this world")
	}
	if _, ok := reg.Get(id); !ok {
		return false, fmt.Errorf("no feat named %q", id)
	}
	if a.HasFeat(id) {
		return false, nil // idempotent
	}
	// No param on an admin grant; GrantFeat re-validates, records, and applies
	// the feat's stat/ability grants.
	a.GrantFeat(id, "")
	return true, nil
}

func (a *connActor) revokeFeat(id string) (bool, error) {
	id = strings.TrimSpace(id)
	a.mu.Lock()
	changed := false
	if a.save != nil {
		kept := a.save.KnownFeats[:0]
		for _, kf := range a.save.KnownFeats {
			if strings.EqualFold(kf.FeatID, id) {
				changed = true
				continue
			}
			kept = append(kept, kf)
		}
		a.save.KnownFeats = kept
		if changed {
			a.markDirtyLocked()
		}
	}
	a.mu.Unlock()
	if changed {
		// Recompute the feat-derived stat/ability bonuses from the remaining set
		// (applyFeatGrants reads the full KnownFeats each call, so removal is a
		// clean recompute).
		a.applyFeatGrants()
	}
	return changed, nil
}

// --- ability / skill ---

func (a *connActor) grantAbility(id string) (bool, error) {
	id = strings.TrimSpace(id)
	a.mu.Lock()
	reg, prof, pid := a.abilities, a.prof, a.playerID
	a.mu.Unlock()
	// Both the proficiency store (to learn) and the ability registry (to
	// validate) are required — a nil either way means abilities aren't wired,
	// matching how feat/recipe hard-fail rather than learning an unvalidated id.
	if prof == nil || reg == nil {
		return false, errors.New("abilities aren't enabled on this world")
	}
	if _, ok := reg.Get(id); !ok {
		return false, fmt.Errorf("no ability named %q", id)
	}
	if prof.Has(pid, id) {
		return false, nil
	}
	prof.Learn(pid, id, grantDefaultAbilityProf)
	a.markDirty()
	return true, nil
}

func (a *connActor) revokeAbility(id string) (bool, error) {
	id = strings.TrimSpace(id)
	a.mu.Lock()
	prof, pid := a.prof, a.playerID
	a.mu.Unlock()
	if prof == nil || !prof.Has(pid, id) {
		return false, nil
	}
	prof.Forget(pid, id)
	a.markDirty()
	return true, nil
}

// --- recipe ---

func (a *connActor) grantRecipe(id string) (bool, error) {
	id = strings.TrimSpace(id)
	a.mu.Lock()
	km, pid := a.known, a.playerID
	a.mu.Unlock()
	if km == nil {
		return false, errors.New("recipes aren't enabled on this world")
	}
	if !km.Defined(recipe.RecipeID(id)) {
		return false, fmt.Errorf("no recipe named %q", id)
	}
	if !km.Learn(pid, recipe.RecipeID(id)) {
		return false, nil // already known
	}
	a.markDirty()
	return true, nil
}

func (a *connActor) revokeRecipe(id string) (bool, error) {
	id = strings.TrimSpace(id)
	a.mu.Lock()
	km, pid := a.known, a.playerID
	a.mu.Unlock()
	if km == nil || !km.Forget(pid, recipe.RecipeID(id)) {
		return false, nil
	}
	a.markDirty()
	return true, nil
}

// --- language ---

func (a *connActor) grantLanguage(id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, errors.New(`no language named ""`)
	}
	// Two-phase lock (read registry, release, re-acquire for the save mutation)
	// is safe: a.languages is set once at construction and never swapped, so the
	// gap can't see a torn/changed registry.
	a.mu.Lock()
	reg := a.languages
	a.mu.Unlock()
	if reg != nil {
		if _, ok := reg.Get(id); !ok {
			return false, fmt.Errorf("no language named %q", id)
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return false, errors.New("no character loaded")
	}
	if slices.Contains(a.save.KnownLanguages, id) {
		return false, nil
	}
	a.save.KnownLanguages = append(a.save.KnownLanguages, id)
	a.markDirtyLocked()
	return true, nil
}

func (a *connActor) revokeLanguage(id string) (bool, error) {
	id = strings.TrimSpace(id)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return false, nil
	}
	idx := slices.Index(a.save.KnownLanguages, id)
	if idx < 0 {
		return false, nil
	}
	a.save.KnownLanguages = slices.Delete(a.save.KnownLanguages, idx, idx+1)
	a.markDirtyLocked()
	return true, nil
}

// markDirty flips the save-dirty bit under the actor lock (the lock-free wrapper
// for callers not already holding a.mu). Mirrors markDirtyLocked.
func (a *connActor) markDirty() {
	a.mu.Lock()
	a.markDirtyLocked()
	a.mu.Unlock()
}
