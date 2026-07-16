//go:build unix

package main

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_GmcpRoomMap proves the web-client P2 additive package end to end: on
// login the server emits a Room.Map GMCP frame — the local neighbourhood graph —
// and after a step it re-emits centred on the new room. Confirms the rich map a
// browser client draws (nodes, exits, fog-of-war visited flags) reaches the wire.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_GmcpRoomMap -v
func TestLive_GmcpRoomMap(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil) // starter-world: a small, well-connected room graph

	frames := &frameLog{}
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second), telnettest.WithGMCPCapture(frames.add))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := c.ActivateGMCP(); err != nil {
		t.Fatalf("activate gmcp: %v", err)
	}
	if err := createAndLogin(c, "Cartographer"); err != nil {
		t.Fatalf("create+login: %v", err)
	}
	c.Drain(2000 * time.Millisecond)

	type node struct {
		Num     string            `json:"num"`
		Exits   map[string]string `json:"exits"`
		Visited bool              `json:"visited"`
	}
	type roomMap struct {
		Center string `json:"center"`
		Radius int    `json:"radius"`
		Rooms  []node `json:"rooms"`
	}

	f, ok := frames.lastAfter("Room.Map", 0)
	if !ok {
		t.Fatalf("no Room.Map frame captured on login (web-client P2 neighbourhood)")
	}
	var rm roomMap
	if err := json.Unmarshal([]byte(f.JSON), &rm); err != nil {
		t.Fatalf("unmarshal Room.Map %q: %v", f.JSON, err)
	}
	if rm.Center == "" || len(rm.Rooms) == 0 {
		t.Fatalf("Room.Map has no center/rooms: %s", f.JSON)
	}
	// The current room is in the neighbourhood, marked visited (we're standing in
	// it), and has at least one exit to walk.
	var cur *node
	for i := range rm.Rooms {
		if rm.Rooms[i].Num == rm.Center {
			cur = &rm.Rooms[i]
		}
	}
	if cur == nil {
		t.Fatalf("center %q not among Room.Map rooms:\n%s", rm.Center, f.JSON)
	}
	if !cur.Visited {
		t.Errorf("the current room should be visited=true: %+v", *cur)
	}
	if len(cur.Exits) == 0 {
		t.Fatalf("current room has no exits to walk:\n%s", f.JSON)
	}
	t.Logf("login Room.Map: center=%s radius=%d rooms=%d", rm.Center, rm.Radius, len(rm.Rooms))

	// Walk one exit → a fresh Room.Map re-centres on the new room.
	before := frames.count()
	dir := ""
	for d := range cur.Exits {
		dir = d
		break
	}
	if err := c.SendLine(dir); err != nil {
		t.Fatalf("send %q: %v", dir, err)
	}
	c.Drain(1500 * time.Millisecond)

	f2, ok := frames.lastAfter("Room.Map", before)
	if !ok {
		t.Fatalf("no Room.Map re-emitted after walking %q", dir)
	}
	var rm2 roomMap
	if err := json.Unmarshal([]byte(f2.JSON), &rm2); err != nil {
		t.Fatalf("unmarshal Room.Map #2: %v", err)
	}
	if rm2.Center == "" || rm2.Center == rm.Center {
		t.Errorf("Room.Map center did not advance after walking %q: %q → %q", dir, rm.Center, rm2.Center)
	}
	t.Logf("after %q: center advanced %s → %s", dir, rm.Center, rm2.Center)
}
