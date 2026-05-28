package quest

import "strings"

// Marker queries (§8). Renderers ask whether an entity (by template id)
// is relevant to one of a player's active quests so it can be decorated
// (e.g. a `!` next to a quest giver). Markers read the live active state
// and the registry; they are pure (no per-call mutation).

// HasMarker reports whether templateID carries a quest marker for the
// player (§8.2). An entity is marked when, for some active non-secret
// quest, it is the quest's giver (always, even after acceptance), or a
// deliver objective's npc target in the player's CURRENT stage, or a
// collect objective's target in the current stage. Kill objectives never
// mark (§8.2), and secret quests contribute nothing (§8.3).
func (s *Service) HasMarker(playerID, templateID string) bool {
	if templateID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.states[playerID]
	if !ok {
		return false
	}
	for i := range st.Active {
		active := &st.Active[i]
		def, ok := s.registry.Lookup(active.QuestID)
		if !ok || def.Secret {
			continue
		}
		if def.Giver == templateID {
			return true
		}
		if active.StageIndex >= len(def.Stages) {
			continue
		}
		for _, o := range def.Stages[active.StageIndex].Objectives {
			switch {
			case strings.EqualFold(o.Type, "deliver") && o.NPC == templateID:
				return true
			case strings.EqualFold(o.Type, "collect") && o.Target == templateID:
				return true
			}
		}
	}
	return false
}

// MarkedTemplates returns the subset of templateIDs that carry a marker
// for the player (§8.1 bulk query), preserving input order. Each entity
// resolves to at most one marker (HasMarker short-circuits on the first
// matching active quest).
func (s *Service) MarkedTemplates(playerID string, templateIDs []string) []string {
	var out []string
	for _, id := range templateIDs {
		if s.HasMarker(playerID, id) {
			out = append(out, id)
		}
	}
	return out
}
