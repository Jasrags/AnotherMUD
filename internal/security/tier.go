// Package security implements the heat consequence engine (security-response.md):
// a crime in a policed zone raises a player's heat, and crossing the zone's
// threshold dispatches a timed patrol response that grudge-hunts the offender.
// Heat is runtime-only (no persistence). The package is decoupled from world /
// entities / session via injected Deps closures; it imports world only for the
// RoomID type.
package security

import "strings"

// Tier is a parsed area enforcement level (security-response.md §2). Higher =
// more policed. TierNone is unpoliced (unset / unrecognized) — the barrens Z tier
// is also unpoliced but distinct so content can name it explicitly.
type Tier int

const (
	// TierNone — unset / unrecognized: no law (same effect as Z).
	TierNone Tier = iota
	// TierZ — the barrens: law does not come.
	TierZ
	TierD
	TierC
	TierB
	TierA
	TierAA
	TierAAA
)

// ParseTier maps a content `security` string (AAA…Z, case/space-insensitive) to a
// Tier. An empty or unrecognized value is TierNone (unpoliced).
func ParseTier(s string) Tier {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "AAA":
		return TierAAA
	case "AA":
		return TierAA
	case "A":
		return TierA
	case "B":
		return TierB
	case "C":
		return TierC
	case "D":
		return TierD
	case "Z":
		return TierZ
	default:
		return TierNone
	}
}

// String renders a Tier back to its content label (for logs).
func (t Tier) String() string {
	switch t {
	case TierAAA:
		return "AAA"
	case TierAA:
		return "AA"
	case TierA:
		return "A"
	case TierB:
		return "B"
	case TierC:
		return "C"
	case TierD:
		return "D"
	case TierZ:
		return "Z"
	default:
		return "unpoliced"
	}
}

// Policy is a tier's response tuning (security-response.md §3/§5). A zero-value
// policy (HeatPerCrime <= 0) is unpoliced — no heat, no response.
type Policy struct {
	// HeatPerCrime is the heat one crime adds in this tier. <= 0 ⇒ unpoliced.
	HeatPerCrime int
	// Threshold is the heat at/above which a response is scheduled.
	Threshold int
	// DelayTicks is how long after the threshold crossing the responders arrive.
	DelayTicks uint64
	// Responders is how many law mobs the response spawns.
	Responders int
}

// DefaultPolicies is the indicative per-tier table (security-response.md §5). The
// shape is normative — a policed tier reacts to less heat, faster, with more
// responders as the tier climbs; Z / None are absent (unpoliced, zero-value).
func DefaultPolicies() map[Tier]Policy {
	return map[Tier]Policy{
		TierAAA: {HeatPerCrime: 50, Threshold: 40, DelayTicks: 30, Responders: 3},
		TierAA:  {HeatPerCrime: 40, Threshold: 60, DelayTicks: 50, Responders: 2},
		TierA:   {HeatPerCrime: 30, Threshold: 80, DelayTicks: 100, Responders: 2},
		TierB:   {HeatPerCrime: 20, Threshold: 100, DelayTicks: 200, Responders: 1},
		TierC:   {HeatPerCrime: 15, Threshold: 120, DelayTicks: 300, Responders: 1},
		TierD:   {HeatPerCrime: 15, Threshold: 120, DelayTicks: 400, Responders: 1},
		// TierZ / TierNone omitted ⇒ zero-value Policy ⇒ unpoliced.
	}
}
