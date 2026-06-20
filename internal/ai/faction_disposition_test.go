package ai

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/mob"
)

// faction.md §6: a disposition rule's faction clause matches against the
// player's effective standing (PlayerView.Standings), the same way the
// alignment clause matches PlayerView.Alignment. These tests lock the seed-mob
// shape — the Baerlon Whitecloak turns on a character whose Children-of-the-
// Light standing has fallen below Neutral.
func TestDecideReaction_FactionStandingClause(t *testing.T) {
	const watchID = "wot:children-of-the-light"
	whitecloak := &mob.Template{
		ID: "wot:child-of-the-light",
		DispositionRules: &mob.Definition{
			Default: mob.ReactionNeutral,
			Rules: []mob.Rule{
				{Faction: watchID, MaxStanding: -1, HasMaxStanding: true, Reaction: mob.ReactionHostile},
			},
		},
	}

	cases := []struct {
		name      string
		standings map[string]int
		want      mob.Reaction
	}{
		{"hostile-standing enemy", map[string]int{watchID: -400}, mob.ReactionHostile},
		{"just below neutral", map[string]int{watchID: -1}, mob.ReactionHostile},
		{"at neutral (untouched effective)", map[string]int{watchID: 0}, mob.ReactionNeutral},
		{"friendly standing", map[string]int{watchID: 500}, mob.ReactionNeutral},
		{"no standing data at all", nil, mob.ReactionNeutral},
		{"only a different faction known", map[string]int{"wot:darkfriends": -900}, mob.ReactionNeutral},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := decideReaction(whitecloak, PlayerView{ID: "p", Standings: c.standings})
			if !ok || got != c.want {
				t.Errorf("decideReaction = %q,%v; want %q,true", got, ok, c.want)
			}
		})
	}
}

// A min_standing clause (the Darkfriend warming to a friend of the Dark).
func TestDecideReaction_FactionMinStandingClause(t *testing.T) {
	const dfID = "wot:darkfriends"
	merchant := &mob.Template{
		ID: "wot:smooth-merchant",
		DispositionRules: &mob.Definition{
			Default: mob.ReactionNeutral,
			Rules: []mob.Rule{
				{Faction: dfID, MinStanding: 300, HasMinStanding: true, Reaction: mob.ReactionFriendly},
			},
		},
	}
	if got, _ := decideReaction(merchant, PlayerView{ID: "p", Standings: map[string]int{dfID: 300}}); got != mob.ReactionFriendly {
		t.Errorf("at the threshold = %q, want friendly", got)
	}
	if got, _ := decideReaction(merchant, PlayerView{ID: "p", Standings: map[string]int{dfID: 299}}); got != mob.ReactionNeutral {
		t.Errorf("just below threshold = %q, want neutral default", got)
	}
}
