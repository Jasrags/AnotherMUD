package ai

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/mob"
)

// reputation.md §6/§7: a disposition rule's renown clause matches on the
// player's EFFECTIVE renown magnitude (PlayerView.Renown) and the Infamy flag
// (PlayerView.Infamous), the way the faction clause matches Standings. A view
// without renown data (HasRenown=false) never matches a renown clause.
func TestDecideReaction_RenownMinClause(t *testing.T) {
	// A herald who bows to the widely-known (fame OR infamy of magnitude 400+).
	herald := &mob.Template{
		ID: "town:herald",
		DispositionRules: &mob.Definition{
			Default: mob.ReactionNeutral,
			Rules: []mob.Rule{
				{MinRenown: 400, HasMinRenown: true, Reaction: mob.ReactionFriendly},
			},
		},
	}
	cases := []struct {
		name      string
		renown    int
		hasRenown bool
		want      mob.Reaction
	}{
		{"famous enough", 500, true, mob.ReactionFriendly},
		{"infamous of equal magnitude", -500, true, mob.ReactionFriendly}, // PD-5 magnitude
		{"just under the floor", 399, true, mob.ReactionNeutral},
		{"at the floor", 400, true, mob.ReactionFriendly},
		{"unknown", 0, true, mob.ReactionNeutral},
		{"no renown data (minimal view)", 999, false, mob.ReactionNeutral},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := decideReaction(herald, PlayerView{ID: "p", Renown: c.renown, HasRenown: c.hasRenown})
			if !ok || got != c.want {
				t.Errorf("decideReaction = %q,%v; want %q,true", got, ok, c.want)
			}
		})
	}
}

// The Infamy flag clause (a commoner who is wary of the infamous, regardless of how
// famous — the reaction's KIND, not its magnitude, per PD-5).
func TestDecideReaction_InfamyClause(t *testing.T) {
	commoner := &mob.Template{
		ID: "town:commoner",
		DispositionRules: &mob.Definition{
			Default: mob.ReactionNeutral,
			Rules: []mob.Rule{
				{RequireInfamous: true, HasInfamous: true, Reaction: mob.ReactionWary},
			},
		},
	}
	// Infamous → wary.
	if got, _ := decideReaction(commoner, PlayerView{ID: "p", Infamous: true, HasRenown: true}); got != mob.ReactionWary {
		t.Errorf("infamous = %q, want wary", got)
	}
	// Not infamous (even if famous) → default.
	if got, _ := decideReaction(commoner, PlayerView{ID: "p", Renown: 900, Infamous: false, HasRenown: true}); got != mob.ReactionNeutral {
		t.Errorf("famous-not-infamous = %q, want neutral", got)
	}
	// No renown data → never matches.
	if got, _ := decideReaction(commoner, PlayerView{ID: "p"}); got != mob.ReactionNeutral {
		t.Errorf("no renown data = %q, want neutral", got)
	}
}
