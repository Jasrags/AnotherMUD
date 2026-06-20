package ai

import (
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

const (
	// tagShadowspawn marks a mob as Shadowspawn — a Trolloc, a Myrddraal, or a
	// raven under the Dark One's control. Authored as a mob `tags:` entry.
	tagShadowspawn = "shadowspawn"
	// tagStedding marks a room as lying within a stedding's bound (other-worlds.md
	// §Stedding). Authored as a room `tags:` entry; shared with the channeling
	// suppression gate in the command layer.
	tagStedding = "stedding"
)

// mobRefusesEntry reports whether mob m will not enter dst of its own accord.
// Shadowspawn instinctively refuse to cross a stedding's bound (other-worlds.md
// §Stedding) — they recoil at the threshold rather than step inside, whether
// driven by an idle wander or a pursuit. So a stedding is sanctuary: a quarry
// who flees across the bound loses its Shadowspawn hunter at the edge. A nil mob
// or destination never refuses (defensive).
func mobRefusesEntry(m *entities.MobInstance, dst *world.Room) bool {
	if m == nil || dst == nil {
		return false
	}
	return m.HasTag(tagShadowspawn) && dst.HasTag(tagStedding)
}
