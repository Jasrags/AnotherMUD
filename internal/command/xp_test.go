package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// xpActor satisfies both command.Actor and command.ProgressionHolder
// for the verb's narrow surface. Wraps a testActor for the Actor side
// — the verb only ever calls Write on the actor — and a fresh
// ProgressionState for the Holder side.
type xpActor struct {
	*testActor
	state *progression.ProgressionState
}

func newXPActor() *xpActor {
	return &xpActor{
		testActor: newTestActor(nil),
		state:     progression.NewProgressionState(),
	}
}

func (a *xpActor) lastLine() string {
	a.testActor.mu.Lock()
	defer a.testActor.mu.Unlock()
	if len(a.testActor.lines) == 0 {
		return ""
	}
	return a.testActor.lines[len(a.testActor.lines)-1]
}

func (a *xpActor) GrantXP(ctx context.Context, mgr *progression.Manager, track, source string, amount int64) progression.GrantResult {
	if mgr == nil {
		return progression.GrantResult{Track: track, TrackUnknown: true}
	}
	return mgr.GrantExperience(ctx, a.state, "test-actor", track, amount, source)
}

func (a *xpActor) DeductXP(ctx context.Context, mgr *progression.Manager, track string, amount int64) progression.DeductResult {
	if mgr == nil {
		return progression.DeductResult{Track: track, TrackUnknown: true}
	}
	return mgr.DeductExperience(ctx, a.state, "test-actor", track, amount)
}

func (a *xpActor) TrackInfo(mgr *progression.Manager, track string) (progression.TrackInfo, bool) {
	if mgr == nil {
		return progression.TrackInfo{}, false
	}
	return mgr.GetTrackInfo(a.state, track)
}

func makeXPManager(t *testing.T) *progression.Manager {
	t.Helper()
	r := progression.NewTrackRegistry()
	if err := r.Register(&progression.TrackDef{
		Name:        "adventurer",
		DisplayName: "Adventurer",
		MaxLevel:    5,
		XPTable:     []int64{0, 0, 100, 300, 600, 1000},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return progression.NewManager(r, nil)
}

func TestXP_GrantsToDefaultTrack(t *testing.T) {
	a := newXPActor()
	mgr := makeXPManager(t)
	ctx := &command.Context{
		Actor:       a,
		Progression: mgr,
		Verb:        "xp",
		Args:        []string{"50"},
	}
	if err := command.XPHandler(context.Background(), ctx); err != nil {
		t.Fatalf("XPHandler: %v", err)
	}
	if !strings.Contains(a.lastLine(), "50 XP on adventurer") {
		t.Errorf("write = %q, want 'gain 50 XP on adventurer'", a.lastLine())
	}
	if a.state.XP("adventurer") != 50 {
		t.Errorf("state XP = %d, want 50", a.state.XP("adventurer"))
	}
}

func TestXP_CascadeRendersLevelUp(t *testing.T) {
	a := newXPActor()
	mgr := makeXPManager(t)
	ctx := &command.Context{
		Actor:       a,
		Progression: mgr,
		Verb:        "xp",
		Args:        []string{"650"},
	}
	if err := command.XPHandler(context.Background(), ctx); err != nil {
		t.Fatalf("XPHandler: %v", err)
	}
	if !strings.Contains(a.lastLine(), "level 4") {
		t.Errorf("write = %q, want 'level 4'", a.lastLine())
	}
	if a.state.Level("adventurer") != 4 {
		t.Errorf("level = %d, want 4", a.state.Level("adventurer"))
	}
}

func TestXP_UnknownTrackReportsError(t *testing.T) {
	a := newXPActor()
	mgr := makeXPManager(t)
	ctx := &command.Context{
		Actor:       a,
		Progression: mgr,
		Verb:        "xp",
		Args:        []string{"100", "wizardry"},
	}
	_ = command.XPHandler(context.Background(), ctx)
	if !strings.Contains(a.lastLine(), "wizardry") {
		t.Errorf("write = %q, want diagnostic mentioning the unknown track", a.lastLine())
	}
	if a.state.XP("wizardry") != 0 {
		t.Errorf("state should not change for unknown track")
	}
}

func TestXP_BadAmountUsageLine(t *testing.T) {
	a := newXPActor()
	mgr := makeXPManager(t)
	ctx := &command.Context{
		Actor:       a,
		Progression: mgr,
		Verb:        "xp",
		Args:        []string{"notanumber"},
	}
	_ = command.XPHandler(context.Background(), ctx)
	if !strings.Contains(strings.ToLower(a.lastLine()), "positive integer") {
		t.Errorf("write = %q, want validation message", a.lastLine())
	}
}

func TestXP_NoArgsListsTracks(t *testing.T) {
	a := newXPActor()
	mgr := makeXPManager(t)
	ctx := &command.Context{
		Actor:       a,
		Progression: mgr,
		Verb:        "xp",
		Args:        nil,
	}
	_ = command.XPHandler(context.Background(), ctx)
	if !strings.Contains(a.lastLine(), "Adventurer") {
		t.Errorf("listing = %q, want display name 'Adventurer'", a.lastLine())
	}
}

func TestXP_NoProgressionMgrSafe(t *testing.T) {
	a := newXPActor()
	ctx := &command.Context{
		Actor:       a,
		Progression: nil,
		Verb:        "xp",
		Args:        []string{"100"},
	}
	if err := command.XPHandler(context.Background(), ctx); err != nil {
		t.Fatalf("XPHandler with nil Progression: %v", err)
	}
	if !strings.Contains(strings.ToLower(a.lastLine()), "not enabled") {
		t.Errorf("write = %q, want 'not enabled' diagnostic", a.lastLine())
	}
}
