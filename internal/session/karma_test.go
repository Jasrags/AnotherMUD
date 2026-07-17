package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/karma"
	"github.com/Jasrags/AnotherMUD/internal/pack"
)

// newAdvancementLedger returns a ledger ONLY for a world that selected
// karma-ledger; every other world (the default level-track) resolves to nil,
// which is the signal the reward paths and score sheet key off (SR-M5).
func TestNewAdvancementLedger(t *testing.T) {
	sel := map[string]string{
		"shadowrun":     pack.AdvancementKarmaLedger,
		"starter-world": pack.AdvancementLevelTrack, // an explicit level-track never appears in the map in practice, but must resolve to nil
	}
	tests := []struct {
		name      string
		selection map[string]string
		worldID   string
		wantNil   bool
	}{
		{"karma-ledger world gets a ledger", sel, "shadowrun", false},
		{"explicit level-track world gets nil", sel, "starter-world", true},
		{"unlisted world gets nil", sel, "wot", true},
		{"nil selection gets nil", nil, "shadowrun", true},
		{"empty worldID gets nil", sel, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newAdvancementLedger(tt.selection, tt.worldID)
			if tt.wantNil && got != nil {
				t.Errorf("newAdvancementLedger(%q) = non-nil, want nil (level-track)", tt.worldID)
			}
			if !tt.wantNil && got == nil {
				t.Errorf("newAdvancementLedger(%q) = nil, want a ledger (karma-ledger)", tt.worldID)
			}
			// A fresh ledger starts empty.
			if got != nil && (got.Current() != 0 || got.Total() != 0) {
				t.Errorf("fresh ledger not empty: current=%d total=%d", got.Current(), got.Total())
			}
		})
	}
}

// resolveKarmaCosts returns a world's declared `improve` multipliers (default-
// filled), and the SR canon defaults for any world that declared none (SR-M5b).
func TestResolveKarmaCosts(t *testing.T) {
	sel := map[string]karma.Costs{
		"shadowrun": {SkillMult: 3, AttributeMult: 8},
		"partial":   {SkillMult: 4}, // attribute mult filled to canon at load; here already whole
	}
	tests := []struct {
		name    string
		worldID string
		want    karma.Costs
	}{
		{"declared world", "shadowrun", karma.Costs{SkillMult: 3, AttributeMult: 8}},
		{"partial world fills attr", "partial", karma.Costs{SkillMult: 4, AttributeMult: 5}},
		{"unlisted world -> canon", "wot", karma.DefaultCosts()},
		{"empty world -> canon", "", karma.DefaultCosts()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveKarmaCosts(sel, tt.worldID); got != tt.want {
				t.Errorf("resolveKarmaCosts(%q) = %+v, want %+v", tt.worldID, got, tt.want)
			}
		})
	}
	if got := resolveKarmaCosts(nil, "shadowrun"); got != karma.DefaultCosts() {
		t.Errorf("nil selection = %+v, want canon defaults", got)
	}
}
