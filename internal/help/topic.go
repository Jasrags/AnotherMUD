// Package help is the M10 help-topic registry and renderer
// (ui-rendering-help §9-§10). Packs register topics; the help command
// queries them by id, title, or fuzzy keyword and renders the result
// with semantic color tags the internal/render pipeline resolves.
package help

import "strings"

// Role is the visibility tier of a topic and the entitlement tier of a
// requester. Higher tiers see everything at or below them. RoleNone is a
// topic visible to everyone (including pre-login) and the tier of a
// requester with no entity id.
//
// Builder/admin elevation is a placeholder (§9.5 / §13): the requester
// tier resolution caps at TierPlayer today, so builder/admin topics are
// authored but not yet surfaced to anyone.
type Role int

const (
	RoleNone    Role = iota // visible to all (no `role` declared)
	RolePlayer              // visible to any logged-in player
	RoleBuilder            // builders + admins (not yet elevated)
	RoleAdmin              // admins only (not yet elevated)
)

// ParseRole maps a YAML role string to a Role. An empty/unknown value is
// RoleNone (visible to all) — the safe default for content typos.
func ParseRole(s string) Role {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "player":
		return RolePlayer
	case "builder":
		return RoleBuilder
	case "admin":
		return RoleAdmin
	default:
		return RoleNone
	}
}

// Topic is a content-defined help record (§9.1). ID and Title are
// required; the loader skips topics missing either. PackName and
// NamespacedID are computed by the loader.
type Topic struct {
	ID           string
	Title        string
	Category     string
	Brief        string
	Body         string
	Syntax       []string
	Keywords     []string
	SeeAlso      []string
	Role         Role
	PackName     string
	NamespacedID string
}

// Summary is the compact form returned in disambiguation lists (§9.6).
type Summary struct {
	ID    string
	Title string
	Brief string
}

func (t *Topic) summary() Summary {
	return Summary{ID: t.ID, Title: t.Title, Brief: t.Brief}
}
