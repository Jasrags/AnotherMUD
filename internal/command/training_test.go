package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// trainActor satisfies both command.Actor (via testActor) and the
// TrainingActor surface. trainsAvailable and the stat block are
// independently controllable so the verb-side tests can exercise
// the structured-result branches without rebuilding state.
type trainActor struct {
	*testActor
	sb     *progression.StatBlock
	trains int
	race   string
	safe   bool
}

func newTrainActor() *trainActor {
	return &trainActor{
		testActor: newTestActor(nil),
		sb:        progression.NewWithBase(map[progression.StatType]int{progression.StatSTR: 10}),
		race:      "human",
	}
}

func (a *trainActor) StatBlock() *progression.StatBlock { return a.sb }
func (a *trainActor) TrainsAvailable() int              { return a.trains }
func (a *trainActor) SpendTrain() bool {
	if a.trains <= 0 {
		return false
	}
	a.trains--
	return true
}
func (a *trainActor) RaceID() string             { return a.race }
func (a *trainActor) HasRoomTag(tag string) bool { return tag == "safe" && a.safe }

func (a *trainActor) lastLine() string {
	a.testActor.mu.Lock()
	defer a.testActor.mu.Unlock()
	if len(a.testActor.lines) == 0 {
		return ""
	}
	return a.testActor.lines[len(a.testActor.lines)-1]
}

func newTrainingMgr(t *testing.T, trainerCfg *progression.TrainerConfig, trainerName string) (*progression.TrainingManager, *progression.RaceRegistry) {
	t.Helper()
	races := progression.NewRaceRegistry()
	if err := races.Register(&progression.Race{
		ID: "human", StatCaps: map[progression.StatType]int{progression.StatSTR: 18},
	}); err != nil {
		t.Fatal(err)
	}
	var ts progression.TrainerSource
	if trainerCfg != nil {
		ts = fixedTrainerSource{cfg: trainerCfg, name: trainerName, ok: true}
	} else {
		ts = fixedTrainerSource{ok: false}
	}
	return progression.NewTrainingManager(progression.DefaultTrainingConfig(), races, ts, nil), races
}

type fixedTrainerSource struct {
	cfg  *progression.TrainerConfig
	name string
	ok   bool
}

func (f fixedTrainerSource) TrainerInRoom(string, string) (*progression.TrainerConfig, string, bool) {
	return f.cfg, f.name, f.ok
}

func TestTrain_SuccessBumpsStatAndDecrementsPool(t *testing.T) {
	a := newTrainActor()
	a.trains = 2
	tm, _ := newTrainingMgr(t, nil, "")
	ctx := &command.Context{
		Actor: a, Training: tm,
		Verb: "train", Args: []string{"str"},
	}
	if err := command.TrainHandler(context.Background(), ctx); err != nil {
		t.Fatalf("TrainHandler: %v", err)
	}
	if !strings.Contains(a.lastLine(), "stronger") {
		t.Errorf("output = %q", a.lastLine())
	}
	if a.trains != 1 {
		t.Errorf("trains = %d, want 1", a.trains)
	}
	if a.sb.Effective(progression.StatSTR) != 11 {
		t.Errorf("STR = %d, want 11", a.sb.Effective(progression.StatSTR))
	}
}

func TestTrain_NoArgsReportsPool(t *testing.T) {
	a := newTrainActor()
	a.trains = 3
	tm, _ := newTrainingMgr(t, nil, "")
	ctx := &command.Context{Actor: a, Training: tm, Verb: "train"}
	_ = command.TrainHandler(context.Background(), ctx)
	if !strings.Contains(a.lastLine(), "Trains available: 3") {
		t.Errorf("output = %q", a.lastLine())
	}
}

func TestTrain_NoTrainsFails(t *testing.T) {
	a := newTrainActor()
	a.trains = 0
	tm, _ := newTrainingMgr(t, nil, "")
	ctx := &command.Context{Actor: a, Training: tm, Verb: "train", Args: []string{"str"}}
	_ = command.TrainHandler(context.Background(), ctx)
	if !strings.Contains(strings.ToLower(a.lastLine()), "no trains") {
		t.Errorf("output = %q", a.lastLine())
	}
}

func TestTrain_NilTrainingMgrSafe(t *testing.T) {
	a := newTrainActor()
	ctx := &command.Context{Actor: a, Training: nil, Verb: "train", Args: []string{"str"}}
	if err := command.TrainHandler(context.Background(), ctx); err != nil {
		t.Fatalf("TrainHandler: %v", err)
	}
	if !strings.Contains(strings.ToLower(a.lastLine()), "not enabled") {
		t.Errorf("output = %q", a.lastLine())
	}
}

func TestPractice_NoTrainerReportsCleanly(t *testing.T) {
	a := newTrainActor()
	tm, _ := newTrainingMgr(t, nil, "")
	ctx := &command.Context{Actor: a, Training: tm, Verb: "practice", Args: []string{"slash"}}
	_ = command.PracticeHandler(context.Background(), ctx)
	// nil proficiency seam → NotLearned wins over NoTrainer.
	if !strings.Contains(strings.ToLower(a.lastLine()), "haven't learned") {
		t.Errorf("output = %q", a.lastLine())
	}
}

func TestPractice_NoArgsAsksWhat(t *testing.T) {
	a := newTrainActor()
	tm, _ := newTrainingMgr(t, nil, "")
	ctx := &command.Context{Actor: a, Training: tm, Verb: "practice"}
	_ = command.PracticeHandler(context.Background(), ctx)
	if !strings.Contains(strings.ToLower(a.lastLine()), "practice what") {
		t.Errorf("output = %q", a.lastLine())
	}
}
