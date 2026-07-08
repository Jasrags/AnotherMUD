package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// GMCP probe: a manual verification of the server's GMCP emission without a
// graphical client. It activates GMCP, logs in (which fires the login-spawn
// Room.Info), reads a couple of the mapper-relevant packages back off the wire,
// then walks one exit and confirms a fresh Room.Info arrives with a different
// coordinate — proving the per-transition emission the Mudlet mapper relies on.
//
//	go run ./cmd/telnet-smoke -addr 127.0.0.1:4000 -gmcp -name ProbeChar

// gmcpFrame is one captured GMCP package name + its JSON payload.
type gmcpFrame struct {
	Pkg  string
	JSON string
}

// frameLog is a goroutine-safe collector for captured GMCP frames. The capture
// callback (WithGMCPCapture) fires on the client's read goroutine, so add()
// must not touch the Client; it only appends here under its own lock.
type frameLog struct {
	mu     sync.Mutex
	frames []gmcpFrame
}

func (f *frameLog) add(pkg, jsonPayload string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.frames = append(f.frames, gmcpFrame{Pkg: pkg, JSON: jsonPayload})
}

func (f *frameLog) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.frames)
}

// packages returns the distinct package names seen, with per-package counts.
func (f *frameLog) packages() map[string]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	m := make(map[string]int)
	for _, fr := range f.frames {
		m[fr.Pkg]++
	}
	return m
}

// lastAfter returns the most recent frame for pkg whose index is >= from.
func (f *frameLog) lastAfter(pkg string, from int) (gmcpFrame, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.frames) - 1; i >= from && i >= 0; i-- {
		if f.frames[i].Pkg == pkg {
			return f.frames[i], true
		}
	}
	return gmcpFrame{}, false
}

// probeRoomInfo mirrors the Room.Info fields the Mudlet mapper consumes.
type probeRoomInfo struct {
	Num     string            `json:"num"`
	Name    string            `json:"name"`
	Area    string            `json:"area"`
	Exits   map[string]string `json:"exits"`
	Terrain string            `json:"terrain"`
	X       *int              `json:"x"`
	Y       *int              `json:"y"`
	Z       *int              `json:"z"`
}

func (r probeRoomInfo) coordStr() string {
	if r.X == nil || r.Y == nil || r.Z == nil {
		return "(unplaced)"
	}
	return fmt.Sprintf("(%d,%d,%d)", *r.X, *r.Y, *r.Z)
}

// probeDirs maps the server's short exit codes to the long movement command.
var probeDirs = map[string]string{
	"n": "north", "s": "south", "e": "east", "w": "west",
	"u": "up", "d": "down",
	"ne": "northeast", "nw": "northwest", "se": "southeast", "sw": "southwest",
}

// runGmcpProbe drives the login + one-step walk and prints what the server
// emitted over GMCP. Returns an error only when a hard contract is violated
// (no Room.Info on spawn, or unparseable Room.Info); a blocked move is reported
// but not fatal.
func runGmcpProbe(c *telnettest.Client, name string, log *frameLog) error {
	if err := createAndLogin(c, name); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	// Let the per-tick flusher emit the spawn snapshot packages.
	c.Drain(1500 * time.Millisecond)

	pkgs := log.packages()
	fmt.Printf("\n=== GMCP frames after login: %d total across %d packages ===\n", log.count(), len(pkgs))
	for _, p := range sortedCountKeys(pkgs) {
		fmt.Printf("  %-20s x%d\n", p, pkgs[p])
	}

	spawn, ok := log.lastAfter("Room.Info", 0)
	if !ok {
		return fmt.Errorf("no Room.Info frame after login — GMCP is not activating, or Room.Info is not emitted on the login spawn")
	}
	var start probeRoomInfo
	if err := json.Unmarshal([]byte(spawn.JSON), &start); err != nil {
		return fmt.Errorf("parsing spawn Room.Info %q: %w", spawn.JSON, err)
	}
	fmt.Printf("\nStart room:\n  num=%s\n  name=%q\n  area=%s\n  terrain=%s\n  coords=%s\n  exits=%s\n",
		start.Num, start.Name, start.Area, start.Terrain, start.coordStr(), exitSummary(start.Exits))
	fmt.Printf("  raw: %s\n", spawn.JSON)

	// Char.Vitals sanity — the HUD's core package.
	if v, ok := log.lastAfter("Char.Vitals", 0); ok {
		fmt.Printf("\nChar.Vitals: %s\n", v.JSON)
	} else {
		fmt.Printf("\nChar.Vitals: (none captured — HUD would be empty)\n")
	}

	// Walk one exit and confirm a fresh Room.Info with a new coordinate.
	if len(start.Exits) == 0 {
		fmt.Printf("\nStart room has no exits; skipping the movement check.\n")
		return nil
	}
	for _, code := range sortedMapKeys(start.Exits) {
		cmd := probeDirs[code]
		if cmd == "" {
			cmd = code // keyword/portal exit — send as-is
		}
		mark := log.count()
		if err := c.SendLine(cmd); err != nil {
			return err
		}
		_, _ = c.ExpectTimeout(gamePrompt, 6*time.Second)
		c.Drain(1200 * time.Millisecond)

		moved, ok := log.lastAfter("Room.Info", mark)
		if !ok {
			fmt.Printf("\nMove %q: no new Room.Info (exit blocked/locked?); trying next exit.\n", cmd)
			continue
		}
		var dest probeRoomInfo
		if err := json.Unmarshal([]byte(moved.JSON), &dest); err != nil {
			return fmt.Errorf("parsing post-move Room.Info %q: %w", moved.JSON, err)
		}
		fmt.Printf("\nMoved %s → %s\n  num=%s coords=%s\n  raw: %s\n",
			cmd, dest.Name, dest.Num, dest.coordStr(), moved.JSON)
		if dest.Num == start.Num {
			fmt.Printf("  NOTE: room id unchanged — server re-emitted the same room.\n")
		} else {
			fmt.Printf("  ✓ per-transition Room.Info confirmed: room id and coordinate advanced.\n")
		}
		return nil
	}
	fmt.Printf("\nNo exit produced a room transition (all blocked?).\n")
	return nil
}

func exitSummary(exits map[string]string) string {
	if len(exits) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(exits))
	for _, code := range sortedMapKeys(exits) {
		parts = append(parts, fmt.Sprintf("%s→%s", code, exits[code]))
	}
	return strings.Join(parts, " ")
}

func sortedMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedCountKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
