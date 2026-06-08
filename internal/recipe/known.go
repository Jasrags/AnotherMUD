package recipe

import (
	"sort"
	"strings"
	"sync"
)

// KnownManager holds the per-character set of known crafting recipes
// (crafting-and-cooking §7, §9). It mirrors the lifecycle of
// progression.ProficiencyManager: the session-load path restores a
// character's saved id list into it, mutations happen at runtime (a
// discipline grants its baseline recipes, a quest/drop teaches more), the
// persist path snapshots it back to the save, and logout drops the
// in-memory entry.
//
// Recipe knowledge is breadth-only state — a flat set of ids. Quality and
// the craft itself live elsewhere. The manager holds a registry reference
// so Restore can drop ids whose recipe is no longer in content (§9: "a
// known-but-now-unknown recipe id is ignored, never an error") and so
// GrantBaseline can resolve a discipline's baseline recipes.
//
// Lock-order invariant: KnownManager.mu is never held while the Registry's
// lock is held, and vice versa. GrantBaseline relies on this — it calls
// reg.ByDiscipline (which acquires and fully releases the registry lock)
// before taking m.mu, so the two locks are never held simultaneously. Any
// future method that touches both MUST preserve this ordering.
type KnownManager struct {
	reg *Registry
	mu  sync.RWMutex
	// known maps entityID -> set of known recipe ids.
	known map[string]map[RecipeID]struct{}
}

// NewKnownManager returns an empty manager. reg may be nil (test wiring):
// with no registry, Restore accepts every saved id verbatim and
// GrantBaseline finds nothing.
func NewKnownManager(reg *Registry) *KnownManager {
	return &KnownManager{reg: reg, known: make(map[string]map[RecipeID]struct{})}
}

// Learn adds id to entityID's known set. Returns true when the id was
// newly added, false when already known (learning a known recipe is a
// no-op, §7). A blank id is ignored.
func (m *KnownManager) Learn(entityID string, id RecipeID) bool {
	if id == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	set := m.known[entityID]
	if set == nil {
		set = make(map[RecipeID]struct{})
		m.known[entityID] = set
	}
	if _, ok := set[id]; ok {
		return false
	}
	set[id] = struct{}{}
	return true
}

// Knows reports whether entityID knows id.
func (m *KnownManager) Knows(entityID string, id RecipeID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.known[entityID][id]
	return ok
}

// Recipes returns entityID's known recipe ids, sorted for determinism.
func (m *KnownManager) Recipes(entityID string) []RecipeID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	set := m.known[entityID]
	out := make([]RecipeID, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	// Sorting a local copy under the read lock is harmless and keeps the
	// unlock on a single defer (no early-return lock-leak hazard).
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Snapshot returns entityID's known ids as a sorted []string for the
// player save. An empty set returns nil so the save omits the key.
func (m *KnownManager) Snapshot(entityID string) []string {
	ids := m.Recipes(entityID)
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}

// Restore replaces entityID's known set with the saved ids, dropping any
// whose recipe is no longer registered (§9). A nil/empty list clears the
// entry. When the manager has no registry (test wiring), every saved id is
// accepted verbatim.
func (m *KnownManager) Restore(entityID string, saved []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(saved) == 0 {
		delete(m.known, entityID)
		return
	}
	set := make(map[RecipeID]struct{}, len(saved))
	for _, raw := range saved {
		id := RecipeID(strings.TrimSpace(raw))
		if id == "" {
			continue
		}
		if m.reg != nil && !m.reg.Has(id) {
			// §9: a known id whose recipe was removed from content is
			// ignored, never an error. Dropped here rather than carried.
			continue
		}
		set[id] = struct{}{}
	}
	if len(set) == 0 {
		delete(m.known, entityID)
		return
	}
	m.known[entityID] = set
}

// Drop removes entityID's in-memory known set (logout).
func (m *KnownManager) Drop(entityID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.known, entityID)
}

// GrantBaseline learns every baseline-acquisition recipe for discipline
// (§2: "Acquiring a discipline grants its baseline recipes immediately")
// into entityID's set. Returns the ids newly learned (already-known ones
// are skipped) so callers can report them. A nil registry or unknown
// discipline grants nothing.
//
// The grant is atomic: ByDiscipline resolves the candidate list (releasing
// the registry lock), then a single m.mu acquisition applies every insert.
// A concurrent Drop/Restore therefore cannot interleave between inserts and
// leave a half-granted set.
func (m *KnownManager) GrantBaseline(entityID, discipline string) []RecipeID {
	if m.reg == nil {
		return nil
	}
	candidates := m.reg.ByDiscipline(discipline) // registry lock released here

	var learned []RecipeID
	m.mu.Lock()
	set := m.known[entityID]
	if set == nil {
		set = make(map[RecipeID]struct{})
		m.known[entityID] = set
	}
	for _, r := range candidates {
		if r.Acquisition != AcqBaseline {
			continue
		}
		if _, ok := set[r.ID]; ok {
			continue
		}
		set[r.ID] = struct{}{}
		learned = append(learned, r.ID)
	}
	m.mu.Unlock()

	sort.Slice(learned, func(i, j int) bool { return learned[i] < learned[j] })
	return learned
}
