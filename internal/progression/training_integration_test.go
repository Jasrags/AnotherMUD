package progression

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
)

// trainsEntity is a minimal TrainingEntity that also implements
// TrainsCrediter so a single fixture serves both sides of the
// XP-to-train pipeline in this test. SpendTrain decrements the
// pool the same way the production connActor does.
type trainsEntity struct {
	sb       *StatBlock
	trains   int
	raceID   string
	roomTags map[string]bool
}

func (t *trainsEntity) StatBlock() *StatBlock                   { return t.sb }
func (t *trainsEntity) TrainsAvailable() int                    { return t.trains }
func (t *trainsEntity) RaceID() string                          { return t.raceID }
func (t *trainsEntity) HasRoomTag(tag string) bool              { return t.roomTags[tag] }
func (t *trainsEntity) SpendTrain() bool {
	if t.trains <= 0 {
		return false
	}
	t.trains--
	return true
}
func (t *trainsEntity) CreditTrains(_ context.Context, _ string, n int) {
	t.trains += n
}

// TestTrainingPipeline_XPToLevelToTrainToStat is the M8.6 ROADMAP
// integration acceptance criterion: grant XP → level up → trains
// credited (M8.4 stat-growth subscriber) → `train str` succeeds →
// base STR increases → effective STR matches.
func TestTrainingPipeline_XPToLevelToTrainToStat(t *testing.T) {
	tracks := NewTrackRegistry()
	if err := tracks.Register(&TrackDef{
		Name: "adventurer", MaxLevel: 5,
		XPTable: []int64{0, 0, 100, 300},
	}); err != nil {
		t.Fatal(err)
	}

	classes := NewClassRegistry()
	if err := classes.Register(&Class{
		ID: "fighter", BoundTrack: "adventurer",
		StatGrowth:     map[StatType]combat.DiceExpr{},
		TrainsPerLevel: 5,
	}); err != nil {
		t.Fatal(err)
	}

	races := NewRaceRegistry()
	if err := races.Register(&Race{
		ID: "human", StatCaps: map[StatType]int{StatSTR: 18},
	}); err != nil {
		t.Fatal(err)
	}

	entity := &trainsEntity{
		sb:     NewWithBase(map[StatType]int{StatSTR: 10}),
		raceID: "human",
	}
	state := NewProgressionState()
	const entityID = "p:alice"

	// Trampoline sink — replays the stat-growth subscriber on
	// every level-up so trains accumulate the way they would in
	// production via the bus.
	sink := levelUpTrampoline{
		onLevelUp: func(ctx context.Context, _, track string, _, _ int) {
			cls, _ := classes.Get("fighter")
			ApplyStatGrowth(ctx, cls, entity.sb, fixedRoller{}, entity, entityID)
		},
	}
	mgr := NewManager(tracks, sink)

	// Grant 100 XP — exactly hits the level 2 threshold. One
	// level-up fires; 5 trains are credited.
	res := mgr.GrantExperience(context.Background(), state, entityID, "adventurer", 100, "test")
	if res.NewLevel != 2 {
		t.Fatalf("NewLevel = %d, want 2", res.NewLevel)
	}
	if entity.trains != 5 {
		t.Fatalf("trains after level-up = %d, want 5", entity.trains)
	}

	// Train STR. Default training config has STR trainable, no
	// safe-room gate, race cap 18.
	tm := NewTrainingManager(DefaultTrainingConfig(), races, nil, nil)
	r := tm.TryTrain(context.Background(), entity, "str")
	if r.Outcome != TrainSuccess {
		t.Fatalf("TryTrain outcome = %v, msg=%q", r.Outcome, r.Message)
	}
	if r.NewBase != 11 || r.NewEffective != 11 {
		t.Errorf("base/eff = %d/%d, want 11/11", r.NewBase, r.NewEffective)
	}
	if entity.trains != 4 {
		t.Errorf("trains after train = %d, want 4", entity.trains)
	}
	// The StatBlock's cache was invalidated; a fresh Effective
	// read should observe the new base.
	if got := entity.sb.Effective(StatSTR); got != 11 {
		t.Errorf("Effective(STR) post-train = %d, want 11", got)
	}
}
