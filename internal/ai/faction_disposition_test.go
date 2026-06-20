package ai

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/mob"
)

// Faction reactions ride the existing has_tag matcher: the faction manager
// mirrors a character's standing as a rank tag (faction.md §3.3:
// faction:<id>:<rank>), folded into PlayerView.Tags, and a disposition rule
// matches it like any other tag. These tests lock the seed-mob shape — a
// Whitecloak that turns on a known enemy of the Children of the Light.
func TestDecideReaction_FactionRankTagDrivesHostility(t *testing.T) {
	// The Baerlon child-of-the-light disposition: hostile to a character at
	// Hostile or Unfriendly standing with the order, neutral otherwise.
	whitecloak := &mob.Template{
		ID: "wot:child-of-the-light",
		DispositionRules: &mob.Definition{
			Default: mob.ReactionNeutral,
			Rules: []mob.Rule{
				{HasTag: "faction:wot:children-of-the-light:Hostile", Reaction: mob.ReactionHostile},
				{HasTag: "faction:wot:children-of-the-light:Unfriendly", Reaction: mob.ReactionHostile},
			},
		},
	}

	cases := []struct {
		name string
		tags []string
		want mob.Reaction
	}{
		{"hostile-standing enemy", []string{"faction:wot:children-of-the-light:Hostile"}, mob.ReactionHostile},
		{"unfriendly-standing enemy", []string{"faction:wot:children-of-the-light:Unfriendly"}, mob.ReactionHostile},
		{"neutral-standing character", []string{"faction:wot:children-of-the-light:Neutral"}, mob.ReactionNeutral},
		{"untouched character (no faction tag)", nil, mob.ReactionNeutral},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := decideReaction(whitecloak, PlayerView{ID: "p", Tags: c.tags})
			if !ok || got != c.want {
				t.Errorf("decideReaction = %q,%v; want %q,true", got, ok, c.want)
			}
		})
	}
}
