package command_test

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/gameclock"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Ultrasound end-to-end (visibility §3/§4.3): a runner with the `ultrasound`
// capability pierces total darkness in the visibility filter — echolocation is
// not sight, so it detects the physical body regardless of light. A plain viewer
// in the same pitch-black room is blind to the room's occupants.
//
// Note ultrasound feeds the VISIBILITY filter, not the light system: the sonar
// viewer's effective light is still Black (they can't read the room's prose),
// but they can detect and target occupants — the intended "spatial awareness,
// not vision" model.
func TestUltrasound_PiercesDarknessEndToEnd(t *testing.T) {
	f := newRenderFixture()
	f.room.Terrain = world.TerrainUnderground // pitch black at any hour
	res := newLightResolver(gameclock.PeriodDay)

	build := func(a *testActor) command.ResolveContext {
		return (&command.Context{Actor: a, Light: res, Items: f.store, Placement: f.place}).BuildResolveContext()
	}

	// Baseline: a plain viewer cannot see a room occupant in the dark.
	plain := newTestActor(f.room)
	rc := build(plain)
	if rc.CanSee == nil {
		t.Fatal("a dark room should build a visibility predicate")
	}
	if rc.CanSee("thug") {
		t.Error("a plain viewer must be blind to occupants in a pitch-black room")
	}

	// An ultrasound viewer (racial tag here; a cybereye grant is the other
	// source) pierces the darkness.
	sonar := newTestActor(f.room)
	sonar.tags = []string{command.UltrasoundFlag}
	rcU := build(sonar)
	if rcU.CanSee != nil && !rcU.CanSee("thug") {
		t.Error("an ultrasound viewer must detect occupants through darkness")
	}
}
