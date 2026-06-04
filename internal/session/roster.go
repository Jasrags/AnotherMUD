package session

import (
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/command"
)

// WhoConfig is the small policy the `who` roster adapter applies (who §5).
// Hardcoded defaults today; externalizing the full §5 surface (columns,
// wording, ordering) is deferrable.
type WhoConfig struct {
	// IdleThreshold is how long without input before a character is marked
	// idle (who §2/§6). Zero disables the idle marker.
	IdleThreshold time.Duration
	// StaffRole is the role whose holders get a roster marker; StaffMarker
	// is the bracket-less text shown (e.g. role "admin" → "[Admin]").
	// Empty StaffRole disables role markers.
	StaffRole   string
	StaffMarker string
}

// DefaultWhoConfig returns the v1 policy: idle after 60s of no input, admins
// tagged "[Admin]".
func DefaultWhoConfig() WhoConfig {
	return WhoConfig{IdleThreshold: 60 * time.Second, StaffRole: "admin", StaffMarker: "Admin"}
}

// managerRoster adapts *Manager to command.Roster (the `who` seam). It
// snapshots the playing population once per call and maps each actor to a
// command.WhoEntry value, so the command layer never reads connActor state
// directly. Per-viewer visibility filtering attaches here when visibility
// rules land (who §4); v1 returns everyone.
type managerRoster struct {
	m   *Manager
	clk clock.Clock
	cfg WhoConfig
}

// NewWhoRoster builds the command.Roster the `who` verb reads. A nil clock
// falls back to the real clock so test/headless wiring can omit it.
func NewWhoRoster(m *Manager, clk clock.Clock, cfg WhoConfig) command.Roster {
	if clk == nil {
		clk = clock.RealClock{}
	}
	return managerRoster{m: m, clk: clk, cfg: cfg}
}

// OnlineRoster satisfies command.Roster. Lock discipline mirrors
// PlayersInRoom: snapshot actor pointers under the manager lock, release it,
// then read each actor's fields under that actor's own lock.
func (mr managerRoster) OnlineRoster() []command.WhoEntry {
	actors := mr.m.playingActors()
	now := mr.clk.Now()
	out := make([]command.WhoEntry, 0, len(actors))
	for _, a := range actors {
		name, idle, staff := a.whoFields(now, mr.cfg.IdleThreshold, mr.cfg.StaffRole)
		if name == "" {
			continue // mid-construction actor with no save yet — skip.
		}
		marker := ""
		if staff {
			marker = mr.cfg.StaffMarker
		}
		out = append(out, command.WhoEntry{Name: name, Idle: idle, RoleMarker: marker})
	}
	return out
}

// whoFields returns the actor's display name, idle flag, and staff flag for
// the `who` roster, read under a single actor-lock acquisition. Idle uses the
// same lastInputAt the idle-sweep reads (who §6). staffRole is checked via
// the lock-free hasRoleLocked so name/idle/role come from one consistent
// snapshot without re-locking.
func (a *connActor) whoFields(now time.Time, idleThreshold time.Duration, staffRole string) (name string, idle, staff bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save != nil {
		name = a.save.Name
	}
	idle = idleThreshold > 0 && now.Sub(a.lastInputAt) > idleThreshold
	staff = staffRole != "" && a.hasRoleLocked(staffRole)
	return
}
