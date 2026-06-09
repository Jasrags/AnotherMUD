// Package gathering implements the gathering feature (gathering.md): the
// `forage` (ambient) and `harvest` (resource node) verbs that turn the
// world's biomes into ingredient sources, the gathering proficiency + its
// quality roll, and the scarcity controls. It is the non-vendor ingredient
// source crafting-and-cooking §8 demands.
//
// This file is the quality roll (gathering.md §4). It MIRRORS the crafting
// quality roll (crafting-and-cooking §5, internal/crafting/quality.go) —
// same weighted-score → RNG band → rarity-ladder-position → ceiling shape —
// but reads richness + a per-source ceiling rather than ingredient + station
// tiers. It deliberately does NOT import crafting: the shared leaf is the
// rarity ladder (decoration.RarityRegistry), which both features consume.
package gathering

import (
	"math"
	"strings"
)

// Roller is the RNG seam (math/rand/v2.Rand-style IntN), identical in shape
// to crafting/combat. The Service guards roller use with its own mutex
// since gathers arrive from per-session goroutines.
type Roller interface {
	// IntN returns a pseudo-random int in [0, n); panics on n <= 0.
	IntN(n int) int
}

// Config is the §8 quality-roll configuration surface. Weights blend into a
// 0..100 score (they should sum to ~1.0). RollBand is the ± RNG spread.
// Skill dominates; tool quality and source richness are secondary weights —
// friction lowers quality, never availability (§1.1).
type Config struct {
	SkillWeight    float64
	ToolWeight     float64
	RichnessWeight float64
	RollBand       int
}

// DefaultConfig returns first-pass tunable values (§8). Mirrors crafting's
// skill-dominant blend; richness takes the slot crafting gives ingredients.
func DefaultConfig() Config {
	return Config{
		SkillWeight:    0.6,
		ToolWeight:     0.15,
		RichnessWeight: 0.25,
		RollBand:       15,
	}
}

// QualityInputs are the §4 weights for one gather. Skill is the gatherer's
// effective gathering proficiency (0..100). ToolTierKey is the held tool's
// rarity key ("" = no tool = baseline, never a refusal for forage).
// Richness is the source's richness 0..100 (a rich vein/grove rolls higher).
// SourceCeiling is the hard cap on the rolled ladder position (§4 — a
// source's tier ceiling caps the roll regardless of skill/tool); a value
// < 0 means "no ceiling" (clamped to the ladder top).
type QualityInputs struct {
	Skill         int
	ToolTierKey   string
	Richness      int
	SourceCeiling int
}

// rollQuality computes the yielded item's rarity-tier key (§4): a weighted
// score → RNG band → ladder position → clamp to the source ceiling. Returns
// "" when no rarity ladder is registered (the yield carries no rarity).
// Works in ladder POSITIONS so it is content-agnostic (mirrors crafting).
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
	richness := clampInt(in.Richness, 0, 100)
	toolPos := posOf(in.ToolTierKey)

	// skill and richness are already 0..100 (content/spec units, §4), fed
	// in directly; toolPos is a ladder index normalized to 0..100. Do NOT
	// norm() richness — it is not a ladder position.
	norm := func(pos int) float64 { return float64(pos) / float64(k-1) * 100 }
	score := s.cfg.SkillWeight*float64(skill) +
		s.cfg.ToolWeight*norm(toolPos) +
		s.cfg.RichnessWeight*float64(richness)

	roll := int(math.Round(score))
	if s.cfg.RollBand > 0 && s.roller != nil {
		s.rollMu.Lock()
		jitter := s.roller.IntN(2*s.cfg.RollBand+1) - s.cfg.RollBand
		s.rollMu.Unlock()
		roll += jitter
	}
	roll = clampInt(roll, 0, 100)

	pos := int(math.Round(float64(roll) / 100 * float64(k-1)))

	// Hard source ceiling (§4): a source caps the rolled tier regardless of
	// skill/tool. A negative ceiling means "no cap" (the ladder top).
	ceil := k - 1
	if in.SourceCeiling >= 0 {
		ceil = clampInt(in.SourceCeiling, 0, k-1)
	}
	if pos > ceil {
		pos = ceil
	}
	pos = clampInt(pos, 0, k-1)

	return ladder[pos].Key
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
