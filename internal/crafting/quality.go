// Package crafting implements the craft resolution + quality roll
// (crafting-and-cooking §5) and the atomic craft execution (§3). It is the
// service layer: it owns the roller, the rarity ladder, the recipe + known
// registries, and the proficiency manager, and exposes Craft() to the
// command layer. The command handler supplies a Crafter adapter (the
// player) and the inventory mutation happens through that interface, so the
// package depends on entities but not on the session/command layers.
package crafting

import (
	"math"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/decoration"
)

// Roller is the RNG seam (math/rand/v2.Rand-style IntN). Mirrors the
// combat/progression Roller. Implementations need not be concurrent-safe;
// the Service guards roller use with its own mutex since crafts arrive from
// per-session goroutines (unlike combat's single tick goroutine).
type Roller interface {
	// IntN returns a pseudo-random int in [0, n); panics on n <= 0.
	IntN(n int) int
}

// Config is the §10 quality-roll configuration surface. Weights should sum
// to 1.0 (the score is a 0..100 blend). StationCeilings maps a station tier
// (0/1/2) to the maximum rarity-ladder position (0 = lowest tier) a craft
// at that station may produce — the §4 hard ceiling. RollBand is the ± RNG
// spread around the weighted score. IngredientSoftMargin is how many ladder
// positions above the weakest ingredient the output may reach (§5 soft
// ceiling: "no masterwork stew from rotten meat").
type Config struct {
	SkillWeight          float64
	ToolWeight           float64
	IngredientWeight     float64
	RollBand             int
	StationCeilings      map[int]int
	IngredientSoftMargin int
}

// DefaultConfig returns first-pass tunable values (§10). Skill dominates;
// tool and ingredient quality are secondary independent weights. Tier-0
// crafts cap at ladder position 1 (the second-lowest tier), Tier 1 at 2,
// Tier 2 unbounded (clamped to the ladder top at roll time).
func DefaultConfig() Config {
	return Config{
		SkillWeight:          0.6,
		ToolWeight:           0.15,
		IngredientWeight:     0.25,
		RollBand:             15,
		StationCeilings:      map[int]int{0: 1, 1: 2, 2: 99},
		IngredientSoftMargin: 2,
	}
}

// QualityInputs are the four §5 weights for one craft. Skill is the
// crafter's effective proficiency (0..100). ToolTierKey is the tool item's
// rarity key ("" = no tool, baseline). IngredientTierKeys are the consumed
// inputs' rarity keys ("" entries treated as the lowest tier); the weakest
// drives the soft ceiling. StationTier is the present station (0 in the
// pre-stations MVP).
type QualityInputs struct {
	Skill              int
	ToolTierKey        string
	IngredientTierKeys []string
	StationTier        int
}

// rollQuality computes the output rarity-tier key for a craft (§5):
// a weighted score → RNG band → ladder position → clamp to the station
// hard ceiling and the ingredient soft ceiling. Returns "" when no rarity
// ladder is registered (the output simply carries no rarity).
//
// The roll works in ladder POSITIONS (index into the Order-sorted ladder)
// rather than named tiers, so it is content-agnostic: a pack with a
// different rarity vocabulary gets the same behavior.
func (s *Service) rollQuality(in QualityInputs) string {
	if s.rarity == nil {
		return ""
	}
	ladder := s.rarity.All() // sorted by Order ascending
	k := len(ladder)
	if k == 0 {
		return ""
	}
	if k == 1 {
		return ladder[0].Key
	}

	posOf := func(key string) int {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			return 0
		}
		for i, t := range ladder {
			if t.Key == key {
				return i
			}
		}
		return 0 // unknown key → lowest position (fail-soft)
	}

	skill := clampInt(in.Skill, 0, 100)
	toolPos := posOf(in.ToolTierKey)
	ingPos := k - 1
	if len(in.IngredientTierKeys) == 0 {
		ingPos = 0
	} else {
		for _, key := range in.IngredientTierKeys {
			if p := posOf(key); p < ingPos {
				ingPos = p
			}
		}
	}

	norm := func(pos int) float64 { return float64(pos) / float64(k-1) * 100 }
	score := s.cfg.SkillWeight*float64(skill) +
		s.cfg.ToolWeight*norm(toolPos) +
		s.cfg.IngredientWeight*norm(ingPos)

	roll := int(math.Round(score))
	if s.cfg.RollBand > 0 && s.roller != nil {
		s.rollMu.Lock()
		jitter := s.roller.IntN(2*s.cfg.RollBand+1) - s.cfg.RollBand
		s.rollMu.Unlock()
		roll += jitter
	}
	roll = clampInt(roll, 0, 100)

	pos := int(math.Round(float64(roll) / 100 * float64(k-1)))

	// Hard station ceiling (§4): absolute, applied here.
	ceil := k - 1
	if c, ok := s.cfg.StationCeilings[in.StationTier]; ok {
		ceil = clampInt(c, 0, k-1)
	}
	if pos > ceil {
		pos = ceil
	}
	// Soft ingredient ceiling (§5): can't exceed the weakest input by more
	// than the configured margin.
	if soft := ingPos + s.cfg.IngredientSoftMargin; pos > soft {
		pos = soft
	}
	pos = clampInt(pos, 0, k-1)

	return ladder[pos].Key
}

// ladderPosition returns the index of key in the Order-sorted ladder, or
// -1 if the key is not registered. Exposed for callers/tests that need to
// reason about tier ordering.
func ladderPosition(reg *decoration.RarityRegistry, key string) int {
	if reg == nil {
		return -1
	}
	key = strings.ToLower(strings.TrimSpace(key))
	for i, t := range reg.All() {
		if t.Key == key {
			return i
		}
	}
	return -1
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
