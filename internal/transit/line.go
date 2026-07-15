package transit

import "github.com/Jasrags/AnotherMUD/internal/world"

// CallPolicy is how a line's request queue is fed (spec §4). v1 drives
// on-demand; scheduled is parsed and reserved for the subway (§4.2, §11).
type CallPolicy string

const (
	// PolicyOnDemand: a rider presses a call control to summon the car; the
	// car idles when the queue empties. The elevator.
	PolicyOnDemand CallPolicy = "on-demand"
	// PolicyScheduled: the queue is a timetable the car follows regardless of
	// riders. The subway — reserved for a later slice.
	PolicyScheduled CallPolicy = "scheduled"
)

// State is a car's motion state (spec §5). Exactly one at a time; the Service's
// per-tick Step advances it.
type State int

const (
	// StateIdle: parked at a stop, doors open, doorway bound. Awaiting a
	// request (and, after an arrival, serving out the dwell window).
	StateIdle State = iota
	// StateDoorsClosing: a request exists and dwell has elapsed; the closing
	// warning has fired and the doors are closing. A hold returns to Idle.
	StateDoorsClosing
	// StateInTransit: doors closed, doorway unbound, car moving toward its
	// target stop. Riders are sealed inside.
	StateInTransit
)

// String renders a State for logs.
func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateDoorsClosing:
		return "doors-closing"
	case StateInTransit:
		return "in-transit"
	default:
		return "unknown"
	}
}

// Stop is one position on a line (spec §2.2): a landing room a rider waits,
// boards, and alights at, plus a human label and a short button code.
type Stop struct {
	// Landing is the permanent room that is this stop's platform. It exists on
	// the normal world graph independent of car presence.
	Landing world.RoomID
	// Label is the rider-facing name of the stop ("Ground Concourse"). Used in
	// the press-a-floor UI and the chime/arrival copy.
	Label string
	// Code is the short button label a rider presses ("G", "C"). Matched
	// case-insensitively, ahead of a floor number or a label substring, so
	// `press C` is unambiguous where two labels share a first letter.
	Code string
}

// Line is the content definition of a conveyance (spec §2.1). Timing fields are
// in Step units — one Step is one invocation of Service.Step, i.e. one transit
// tick-handler firing (see the transit cadence in the composition root).
type Line struct {
	// ID is the stable, namespaced line id ("shadowrun:ache-express").
	ID string
	// Name is the rider-facing name of the conveyance.
	Name string
	// Policy is on-demand (v1) or scheduled (reserved).
	Policy CallPolicy
	// Stops is the ordered stop list; index distance is hop distance (§5.3).
	Stops []Stop
	// Car is the orphan room that is the car interior (§3.1).
	Car world.RoomID
	// DoorDir is the direction a rider walks at a landing to board — the
	// landing->car half is a real directional exit carrying the elevator door
	// (§3). The car->landing half is the OutKeyword (a dynamic keyword exit).
	DoorDir world.Direction
	// DoorName is the display name of the elevator door (the closed-door and
	// "doors open/close" copy), e.g. "elevator door".
	DoorName string
	// DoorKeyID is a key id no player carries, so the landing door — kept
	// closed+locked while the car is away — cannot be manually opened or
	// unlocked to walk into an absent car. The service unlocks it (raw, no key
	// check) only when the car is present.
	DoorKeyID string
	// OutKeyword is the keyword a rider types inside the car to alight (the
	// car->landing half), e.g. "out".
	OutKeyword string
	// TravelSteps is the time to cross one hop between adjacent stops (§5.3);
	// a farther stop costs proportionally more.
	TravelSteps uint64
	// DwellSteps is how long the doors stay open at a stop before the car will
	// depart (§6.1) — the boarding window.
	DwellSteps uint64
	// WarnSteps is the doors-closing warning lead before the doors seal (§5.1).
	WarnSteps uint64
	// DefaultStop is the stop index the car seeds at on boot (§10).
	DefaultStop int
	// SafeLanding is the never-strand deposit target (§6.2). Defaults to the
	// default stop's landing when a line omits it.
	//
	// CURRENTLY INERT: v1 satisfies never-strand via the boot reseed (every car
	// seeds IDLE with doors open, so a player who saved inside a car loads with a
	// way out — doc.go, spec §10). The active-deposit paths §6.2 enumerates
	// (shutdown, line reset/reload, a trip whose target became invalid) are not
	// built yet, so nothing reads this field. It is parsed and reserved for when
	// those hooks land — a pack author setting safe_landing today is a no-op.
	SafeLanding world.RoomID
}

// stopIndexByLanding returns the index of the stop whose landing is room, or -1.
func (l *Line) stopIndexByLanding(room world.RoomID) int {
	for i, s := range l.Stops {
		if s.Landing == room {
			return i
		}
	}
	return -1
}
