package progression

import (
	"context"
	"testing"
)

// fakeTrainee implements TrainingEntity for tests.
type fakeTrainee struct {
	sb       *StatBlock
	trains   int
	raceID   string
	roomTags map[string]bool
}

func (f *fakeTrainee) StatBlock() *StatBlock   { return f.sb }
func (f *fakeTrainee) TrainsAvailable() int    { return f.trains }
func (f *fakeTrainee) SpendTrain() bool {
	if f.trains <= 0 {
		return false
	}
	f.trains--
	return true
}
func (f *fakeTrainee) RaceID() string             { return f.raceID }
func (f *fakeTrainee) HasRoomTag(tag string) bool { return f.roomTags[tag] }

// fakeTrainerSource returns a fixed trainer config / name pair.
type fakeTrainerSource struct {
	cfg  *TrainerConfig
	name string
	ok   bool
}

func (f fakeTrainerSource) TrainerInRoom(string, string) (*TrainerConfig, string, bool) {
	return f.cfg, f.name, f.ok
}

// fakeProficiency is an in-memory proficiency store.
type fakeProficiency struct {
	caps   map[string]int
	prof   map[string]int
	known  map[string]bool
	names  map[string]string
}

func newFakeProf() *fakeProficiency {
	return &fakeProficiency{
		caps: map[string]int{}, prof: map[string]int{},
		known: map[string]bool{}, names: map[string]string{},
	}
}
func (f *fakeProficiency) learn(id, name string, cap, prof int) {
	f.known[id] = true
	f.caps[id] = cap
	f.prof[id] = prof
	f.names[id] = name
}
func (f *fakeProficiency) GetCap(_, abilityID string) (int, int, bool) {
	return f.caps[abilityID], f.prof[abilityID], f.known[abilityID]
}
func (f *fakeProficiency) SetCap(_, abilityID string, cap int)            { f.caps[abilityID] = cap }
func (f *fakeProficiency) AddProficiency(_, abilityID string, delta int)  { f.prof[abilityID] += delta }
func (f *fakeProficiency) AbilityName(abilityID string) (string, bool) {
	n, ok := f.names[abilityID]
	return n, ok
}

func TestNextTier(t *testing.T) {
	cases := []struct {
		in   int
		want CapTier
	}{
		{0, CapNovice},
		{-5, CapNovice},
		{24, CapNovice},
		{25, CapApprentice},
		{49, CapApprentice},
		{50, CapJourneyman},
		{75, CapMaster},
		{100, CapNone},
		{150, CapNone},
	}
	for _, c := range cases {
		if got := NextTier(c.in); got != c.want {
			t.Errorf("NextTier(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestTrainerConfigCanTeach(t *testing.T) {
	tc := &TrainerConfig{Tier: CapNovice, Teach: []string{"slash", "parry"}}
	if !tc.CanTeach("slash") || !tc.CanTeach("SLASH") {
		t.Error("CanTeach should be case-insensitive on known abilities")
	}
	if tc.CanTeach("backstab") {
		t.Error("CanTeach reported true for unknown ability")
	}
	var nilTC *TrainerConfig
	if nilTC.CanTeach("slash") {
		t.Error("nil receiver should report false")
	}
}

func TestTrainingConfigSetTrainable(t *testing.T) {
	cfg := DefaultTrainingConfig()
	if !cfg.IsTrainable("STR") {
		t.Error("default config should treat STR as trainable")
	}
	cfg.SetTrainable("str", false)
	if cfg.IsTrainable("str") {
		t.Error("SetTrainable(false) did not disable")
	}
	cfg.SetTrainable("magic", true)
	if !cfg.IsTrainable("magic") {
		t.Error("SetTrainable(true) did not enable new stat")
	}
}

func TestTryTrainSuccess(t *testing.T) {
	sb := NewWithBase(map[StatType]int{StatSTR: 10})
	tr := &fakeTrainee{sb: sb, trains: 2, raceID: "human"}
	races := NewRaceRegistry()
	_ = races.Register(&Race{ID: "human", StatCaps: map[StatType]int{StatSTR: 18}})
	mgr := NewTrainingManager(DefaultTrainingConfig(), races, nil, nil)

	r := mgr.TryTrain(context.Background(), tr, "str")
	if r.Outcome != TrainSuccess {
		t.Fatalf("outcome = %v; want TrainSuccess (msg=%q)", r.Outcome, r.Message)
	}
	if r.NewBase != 11 || r.NewEffective != 11 {
		t.Errorf("expected base/eff = 11, got %d/%d", r.NewBase, r.NewEffective)
	}
	if tr.trains != 1 {
		t.Errorf("trains not decremented: %d", tr.trains)
	}
}

func TestTryTrainAtRaceCap(t *testing.T) {
	sb := NewWithBase(map[StatType]int{StatSTR: 18})
	tr := &fakeTrainee{sb: sb, trains: 1, raceID: "human"}
	races := NewRaceRegistry()
	_ = races.Register(&Race{ID: "human", StatCaps: map[StatType]int{StatSTR: 18}})
	mgr := NewTrainingManager(DefaultTrainingConfig(), races, nil, nil)
	r := mgr.TryTrain(context.Background(), tr, "str")
	if r.Outcome != TrainAtRaceCap {
		t.Fatalf("got %v; want TrainAtRaceCap", r.Outcome)
	}
	if tr.trains != 1 {
		t.Errorf("trains spent on failure: %d", tr.trains)
	}
}

func TestTryTrainNoTrains(t *testing.T) {
	sb := NewWithBase(map[StatType]int{StatSTR: 10})
	tr := &fakeTrainee{sb: sb, trains: 0, raceID: "human"}
	mgr := NewTrainingManager(DefaultTrainingConfig(), NewRaceRegistry(), nil, nil)
	r := mgr.TryTrain(context.Background(), tr, "str")
	if r.Outcome != TrainNoTrains {
		t.Errorf("got %v; want TrainNoTrains", r.Outcome)
	}
}

func TestTryTrainNotTrainable(t *testing.T) {
	sb := NewWithBase(map[StatType]int{StatHPMax: 20})
	tr := &fakeTrainee{sb: sb, trains: 1}
	mgr := NewTrainingManager(DefaultTrainingConfig(), NewRaceRegistry(), nil, nil)
	r := mgr.TryTrain(context.Background(), tr, "hp_max")
	if r.Outcome != TrainNotTrainable {
		t.Errorf("got %v; want TrainNotTrainable", r.Outcome)
	}
}

func TestTryTrainUnsafeRoom(t *testing.T) {
	sb := NewWithBase(map[StatType]int{StatSTR: 10})
	tr := &fakeTrainee{sb: sb, trains: 1}
	cfg := DefaultTrainingConfig()
	cfg.RequireSafeRoomForStats = true
	mgr := NewTrainingManager(cfg, NewRaceRegistry(), nil, nil)
	r := mgr.TryTrain(context.Background(), tr, "str")
	if r.Outcome != TrainUnsafeRoom {
		t.Fatalf("got %v; want TrainUnsafeRoom", r.Outcome)
	}
	tr.roomTags = map[string]bool{"safe": true}
	r = mgr.TryTrain(context.Background(), tr, "str")
	if r.Outcome != TrainSuccess {
		t.Errorf("safe room not honored: %v", r.Outcome)
	}
}

func TestTryTrainDefaultRaceCap(t *testing.T) {
	// Race declares no cap for STR — use config default (25).
	sb := NewWithBase(map[StatType]int{StatSTR: 25})
	tr := &fakeTrainee{sb: sb, trains: 1, raceID: "elf"}
	races := NewRaceRegistry()
	_ = races.Register(&Race{ID: "elf"}) // no caps
	mgr := NewTrainingManager(DefaultTrainingConfig(), races, nil, nil)
	r := mgr.TryTrain(context.Background(), tr, "str")
	if r.Outcome != TrainAtRaceCap {
		t.Errorf("got %v; want TrainAtRaceCap (default 25)", r.Outcome)
	}
}

func TestTryPracticeNotLearned(t *testing.T) {
	prof := newFakeProf()
	mgr := NewTrainingManager(DefaultTrainingConfig(), nil,
		fakeTrainerSource{cfg: &TrainerConfig{Tier: CapNovice, Teach: []string{"slash"}}, name: "Maerys", ok: true},
		prof)
	r := mgr.TryPractice(context.Background(), nil, "p1", "slash")
	if r.Outcome != PracticeNotLearned {
		t.Errorf("got %v; want PracticeNotLearned", r.Outcome)
	}
}

func TestTryPracticeSuccessAndCatchUp(t *testing.T) {
	prof := newFakeProf()
	// Learned with cap 25 (Novice), proficiency lagging at 15.
	prof.learn("slash", "Slash", 25, 15)
	mgr := NewTrainingManager(DefaultTrainingConfig(), nil,
		fakeTrainerSource{cfg: &TrainerConfig{Tier: CapApprentice, Teach: []string{"slash"}}, name: "Maerys", ok: true},
		prof)
	r := mgr.TryPractice(context.Background(), nil, "p1", "slash")
	if r.Outcome != PracticeSuccess {
		t.Fatalf("got %v; want PracticeSuccess (msg=%q)", r.Outcome, r.Message)
	}
	if prof.caps["slash"] != 50 {
		t.Errorf("cap not raised: %d", prof.caps["slash"])
	}
	if !r.Boosted {
		t.Error("catch-up boost not applied")
	}
	// 15 + 5 = 20, clamped at the PRIOR cap (25).
	if prof.prof["slash"] != 20 {
		t.Errorf("proficiency after boost = %d; want 20", prof.prof["slash"])
	}
}

func TestTryPracticeCatchUpClampedAtPriorCap(t *testing.T) {
	prof := newFakeProf()
	prof.learn("slash", "Slash", 25, 23) // 2 below prior cap
	mgr := NewTrainingManager(DefaultTrainingConfig(), nil,
		fakeTrainerSource{cfg: &TrainerConfig{Tier: CapApprentice, Teach: []string{"slash"}}, name: "Maerys", ok: true},
		prof)
	_ = mgr.TryPractice(context.Background(), nil, "p1", "slash")
	// Boost would push 23 → 28, but prior cap is 25 → clamp.
	if prof.prof["slash"] != 25 {
		t.Errorf("catch-up bumped past prior cap: %d", prof.prof["slash"])
	}
}

func TestTryPracticeTierSkip(t *testing.T) {
	prof := newFakeProf()
	prof.learn("slash", "Slash", 25, 20)
	// Trainer is Journeyman (75) but player cap is only Novice (25)
	// — must go to Apprentice (50) first.
	mgr := NewTrainingManager(DefaultTrainingConfig(), nil,
		fakeTrainerSource{cfg: &TrainerConfig{Tier: CapJourneyman, Teach: []string{"slash"}}, name: "Maerys", ok: true},
		prof)
	r := mgr.TryPractice(context.Background(), nil, "p1", "slash")
	if r.Outcome != PracticeTierSkip {
		t.Errorf("got %v; want PracticeTierSkip", r.Outcome)
	}
}

func TestTryPracticeAlreadyAtOrAbove(t *testing.T) {
	prof := newFakeProf()
	prof.learn("slash", "Slash", 50, 40)
	mgr := NewTrainingManager(DefaultTrainingConfig(), nil,
		fakeTrainerSource{cfg: &TrainerConfig{Tier: CapNovice, Teach: []string{"slash"}}, name: "Maerys", ok: true},
		prof)
	r := mgr.TryPractice(context.Background(), nil, "p1", "slash")
	if r.Outcome != PracticeAlreadyAtOrAboveTier {
		t.Errorf("got %v; want PracticeAlreadyAtOrAboveTier", r.Outcome)
	}
}

func TestTryPracticeCannotTeach(t *testing.T) {
	prof := newFakeProf()
	prof.learn("slash", "Slash", 25, 20)
	mgr := NewTrainingManager(DefaultTrainingConfig(), nil,
		fakeTrainerSource{cfg: &TrainerConfig{Tier: CapApprentice, Teach: []string{"parry"}}, name: "Maerys", ok: true},
		prof)
	r := mgr.TryPractice(context.Background(), nil, "p1", "slash")
	if r.Outcome != PracticeCannotTeach {
		t.Errorf("got %v; want PracticeCannotTeach", r.Outcome)
	}
}

func TestTryPracticeNoTrainer(t *testing.T) {
	prof := newFakeProf()
	prof.learn("slash", "Slash", 25, 20)
	mgr := NewTrainingManager(DefaultTrainingConfig(), nil,
		fakeTrainerSource{ok: false}, prof)
	r := mgr.TryPractice(context.Background(), nil, "p1", "slash")
	if r.Outcome != PracticeNoTrainer {
		t.Errorf("got %v; want PracticeNoTrainer", r.Outcome)
	}
}
