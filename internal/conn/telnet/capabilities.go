package telnet

import (
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/render"
)

// knownMudClients is the case-insensitive allowlist of MUD-client
// TTYPE strings per spec §7.2. A match here grants Extended color
// even without a TRUECOLOR / 256COLOR TTYPE hint, and disables
// server-side echo (the MUD client handles its own).
//
// Match is case-insensitive substring against the normalized
// client name. The substring shape is forgiving against clients
// that include version suffixes (`Mudlet 4.x`, `TinTin++/2.x`,
// `MUSHclient/5.x`) — the substring of the bare product name
// still matches.
//
// Adding a client requires editing this list. Per the spec, the
// allowlist is intentionally not pack-extensible — it gates an
// observability + UX policy that belongs to the engine, not
// content.
var knownMudClients = []string{
	"mudlet",
	"mushclient",
	"tintin++",
	"tintin",
	"zmud",
	"cmud",
	"atlantis",
	"potato",
	"blowtorch",
	"kildclient",
	"beip",
	"gnomemud",
}

// isKnownMudClient reports whether the normalized client name
// matches any entry in the MUD-client allowlist. Case-insensitive
// substring match.
func isKnownMudClient(name string) bool {
	if name == "" {
		return false
	}
	lower := strings.ToLower(name)
	for _, candidate := range knownMudClients {
		if strings.Contains(lower, candidate) {
			return true
		}
	}
	return false
}

// deriveColorTier returns the ColorSupport tier (spec §7.2) from
// the most-specific TTYPE value and the IsMudClient flag.
//
// Rules in priority order:
//  1. No TTYPE → render.ColorTierNone.
//  2. TTYPE contains "TRUECOLOR" → render.ColorTierTrueColor.
//  3. TTYPE contains "256COLOR" → render.ColorTierExtended.
//  4. Known MUD client → render.ColorTierExtended.
//  5. Any other TTYPE → render.ColorTierBasic.
//
// The TRUECOLOR hint outranks the known-client bump because a
// client that advertises TRUECOLOR has explicitly opted in to
// the wider palette — even if it's also on the allowlist.
func deriveColorTier(ttype string, isMudClient bool) render.ColorTier {
	if ttype == "" {
		return render.ColorTierNone
	}
	upper := strings.ToUpper(ttype)
	if strings.Contains(upper, "TRUECOLOR") {
		return render.ColorTierTrueColor
	}
	if strings.Contains(upper, "256COLOR") {
		return render.ColorTierExtended
	}
	if isMudClient {
		return render.ColorTierExtended
	}
	return render.ColorTierBasic
}
