package quest

import (
	"strings"
	"testing"
)

// --- test doubles ---

type fakePlayer struct {
	id      string
	level   int
	class   string
	setCls  string
	setRace string
}

func (p *fakePlayer) EntityID() string { return p.id }
func (p *fakePlayer) Level(string) int {
	if p.level == 0 {
		return 1
	}
	return p.level
}
func (p *fakePlayer) Class() string     { return p.class }
func (p *fakePlayer) SetClass(c string) { p.setCls = c; p.class = c }
func (p *fakePlayer) SetRace(r string)  { p.setRace = r }

type recSink struct {
	started   []StartedEvent
	advanced  []ObjectiveAdvancedEvent
	staged    []StageAdvancedEvent
	completed []CompletedEvent
	abandoned []AbandonedEvent
}

func (s *recSink) Started(e StartedEvent)                     { s.started = append(s.started, e) }
func (s *recSink) ObjectiveAdvanced(e ObjectiveAdvancedEvent) { s.advanced = append(s.advanced, e) }
func (s *recSink) StageAdvanced(e StageAdvancedEvent)         { s.staged = append(s.staged, e) }
func (s *recSink) Completed(e CompletedEvent)                 { s.completed = append(s.completed, e) }
func (s *recSink) Abandoned(e AbandonedEvent)                 { s.abandoned = append(s.abandoned, e) }

type recRewards struct {
	xp    []string
	gold  []string
	learn []string
	items []string
}

func (r *recRewards) GrantExperience(id string, amt int64, track, src string) {
	r.xp = append(r.xp, id)
}
func (r *recRewards) AddGold(id string, d int, reason string) { r.gold = append(r.gold, id) }
func (r *recRewards) Learn(id, ab string, p int)              { r.learn = append(r.learn, ab) }
func (r *recRewards) GrantItem(id, tmpl string, silent bool)  { r.items = append(r.items, tmpl) }

type countingPersist struct{ saves int }

func (c *countingPersist) Save(string, *State) { c.saves++ }

func twoStageDef(id string) *Definition {
	return &Definition{
		ID: id, Name: "Test Quest", Classification: "side", Abandonable: true,
		Stages: []Stage{
			{ID: "s0", Objectives: []Objective{
				{ID: "s0-kill-0", Type: "kill", Target: "core:rat", Count: 2, Description: "Kill rats"},
			}},
			{ID: "s1", Objectives: []Objective{
				{ID: "s1-visit-0", Type: "visit", Target: "core:home", Count: 1},
			}},
		},
		Reward: Reward{XP: 50, Gold: 10, Items: []string{"core:potion"}, Abilities: []string{"bless"}},
	}
}

func newTestService(t *testing.T) (*Service, *Registry, *recSink) {
	t.Helper()
	reg := NewRegistry()
	if err := reg.Register(twoStageDef("q")); err != nil {
		t.Fatal(err)
	}
	sink := &recSink{}
	svc := NewService(Config{Registry: reg, Events: sink})
	return svc, reg, sink
}

// --- acceptance ---

func TestAcceptOutcomes(t *testing.T) {
	svc, _, sink := newTestService(t)
	p := &fakePlayer{id: "p1"}

	if r := svc.Accept(p, "missing", false); r.Status != NotFound {
		t.Errorf("missing = %v, want NotFound", r.Status)
	}
	if r := svc.Accept(p, "q", false); r.Status != Accepted || r.Banner == "" {
		t.Errorf("accept = %v banner=%q", r.Status, r.Banner)
	}
	if r := svc.Accept(p, "q", false); r.Status != AlreadyActive {
		t.Errorf("re-accept = %v, want AlreadyActive", r.Status)
	}
	if len(sink.started) != 1 {
		t.Errorf("started events = %d, want 1", len(sink.started))
	}
}

func TestAcceptAlreadyCompletedNonRepeatable(t *testing.T) {
	svc, _, _ := newTestService(t)
	p := &fakePlayer{id: "p1"}
	svc.LoadState("p1", &State{Completed: []string{"q"}})
	if r := svc.Accept(p, "q", false); r.Status != AlreadyCompleted {
		t.Errorf("= %v, want AlreadyCompleted", r.Status)
	}
}

func TestAcceptRepeatableWhenCompleted(t *testing.T) {
	reg := NewRegistry()
	d := twoStageDef("q")
	d.Repeatable = true
	_ = reg.Register(d)
	svc := NewService(Config{Registry: reg})
	svc.LoadState("p1", &State{Completed: []string{"q"}})
	if r := svc.Accept(&fakePlayer{id: "p1"}, "q", false); r.Status != Accepted {
		t.Errorf("repeatable re-accept = %v, want Accepted", r.Status)
	}
}

func TestAcceptPrereqs(t *testing.T) {
	reg := NewRegistry()
	d := twoStageDef("q")
	d.Prereq = Prerequisite{MinLevel: 5, Class: "fighter", QuestsCompleted: []string{"intro"}, QuestsNotCompleted: []string{"rival"}}
	_ = reg.Register(d)
	svc := NewService(Config{Registry: reg})

	// fails min level
	if r := svc.Accept(&fakePlayer{id: "a", level: 1, class: "fighter"}, "q", false); r.Status != PrereqNotMet {
		t.Errorf("low level = %v", r.Status)
	}
	// fails class
	if r := svc.Accept(&fakePlayer{id: "b", level: 9, class: "mage"}, "q", false); r.Status != PrereqNotMet {
		t.Errorf("wrong class = %v", r.Status)
	}
	// fails quests-completed (intro not done)
	if r := svc.Accept(&fakePlayer{id: "c", level: 9, class: "fighter"}, "q", false); r.Status != PrereqNotMet {
		t.Errorf("missing prereq quest = %v", r.Status)
	}
	// passes
	svc.LoadState("d", &State{Completed: []string{"intro"}})
	if r := svc.Accept(&fakePlayer{id: "d", level: 9, class: "fighter"}, "q", false); r.Status != Accepted {
		t.Errorf("all prereqs met = %v, want Accepted", r.Status)
	}
	// fails quests-not-completed (rival done)
	svc.LoadState("e", &State{Completed: []string{"intro", "rival"}})
	if r := svc.Accept(&fakePlayer{id: "e", level: 9, class: "fighter"}, "q", false); r.Status != PrereqNotMet {
		t.Errorf("excluded quest done = %v", r.Status)
	}
}

func TestAcceptCap(t *testing.T) {
	reg := NewRegistry()
	for _, id := range []string{"a", "b"} {
		d := twoStageDef(id)
		_ = reg.Register(d)
	}
	// non-abandonable quest bypasses the cap
	nb := twoStageDef("plot")
	nb.Abandonable = false
	_ = reg.Register(nb)

	svc := NewService(Config{Registry: reg, Cap: 1})
	p := &fakePlayer{id: "p1"}
	if r := svc.Accept(p, "a", false); r.Status != Accepted {
		t.Fatalf("first = %v", r.Status)
	}
	if r := svc.Accept(p, "b", false); r.Status != CapReached {
		t.Errorf("at cap = %v, want CapReached", r.Status)
	}
	// non-abandonable still grants despite cap
	if r := svc.Accept(p, "plot", false); r.Status != Accepted {
		t.Errorf("non-abandonable at cap = %v, want Accepted", r.Status)
	}
}

func TestAcceptSecretAndSilentSuppressBanner(t *testing.T) {
	reg := NewRegistry()
	secret := twoStageDef("sec")
	secret.Secret = true
	_ = reg.Register(secret)
	_ = reg.Register(twoStageDef("q"))
	svc := NewService(Config{Registry: reg})

	if r := svc.Accept(&fakePlayer{id: "p1"}, "sec", false); r.Banner != "" {
		t.Errorf("secret banner = %q, want empty", r.Banner)
	}
	if r := svc.Accept(&fakePlayer{id: "p2"}, "q", true); r.Banner != "" {
		t.Errorf("silent banner = %q, want empty", r.Banner)
	}
}

// --- progression ---

func TestAdvanceClampsAndStageAdvances(t *testing.T) {
	svc, _, sink := newTestService(t)
	p := &fakePlayer{id: "p1"}
	svc.Accept(p, "q", false)

	// over-advance clamps at required (2)
	svc.AdvanceObjective("p1", "q", "s0-kill-0", 5)
	snap := svc.Snapshot("p1")
	if snap.Active[0].StageIndex != 1 {
		t.Fatalf("stage = %d, want 1 (advanced)", snap.Active[0].StageIndex)
	}
	if len(sink.staged) != 1 || sink.staged[0].StageIndex != 1 {
		t.Errorf("stage event = %+v", sink.staged)
	}
	// new stage objectives seeded at zero
	if snap.Active[0].Objectives[0].Current != 0 {
		t.Errorf("new stage objective not zero: %+v", snap.Active[0].Objectives)
	}
}

func TestAdvanceNoOps(t *testing.T) {
	svc, _, _ := newTestService(t)
	// no state
	svc.AdvanceObjective("ghost", "q", "x", 1)
	// state but no quest
	svc.LoadState("p1", &State{})
	svc.AdvanceObjective("p1", "q", "x", 1)
	if snap := svc.Snapshot("p1"); len(snap.Active) != 0 {
		t.Error("advance on absent quest mutated state")
	}
}

func TestCompleteDispatchesReward(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(twoStageDef("q"))
	rew := &recRewards{}
	sink := &recSink{}
	svc := NewService(Config{
		Registry: reg, Events: sink,
		Rewards: NewDispatcher(WithExperience(rew), WithGold(rew), WithAbilities(rew), WithItems(rew)),
	})
	p := &fakePlayer{id: "p1"}
	svc.Accept(p, "q", false)
	svc.AdvanceObjective("p1", "q", "s0-kill-0", 2)  // → stage 1
	svc.AdvanceObjective("p1", "q", "s1-visit-0", 1) // → complete

	if len(sink.completed) != 1 {
		t.Fatalf("completed events = %d, want 1", len(sink.completed))
	}
	if len(rew.xp) != 1 || len(rew.gold) != 1 || len(rew.items) != 1 || len(rew.learn) != 1 {
		t.Errorf("rewards not all dispatched: xp=%v gold=%v items=%v learn=%v", rew.xp, rew.gold, rew.items, rew.learn)
	}
	snap := svc.Snapshot("p1")
	if len(snap.Active) != 0 || !snap.hasCompleted("q") {
		t.Errorf("completion state wrong: %+v", snap)
	}
}

func TestCompleteSkipsRewardOnCacheMiss(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(twoStageDef("q"))
	rew := &recRewards{}
	sink := &recSink{}
	svc := NewService(Config{Registry: reg, Events: sink, Rewards: NewDispatcher(WithExperience(rew))})
	// inject an active quest at the final stage WITHOUT going through
	// Accept, so the player cache has no entry.
	svc.LoadState("p1", &State{Active: []ActiveQuest{
		{QuestID: "q", StageIndex: 1, Objectives: []ObjectiveProgress{{ObjectiveID: "s1-visit-0", Required: 1}}},
	}})
	svc.AdvanceObjective("p1", "q", "s1-visit-0", 1)
	if len(sink.completed) != 1 {
		t.Errorf("completed event should still fire on cache miss: %d", len(sink.completed))
	}
	if len(rew.xp) != 0 {
		t.Errorf("reward should be skipped on cache miss: %v", rew.xp)
	}
}

func TestRewardClassRaceUnlockSetters(t *testing.T) {
	reg := NewRegistry()
	d := twoStageDef("q")
	d.Reward = Reward{ClassUnlock: "paladin", RaceUnlock: "elf"}
	_ = reg.Register(d)
	svc := NewService(Config{Registry: reg})
	p := &fakePlayer{id: "p1"}
	svc.Accept(p, "q", false)
	svc.AdvanceObjective("p1", "q", "s0-kill-0", 2)
	svc.AdvanceObjective("p1", "q", "s1-visit-0", 1)
	if p.setCls != "paladin" || p.setRace != "elf" {
		t.Errorf("unlock setters: class=%q race=%q", p.setCls, p.setRace)
	}
}

// --- advance-by-predicate ---

func TestAdvanceMatching(t *testing.T) {
	svc, _, _ := newTestService(t)
	p := &fakePlayer{id: "p1"}
	svc.Accept(p, "q", false)
	// advance kill objectives whose target is core:rat
	svc.AdvanceMatching("p1", "kill", func(o Objective) bool { return o.Target == "core:rat" })
	snap := svc.Snapshot("p1")
	if snap.Active[0].Objectives[0].Current != 1 {
		t.Errorf("matching advance = %d, want 1", snap.Active[0].Objectives[0].Current)
	}
	// non-matching type does nothing
	svc.AdvanceMatching("p1", "collect", func(o Objective) bool { return true })
	snap = svc.Snapshot("p1")
	if snap.Active[0].Objectives[0].Current != 1 {
		t.Errorf("non-matching type advanced: %d", snap.Active[0].Objectives[0].Current)
	}
}

// --- abandonment ---

func TestAbandon(t *testing.T) {
	svc, _, sink := newTestService(t)
	p := &fakePlayer{id: "p1"}
	svc.Accept(p, "q", false)
	svc.Abandon("p1", "q")
	if snap := svc.Snapshot("p1"); len(snap.Active) != 0 {
		t.Error("abandon did not remove active quest")
	}
	if len(sink.abandoned) != 1 {
		t.Errorf("abandoned events = %d, want 1", len(sink.abandoned))
	}
}

func TestAbandonNonAbandonableRejected(t *testing.T) {
	reg := NewRegistry()
	d := twoStageDef("plot")
	d.Abandonable = false
	_ = reg.Register(d)
	sink := &recSink{}
	svc := NewService(Config{Registry: reg, Events: sink})
	p := &fakePlayer{id: "p1"}
	svc.Accept(p, "plot", false)
	svc.Abandon("p1", "plot")
	if snap := svc.Snapshot("p1"); len(snap.Active) != 1 {
		t.Error("non-abandonable quest should not be abandoned")
	}
	if len(sink.abandoned) != 0 {
		t.Errorf("no abandoned event expected: %d", len(sink.abandoned))
	}
}

func TestPersistOnEveryMutation(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(twoStageDef("q"))
	cp := &countingPersist{}
	svc := NewService(Config{Registry: reg, Persist: cp})
	p := &fakePlayer{id: "p1"}
	svc.Accept(p, "q", false)                        // +1
	svc.AdvanceObjective("p1", "q", "s0-kill-0", 1)  // +1 (in-stage)
	svc.AdvanceObjective("p1", "q", "s0-kill-0", 1)  // +1 (stage advance)
	svc.AdvanceObjective("p1", "q", "s1-visit-0", 1) // +1 (complete)
	svc.Accept(p, "q", false)                        // already completed (non-repeatable) → no persist
	if cp.saves < 4 {
		t.Errorf("saves = %d, want >= 4 (every mutation persists)", cp.saves)
	}
}

func TestDropState(t *testing.T) {
	svc, _, _ := newTestService(t)
	svc.Accept(&fakePlayer{id: "p1"}, "q", false)
	svc.DropState("p1")
	if snap := svc.Snapshot("p1"); snap != nil {
		t.Errorf("DropState left state: %+v", snap)
	}
}

func TestNewServiceDefaultsDontPanic(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(twoStageDef("q"))
	// bare config: defaults to NopEventSink/NopPersister/NewDispatcher.
	svc := NewService(Config{Registry: reg})
	if r := svc.Accept(&fakePlayer{id: "p1"}, "q", false); r.Status != Accepted {
		t.Fatalf("default-config accept = %v", r.Status)
	}
	svc.AdvanceObjective("p1", "q", "s0-kill-0", 2)
	svc.AdvanceObjective("p1", "q", "s1-visit-0", 1) // completes through nop reward dispatch
}

func TestDispatcherWithTrack(t *testing.T) {
	var gotTrack string
	x := trackCapture{&gotTrack}
	d := NewDispatcher(WithExperience(x), WithTrack("crafting"))
	d.Dispatch(&fakePlayer{id: "p1"}, Reward{XP: 5})
	if gotTrack != "crafting" {
		t.Errorf("track = %q, want crafting", gotTrack)
	}
}

type trackCapture struct{ track *string }

func (t trackCapture) GrantExperience(_ string, _ int64, track, _ string) { *t.track = track }

func TestBuildBannerWithoutClassification(t *testing.T) {
	d := &Definition{ID: "q", Name: "Plain", Stages: []Stage{
		{Description: "Do the thing.", Objectives: []Objective{{ID: "o", Type: "kill", Count: 1}}},
	}}
	a := newActiveQuest("q", 0, d.Stages[0])
	got := buildBanner(d, &a)
	if !strings.Contains(got, "Plain") {
		t.Errorf("banner = %q", got)
	}
}
