package progression

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
)

// TestLevelUpCascade_FiresPathAndGrowthAcrossThresholds is the
// M8.4 ROADMAP integration acceptance criterion: grant enough XP
// to cross 2-3 thresholds on the class's bound track and assert
// both subscribers fired the expected number of times.
//
// We don't have the bus wired in this test (that lives at the
// composition root); instead we drive the Manager directly with a
// trampoline EventSink that re-invokes the class subscribers
// inline. That mirrors what cmd/anothermud does on every level.up
// event.
func TestLevelUpCascade_FiresPathAndGrowthAcrossThresholds(t *testing.T) {
	// Track: 1d10 thresholds — simple incremental ladder.
	tracks := NewTrackRegistry()
	if err := tracks.Register(&TrackDef{
		Name:     "adventurer",
		MaxLevel: 5,
		// index 0 unused; level 1 threshold 0, level 2 = 100, ...
		XPTable: []int64{0, 0, 100, 300, 600, 1000},
	}); err != nil {
		t.Fatal(err)
	}

	classes := NewClassRegistry()
	if err := classes.Register(&Class{
		ID:         "fighter",
		BoundTrack: "adventurer",
		StatGrowth: map[StatType]combat.DiceExpr{
			StatHPMax: {Count: 1, Sides: 1}, // deterministic +1 per level (1d1)
		},
		// CON bonus contributes (14-10)/2 = +2 per level
		GrowthBonuses:  map[StatType]StatType{StatHPMax: StatCON},
		TrainsPerLevel: 5,
		Path: []ClassPathEntry{
			{Level: 1, AbilityID: "basic-strike"},
			{Level: 2, AbilityID: "power-attack"},
			{Level: 3, AbilityID: "cleave"},
			{Level: 4, AbilityID: "locked", UnlockedVia: "quest:vigil"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	sb := NewWithBase(map[StatType]int{StatHPMax: 20, StatCON: 14})
	state := NewProgressionState()
	granter := &captureGranter{known: map[string]string{
		"basic-strike": "Basic Strike",
		"power-attack": "Power Attack",
		"cleave":       "Cleave",
	}}
	notify := &captureNotifier{}
	trains := &captureTrains{}
	processor := ClassPathProcessor{Classes: classes, Granter: granter, Notifier: notify}

	// Trampoline sink: on every LevelUp, run BOTH subscribers
	// against the live state. Mirrors the cmd/anothermud bus wiring.
	const entityID = "p:alice"
	const classID = "fighter"
	sink := levelUpTrampoline{
		onLevelUp: func(ctx context.Context, _, track string, _, newLevel int) {
			processor.Apply(ctx, entityID, classID, track, newLevel)
			cls, _ := classes.Get(classID)
			ApplyStatGrowth(ctx, cls, sb, fixedRoller{}, trains, entityID)
		},
	}

	// Grant character-created (treat as level 1, no track gate).
	processor.Apply(context.Background(), entityID, classID, "", 1)

	mgr := NewManager(tracks, sink)

	// Grant 700 XP — crosses thresholds 2 (100), 3 (300), 4 (600);
	// total = 700 lands inside level 4 (next is 1000).
	res := mgr.GrantExperience(context.Background(), state, entityID, "adventurer", 700, "test")
	if res.NewLevel != 4 {
		t.Fatalf("NewLevel = %d, want 4", res.NewLevel)
	}

	// Path processor expectations:
	//  - char-created fired level 1: basic-strike
	//  - level-up 2: power-attack
	//  - level-up 3: cleave
	//  - level-up 4: locked entry is unlocked_via → skipped, no call
	wantCalls := []string{"basic-strike", "power-attack", "cleave"}
	if !equalStrings(granter.calls, wantCalls) {
		t.Errorf("path grants = %v, want %v", granter.calls, wantCalls)
	}
	if len(notify.msgs) != 3 {
		t.Errorf("notifications = %d (%v), want 3", len(notify.msgs), notify.msgs)
	}

	// Stat growth expectations:
	//  3 level-ups (1→2, 2→3, 3→4) each rolled 1d1 (=1) + CON bonus (+2) = +3 hp_max
	//  Total hp_max = 20 + 3*3 = 29
	if base := sb.Base(StatHPMax); base != 29 {
		t.Errorf("hp_max base = %d, want 29 (20 + 3 levels * (1 + 2 con bonus))", base)
	}
	if trains.total != 15 {
		t.Errorf("trains credited = %d, want 15 (3 levels * 5)", trains.total)
	}
}

// levelUpTrampoline is an EventSink that invokes a callback on
// every LevelUp; other events are dropped (this is a tight integration
// test fixture, not a general sink).
type levelUpTrampoline struct {
	onLevelUp func(ctx context.Context, entityID, track string, oldLevel, newLevel int)
}

func (t levelUpTrampoline) OnXPGained(context.Context, string, string, int64, int64, string) {
}
func (t levelUpTrampoline) OnLevelUp(ctx context.Context, entityID, track string, oldLevel, newLevel int) {
	if t.onLevelUp != nil {
		t.onLevelUp(ctx, entityID, track, oldLevel, newLevel)
	}
}
func (t levelUpTrampoline) OnXPLost(context.Context, string, string, int64, int64) {}
func (t levelUpTrampoline) OnTrackReset(context.Context, string, string)           {}
