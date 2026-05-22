// Package world is the spatial substrate from
// docs/specs/world-rooms-movement.md.
//
// M1 scope: Direction enum, Room, Exit, World registry, and the move
// primitive (§3.3). No doors, no tag index, no temporary exits, no
// areas — those land when later milestones need them.
package world

import "strings"

// Direction is the engine's movement axis enumeration. Cardinal +
// vertical is the M1 set per spec §3.1.
type Direction uint8

const (
	DirInvalid Direction = iota
	DirNorth
	DirSouth
	DirEast
	DirWest
	DirUp
	DirDown
)

// Long returns the canonical full-word name ("north").
func (d Direction) Long() string {
	switch d {
	case DirNorth:
		return "north"
	case DirSouth:
		return "south"
	case DirEast:
		return "east"
	case DirWest:
		return "west"
	case DirUp:
		return "up"
	case DirDown:
		return "down"
	}
	return ""
}

// Short returns the canonical short-form alias ("n").
func (d Direction) Short() string {
	switch d {
	case DirNorth:
		return "n"
	case DirSouth:
		return "s"
	case DirEast:
		return "e"
	case DirWest:
		return "w"
	case DirUp:
		return "u"
	case DirDown:
		return "d"
	}
	return ""
}

// Opposite returns the reverse direction. DirInvalid for unknown.
func (d Direction) Opposite() Direction {
	switch d {
	case DirNorth:
		return DirSouth
	case DirSouth:
		return DirNorth
	case DirEast:
		return DirWest
	case DirWest:
		return DirEast
	case DirUp:
		return DirDown
	case DirDown:
		return DirUp
	}
	return DirInvalid
}

// String returns the long form for logging/debug.
func (d Direction) String() string {
	if s := d.Long(); s != "" {
		return s
	}
	return "invalid"
}

// ParseDirection resolves a token (case-insensitive) to a Direction,
// matching either the short or long form. Returns DirInvalid, false on
// no match.
func ParseDirection(s string) (Direction, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "n", "north":
		return DirNorth, true
	case "s", "south":
		return DirSouth, true
	case "e", "east":
		return DirEast, true
	case "w", "west":
		return DirWest, true
	case "u", "up":
		return DirUp, true
	case "d", "down":
		return DirDown, true
	}
	return DirInvalid, false
}
