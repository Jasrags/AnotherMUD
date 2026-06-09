package gathering

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/decoration"
)

// fixedRoller returns a constant jitter from IntN — deterministic rolls.
type fixedRoller struct{ v int }

func (r fixedRoller) IntN(n int) int {
	if r.v >= n {
		return n - 1
	}
	return r.v
}

// coreLadder: common(0) < uncommon(1) < rare(2) < legendary(3).
func coreLadder() *decoration.RarityRegistry {
	r := decoration.NewRarityRegistry()
	r.Register(decoration.Tier{Key: "common", Order: 10})
	r.Register(decoration.Tier{Key: "uncommon", Order: 20})
	r.Register(decoration.Tier{Key: "rare", Order: 30})
	r.Register(decoration.Tier{Key: "legendary", Order: 50})
	return r
}

// midRoller picks the middle of the jitter band (jitter = 0), so the rolled
// position reflects the weighted score alone.
func midRoller(cfg Config) fixedRoller { return fixedRoller{v: cfg.RollBand} }

func TestRollQuality_SkillAndRichnessRaiseTier(t *testing.T) {
	cfg := DefaultConfig()
	s := NewService(coreLadder(), nil, midRoller(cfg), cfg, nil)

	// Low skill + low richness + no tool → bottom of the ladder.
	low := s.RollQuality(QualityInputs{Skill: 0, Richness: 0, SourceCeiling: -1})
	if low != "common" {
		t.Errorf("low inputs = %q, want common", low)
	}
	// Max skill + max richness + top tool → top of the ladder.
	high := s.RollQuality(QualityInputs{Skill: 100, Richness: 100, ToolTierKey: "legendary", SourceCeiling: -1})
	if high != "legendary" {
		t.Errorf("max inputs = %q, want legendary", high)
	}
}

func TestRollQuality_SourceCeilingCaps(t *testing.T) {
	cfg := DefaultConfig()
	s := NewService(coreLadder(), nil, midRoller(cfg), cfg, nil)

	// Max everything, but the source ceiling caps at uncommon (position 1).
	got := s.RollQuality(QualityInputs{Skill: 100, Richness: 100, ToolTierKey: "legendary", SourceCeiling: 1})
	if got != "uncommon" {
		t.Errorf("ceiling-capped roll = %q, want uncommon (source ceiling 1)", got)
	}
}

func TestRollQuality_NoLadderYieldsNoTier(t *testing.T) {
	cfg := DefaultConfig()
	s := NewService(decoration.NewRarityRegistry(), nil, midRoller(cfg), cfg, nil)
	if got := s.RollQuality(QualityInputs{Skill: 50, SourceCeiling: -1}); got != "" {
		t.Errorf("empty ladder roll = %q, want \"\"", got)
	}
}

func TestRollQuality_NegativeCeilingMeansLadderTop(t *testing.T) {
	cfg := DefaultConfig()
	s := NewService(coreLadder(), nil, midRoller(cfg), cfg, nil)
	// SourceCeiling -1 = no cap; max inputs reach the ladder top.
	if got := s.RollQuality(QualityInputs{Skill: 100, Richness: 100, ToolTierKey: "legendary", SourceCeiling: -1}); got != "legendary" {
		t.Errorf("uncapped roll = %q, want legendary", got)
	}
}
