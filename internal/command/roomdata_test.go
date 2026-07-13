package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/biome"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// roomDataActor extends the shared testActor with the admin role check
// and the persisted room-data toggle the look block + roomdata verb read.
type roomDataActor struct {
	*testActor
	admin    bool
	showData bool
}

func (a *roomDataActor) HasRole(role string) bool { return a.admin && role == "admin" }
func (a *roomDataActor) ShowRoomData() bool       { return a.showData }
func (a *roomDataActor) SetShowRoomData(v bool)   { a.showData = v }

func coordTestRoom() *world.Room {
	return &world.Room{
		ID:          "core:square",
		AreaID:      "core:town",
		Name:        "The Square",
		Description: "A wide cobbled plaza.",
		Terrain:     "outdoors",
		Tags:        []string{"safe-room", "no-summon"},
		Properties:  map[string]any{"scripted": true},
		Exits: map[world.Direction]world.Exit{
			world.DirNorth: {Target: "core:market"},
			world.DirEast:  {Target: "core:forge", Door: &world.DoorState{Closed: true, Locked: true}},
		},
		Coord: &world.Coord{X: 0, Y: 0, Z: 0},
	}
}

func TestLook_RoomDataBlock_ShownForAdminWithToggleOn(t *testing.T) {
	room := coordTestRoom()
	a := &roomDataActor{testActor: newTestActor(room), admin: true, showData: true}
	if err := command.LookHandler(context.Background(), &command.Context{Actor: a}); err != nil {
		t.Fatalf("LookHandler: %v", err)
	}
	out := a.lastLine()
	for _, want := range []string{
		"room data",                      // block header
		"core:square",                    // room id
		"core:town",                      // area id
		"(0,0,0) derived",                // coordinate + source
		"outdoors",                       // terrain
		"no-summon, safe-room",           // sorted tags
		"scripted=true",                  // property
		"n -> core:market",               // exit + target
		"e -> core:forge (door: locked)", // exit + door state
	} {
		if !strings.Contains(out, want) {
			t.Errorf("look output missing %q\n---\n%s", want, out)
		}
	}
}

// The room-data block surfaces the resolved biome and its intrinsic ambient
// hazard (area-effects.md §4.6) — the builder view a GM needs to see why a
// `toxic` room is dangerous.
func TestLook_RoomDataBlock_BiomeAndHazard(t *testing.T) {
	room := coordTestRoom()
	room.Terrain = "toxic"
	reg := biome.NewRegistry()
	if err := reg.RegisterPack("test", &biome.Biome{
		ID:          "toxic",
		DisplayName: "the Glow",
		MoveCost:    2,
		Ambience:    []string{"The rubble glows faintly."},
		Hazard:      &biome.Hazard{Damage: 4, DamageType: "radiation", ProtectionKey: "rad-shielded"},
	}); err != nil {
		t.Fatalf("register biome: %v", err)
	}
	a := &roomDataActor{testActor: newTestActor(room), admin: true, showData: true}
	if err := command.LookHandler(context.Background(), &command.Context{Actor: a, Biomes: reg}); err != nil {
		t.Fatalf("LookHandler: %v", err)
	}
	out := a.lastLine()
	// Assertions target value-side substrings — the block interleaves
	// <subtle>label</subtle> markup, so "label value" isn't contiguous.
	for _, want := range []string{
		"biome", "the Glow (toxic)", "1 line(s)",
		"hazard", "4 radiation / tick", "rad-shielded (worn)",
		"(effective)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("room-data block missing %q\n---\n%s", want, out)
		}
	}
}

// A biome with no hazard shows the biome line but no hazard line.
func TestLook_RoomDataBlock_BiomeNoHazard(t *testing.T) {
	room := coordTestRoom()
	room.Terrain = "sprawl"
	reg := biome.NewRegistry()
	if err := reg.RegisterPack("test", &biome.Biome{ID: "sprawl", DisplayName: "the sprawl", MoveCost: 1}); err != nil {
		t.Fatalf("register biome: %v", err)
	}
	a := &roomDataActor{testActor: newTestActor(room), admin: true, showData: true}
	if err := command.LookHandler(context.Background(), &command.Context{Actor: a, Biomes: reg}); err != nil {
		t.Fatalf("LookHandler: %v", err)
	}
	out := a.lastLine()
	if !strings.Contains(out, "the sprawl (sprawl)") {
		t.Errorf("room-data block missing the biome line:\n%s", out)
	}
	if strings.Contains(out, "hazard") {
		t.Errorf("harmless biome should show no hazard line:\n%s", out)
	}
}

func TestLook_RoomDataBlock_HiddenWhenToggleOff(t *testing.T) {
	room := coordTestRoom()
	a := &roomDataActor{testActor: newTestActor(room), admin: true, showData: false}
	if err := command.LookHandler(context.Background(), &command.Context{Actor: a}); err != nil {
		t.Fatalf("LookHandler: %v", err)
	}
	if strings.Contains(a.lastLine(), "room data") {
		t.Errorf("room data block shown with toggle off:\n%s", a.lastLine())
	}
}

func TestLook_RoomDataBlock_HiddenForNonAdmin(t *testing.T) {
	room := coordTestRoom()
	// Toggle on, but the actor is NOT an admin — the role is the gate.
	a := &roomDataActor{testActor: newTestActor(room), admin: false, showData: true}
	if err := command.LookHandler(context.Background(), &command.Context{Actor: a}); err != nil {
		t.Fatalf("LookHandler: %v", err)
	}
	if strings.Contains(a.lastLine(), "room data") {
		t.Errorf("room data block shown for non-admin:\n%s", a.lastLine())
	}
}

func TestRoomDataHandler_TogglesPreference(t *testing.T) {
	a := &roomDataActor{testActor: newTestActor(nil), admin: true}

	// Bare toggle: off → on.
	if err := command.RoomDataHandler(context.Background(), &command.Context{Actor: a}); err != nil {
		t.Fatalf("RoomDataHandler: %v", err)
	}
	if !a.ShowRoomData() {
		t.Error("toggle did not turn room data ON")
	}
	if !strings.Contains(a.lastLine(), "ON") {
		t.Errorf("expected ON confirmation, got %q", a.lastLine())
	}

	// Bare toggle again: on → off.
	if err := command.RoomDataHandler(context.Background(), &command.Context{Actor: a}); err != nil {
		t.Fatalf("RoomDataHandler: %v", err)
	}
	if a.ShowRoomData() {
		t.Error("toggle did not turn room data OFF")
	}

	// Explicit `roomdata on`.
	if err := command.RoomDataHandler(context.Background(), &command.Context{Actor: a, Args: []string{"on"}}); err != nil {
		t.Fatalf("RoomDataHandler on: %v", err)
	}
	if !a.ShowRoomData() {
		t.Error("explicit `roomdata on` did not enable")
	}
}

// AppendRoomData is the shared gate behind every arrival render
// (movement/recall/teleport/flee/login/reattach), not just look. These
// cover its gate directly so the on-entry paths can't regress.
func TestAppendRoomData_Gate(t *testing.T) {
	room := coordTestRoom()
	const base = "You arrive."

	cases := []struct {
		name      string
		admin     bool
		toggle    bool
		adminRole string
		wantBlock bool
	}{
		{"admin + toggle on", true, true, "admin", true},
		{"admin + toggle off", true, false, "admin", false},
		{"non-admin + toggle on", false, true, "admin", false},
		{"empty role falls back to default admin", true, true, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &roomDataActor{testActor: newTestActor(room), admin: tc.admin, showData: tc.toggle}
			got := command.AppendRoomData(base, room, a, tc.adminRole)
			hasBlock := strings.Contains(got, "room data")
			if hasBlock != tc.wantBlock {
				t.Errorf("AppendRoomData block=%v, want %v\n%s", hasBlock, tc.wantBlock, got)
			}
			if !strings.HasPrefix(got, base) {
				t.Errorf("AppendRoomData dropped base text: %q", got)
			}
		})
	}
}

func TestAppendRoomData_NilSafe(t *testing.T) {
	a := &roomDataActor{testActor: newTestActor(nil), admin: true, showData: true}
	if got := command.AppendRoomData("base", nil, a, "admin"); got != "base" {
		t.Errorf("nil room: got %q, want unchanged base", got)
	}
}

// An unplaced room reports "unplaced" rather than a coordinate.
func TestLook_RoomDataBlock_UnplacedRoom(t *testing.T) {
	room := coordTestRoom()
	room.Coord = nil // unplaced (room-coordinates §4.3)
	a := &roomDataActor{testActor: newTestActor(room), admin: true, showData: true}
	if err := command.LookHandler(context.Background(), &command.Context{Actor: a}); err != nil {
		t.Fatalf("LookHandler: %v", err)
	}
	if !strings.Contains(a.lastLine(), "unplaced") {
		t.Errorf("expected 'unplaced' coord label:\n%s", a.lastLine())
	}
}
