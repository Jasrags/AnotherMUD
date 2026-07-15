package transit

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// fakeBcast records every SendToRoom call for assertions.
type fakeBcast struct {
	mu   sync.Mutex
	msgs []bmsg
}

type bmsg struct {
	room world.RoomID
	text string
}

func (b *fakeBcast) SendToRoom(_ context.Context, room world.RoomID, text string, _ ...string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.msgs = append(b.msgs, bmsg{room, text})
}

func (b *fakeBcast) drain() []bmsg {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.msgs
	b.msgs = nil
	return out
}

// sawAt reports whether any drained message to room contains substr.
func sawAt(msgs []bmsg, room world.RoomID, substr string) bool {
	for _, m := range msgs {
		if m.room == room && strings.Contains(m.text, substr) {
			return true
		}
	}
	return false
}

const (
	rG   world.RoomID = "sr:g"
	rC   world.RoomID = "sr:c"
	rR   world.RoomID = "sr:r"
	rX   world.RoomID = "sr:x"
	rCar world.RoomID = "sr:car"
)

func fourFloorWorld() *world.World {
	w := world.New()
	w.AddRoom(&world.Room{ID: rG, Name: "Ground"})
	w.AddRoom(&world.Room{ID: rC, Name: "Commercial"})
	w.AddRoom(&world.Room{ID: rR, Name: "Residential"})
	w.AddRoom(&world.Room{ID: rX, Name: "Corporate"})
	w.AddRoom(&world.Room{ID: rCar, Name: "Car"})
	return w
}

// testLine is a fast line: 1 warn / 1 travel-per-hop / 0 dwell so a trip needs
// the fewest Steps to assert on. The landing->car doorway is a north directional
// door; the car->landing side is the "out" keyword.
func testLine() Line {
	return Line{
		ID: "sr:express", Name: "the elevator", Policy: PolicyOnDemand,
		Car: rCar, DoorDir: world.DirNorth, DoorName: "elevator door",
		DoorKeyID: "transit-control", OutKeyword: "out",
		TravelSteps: 1, DwellSteps: 0, WarnSteps: 1, DefaultStop: 0,
		SafeLanding: rG,
		Stops: []Stop{
			{Landing: rG, Label: "Ground Concourse", Code: "G"},
			{Landing: rC, Label: "Commercial Concourse", Code: "C"},
			{Landing: rR, Label: "Residential Enclave", Code: "R"},
			{Landing: rX, Label: "Corporate Suites", Code: "X"},
		},
	}
}

func newSvc(t *testing.T) (*Service, *fakeBcast, *world.World) {
	t.Helper()
	w := fourFloorWorld()
	b := &fakeBcast{}
	s := NewService(w, b)
	if err := s.AddLine(testLine()); err != nil {
		t.Fatalf("AddLine: %v", err)
	}
	return s, b, w
}

func stepN(s *Service, n int) {
	for i := 0; i < n; i++ {
		s.Step(context.Background(), uint64(i))
	}
}

func TestAddLine_SeedsDoorsOpenAtDefaultFloor(t *testing.T) {
	_, _, w := newSvc(t)
	// Ground is the seed floor: its elevator door is present and OPEN, so
	// walking north boards the car.
	d, ok := w.GetDoor(rG, world.DirNorth)
	if !ok {
		t.Fatal("expected an elevator door at Ground")
	}
	if d.Closed {
		t.Error("Ground doors should be open at the seed floor")
	}
	dst, err := w.Move(rG, world.DirNorth)
	if err != nil || dst.ID != rCar {
		t.Errorf("north from Ground -> %v (err %v), want car", dst, err)
	}
	// The car's alight keyword leads back to Ground.
	dst, err = w.MoveByKeyword(rCar, "out")
	if err != nil || dst.ID != rG {
		t.Errorf("out -> %v (err %v), want Ground", dst, err)
	}
	// A floor the car is NOT at has its door closed+locked: you can't board.
	d2, ok := w.GetDoor(rC, world.DirNorth)
	if !ok || !d2.Closed || !d2.Locked {
		t.Errorf("Commercial doors should be closed+locked while car is at Ground: %+v ok=%v", d2, ok)
	}
	if _, err := w.Move(rC, world.DirNorth); err == nil {
		t.Error("should not be able to board from a floor the car isn't at")
	}
}

func TestAddLine_RejectsUnknownRoom(t *testing.T) {
	w := fourFloorWorld()
	s := NewService(w, nil)
	l := testLine()
	l.Car = "sr:nope"
	if err := s.AddLine(l); err == nil {
		t.Fatal("expected error for unknown car room")
	}
}

func TestSummon_FromLanding_TravelsAndArrives(t *testing.T) {
	s, b, w := newSvc(t)
	// A rider at Corporate (stop 3) calls the car (which is at Ground).
	msg, handled := s.Press(context.Background(), rX, "", "p1")
	if !handled || !strings.Contains(msg, "call button") {
		t.Fatalf("summon msg=%q handled=%v", msg, handled)
	}
	b.drain()

	// Trip 0 -> 3: doors-closing (1) + transit (3 hops * 1) + arrive.
	// Step 1: idle -> doors-closing.
	stepN(s, 1)
	// Step 2: doors close -> in transit; the Ground door must be closed and the
	// car's alight keyword unbound (sealed in).
	stepN(s, 1)
	if d, _ := w.GetDoor(rG, world.DirNorth); !d.Closed {
		t.Error("Ground door must be closed while in transit")
	}
	if _, err := w.MoveByKeyword(rCar, "out"); err == nil {
		t.Error("car alight keyword must be unbound while in transit")
	}
	// Steps 3-5: cross Commercial, Residential, then arrive at Corporate.
	b.drain()
	stepN(s, 3)
	msgs := b.drain()
	if !sawAt(msgs, rCar, "passes Commercial Concourse") {
		t.Error("expected a passing-floor line for Commercial")
	}
	if !sawAt(msgs, rCar, "doors open on Corporate Suites") {
		t.Error("expected arrival chime inside car")
	}
	if !sawAt(msgs, rX, "doors open") {
		t.Error("expected arrival chime at the Corporate landing")
	}
	// Doors now open at Corporate; the alight keyword leads there.
	if d, ok := w.GetDoor(rX, world.DirNorth); !ok || d.Closed {
		t.Errorf("Corporate door should be open on arrival: %+v ok=%v", d, ok)
	}
	dst, err := w.MoveByKeyword(rCar, "out")
	if err != nil || dst.ID != rX {
		t.Errorf("alight keyword -> %v (err %v), want Corporate", dst, err)
	}
}

func TestPress_ByCodeUnambiguous(t *testing.T) {
	s, _, _ := newSvc(t)
	// "C" is the Commercial code — exact code beats the label substring, which
	// would otherwise be ambiguous with Corporate.
	msg, handled := s.Press(context.Background(), rCar, "C", "p1")
	if !handled || !strings.Contains(msg, "Commercial Concourse") {
		t.Fatalf("press C msg=%q handled=%v", msg, handled)
	}
	stepN(s, 4) // 0 -> 1: warn + travel + arrive
	if got := currentStop(s); got != 1 {
		t.Fatalf("car at stop %d, want 1 (Commercial via code C)", got)
	}
}

func TestPress_InsideCar_SelectsDestinationByLabel(t *testing.T) {
	s, b, _ := newSvc(t)
	b.drain()
	msg, handled := s.Press(context.Background(), rCar, "residential", "p1")
	if !handled || !strings.Contains(msg, "Residential Enclave") {
		t.Fatalf("press msg=%q handled=%v", msg, handled)
	}
	// Drive to arrival (0 -> 2): 1 warn + 2 travel + arrive.
	stepN(s, 5)
	if got := currentStop(s); got != 2 {
		t.Fatalf("car at stop %d, want 2 (Residential)", got)
	}
}

func TestPress_InsideCar_NumericFloor(t *testing.T) {
	s, _, _ := newSvc(t)
	// "4" is the 1-based Corporate Suites.
	if _, handled := s.Press(context.Background(), rCar, "4", "p1"); !handled {
		t.Fatal("numeric press not handled")
	}
	stepN(s, 6)
	if got := currentStop(s); got != 3 {
		t.Fatalf("car at stop %d, want 3", got)
	}
}

func TestPress_AlreadyHere_NoOp(t *testing.T) {
	s, _, _ := newSvc(t)
	// Car idle at Ground; a rider inside presses Ground.
	msg, handled := s.Press(context.Background(), rCar, "ground", "p1")
	if !handled || !strings.Contains(msg, "already open") {
		t.Fatalf("msg=%q handled=%v", msg, handled)
	}
	stepN(s, 3)
	if got := currentStop(s); got != 0 {
		t.Fatalf("car moved to %d; should have stayed at Ground", got)
	}
}

func TestPress_NonTransitRoom_NotHandled(t *testing.T) {
	s, _, _ := newSvc(t)
	if _, handled := s.Press(context.Background(), world.RoomID("sr:elsewhere"), "", "p1"); handled {
		t.Fatal("a non-transit room must report handled=false")
	}
}

// currentStop reaches into the service for the single test line's car stop.
func currentStop(s *Service) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cars["sr:express"].stop
}
