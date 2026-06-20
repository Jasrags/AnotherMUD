// Package reputation implements a single-axis renown score per character
// (reputation.md): how widely known a character is — fame at the high end,
// infamy at the low, Unknown at zero. It is alignment's architecture
// (progression §6) specialized to ONE absolute axis, the sibling of the
// per-faction standing in internal/faction: a signed score, content-defined
// named tiers partitioning it by MAGNITUDE (a notorious villain is as "known"
// as a famous hero — §3, PD-5), a mirrored tier tag, a bounded history, the
// cancellable Shift pipeline, and the recognition-check primitive.
//
// The package is a leaf: it knows nothing of entities, sessions, or the bus.
// The composition root writes the Entity adapter (for players) and the Sink
// (bridging to eventbus), exactly as it does for faction and alignment.
package reputation

import (
	"sort"
	"strings"
)

// AdminRoleTag is the role tag whose presence makes an entity renown-immune
// (reputation.md §4 step 2 — mirrors faction §4 / progression §6.4 step 2).
const AdminRoleTag = "admin"

// TierTagPrefix is the namespace of the mirrored tier tag (reputation.md §3).
// A character carries exactly one renown:<slug> tag at a time.
const TierTagPrefix = "renown:"

// TierTag is the tier-mirror tag for a tier name (e.g. "Known Locally" →
// "renown:known-locally"). Empty tier → "" (no tag). Centralized so the format
// is named one place, mirroring faction.RankTag.
func TierTag(tier string) string {
	if tier == "" {
		return ""
	}
	return TierTagPrefix + slug(tier)
}

// slug lowercases a tier name and replaces spaces with hyphens for the tag form.
func slug(name string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), " ", "-")
}

// Tier is one named band in the renown ladder (reputation.md §3). Threshold is
// the lowest renown MAGNITUDE at/above which the tier applies — the ladder is
// symmetric across the sign, so |value| selects the tier and the sign (or the
// Infamy flag, §7) drives whether the reaction reads as fame or infamy (PD-5).
type Tier struct {
	Name      string `yaml:"name"`
	Threshold int    `yaml:"threshold"`
}

// Config is the §11 configuration surface: the shared tier ladder, the score
// bounds, the starting renown for an untouched character, and the combined
// history cap. The earn knobs (level-up increment, deed gains, recognition
// difficulties, the Fame bonus, the Low-Profile factor) are host-side and land
// with their consumers (R3/R5), not here.
type Config struct {
	// Ladder is the ordered tier ladder by ascending MAGNITUDE threshold. The
	// lowest entry (threshold 0) is the Unknown floor every character starts at.
	Ladder []Tier
	// Min / Max bound the signed score (infamy negative, fame positive).
	Min int
	Max int
	// Starting is the renown an untouched character reads at (default 0 =
	// Unknown); a class/background may raise it at creation (reputation.md §2).
	Starting int
	// HistoryCapacity caps the bounded shift history (§4 / §10).
	HistoryCapacity int
}

// DefaultConfig returns the engine defaults (reputation.md §11 example): a
// symmetric magnitude ladder Unknown → Known throughout the land, bounds
// ±1000, starting 0 (Unknown), history capacity 20.
func DefaultConfig() Config {
	return Config{
		Ladder: []Tier{
			{Name: "Unknown", Threshold: 0},
			{Name: "Known Locally", Threshold: 100},
			{Name: "Known in the Region", Threshold: 400},
			{Name: "Known Throughout the Land", Threshold: 800},
		},
		Min:             -1000,
		Max:             1000,
		Starting:        0,
		HistoryCapacity: 20,
	}
}

// normalize fills empty/invalid fields with the defaults and sorts the ladder
// ascending by threshold. Called by NewManager so a zero-value Config is safe.
func (c Config) normalize() Config {
	d := DefaultConfig()
	if len(c.Ladder) == 0 {
		c.Ladder = d.Ladder
	}
	c.Ladder = append([]Tier(nil), c.Ladder...)
	sort.SliceStable(c.Ladder, func(i, j int) bool { return c.Ladder[i].Threshold < c.Ladder[j].Threshold })
	if c.Min == 0 && c.Max == 0 {
		c.Min, c.Max = d.Min, d.Max
	}
	if c.HistoryCapacity < 1 {
		c.HistoryCapacity = 1
	}
	c.Starting = c.clamp(c.Starting)
	return c
}

// TierOf returns the name of the tier value falls in, selected by |value|
// against the magnitude ladder (reputation.md §3 — fame and infamy of equal
// magnitude share a tier). The highest ladder entry whose threshold the
// magnitude meets or exceeds wins. Empty ladder → "".
func (c Config) TierOf(value int) string {
	mag := value
	if mag < 0 {
		mag = -mag
	}
	name := ""
	for _, t := range c.Ladder {
		if mag >= t.Threshold {
			name = t.Name
		} else {
			break
		}
	}
	return name
}

// clamp constrains value to [Min, Max].
func (c Config) clamp(value int) int {
	if value < c.Min {
		return c.Min
	}
	if value > c.Max {
		return c.Max
	}
	return value
}
