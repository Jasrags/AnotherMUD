// Package mssp implements the MUD Server Status Protocol variable
// table used by crawlers like grapevine.haus to discover and list
// MUD servers.
//
// Spec: docs/specs/networking-protocols.md §8.
//
// Scope: this package owns the Config shape and the Encode function
// that turns a Config snapshot into the raw VAR/VAL byte sequence
// the spec §8.1 wire format requires. The IAC SB ... IAC SE framing
// around that payload is the telnet package's job (negotiator
// handles DO MSSP and wraps the payload in the subneg envelope).
//
// Dynamic values (PLAYERS, UPTIME) come from caller-supplied
// closures so each emission reflects current state — the
// composition root wires them against session.Manager.Count() and
// the server start time.
package mssp

import (
	"sort"
	"strconv"
	"strings"
)

// MSSP subneg control bytes per spec §8.1. The variable-table
// payload is a sequence of (VAR, name, VAL, value) records;
// names and values are ASCII byte runs with no terminator.
const (
	varByte byte = 1
	valByte byte = 2
)

// Config is the per-server MSSP variable table. All string fields
// emit verbatim; bool fields emit as "1" / "0" per crawler
// convention; PLAYERS and UPTIME are read through their factories
// at every Encode call so the snapshot reflects live state.
//
// A zero-value Config is legal but emits an empty table. The
// composition root populates the fields it has values for and
// leaves the rest empty.
//
// Spec §8.2 standard variables:
//
//   - NAME, CODEBASE, CONTACT, HOSTNAME, PORT, CREATED, LANGUAGE,
//     FAMILY — static config
//   - GAMEPLAY — pipe-joined list (e.g. "Hack and Slash|Roleplaying")
//   - CLASSES, RACES, LEVELS, EQUIPMENT, MULTIPLAYING,
//     PLAYERKILLING — 1/0 booleans
//   - PLAYERS — live player count
//   - UPTIME — seconds since server start
//   - ANSI, UTF-8, GMCP — capability flags
//   - MCCP — "0" until compression is implemented
type Config struct {
	Name     string
	Codebase string
	Contact  string
	Hostname string
	Port     string
	Created  string
	Language string
	Family   string

	Gameplay []string

	Classes       bool
	Races         bool
	Levels        bool
	Equipment     bool
	Multiplaying  bool
	Playerkilling bool

	// ANSI defaults to true: the engine emits ANSI escape sequences
	// (spec §9). Set false only when a deployment routes through a
	// terminator that strips them.
	ANSI bool
	// UTF-8 defaults to true: all engine text is UTF-8.
	UTF8 bool
	// GMCP defaults to false until M16.3 lands the GMCP transport.
	// Composition flips it true once the protocol is live.
	GMCP bool
	// MCCP stays false (the engine doesn't implement compression).
	MCCP bool

	// Players returns the current logged-in player count. nil-safe
	// (the encoder emits 0 when unset).
	Players func() int
	// Uptime returns the number of seconds since server start.
	// nil-safe (the encoder emits 0 when unset).
	Uptime func() int64
}

// Encode produces the MSSP subneg PAYLOAD bytes per spec §8.1
// — the (VAR name VAL value) records that go between the
// `IAC SB MSSP` and `IAC SE` framing. The framing itself is the
// caller's responsibility.
//
// Variables emit in a stable order so the output is comparable
// across calls (helps test assertions and any caching crawler).
// Empty string-valued fields are skipped (no point in emitting
// `VAR PORT VAL ""`). Bool fields and the always-present
// PLAYERS / UPTIME emit unconditionally.
func Encode(cfg Config) []byte {
	pairs := []pair{
		{"NAME", cfg.Name},
		{"CODEBASE", cfg.Codebase},
		{"CONTACT", cfg.Contact},
		{"HOSTNAME", cfg.Hostname},
		{"PORT", cfg.Port},
		{"CREATED", cfg.Created},
		{"LANGUAGE", cfg.Language},
		{"FAMILY", cfg.Family},
		{"GAMEPLAY", strings.Join(cfg.Gameplay, "|")},
	}
	// String pairs filter out empty values per the doc above.
	stringEntries := make([]pair, 0, len(pairs))
	for _, p := range pairs {
		if p.value != "" {
			stringEntries = append(stringEntries, p)
		}
	}
	// Stable iteration order; the source slice already encodes
	// the canonical spec order, but sort by name is the safer
	// floor if a future change adds fields out of order.
	sort.SliceStable(stringEntries, func(i, j int) bool {
		return stringEntries[i].name < stringEntries[j].name
	})

	bools := []pair{
		{"CLASSES", boolStr(cfg.Classes)},
		{"RACES", boolStr(cfg.Races)},
		{"LEVELS", boolStr(cfg.Levels)},
		{"EQUIPMENT", boolStr(cfg.Equipment)},
		{"MULTIPLAYING", boolStr(cfg.Multiplaying)},
		{"PLAYERKILLING", boolStr(cfg.Playerkilling)},
		{"ANSI", boolStr(cfg.ANSI)},
		{"UTF-8", boolStr(cfg.UTF8)},
		{"GMCP", boolStr(cfg.GMCP)},
		{"MCCP", boolStr(cfg.MCCP)},
	}
	sort.SliceStable(bools, func(i, j int) bool {
		return bools[i].name < bools[j].name
	})

	players := 0
	if cfg.Players != nil {
		players = cfg.Players()
	}
	uptime := int64(0)
	if cfg.Uptime != nil {
		uptime = cfg.Uptime()
	}
	dynamic := []pair{
		{"PLAYERS", strconv.Itoa(players)},
		{"UPTIME", strconv.FormatInt(uptime, 10)},
	}
	sort.SliceStable(dynamic, func(i, j int) bool {
		return dynamic[i].name < dynamic[j].name
	})

	// Final order: strings, then bools, then dynamic. Each group
	// internally alpha-sorted. Stable across calls so MSSP-caching
	// crawlers don't see noise from reordering.
	out := make([]byte, 0, 256)
	for _, p := range stringEntries {
		out = appendRecord(out, p.name, p.value)
	}
	for _, p := range bools {
		out = appendRecord(out, p.name, p.value)
	}
	for _, p := range dynamic {
		out = appendRecord(out, p.name, p.value)
	}
	return out
}

// pair is one VAR/VAL record before serialization.
type pair struct {
	name, value string
}

// appendRecord emits one (VAR name VAL value) record.
func appendRecord(dst []byte, name, value string) []byte {
	dst = append(dst, varByte)
	dst = append(dst, name...)
	dst = append(dst, valByte)
	dst = append(dst, value...)
	return dst
}

// boolStr maps a Go bool to the MSSP "1" / "0" string convention.
func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
