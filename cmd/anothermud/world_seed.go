package main

import "github.com/Jasrags/AnotherMUD/internal/world"

// startingRoom is the room id players enter on connect. Hardcoded in
// M1; replaced by login-driven location in M3.
const startingRoom world.RoomID = "town:square"

// seedWorld returns the hardcoded two-room M1 world. M2 replaces this
// with the pack loader; until then, the world is whatever this
// function builds at boot.
func seedWorld() *world.World {
	w := world.New()

	square := &world.Room{
		ID:   "town:square",
		Name: "Town Square",
		Description: "Worn cobblestones spread out beneath your feet, ringed by " +
			"low buildings whose painted signs have faded to suggestions. " +
			"A dusty road leads north toward the gate.",
	}
	gate := &world.Room{
		ID:   "town:gate",
		Name: "The North Gate",
		Description: "Iron-bound timbers loom overhead, the gate itself standing " +
			"open. The square lies south, back the way you came; beyond the " +
			"gate the road continues into open country.",
	}

	square.Exits = map[world.Direction]world.Exit{
		world.DirNorth: {Target: gate.ID},
	}
	gate.Exits = map[world.Direction]world.Exit{
		world.DirSouth: {Target: square.ID},
	}

	w.AddRoom(square)
	w.AddRoom(gate)
	return w
}
