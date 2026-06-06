package light

// Per-viewer sight (spec §4). Effective light is per-viewer because
// some viewers see in the dark: a racial darkvision floor, and
// light/sight effects that raise the floor for a duration. Both feed
// the single ViewerFloor term Resolve consumes after everything else.
//
// This file is pure derivation. The call site (Phase 5 render/combat)
// reads the viewer's darkvision flag (a racial tag) and its active
// effect flags, then asks Config to combine them into one floor — no
// state lives here, so an expired effect simply stops contributing on
// the next recompute (no explicit reversal needed, §4).

// DarkvisionFlag is the racial-flag / viewer-tag name marking a viewer
// that sees in the dark. A race declares it in its racial_flags; the
// live viewer then reports it via HasTag(DarkvisionFlag). The call site
// passes the result to Config.ViewerFloor.
const DarkvisionFlag = "darkvision"

// EffectFloorFor returns the brightest floor among the viewer's active
// effect flags, per the Config.EffectFloors map (flag → floor). A
// viewer with no matching flag gets Black (no contribution). This is
// how a cast-light / infravision effect raises sight (§4): the effect
// carries a flag listed in EffectFloors; while the effect is active the
// flag is present and floors the viewer, and when it expires the flag
// is gone and the floor drops on the next recompute.
func (c Config) EffectFloorFor(flags []string) Level {
	if len(c.EffectFloors) == 0 {
		return Black
	}
	best := Black
	for _, f := range flags {
		if lvl, ok := c.EffectFloors[f]; ok && lvl > best {
			best = lvl
		}
	}
	return clamp(best)
}

// ViewerFloor combines a viewer's darkvision floor (capped per §4) and
// its active light/sight effect floor into the single per-viewer floor
// Resolve consumes. The brighter of the two wins; the result is the
// `viewerFloor` argument to Resolver.Effective.
func (c Config) ViewerFloor(hasDarkvision bool, effectFlags []string) Level {
	floor := c.DarkvisionViewerFloor(hasDarkvision)
	if ef := c.EffectFloorFor(effectFlags); ef > floor {
		floor = ef
	}
	return clamp(floor)
}
