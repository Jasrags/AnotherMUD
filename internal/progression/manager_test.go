package progression_test

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// recordingSink captures every event for assertions.
type recordingSink struct {
	mu      sync.Mutex
	gained  []gainedEvt
	leveled []levelEvt
	lost    []lostEvt
	resets  []resetEvt
}

type gainedEvt struct {
	entity, track string
	amount, total int64
	source        string
}
type levelEvt struct {
	entity, track  string
	oldLvl, newLvl int
}
type lostEvt struct {
	entity, track string
	amount, total int64
}
type resetEvt struct{ entity, track string }

func (s *recordingSink) OnXPGained(_ context.Context, e, t string, a, n int64, src string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gained = append(s.gained, gainedEvt{e, t, a, n, src})
}
func (s *recordingSink) OnLevelUp(_ context.Context, e, t string, o, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.leveled = append(s.leveled, levelEvt{e, t, o, n})
}
func (s *recordingSink) OnXPLost(_ context.Context, e, t string, a, n int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lost = append(s.lost, lostEvt{e, t, a, n})
}
func (s *recordingSink) OnTrackReset(_ context.Context, e, t string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resets = append(s.resets, resetEvt{e, t})
}

func makeRegistry(t *testing.T) *progression.TrackRegistry {
	t.Helper()
	r := progression.NewTrackRegistry()
	// fighter: 5 levels at 100/300/600/1000 from level 1.
	err := r.Register(&progression.TrackDef{
		Name:     "fighter",
		MaxLevel: 5,
		XPTable:  []int64{0, 0, 100, 300, 600, 1000},
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	return r
}

func TestGrantExperience_LazyInitAndEmit(t *testing.T) {
	r := makeRegistry(t)
	sink := &recordingSink{}
	m := progression.NewManager(r, sink)
	state := progression.NewProgressionState()

	res := m.GrantExperience(context.Background(), state, "player:p1", "fighter", 50, "kill:wolf")
	if res.TrackUnknown {
		t.Fatal("TrackUnknown true for known track")
	}
	if res.OldLevel != 1 || res.NewLevel != 1 {
		t.Errorf("levels = (%d → %d), want (1 → 1)", res.OldLevel, res.NewLevel)
	}
	if res.NewXP != 50 {
		t.Errorf("NewXP = %d, want 50", res.NewXP)
	}
	if len(sink.gained) != 1 {
		t.Fatalf("gained events = %d, want 1", len(sink.gained))
	}
	if sink.gained[0].amount != 50 || sink.gained[0].source != "kill:wolf" {
		t.Errorf("gained[0] = %+v", sink.gained[0])
	}
	if len(sink.leveled) != 0 {
		t.Errorf("leveled events = %d, want 0", len(sink.leveled))
	}
	if state.Level("fighter") != 1 || state.XP("fighter") != 50 {
		t.Errorf("state = (%d, %d), want (1, 50)", state.Level("fighter"), state.XP("fighter"))
	}
}

func TestGrantExperience_CascadesMultipleLevels(t *testing.T) {
	r := makeRegistry(t)
	sink := &recordingSink{}
	m := progression.NewManager(r, sink)
	state := progression.NewProgressionState()

	// Grant enough to leap from level 1 straight past 100 (→2), past
	// 300 (→3), past 600 (→4) in one call. 650 XP crosses three
	// thresholds.
	res := m.GrantExperience(context.Background(), state, "e1", "fighter", 650, "boss")
	if res.OldLevel != 1 || res.NewLevel != 4 {
		t.Errorf("levels = (%d → %d), want (1 → 4)", res.OldLevel, res.NewLevel)
	}
	if len(sink.leveled) != 3 {
		t.Fatalf("level-up events = %d, want 3", len(sink.leveled))
	}
	wantLevels := []int{2, 3, 4}
	for i, le := range sink.leveled {
		if le.newLvl != wantLevels[i] {
			t.Errorf("leveled[%d].newLvl = %d, want %d", i, le.newLvl, wantLevels[i])
		}
	}
}

func TestGrantExperience_StopsAtMaxLevel(t *testing.T) {
	r := makeRegistry(t)
	m := progression.NewManager(r, nil)
	state := progression.NewProgressionState()

	// Massive grant — way past max. Should cap at MaxLevel=5 and
	// keep the overflow in XP.
	res := m.GrantExperience(context.Background(), state, "e1", "fighter", 99999, "test")
	if res.NewLevel != 5 {
		t.Errorf("NewLevel = %d, want 5 (max)", res.NewLevel)
	}
	if res.NewXP != 99999 {
		t.Errorf("NewXP = %d, want 99999 (overflow not clamped)", res.NewXP)
	}

	info, _ := m.GetTrackInfo(state, "fighter")
	if info.Overflow != 99999-1000 {
		t.Errorf("Overflow = %d, want %d", info.Overflow, 99999-1000)
	}
	if info.XpToNext != 0 {
		t.Errorf("XpToNext at max = %d, want 0", info.XpToNext)
	}
}

func TestGrantExperience_UnknownTrackIsNoOp(t *testing.T) {
	r := makeRegistry(t)
	sink := &recordingSink{}
	m := progression.NewManager(r, sink)
	state := progression.NewProgressionState()

	res := m.GrantExperience(context.Background(), state, "e1", "wizardry", 100, "src")
	if !res.TrackUnknown {
		t.Error("TrackUnknown = false for unregistered track")
	}
	if len(sink.gained) != 0 {
		t.Errorf("gained events for unknown track = %d, want 0", len(sink.gained))
	}
}

func TestGrantExperience_NonPositiveAmountIsNoOp(t *testing.T) {
	r := makeRegistry(t)
	sink := &recordingSink{}
	m := progression.NewManager(r, sink)
	state := progression.NewProgressionState()

	res := m.GrantExperience(context.Background(), state, "e1", "fighter", 0, "src")
	if res.XPAdded != 0 || len(sink.gained) != 0 {
		t.Errorf("zero-amount grant emitted: %+v / gained=%d", res, len(sink.gained))
	}

	res = m.GrantExperience(context.Background(), state, "e1", "fighter", -10, "src")
	if res.XPAdded != 0 || len(sink.gained) != 0 {
		t.Errorf("negative-amount grant emitted: %+v / gained=%d", res, len(sink.gained))
	}
	// Lazy init still ran.
	if state.Level("fighter") != 1 {
		t.Errorf("level after no-op grant = %d, want 1 (lazy init)", state.Level("fighter"))
	}
}

func TestDeductExperience_FloorsAtCurrentLevelThreshold(t *testing.T) {
	r := makeRegistry(t)
	sink := &recordingSink{}
	m := progression.NewManager(r, sink)
	state := progression.NewProgressionState()

	// Level up to 3 (threshold 300).
	m.GrantExperience(context.Background(), state, "e1", "fighter", 400, "")
	if state.Level("fighter") != 3 {
		t.Fatalf("setup: level = %d, want 3", state.Level("fighter"))
	}

	// Deduct a huge amount — should floor at threshold(3) = 300, not
	// go below.
	res := m.DeductExperience(context.Background(), state, "e1", "fighter", 9999)
	if res.NewXP != 300 {
		t.Errorf("NewXP after over-deduct = %d, want 300 (floor)", res.NewXP)
	}
	if res.XPLost != 100 {
		t.Errorf("XPLost = %d, want 100 (only the above-floor delta)", res.XPLost)
	}
	if res.Level != 3 {
		t.Errorf("Level after deduct = %d, want 3 (no de-level)", res.Level)
	}
	if len(sink.lost) != 1 {
		t.Errorf("lost events = %d, want 1", len(sink.lost))
	}
}

func TestDeductExperience_ZeroLossEmitsNothing(t *testing.T) {
	r := makeRegistry(t)
	sink := &recordingSink{}
	m := progression.NewManager(r, sink)
	state := progression.NewProgressionState()

	// Player is at level 1, xp 0. Floor = 0. Deduct can't lose
	// anything.
	res := m.DeductExperience(context.Background(), state, "e1", "fighter", 50)
	if res.XPLost != 0 {
		t.Errorf("XPLost = %d, want 0 (already at floor)", res.XPLost)
	}
	if len(sink.lost) != 0 {
		t.Errorf("lost events on zero-loss = %d, want 0", len(sink.lost))
	}
}

func TestGetTrackInfo_StructuredView(t *testing.T) {
	r := makeRegistry(t)
	m := progression.NewManager(r, nil)
	state := progression.NewProgressionState()

	m.GrantExperience(context.Background(), state, "e1", "fighter", 200, "")
	info, ok := m.GetTrackInfo(state, "fighter")
	if !ok {
		t.Fatal("GetTrackInfo returned ok=false")
	}
	want := progression.TrackInfo{
		Track:                 "fighter",
		Level:                 2,
		XP:                    200,
		XpToNext:              100, // 300 - 200
		CurrentLevelThreshold: 100,
		MaxLevel:              5,
		Overflow:              0,
	}
	if !reflect.DeepEqual(info, want) {
		t.Errorf("info = %+v\nwant %+v", info, want)
	}
}

func TestGetTrackInfo_UnknownTrack(t *testing.T) {
	r := makeRegistry(t)
	m := progression.NewManager(r, nil)
	state := progression.NewProgressionState()

	_, ok := m.GetTrackInfo(state, "wizardry")
	if ok {
		t.Error("GetTrackInfo for unknown track returned ok=true")
	}
}

func TestResetTrack(t *testing.T) {
	r := makeRegistry(t)
	sink := &recordingSink{}
	m := progression.NewManager(r, sink)
	state := progression.NewProgressionState()

	m.GrantExperience(context.Background(), state, "e1", "fighter", 700, "")
	if state.Level("fighter") < 2 {
		t.Fatalf("setup: did not level up; level = %d", state.Level("fighter"))
	}
	preLeveled := len(sink.leveled)

	m.ResetTrack(context.Background(), state, "e1", "fighter")
	if state.Level("fighter") != 1 {
		t.Errorf("level after reset = %d, want 1", state.Level("fighter"))
	}
	if state.XP("fighter") != 0 {
		t.Errorf("xp after reset = %d, want 0", state.XP("fighter"))
	}
	if len(sink.leveled) != preLeveled {
		t.Errorf("reset triggered level-up events: pre=%d, post=%d", preLeveled, len(sink.leveled))
	}
	if len(sink.resets) != 1 {
		t.Errorf("reset events = %d, want 1", len(sink.resets))
	}
}

func TestOnLevelUpCallbackFiresPerStep(t *testing.T) {
	r := progression.NewTrackRegistry()
	var fired []int
	_ = r.Register(&progression.TrackDef{
		Name:     "achievement",
		MaxLevel: 3,
		XPTable:  []int64{0, 0, 10, 20},
		OnLevelUp: func(_ string, _ string, newLevel int) {
			fired = append(fired, newLevel)
		},
	})
	m := progression.NewManager(r, nil)
	state := progression.NewProgressionState()

	// Grant 50: should cascade through level 2 and 3.
	m.GrantExperience(context.Background(), state, "e1", "achievement", 50, "")
	if !reflect.DeepEqual(fired, []int{2, 3}) {
		t.Errorf("OnLevelUp fired with %v, want [2 3]", fired)
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	state := progression.NewProgressionState()
	r := makeRegistry(t)
	m := progression.NewManager(r, nil)
	m.GrantExperience(context.Background(), state, "e1", "fighter", 250, "")

	snap := state.Snapshot()
	if len(snap) != 1 || snap[0].Name != "fighter" || snap[0].Level != 2 || snap[0].XP != 250 {
		t.Fatalf("Snapshot = %+v", snap)
	}

	restored := progression.NewProgressionState()
	restored.Restore(snap)
	if restored.Level("fighter") != 2 {
		t.Errorf("restored level = %d, want 2", restored.Level("fighter"))
	}
	if restored.XP("fighter") != 250 {
		t.Errorf("restored xp = %d, want 250", restored.XP("fighter"))
	}
}

func TestSnapshotEmpty(t *testing.T) {
	state := progression.NewProgressionState()
	if snap := state.Snapshot(); snap != nil {
		t.Errorf("empty Snapshot = %+v, want nil", snap)
	}
}

func TestUndefinedTrackBeyondTableStopsCascade(t *testing.T) {
	// A table that stops short of MaxLevel: thresholds beyond
	// table[len-1] return -1, halting the cascade per spec §5.1.
	r := progression.NewTrackRegistry()
	_ = r.Register(&progression.TrackDef{
		Name:     "stub",
		MaxLevel: 10,
		// Only levels 1-3 defined; levels 4+ have undefined thresholds.
		XPTable: []int64{0, 0, 50, 100},
	})
	m := progression.NewManager(r, nil)
	state := progression.NewProgressionState()

	res := m.GrantExperience(context.Background(), state, "e1", "stub", 9999, "")
	if res.NewLevel != 3 {
		t.Errorf("NewLevel = %d, want 3 (cascade stops at undefined threshold)", res.NewLevel)
	}
}

func TestConcurrentGrants(t *testing.T) {
	r := makeRegistry(t)
	m := progression.NewManager(r, nil)
	state := progression.NewProgressionState()

	const goroutines = 16
	const grantsEach = 50
	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			for range grantsEach {
				m.GrantExperience(context.Background(), state, "e1", "fighter", 1, "stress")
			}
		})
	}
	wg.Wait()

	want := int64(goroutines * grantsEach)
	if got := state.XP("fighter"); got != want {
		t.Errorf("concurrent grants total = %d, want %d", got, want)
	}
}
