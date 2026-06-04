package login

import (
	"fmt"
	"strings"
)

// NameDecision is a name-gate's verdict on a submitted character name
// (login spec §3 / §1 Gate).
type NameDecision int

const (
	// NameAllow lets the name proceed to the new-player flow.
	NameAllow NameDecision = iota
	// NameReject refuses the name and reprompts (the phase does not
	// advance).
	NameReject
	// NameDisconnect refuses the name and closes the connection.
	NameDisconnect
)

// NameGate is a pluggable name policy run on the new-player path (spec
// §3). It returns a decision and, for a non-allow decision, a
// user-facing reason. Gates are registered as an ordered list on Config;
// the first non-allow decision wins (runNameGates).
//
// Gates are distinct from the always-on name *validation* (length /
// charset) in validateName, which applies to every submitted name. Gates
// specifically guard entry into character creation — the spec's "a gate
// may block a name from entering the new-player flow."
type NameGate func(name string) (NameDecision, string)

// ReservedNameGate refuses (with a reprompt) a new-player name that
// matches any reserved name, case-insensitively. Reserved names protect
// privileged or system identities (admin, guard, …) from impersonation
// at character creation. Blocklist entries are trimmed and lowercased.
func ReservedNameGate(reserved []string) NameGate {
	set := make(map[string]struct{}, len(reserved))
	for _, r := range reserved {
		if r = strings.ToLower(strings.TrimSpace(r)); r != "" {
			set[r] = struct{}{}
		}
	}
	return func(name string) (NameDecision, string) {
		if _, ok := set[strings.ToLower(strings.TrimSpace(name))]; ok {
			return NameReject, fmt.Sprintf("The name %q is reserved. Please choose another.", name)
		}
		return NameAllow, ""
	}
}

// runNameGates applies gates in order and returns the first non-allow
// decision, or NameAllow if every gate allows (including the empty list).
func runNameGates(name string, gates []NameGate) (NameDecision, string) {
	for _, g := range gates {
		if d, reason := g(name); d != NameAllow {
			return d, reason
		}
	}
	return NameAllow, ""
}
