package session

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// newExperienceGmcpActor builds an actor wired to a real
// progression.Manager backed by a TrackRegistry with the supplied
// tracks pre-registered.
func newExperienceGmcpActor(t *testing.T, playerID string, tracks ...*progression.TrackDef) (*connActor, *gmcpFakeConn, *progression.Manager) {
	t.Helper()
	reg := progression.NewTrackRegistry()
	for _, td := range tracks {
		if err := reg.Register(td); err != nil {
			t.Fatalf("register track %q: %v", td.Name, err)
		}
	}
	mgr := progression.NewManager(reg, nil)

	fc := &gmcpFakeConn{fakeConn: fakeConn{id: "test-" + playerID}}
	room := &world.Room{ID: "test-room", Name: "Test"}
	a := &connActor{
		id:          fc.id,
		conn:        fc,
		playerID:    playerID,
		room:        room,
		vitals:      combat.NewVitalsAt(50, 100),
		save:        &player.Save{ID: playerID, Name: playerID, Sustenance: 100},
		statBlock:   progression.NewWithBase(progression.DefaultPlayerBase()),
		progress:    progression.NewProgressionState(),
		progression: mgr,
	}
	a.sustenance = 100
	return a, fc, mgr
}

func experienceFrames(t *testing.T, fc *gmcpFakeConn) []gmcp.CharExperience {
	t.Helper()
	raw := fc.framesSnapshot()
	out := make([]gmcp.CharExperience, 0, len(raw))
	for _, f := range raw {
		if f.pkg != gmcp.PackageCharExperience {
			continue
		}
		var e gmcp.CharExperience
		if err := json.Unmarshal(f.payload, &e); err != nil {
			t.Fatalf("payload unmarshal: %v (raw %s)", err, f.payload)
		}
		out = append(out, e)
	}
	return out
}

// linearTrack builds a TrackDef with a simple (level^2 * 10) XP
// formula and a configurable max level. Enough to exercise the
// xpnext + at_max paths.
func linearTrack(name string, maxLevel int) *progression.TrackDef {
	return &progression.TrackDef{
		Name:      name,
		MaxLevel:  maxLevel,
		XPFormula: func(l int) int64 { return int64(l*l) * 10 },
	}
}

func TestFlushGmcpExperience_NoSendBeforeActivation(t *testing.T) {
	a, fc, _ := newExperienceGmcpActor(t, "p-1", linearTrack("adventurer", 50))
	a.flushGmcpExperience(context.Background())
	if got := len(fc.framesSnapshot()); got != 0 {
		t.Errorf("pre-activation emitted %d frames, want 0", got)
	}
}

func TestFlushGmcpExperience_FirstFlushSendsBaseline(t *testing.T) {
	// Fresh actor: GetTrackInfo lazy-inits to level 1, so the
	// baseline frame carries one row per registered track.
	a, fc, _ := newExperienceGmcpActor(t, "p-1",
		linearTrack("adventurer", 50),
		linearTrack("crafting", 20),
	)
	fc.setActive(true)

	a.flushGmcpExperience(context.Background())
	frames := experienceFrames(t, fc)
	if len(frames) != 1 {
		t.Fatalf("first flush emitted %d frames, want 1", len(frames))
	}
	tr := frames[0].Tracks
	if len(tr) != 2 {
		t.Fatalf("baseline tracks = %d, want 2", len(tr))
	}
	// TrackRegistry.All sorts by name: "adventurer" < "crafting".
	if tr[0].Track != "adventurer" || tr[0].Level != 1 || tr[0].MaxLevel != 50 {
		t.Errorf("track[0] = %+v", tr[0])
	}
	if tr[1].Track != "crafting" || tr[1].Level != 1 || tr[1].MaxLevel != 20 {
		t.Errorf("track[1] = %+v", tr[1])
	}
}

func TestFlushGmcpExperience_NoRedundantSendsWhenUnchanged(t *testing.T) {
	a, fc, _ := newExperienceGmcpActor(t, "p-1", linearTrack("adventurer", 50))
	fc.setActive(true)

	a.flushGmcpExperience(context.Background())
	a.flushGmcpExperience(context.Background())
	a.flushGmcpExperience(context.Background())

	if got := len(experienceFrames(t, fc)); got != 1 {
		t.Errorf("idempotent flushes emitted %d frames, want 1", got)
	}
}

func TestFlushGmcpExperience_SendsOnXPGrant(t *testing.T) {
	a, fc, mgr := newExperienceGmcpActor(t, "p-1", linearTrack("adventurer", 50))
	fc.setActive(true)

	a.flushGmcpExperience(context.Background())

	// Grant 5 XP — not enough to level (level 2 threshold = 40).
	mgr.GrantExperience(context.Background(), a.progress, "p-1", "adventurer", 5, "test")

	a.flushGmcpExperience(context.Background())
	frames := experienceFrames(t, fc)
	if len(frames) != 2 {
		t.Fatalf("post-grant flushes emitted %d frames, want 2", len(frames))
	}
	got := frames[1].Tracks[0]
	if got.XP != 5 {
		t.Errorf("XP after grant = %d, want 5", got.XP)
	}
	if got.XPNext != 40-5 {
		t.Errorf("XPNext after grant = %d, want 35", got.XPNext)
	}
}

func TestFlushGmcpExperience_MaxLevelEmitsAtMax(t *testing.T) {
	// Construct an actor at max level and verify at_max + overflow
	// emit and xpnext stays zero.
	a, fc, _ := newExperienceGmcpActor(t, "p-1", linearTrack("adventurer", 3))
	fc.setActive(true)

	// Seed past-max state directly via Restore — bypassing the
	// grant cascade to isolate the at-max wire behavior.
	a.progress.Restore(progression.ProgressionSnapshot{
		{Name: "adventurer", Level: 3, XP: 200}, // threshold = 90, overflow = 110
	})

	a.flushGmcpExperience(context.Background())
	frames := experienceFrames(t, fc)
	if len(frames) != 1 {
		t.Fatalf("flush emitted %d frames, want 1", len(frames))
	}
	got := frames[0].Tracks[0]
	if !got.AtMax {
		t.Errorf("AtMax = false at max level: %+v", got)
	}
	if got.XPNext != 0 {
		t.Errorf("XPNext at max = %d, want 0", got.XPNext)
	}
	if got.Overflow != 200-90 {
		t.Errorf("Overflow = %d, want 110", got.Overflow)
	}
}

func TestFlushGmcpExperience_NoTracksRegisteredEmitsEmptyList(t *testing.T) {
	// A pack with no progression tracks still gets a baseline `[]`
	// frame so the panel can clear stale rows from a previous
	// character.
	a, fc, _ := newExperienceGmcpActor(t, "p-1")
	fc.setActive(true)

	a.flushGmcpExperience(context.Background())
	frames := experienceFrames(t, fc)
	if len(frames) != 1 || len(frames[0].Tracks) != 0 || frames[0].Tracks == nil {
		t.Errorf("empty-registry frames = %+v, want one frame with empty non-nil tracks", frames)
	}
}

func TestFlushGmcpExperience_DisplayNameSurfacesWhenDistinct(t *testing.T) {
	a, fc, _ := newExperienceGmcpActor(t, "p-1", &progression.TrackDef{
		Name:        "adventurer",
		DisplayName: "Adventurer",
		MaxLevel:    50,
		XPFormula:   func(l int) int64 { return int64(l) * 10 },
	})
	fc.setActive(true)

	a.flushGmcpExperience(context.Background())
	got := experienceFrames(t, fc)[0].Tracks[0]
	if got.Name != "Adventurer" {
		t.Errorf("Name = %q, want Adventurer", got.Name)
	}
}

func TestFlushGmcpExperience_DisplayNameOmittedWhenEqualToTrack(t *testing.T) {
	a, fc, _ := newExperienceGmcpActor(t, "p-1", &progression.TrackDef{
		Name:        "adventurer",
		DisplayName: "adventurer", // same as Name
		MaxLevel:    50,
		XPFormula:   func(l int) int64 { return int64(l) * 10 },
	})
	fc.setActive(true)

	a.flushGmcpExperience(context.Background())
	got := experienceFrames(t, fc)[0].Tracks[0]
	if got.Name != "" {
		t.Errorf("Name should omit when equal to Track, got %q", got.Name)
	}
}

func TestFlushGmcpExperience_NonGmcpConnIsSilentNoOp(t *testing.T) {
	reg := progression.NewTrackRegistry()
	reg.Register(linearTrack("adventurer", 50))
	mgr := progression.NewManager(reg, nil)
	room := &world.Room{ID: "test-room", Name: "Test"}
	a := &connActor{
		id:          "test-p-1",
		conn:        &fakeConn{id: "test-p-1"},
		playerID:    "p-1",
		room:        room,
		vitals:      combat.NewVitalsAt(50, 100),
		save:        &player.Save{ID: "p-1", Name: "p-1"},
		statBlock:   progression.NewWithBase(progression.DefaultPlayerBase()),
		progress:    progression.NewProgressionState(),
		progression: mgr,
	}
	a.flushGmcpExperience(context.Background()) // no panic, no send
}

func TestFlushGmcpExperience_NilManagerIsSilentNoOp(t *testing.T) {
	a, fc, _ := newExperienceGmcpActor(t, "p-1", linearTrack("adventurer", 50))
	a.progression = nil
	fc.setActive(true)
	a.flushGmcpExperience(context.Background())
	if got := len(fc.framesSnapshot()); got != 0 {
		t.Errorf("nil manager emitted %d frames, want 0", got)
	}
}

func TestManagerFlushGmcpExperience_FansOutToLiveActors(t *testing.T) {
	a, fc, _ := newExperienceGmcpActor(t, "p-1", linearTrack("adventurer", 50))
	fc.setActive(true)

	mgr := NewManager()
	mgr.Add(a)

	mgr.FlushGmcpExperience(context.Background())

	if got := len(experienceFrames(t, fc)); got != 1 {
		t.Errorf("manager flush emitted %d frames, want 1", got)
	}
}

func TestFlushGmcpExperience_ShadowResetForcesResend(t *testing.T) {
	a, fc, _ := newExperienceGmcpActor(t, "p-1", linearTrack("adventurer", 50))
	fc.setActive(true)

	a.flushGmcpExperience(context.Background())
	a.flushGmcpExperience(context.Background()) // idempotent

	a.resetGmcpExperienceShadow()
	a.flushGmcpExperience(context.Background())

	if got := len(experienceFrames(t, fc)); got != 2 {
		t.Errorf("after reset frames = %d, want 2", got)
	}
}
