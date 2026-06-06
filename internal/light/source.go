package light

// Light sources (spec §3): an item contributes light to the room it is
// in only while *lit*. The level it contributes, its lit state, and its
// remaining fuel are reserved item properties read through the small
// Source interface below so this package needs no dependency on
// entities — *entities.ItemInstance already satisfies it.

const (
	// PropItemLight names the level a light source contributes when
	// lit. It is the same key string as the room override
	// (PropRoomLight) — "light" is the light level associated with a
	// thing — but read from an item rather than a room.
	PropItemLight = PropRoomLight
	// PropItemLit is the bool instance property holding the source's
	// lit state. Lives on the instance so it survives pickup / drop /
	// give / store (spec §3.1) and is admin-settable.
	PropItemLit = "lit"
	// PropItemFuel is the int instance property holding remaining fuel
	// for a fuel-burning source (spec §3.2). Absent means the source
	// is permanent (never burns down); zero means spent (guttered).
	PropItemFuel = "fuel"
)

// Source is the slice of an item the light surface reads: its property
// bag. *entities.ItemInstance satisfies it via its Property method.
type Source interface {
	Property(key string) (any, bool)
}

// SourceLevel returns the level named by src's `light` property,
// regardless of lit state (Black when absent, not a string, or not a
// valid level name). Used to identify whether an item is a light
// source at all and what it would contribute if lit.
func SourceLevel(src Source) Level {
	if src == nil {
		return Black
	}
	v, ok := src.Property(PropItemLight)
	if !ok {
		return Black
	}
	s, ok := v.(string)
	if !ok {
		return Black
	}
	lvl, ok := ParseLevel(s)
	if !ok {
		return Black
	}
	return lvl
}

// IsLit reports whether src's `lit` property is the bool true.
func IsLit(src Source) bool {
	if src == nil {
		return false
	}
	v, ok := src.Property(PropItemLit)
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// IsSource reports whether the item is a light source at all (carries a
// valid `light` level), lit or not. The fuel loop and the light/
// extinguish verbs use it to reject non-sources.
func IsSource(src Source) bool {
	return SourceLevel(src) > Black
}

// Contribution returns the level src adds to its room right now: its
// SourceLevel if currently lit, else Black (an unlit source, or a
// non-source, contributes nothing).
func Contribution(src Source) Level {
	if !IsLit(src) {
		return Black
	}
	return SourceLevel(src)
}

// BestContribution returns the brightest contribution across srcs
// (Black when none contribute). This is the `Sources` term Resolve
// consumes — the call site passes the viewer's held light plus the
// luminous items/mobs lying in the room.
func BestContribution(srcs ...Source) Level {
	best := Black
	for _, s := range srcs {
		if c := Contribution(s); c > best {
			best = c
		}
	}
	return best
}
