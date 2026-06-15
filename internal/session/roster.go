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
		name, idle, staff, adminInvis := a.whoFields(now, mr.cfg.IdleThreshold, mr.cfg.StaffRole)
		if name == "" {
			continue // mid-construction actor with no save yet — skip.
		}
		marker := ""
		if staff {
			marker = mr.cfg.StaffMarker
		}
		// Per-viewer admin-invis filtering happens in the command layer
		// (who §4); the roster only snapshots the flag + id as values, so
		// connActor state never crosses the seam. The flag comes from the
		// same whoFields lock as name/idle, so the snapshot can't tear.
		out = append(out, command.WhoEntry{
			Name:           name,
			Idle:           idle,
			RoleMarker:     marker,
			PlayerID:       a.PlayerID(),
			AdminInvisible: adminInvis,
		})
	}
	return out
}

// whoFields returns the actor's display name, idle flag, staff flag, and
// admin-invisibility for the `who` roster, read under a single actor-lock
// acquisition so the snapshot is internally consistent (no tear between, say,
// the name and the admin-invis flag, visibility §3.4). Idle uses the same
// lastInputAt the idle-sweep reads (who §6); staffRole is checked via the
// lock-free hasRoleLocked.
func (a *connActor) whoFields(now time.Time, idleThreshold time.Duration, staffRole string) (name string, idle, staff, adminInvis bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save != nil {
		name = a.save.Name
	}
	idle = idleThreshold > 0 && now.Sub(a.lastInputAt) > idleThreshold
	staff = staffRole != "" && a.hasRoleLocked(staffRole)
	adminInvis = a.adminInvisible
	return
}
