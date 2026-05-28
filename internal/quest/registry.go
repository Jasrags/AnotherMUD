package quest

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Registration errors. Callers check via errors.Is.
var (
	ErrNilDefinition = errors.New("quest: nil definition")
	ErrMissingID     = errors.New("quest: definition missing id")
	ErrNoStages      = errors.New("quest: definition has no stages")
	ErrNoObjectives  = errors.New("quest: stage has no objectives")
)

// Registry is the id-keyed store of quest definitions (§2.1). A later
// registration of the same id replaces the earlier one. Safe for
// concurrent reads; registration is expected at boot.
type Registry struct {
	mu   sync.RWMutex
	byID map[string]*Definition
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{byID: make(map[string]*Definition)}
}

// Register validates d, normalizes its objectives, and stores it by id
// (replacing any earlier definition with the same id). The registry
// takes ownership of d and mutates it in place (filling absent objective
// ids and defaulting counts), so callers must not reuse the pointer.
//
// Validation (§2.3): non-empty id, at least one stage, and at least one
// objective per stage. The reward block may be empty.
func (r *Registry) Register(d *Definition) error {
	if d == nil {
		return ErrNilDefinition
	}
	if strings.TrimSpace(d.ID) == "" {
		return ErrMissingID
	}
	if len(d.Stages) == 0 {
		return fmt.Errorf("%w: quest %q", ErrNoStages, d.ID)
	}
	for i := range d.Stages {
		st := &d.Stages[i]
		if len(st.Objectives) == 0 {
			return fmt.Errorf("%w: quest %q stage %d", ErrNoObjectives, d.ID, i)
		}
		for j := range st.Objectives {
			obj := &st.Objectives[j]
			if obj.Count <= 0 {
				obj.Count = 1
			}
			if strings.TrimSpace(obj.ID) == "" {
				obj.ID = genObjectiveID(st, i, obj.Type, j)
			}
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[d.ID] = d
	return nil
}

// Lookup returns the definition for id and whether it exists.
func (r *Registry) Lookup(id string) (*Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.byID[id]
	return d, ok
}

// All returns every registered definition (fresh slice, unordered).
func (r *Registry) All() []*Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Definition, 0, len(r.byID))
	for _, d := range r.byID {
		out = append(out, d)
	}
	return out
}

// Len returns the number of registered definitions.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byID)
}

// genObjectiveID builds a stable id from the stage key, objective type,
// and position (§2.2). Using the stage id (or its index when absent) plus
// the objective's position keeps ids stable across reloads of unchanged
// content, so per-objective progress survives cosmetic content edits.
func genObjectiveID(st *Stage, stageIdx int, objType string, objIdx int) string {
	stageKey := strings.TrimSpace(st.ID)
	if stageKey == "" {
		stageKey = fmt.Sprintf("stage%d", stageIdx)
	}
	typ := strings.TrimSpace(objType)
	if typ == "" {
		typ = "obj"
	}
	return fmt.Sprintf("%s-%s-%d", stageKey, typ, objIdx)
}
