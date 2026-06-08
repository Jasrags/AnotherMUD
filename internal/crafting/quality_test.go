package crafting

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/decoration"
)

// fixedRoller returns a constant from IntN — deterministic rolls in tests.
type fixedRoller struct{ v int }

func (r fixedRoller) IntN(n int) int {
	if r.v >= n {
		return n - 1
	}
	return r.v
}

// coreLadder builds the standard 4-tier ladder: common(0) < uncommon(1) <
// rare(2) < legendary(3) by ascending Order.
func coreLadder() *decoration.RarityRegistry {
	r := decoration.NewRarityRegistry()
	r.Register(decoration.Tier{Key: "common", Order: 10})
	r.Register(decoration.Tier{Key: "uncommon", Order: 20})
	r.Register(decoration.Tier{Key: "rare", Order: 30})
	r.Register(decoration.Tier{Key: "legendary", Order: 50})
	return r
}

// svc builds a Service with only the quality-roll deps wired.
func svc(rarity *decoration.RarityRegistry, roller Roller, cfg Config) *Service {
	return NewService(nil, nil, nil, nil, nil, rarity, roller, cfg)
}

func noBand() Config {
	c := DefaultConfig()
	c.RollBand = 0 // deterministic: roll == score
	return c
}

func TestRollQuality_NoLadderNoStamp(t *testing.T) {
	s := svc(decoration.NewRarityRegistry(), fixedRoller{}, noBand())
	if got := s.rollQuality(QualityInputs{Skill: 100}); got != "" {
		t.Errorf("rollQuality with empty ladder = %q, want \"\"", got)
	}
}

func TestRollQuality_SkillRaisesTier(t *testing.T) {
	// Tier 2 station so the station ceiling doesn't clamp; legendary
	// ingredients so the soft ceiling doesn't clamp either. Higher skill
	// must land at a strictly higher ladder position.
	rarity := coreLadder()
	s := svc(rarity, fixedRoller{}, noBand())

	low := s.rollQuality(QualityInputs{Skill: 0, StationTier: 2, IngredientTierKeys: []string{"legendary"}})
	high := s.rollQuality(QualityInputs{Skill: 100, StationTier: 2, IngredientTierKeys: []string{"legendary"}})

	if ladderPosition(rarity, high) <= ladderPosition(rarity, low) {
		t.Errorf("skill should raise tier: low=%q high=%q", low, high)
	}
	if high != "legendary" {
		t.Errorf("skill 100 + legendary ingredients at Tier 2 → %q, want legendary", high)
	}
}

func TestRollQuality_StationCeilingClamps(t *testing.T) {
	// Max skill + ingredients, but Tier 0 caps at ladder position 1
	// (uncommon) regardless.
	s := svc(coreLadder(), fixedRoller{}, noBand())
	got := s.rollQuality(QualityInputs{
		Skill: 100, StationTier: 0, IngredientTierKeys: []string{"legendary"},
	})
	if got != "uncommon" {
		t.Errorf("Tier 0 ceiling: skill 100 → %q, want uncommon (capped)", got)
	}
}

func TestRollQuality_IngredientSoftCeilingClamps(t *testing.T) {
	// High skill at a Tier 2 station, but common ingredients cap output
	// at ingPos(0)+margin(1) = uncommon.
	s := svc(coreLadder(), fixedRoller{}, noBand())
	got := s.rollQuality(QualityInputs{
		Skill: 100, StationTier: 2, IngredientTierKeys: []string{"common"},
	})
	if got != "uncommon" {
		t.Errorf("soft ceiling: common ingredients → %q, want uncommon", got)
	}
}

func TestRollQuality_BandProducesVariation(t *testing.T) {
	// With a band and rollers at the extremes, the same inputs span tiers.
	cfg := DefaultConfig() // band 15
	rarity := coreLadder()
	// mid skill at Tier 2 with legendary ingredients: score≈ around the
	// rare boundary; low jitter vs high jitter should differ.
	in := QualityInputs{Skill: 60, StationTier: 2, IngredientTierKeys: []string{"legendary"}}
	lowRoll := svc(rarity, fixedRoller{v: 0}, cfg).rollQuality(in)   // jitter = -band
	highRoll := svc(rarity, fixedRoller{v: 99}, cfg).rollQuality(in) // jitter = +band
	if lowRoll == highRoll {
		t.Errorf("band produced no variation: both %q", lowRoll)
	}
}

func TestLadderPosition(t *testing.T) {
	r := coreLadder()
	if got := ladderPosition(r, "rare"); got != 2 {
		t.Errorf("ladderPosition(rare) = %d, want 2", got)
	}
	if got := ladderPosition(r, "mythic"); got != -1 {
		t.Errorf("ladderPosition(mythic) = %d, want -1", got)
	}
}
