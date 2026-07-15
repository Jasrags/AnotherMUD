// Package transit implements conveyances that carry riders between a fixed,
// ordered set of stops faster than walking the room graph — the elevator (a
// short vertical line, one car, summoned on demand) and, by the same machine, a
// subway/monorail (a scheduled horizontal line). See docs/specs/transit.md.
//
// The model is three things: a Line (ordered Stops + a car + timing), a car
// (the runtime state the Service owns — current stop, motion state, and a
// request queue), and a doorway that binds the car interior to the current
// stop's landing. A rider rides *inside* the car: the car is a real room, and
// when it moves it re-points its doorway rather than relocating its occupants.
//
// The car doorway reuses the world's temporary keyword-exit primitive (the same
// retarget the portal service performs): on arrival at a stop both doorway
// halves bind to that stop's landing; while in transit both halves are unbound
// so no one can step into a shaft. The Service holds the *world.World and
// mutates keyword exits under it, preserving the portal service's lock order
// (transit.Service.mu -> world.World.mu).
//
// State is derived-not-persisted (like weather and temporary exits): at boot
// each line seeds its car at a default stop, IDLE, doors open. Because that seed
// always leaves an open door, a player who saved inside a car loads with a way
// out — the never-strand guarantee (spec §6.2, §10) without a save-version bump.
package transit
