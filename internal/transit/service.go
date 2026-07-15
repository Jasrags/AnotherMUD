package transit

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Broadcaster delivers a line of text to every occupant of a room, optionally
// excluding some player ids. session.Manager.SendToRoom satisfies it; tests
// supply a recorder. Defined here (at the point of use) per the small-interface
// convention.
type Broadcaster interface {
	SendToRoom(ctx context.Context, roomID world.RoomID, text string, exclude ...string)
}

// car is a line's runtime state (spec §3.1) — owned entirely by the Service.
// Not persisted; rebuilt at boot from the line's default stop (spec §10).
type car struct {
	line  *Line
	stop  int   // current stop index (origin while in transit)
	state State // motion state (spec §5)

	target int   // destination stop while moving
	queue  []int // requested stop indices, FIFO, deduped (spec §4)

	timer        uint64 // steps remaining in the current phase
	transitTotal uint64 // full transit duration, for passing-floor flavor
}

// outMsg is a deferred room broadcast, collected under the lock and flushed
// after it is released so the Broadcaster (session lock) is never called while
// holding s.mu — keeping the lock order transit.mu -> world.mu -> session.mu.
type outMsg struct {
	room    world.RoomID
	text    string
	exclude []string
}

// Service is the in-memory transit registry: it owns every line's car state,
// drives the state machine on each Step, and re-binds car doorways through the
// world's keyword-exit primitive. Safe for concurrent use.
//
// Lock order: transit.Service.mu -> world.World.mu (bindDoorway/unbindDoorway
// mutate keyword exits while holding s.mu). Broadcasts are deferred out of the
// locked region (see outMsg), so session.Manager.mu is only ever taken after
// s.mu is released.
type Service struct {
	mu    sync.Mutex
	world *world.World
	bcast Broadcaster

	cars        map[string]*car         // line id -> car
	landingLine map[world.RoomID]string // landing room -> line id
	carLine     map[world.RoomID]string // car room -> line id
}

// NewService returns a Service wired to w and bcast. bcast may be nil in tests
// that don't assert on broadcasts (Step/Press become silent).
func NewService(w *world.World, bcast Broadcaster) *Service {
	return &Service{
		world:       w,
		bcast:       bcast,
		cars:        make(map[string]*car),
		landingLine: make(map[world.RoomID]string),
		carLine:     make(map[world.RoomID]string),
	}
}

// AddLine registers l, validates its rooms exist, seeds its car at the default
// stop (IDLE, doors open, doorway bound), and indexes the room->line lookups.
// Returns an error if the car room or any landing is unknown to the world, or a
// keyword collides — fail fast at boot.
func (s *Service) AddLine(l Line) error {
	if _, err := s.world.Room(l.Car); err != nil {
		return fmt.Errorf("transit line %s: car room %s not in world: %w", l.ID, l.Car, err)
	}
	for i, st := range l.Stops {
		if _, err := s.world.Room(st.Landing); err != nil {
			return fmt.Errorf("transit line %s: stop %d landing %s not in world: %w", l.ID, i, st.Landing, err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, dup := s.cars[l.ID]; dup {
		return fmt.Errorf("transit line %s: already registered", l.ID)
	}
	// Fail fast on room collisions rather than silently clobbering a lookup.
	// A car room shared by two lines, or a landing served by two lines, is
	// rejected: v1 addresses press/call by a single room->line map, so a shared
	// room would misroute. Multi-line stations are a spec §11 open question; when
	// built, this becomes per-line addressing instead of an error.
	if other, taken := s.carLine[l.Car]; taken {
		return fmt.Errorf("transit line %s: car room %s already used by line %s", l.ID, l.Car, other)
	}
	for _, st := range l.Stops {
		if other, taken := s.landingLine[st.Landing]; taken {
			return fmt.Errorf("transit line %s: landing %s already served by line %s (one line per landing in v1; multi-line stations are transit.md §11)", l.ID, st.Landing, other)
		}
	}
	line := l // own a stable copy
	c := &car{line: &line, stop: line.DefaultStop, state: StateIdle}
	s.cars[line.ID] = c
	s.carLine[line.Car] = line.ID
	for _, st := range line.Stops {
		s.landingLine[st.Landing] = line.ID
	}
	// Build the landing-side doorway at every floor: a directional exit
	// (line.DoorDir) into the car, carrying the elevator door — created
	// closed+locked so a rider can neither walk nor hand-open it into an absent
	// car (§3). The service alone toggles it (below).
	for _, st := range line.Stops {
		if !s.world.SetExit(st.Landing, line.DoorDir, line.Car, s.newDoor(&line)) {
			return fmt.Errorf("transit line %s: could not build doorway at %s", l.ID, st.Landing)
		}
	}
	// Seed at the default stop: open that floor's doors + bind the car's alight
	// keyword there (§10 — IDLE, doors open).
	s.openFloor(c, c.stop)
	return nil
}

// newDoor mints the elevator door for a landing exit: closed + locked with a
// key id no rider holds, so it cannot be walked or hand-opened while the car is
// away (only the service's raw Unlock/Open toggles it).
func (s *Service) newDoor(l *Line) *world.DoorState {
	return &world.DoorState{
		Name:     l.DoorName,
		Keywords: doorKeywords(l.DoorName),
		Closed:   true,
		Locked:   true,
		KeyID:    l.DoorKeyID,
	}
}

// openFloor opens the doors at stop idx (the car has arrived / is parked there):
// unlock + open the landing door (reverse-sync opens any paired car-side door),
// and bind the car's alight keyword to that landing. Must be called under s.mu.
func (s *Service) openFloor(c *car, idx int) {
	landing := c.line.Stops[idx].Landing
	s.world.UnlockDoor(landing, c.line.DoorDir)
	s.world.OpenDoor(landing, c.line.DoorDir)
	s.world.RemoveKeywordExit(c.line.Car, c.line.OutKeyword)
	s.world.AddKeywordExit(c.line.Car, c.line.OutKeyword, landing)
}

// closeFloor closes the doors at stop idx (the car is leaving): close + lock the
// landing door and unbind the car's alight keyword, so no one boards or alights
// mid-transit. Must be called under s.mu.
func (s *Service) closeFloor(c *car, idx int) {
	landing := c.line.Stops[idx].Landing
	s.world.CloseDoor(landing, c.line.DoorDir)
	s.world.LockDoor(landing, c.line.DoorDir)
	s.world.RemoveKeywordExit(c.line.Car, c.line.OutKeyword)
}

// Press is the call-control entry (spec §4.1, §8). From inside a car, arg
// selects a destination stop; from a landing, it summons the car to that
// landing (arg ignored). Returns the actor-facing message and true when room is
// a transit room; false when it is not (the command then gives a generic reply).
func (s *Service) Press(ctx context.Context, room world.RoomID, arg, actorID string) (string, bool) {
	s.mu.Lock()
	msg, out, handled := s.pressLocked(room, arg, actorID)
	s.mu.Unlock()
	s.flush(ctx, out)
	return msg, handled
}

func (s *Service) pressLocked(room world.RoomID, arg, actorID string) (string, []outMsg, bool) {
	// Inside the car: pick a destination.
	if id, ok := s.carLine[room]; ok {
		c := s.cars[id]
		idx := resolveStopArg(c.line, arg)
		if idx < 0 {
			return "Press which floor? Try `press " + firstCode(c.line) + "`.", nil, true
		}
		st := c.line.Stops[idx]
		if idx == c.stop && c.state == StateIdle {
			return "The doors are already open on " + st.Label + ".", nil, true
		}
		s.enqueue(c, idx)
		out := []outMsg{{room: c.line.Car, text: "A floor button lights up.", exclude: []string{actorID}}}
		return "You press " + stopButton(st) + "for " + st.Label + ".", out, true
	}
	// At a landing: summon the car here.
	if id, ok := s.landingLine[room]; ok {
		c := s.cars[id]
		stopIdx := c.line.stopIndexByLanding(room)
		if stopIdx == c.stop && c.state == StateIdle {
			return "The " + shortName(c.line) + " is already here, doors open.", nil, true
		}
		s.enqueue(c, stopIdx)
		out := []outMsg{{room: room, text: "Someone presses the call button.", exclude: []string{actorID}}}
		return "You press the call button. " + capitalize(c.line.Name) + " is on its way.", out, true
	}
	return "", nil, false
}

// enqueue appends idx if not already queued and not the current open stop.
// Must be called under s.mu.
func (s *Service) enqueue(c *car, idx int) {
	if idx == c.stop && c.state == StateIdle {
		return
	}
	for _, q := range c.queue {
		if q == idx {
			return
		}
	}
	c.queue = append(c.queue, idx)
}

// Step advances every car one motion step (spec §5). Registered as the transit
// tick handler in the composition root. tick is the engine tick for logging.
func (s *Service) Step(ctx context.Context, tick uint64) {
	s.mu.Lock()
	var out []outMsg
	for _, c := range s.cars {
		s.stepCar(ctx, c, tick, &out)
	}
	s.mu.Unlock()
	s.flush(ctx, out)
}

// stepCar advances one car. Must be called under s.mu; broadcasts are appended
// to out and flushed by the caller after the lock is released.
func (s *Service) stepCar(ctx context.Context, c *car, tick uint64, out *[]outMsg) {
	switch c.state {
	case StateIdle:
		if c.timer > 0 {
			c.timer-- // dwell countdown
		}
		if len(c.queue) == 0 || c.timer > 0 {
			return
		}
		target := c.queue[0]
		if target == c.stop {
			c.dequeue(target) // already here; drop stale request
			return
		}
		c.target = target
		c.state = StateDoorsClosing
		c.timer = c.line.WarnSteps
		land := c.line.Stops[c.stop].Landing
		*out = append(*out,
			outMsg{room: c.line.Car, text: "The doors slide shut."},
			outMsg{room: land, text: "The " + shortName(c.line) + " doors slide shut."},
		)

	case StateDoorsClosing:
		if c.timer > 0 {
			c.timer--
		}
		if c.timer > 0 {
			return
		}
		// Seal and depart: close + lock the doors at the stop we are leaving.
		s.closeFloor(c, c.stop)
		up := c.target > c.stop
		hops := c.target - c.stop
		if hops < 0 {
			hops = -hops
		}
		c.state = StateInTransit
		c.timer = c.line.TravelSteps * uint64(hops)
		c.transitTotal = c.timer
		land := c.line.Stops[c.stop].Landing
		dir := "descends"
		if up {
			dir = "ascends"
		}
		*out = append(*out,
			outMsg{room: c.line.Car, text: "The car " + dir + ", the floor humming underfoot."},
			outMsg{room: land, text: "The doors close and the car " + dir + " away."},
		)
		logTransit(ctx, c, tick, "depart")

	case StateInTransit:
		if c.timer > 0 {
			c.timer--
		}
		s.announcePassing(c, out)
		if c.timer > 0 {
			return
		}
		// Arrive: open the doors at the new floor (and bind the alight keyword).
		c.stop = c.target
		c.dequeue(c.target)
		s.openFloor(c, c.stop)
		c.state = StateIdle
		c.timer = c.line.DwellSteps
		label := c.line.Stops[c.stop].Label
		*out = append(*out,
			outMsg{room: c.line.Car, text: "A soft chime. The doors open on " + label + "."},
			outMsg{room: c.line.Stops[c.stop].Landing, text: "A chime — the " + shortName(c.line) + " doors open."},
		)
		logTransit(ctx, c, tick, "arrive")
	}
}

// announcePassing emits a passing-floor line to the car interior each time the
// car crosses an intermediate stop mid-transit (spec §5.3 indicator update).
func (s *Service) announcePassing(c *car, out *[]outMsg) {
	if c.line.TravelSteps == 0 || c.transitTotal == 0 {
		return
	}
	elapsed := c.transitTotal - c.timer
	if elapsed == 0 || elapsed%c.line.TravelSteps != 0 || elapsed >= c.transitTotal {
		return
	}
	k := int(elapsed / c.line.TravelSteps)
	sign := 1
	if c.target < c.stop {
		sign = -1
	}
	inter := c.stop + sign*k
	if inter == c.target || inter < 0 || inter >= len(c.line.Stops) {
		return
	}
	*out = append(*out, outMsg{room: c.line.Car, text: "The car passes " + c.line.Stops[inter].Label + "."})
}

// flush delivers deferred broadcasts after s.mu is released.
func (s *Service) flush(ctx context.Context, out []outMsg) {
	if s.bcast == nil {
		return
	}
	for _, m := range out {
		s.bcast.SendToRoom(ctx, m.room, m.text, m.exclude...)
	}
}

// dequeue removes idx from the car's request queue.
func (c *car) dequeue(idx int) {
	for i, q := range c.queue {
		if q == idx {
			c.queue = append(c.queue[:i], c.queue[i+1:]...)
			return
		}
	}
}

func logTransit(ctx context.Context, c *car, tick uint64, event string) {
	logging.From(ctx).Debug("transit "+event,
		slog.String("event", "transit."+event),
		slog.String("line", c.line.ID),
		slog.String("car", string(c.line.Car)),
		slog.String("stop", c.line.Stops[c.stop].Label),
		slog.Uint64("tick", tick),
	)
}

// resolveStopArg matches arg against a line's stops: a bare 1-based number, or a
// case-insensitive substring of a stop label. Returns the stop index, or -1.
// resolveStopArg matches a rider's `press` argument to a stop index: an exact
// button code first ("C" for Commercial, unambiguous where two labels share a
// first letter), then a 1-based floor number, then a case-insensitive label
// substring. Returns the stop index, or -1.
func resolveStopArg(l *Line, arg string) int {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return -1
	}
	for i, st := range l.Stops {
		if st.Code != "" && strings.EqualFold(st.Code, arg) {
			return i
		}
	}
	if n, err := strconv.Atoi(arg); err == nil {
		if n >= 1 && n <= len(l.Stops) {
			return n - 1
		}
		return -1
	}
	low := strings.ToLower(arg)
	for i, st := range l.Stops {
		if strings.Contains(strings.ToLower(st.Label), low) {
			return i
		}
	}
	return -1
}

// firstCode returns a sample button code for the "press which floor?" hint.
func firstCode(l *Line) string {
	if len(l.Stops) == 0 {
		return "G"
	}
	if c := l.Stops[0].Code; c != "" {
		return c
	}
	return "1"
}

// stopButton renders a stop's code in brackets when it has one ("[C] ").
func stopButton(st Stop) string {
	if st.Code == "" {
		return ""
	}
	return "[" + st.Code + "] "
}

// shortName strips a leading article from a line name for mid-sentence use.
func shortName(l *Line) string {
	n := l.Name
	for _, art := range []string{"the ", "The ", "a ", "A ", "an ", "An "} {
		if strings.HasPrefix(n, art) {
			return n[len(art):]
		}
	}
	return n
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// doorKeywords derives the door's match tokens from its name (mirrors the
// content loader's derive-when-absent behavior), so the door resolves by name
// for any query path even though riders can't operate it.
func doorKeywords(name string) []string {
	fields := strings.Fields(strings.ToLower(name))
	if len(fields) == 0 {
		return nil
	}
	return fields
}
