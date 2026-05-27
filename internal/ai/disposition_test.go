package ai

import (
	"context"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// ---- test doubles ----

type stubTemplates struct{ m map[mob.TemplateID]*mob.Template }

func (s *stubTemplates) Get(id mob.TemplateID) (*mob.Template, error) {
	t, ok := s.m[id]
	if !ok {
		return nil, mob.ErrTemplateNotFound
	}
	return t, nil
}

type stubPlayers struct {
	byRoom map[world.RoomID][]PlayerView
	byID   map[string]PlayerView
}

func (s *stubPlayers) PlayersInRoom(_ context.Context, r world.RoomID) []PlayerView {
	return s.byRoom[r]
}

func (s *stubPlayers) PlayerByID(_ context.Context, id string) (PlayerView, bool) {
	v, ok := s.byID[id]
	return v, ok
}

// fixture builds a coherent template + mob instance + evaluator
// wired against test stubs. Returns just what each test needs.
type fixture struct {
	tpl       *mob.Template
	inst      *entities.MobInstance
	store     *entities.Store
	placement *entities.Placement
	players   *stubPlayers
	bus       *eventbus.Bus
	captured  []captureSlot
	muCap     sync.Mutex
	eval      *Evaluator
}

type captureSlot struct {
	name  string
	event eventbus.Event
}

func newFixture(t *testing.T, tpl *mob.Template) *fixture {
	t.Helper()
	store := entities.NewStore()
	inst, err := store.SpawnMob(tpl)
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	placement := entities.NewPlacement()
	placement.Place(inst.ID(), "core:r1")

	players := &stubPlayers{
		byRoom: map[world.RoomID][]PlayerView{},
		byID:   map[string]PlayerView{},
	}
	bus := eventbus.New()
	f := &fixture{
		tpl:       tpl,
		inst:      inst,
		store:     store,
		placement: placement,
		players:   players,
		bus:       bus,
	}
	// Capture every reaction event the evaluator publishes.
	for _, name := range []string{
		eventbus.EventMobAggro,
		eventbus.EventMobWary,
		eventbus.EventMobFriendly,
	} {
		n := name
		bus.Subscribe(n, func(_ context.Context, e eventbus.Event) {
			f.muCap.Lock()
			f.captured = append(f.captured, captureSlot{name: n, event: e})
			f.muCap.Unlock()
		})
	}
	f.eval = NewEvaluator(EvaluatorConfig{
		Templates: &stubTemplates{m: map[mob.TemplateID]*mob.Template{tpl.ID: tpl}},
		Players:   players,
		Placement: placement,
		Store:     store,
		Bus:       bus,
	})
	return f
}

func (f *fixture) captureNames() []string {
	f.muCap.Lock()
	defer f.muCap.Unlock()
	out := make([]string, 0, len(f.captured))
	for _, c := range f.captured {
		out = append(out, c.name)
	}
	return out
}

// ---- decideReaction unit tests ----

func TestDecideReaction_StaticHostileShortCircuitsRules(t *testing.T) {
	tpl := &mob.Template{
		ID:              "core:m",
		Name:            "m",
		BaseDisposition: mob.ReactionHostile,
		DispositionRules: &mob.Definition{
			Default: mob.ReactionFriendly,
		},
	}
	r, ok := decideReaction(tpl, PlayerView{ID: "p"})
	if !ok || r != mob.ReactionHostile {
		t.Errorf("decideReaction = %q,%v; want hostile,true", r, ok)
	}
}

func TestDecideReaction_RuleOrderingFirstMatchWins(t *testing.T) {
	tpl := &mob.Template{
		ID:   "core:m",
		Name: "m",
		DispositionRules: &mob.Definition{
			Default: mob.ReactionFriendly,
			Rules: []mob.Rule{
				{HasTag: "outlaw", Reaction: mob.ReactionHostile},
				{HasTag: "outlaw", Reaction: mob.ReactionWary}, // shadowed
			},
		},
	}
	r, ok := decideReaction(tpl, PlayerView{ID: "p", Tags: []string{"outlaw"}})
	if !ok || r != mob.ReactionHostile {
		t.Errorf("decideReaction = %q,%v; want hostile,true", r, ok)
	}
}

func TestDecideReaction_DefaultAppliesWhenNoRuleMatches(t *testing.T) {
	tpl := &mob.Template{
		ID:   "core:m",
		Name: "m",
		DispositionRules: &mob.Definition{
			Default: mob.ReactionFriendly,
			Rules: []mob.Rule{
				{HasTag: "outlaw", Reaction: mob.ReactionHostile},
			},
		},
	}
	r, ok := decideReaction(tpl, PlayerView{ID: "p"}) // no tags
	if !ok || r != mob.ReactionFriendly {
		t.Errorf("decideReaction = %q,%v; want friendly,true", r, ok)
	}
}

func TestDecideReaction_RuleWithoutConditionsMatchesAnything(t *testing.T) {
	tpl := &mob.Template{
		ID: "core:m", Name: "m",
		DispositionRules: &mob.Definition{
			Rules: []mob.Rule{
				{Reaction: mob.ReactionWary}, // matches anything
			},
		},
	}
	r, ok := decideReaction(tpl, PlayerView{ID: "p"})
	if !ok || r != mob.ReactionWary {
		t.Errorf("decideReaction = %q,%v; want wary,true", r, ok)
	}
}

func TestDecideReaction_NoRulesNoStaticReturnsNoDispatch(t *testing.T) {
	tpl := &mob.Template{ID: "core:m", Name: "m"}
	if _, ok := decideReaction(tpl, PlayerView{ID: "p"}); ok {
		t.Error("decideReaction should return ok=false when mob has no policy at all")
	}
}

func TestDecideReaction_AlignmentConditionsRequireData(t *testing.T) {
	// A view without HasAlignment cannot satisfy any alignment
	// condition (preserves pre-M8.5 "never match" for callers
	// that haven't been updated to populate the field).
	min := -100
	tpl := &mob.Template{
		ID: "core:m", Name: "m",
		DispositionRules: &mob.Definition{
			Default: mob.ReactionFriendly,
			Rules: []mob.Rule{
				{HasMinAlignment: true, MinAlignment: min, Reaction: mob.ReactionHostile},
			},
		},
	}
	r, _ := decideReaction(tpl, PlayerView{ID: "p"})
	if r != mob.ReactionFriendly {
		t.Errorf("got %q without HasAlignment, want friendly (rule must not match)", r)
	}
}

func TestDecideReaction_AlignmentMin(t *testing.T) {
	tpl := &mob.Template{
		ID: "g", Name: "guard",
		DispositionRules: &mob.Definition{
			Default: mob.ReactionFriendly,
			Rules: []mob.Rule{
				{HasMinAlignment: true, MinAlignment: 100, Reaction: mob.ReactionHostile},
			},
		},
	}
	hostileView := PlayerView{ID: "h", Alignment: 100, HasAlignment: true}
	if r, _ := decideReaction(tpl, hostileView); r != mob.ReactionHostile {
		t.Errorf("at threshold: got %q, want hostile", r)
	}
	below := PlayerView{ID: "b", Alignment: 99, HasAlignment: true}
	if r, _ := decideReaction(tpl, below); r != mob.ReactionFriendly {
		t.Errorf("below threshold: got %q, want friendly", r)
	}
}

func TestDecideReaction_AlignmentMax(t *testing.T) {
	tpl := &mob.Template{
		ID: "g", Name: "guard",
		DispositionRules: &mob.Definition{
			Default: mob.ReactionFriendly,
			Rules: []mob.Rule{
				{HasMaxAlignment: true, MaxAlignment: -100, Reaction: mob.ReactionHostile},
			},
		},
	}
	hit := PlayerView{ID: "h", Alignment: -100, HasAlignment: true}
	if r, _ := decideReaction(tpl, hit); r != mob.ReactionHostile {
		t.Errorf("at ceiling: got %q, want hostile", r)
	}
	above := PlayerView{ID: "a", Alignment: -99, HasAlignment: true}
	if r, _ := decideReaction(tpl, above); r != mob.ReactionFriendly {
		t.Errorf("above ceiling: got %q, want friendly", r)
	}
}

func TestDecideReaction_BucketRule(t *testing.T) {
	tpl := &mob.Template{
		ID: "g", Name: "guard",
		DispositionRules: &mob.Definition{
			Default: mob.ReactionFriendly,
			Rules: []mob.Rule{
				{Buckets: []string{"evil"}, Reaction: mob.ReactionHostile},
			},
		},
	}
	evil := PlayerView{ID: "e", Alignment: -800, Bucket: "evil", HasAlignment: true}
	if r, _ := decideReaction(tpl, evil); r != mob.ReactionHostile {
		t.Errorf("evil player: got %q, want hostile", r)
	}
	good := PlayerView{ID: "g", Alignment: 800, Bucket: "good", HasAlignment: true}
	if r, _ := decideReaction(tpl, good); r != mob.ReactionFriendly {
		t.Errorf("good player: got %q, want friendly", r)
	}
	// Player with HasAlignment=true but unknown Bucket cannot match.
	missing := PlayerView{ID: "m", Alignment: -800, HasAlignment: true}
	if r, _ := decideReaction(tpl, missing); r != mob.ReactionFriendly {
		t.Errorf("empty Bucket: got %q, want friendly", r)
	}
}

func TestDecideReaction_CombinedAlignmentAndTag(t *testing.T) {
	// A rule with both has_tag and alignment must satisfy ALL.
	tpl := &mob.Template{
		ID: "g", Name: "guard",
		DispositionRules: &mob.Definition{
			Default: mob.ReactionFriendly,
			Rules: []mob.Rule{
				{
					HasTag:          "outlaw",
					HasMinAlignment: true, MinAlignment: 500,
					Reaction: mob.ReactionHostile,
				},
			},
		},
	}
	both := PlayerView{ID: "x", Tags: []string{"outlaw"}, Alignment: 600, HasAlignment: true}
	if r, _ := decideReaction(tpl, both); r != mob.ReactionHostile {
		t.Errorf("tag+align both satisfied: got %q, want hostile", r)
	}
	tagOnly := PlayerView{ID: "x", Tags: []string{"outlaw"}, Alignment: 100, HasAlignment: true}
	if r, _ := decideReaction(tpl, tagOnly); r != mob.ReactionFriendly {
		t.Errorf("tag only (alignment too low): got %q, want friendly", r)
	}
}

// ---- evaluator dispatch + caching tests ----

func TestEvaluate_PerTickDedup(t *testing.T) {
	tpl := &mob.Template{
		ID: "core:m", Name: "guard",
		DispositionRules: &mob.Definition{Default: mob.ReactionFriendly},
	}
	f := newFixture(t, tpl)
	p := PlayerView{ID: "p1", Name: "p1"}

	f.eval.Evaluate(context.Background(), f.inst, p, "core:r1", ModeFull)
	f.eval.Evaluate(context.Background(), f.inst, p, "core:r1", ModeFull)

	if got := f.captureNames(); len(got) != 1 || got[0] != eventbus.EventMobFriendly {
		t.Errorf("captured = %v, want exactly one mob.friendly", got)
	}
}

func TestEvaluate_ResetTickAllowsRedispatch(t *testing.T) {
	// After ResetTick the pair can be evaluated again — but the
	// per-room state still suppresses repeat non-hostile reactions
	// from re-firing. So we should still see exactly one friendly.
	tpl := &mob.Template{
		ID: "core:m", Name: "guard",
		DispositionRules: &mob.Definition{Default: mob.ReactionFriendly},
	}
	f := newFixture(t, tpl)
	p := PlayerView{ID: "p1", Name: "p1"}

	f.eval.Evaluate(context.Background(), f.inst, p, "core:r1", ModeFull)
	f.eval.ResetTick()
	f.eval.Evaluate(context.Background(), f.inst, p, "core:r1", ModeFull)

	if got := f.captureNames(); len(got) != 1 {
		t.Errorf("captured = %v, want 1 (per-room state should still suppress)", got)
	}
}

func TestEvaluate_PerRoomStateClearedByPlayerMoved(t *testing.T) {
	tpl := &mob.Template{
		ID: "core:m", Name: "guard",
		DispositionRules: &mob.Definition{Default: mob.ReactionFriendly},
	}
	f := newFixture(t, tpl)
	p := PlayerView{ID: "p1", Name: "p1"}

	f.eval.Evaluate(context.Background(), f.inst, p, "core:r1", ModeFull)
	// Player leaves r1 → state should clear, so re-entering and
	// re-evaluating produces a fresh dispatch.
	f.bus.Publish(context.Background(), eventbus.PlayerMoved{
		PlayerID: "p1", From: "core:r1", To: "core:r2",
	})
	f.eval.ResetTick()
	f.eval.Evaluate(context.Background(), f.inst, p, "core:r1", ModeFull)

	if got := f.captureNames(); len(got) != 2 {
		t.Errorf("captured = %v, want 2 (room-leave should clear state)", got)
	}
}

func TestEvaluate_HostileBypassesRoomStateSuppression(t *testing.T) {
	tpl := &mob.Template{
		ID: "core:m", Name: "boss", BaseDisposition: mob.ReactionHostile,
	}
	f := newFixture(t, tpl)
	p := PlayerView{ID: "p1", Name: "p1"}

	f.eval.Evaluate(context.Background(), f.inst, p, "core:r1", ModeFull)
	f.eval.ResetTick()
	f.eval.Evaluate(context.Background(), f.inst, p, "core:r1", ModeFull)
	f.eval.ResetTick()
	f.eval.Evaluate(context.Background(), f.inst, p, "core:r1", ModeFull)

	got := f.captureNames()
	if len(got) != 3 {
		t.Errorf("captured = %v, want 3 hostile emissions (no room suppression)", got)
	}
	for _, n := range got {
		if n != eventbus.EventMobAggro {
			t.Errorf("captured %q, want %q", n, eventbus.EventMobAggro)
		}
	}
}

func TestEvaluate_AggroOnlyModeSkipsNonHostile(t *testing.T) {
	tpl := &mob.Template{
		ID: "core:m", Name: "guard",
		DispositionRules: &mob.Definition{Default: mob.ReactionFriendly},
	}
	f := newFixture(t, tpl)
	p := PlayerView{ID: "p1", Name: "p1"}

	f.eval.Evaluate(context.Background(), f.inst, p, "core:r1", ModeAggroOnly)
	if got := f.captureNames(); len(got) != 0 {
		t.Errorf("captured = %v, want empty (aggro-only must drop friendly)", got)
	}
	// And the per-tick cache wasn't populated either, so the next
	// deferred call (full mode) still fires once.
	f.eval.Evaluate(context.Background(), f.inst, p, "core:r1", ModeFull)
	if got := f.captureNames(); len(got) != 1 || got[0] != eventbus.EventMobFriendly {
		t.Errorf("captured after deferred = %v, want one friendly", got)
	}
}

func TestEvaluate_AggroOnlyDispatchesHostile(t *testing.T) {
	tpl := &mob.Template{
		ID: "core:m", Name: "boss", BaseDisposition: mob.ReactionHostile,
	}
	f := newFixture(t, tpl)
	p := PlayerView{ID: "p1", Name: "p1"}

	f.eval.Evaluate(context.Background(), f.inst, p, "core:r1", ModeAggroOnly)
	if got := f.captureNames(); len(got) != 1 || got[0] != eventbus.EventMobAggro {
		t.Errorf("captured = %v, want one mob.aggro", got)
	}
}

func TestEvaluate_NeutralReactionEmitsNoEvent(t *testing.T) {
	tpl := &mob.Template{
		ID: "core:m", Name: "m",
		DispositionRules: &mob.Definition{Default: mob.ReactionNeutral},
	}
	f := newFixture(t, tpl)
	p := PlayerView{ID: "p1", Name: "p1"}

	f.eval.Evaluate(context.Background(), f.inst, p, "core:r1", ModeFull)
	if got := f.captureNames(); len(got) != 0 {
		t.Errorf("captured = %v, want none (neutral has no event)", got)
	}
}

// ---- room-entry hook integration tests ----

func TestOnPlayerEntered_ImmediateThenDeferredFiresOnce(t *testing.T) {
	tpl := &mob.Template{
		ID: "core:m", Name: "guard",
		DispositionRules: &mob.Definition{Default: mob.ReactionFriendly},
	}
	f := newFixture(t, tpl)
	p := PlayerView{ID: "p1", Name: "p1"}

	f.eval.OnPlayerEnteredImmediate(context.Background(), p, "core:r1")
	f.eval.OnPlayerEnteredDeferred(context.Background(), p, "core:r1")

	if got := f.captureNames(); len(got) != 1 || got[0] != eventbus.EventMobFriendly {
		t.Errorf("captured = %v, want one mob.friendly (immediate suppresses, deferred fires)", got)
	}
}

// TestPlayerMoved_FromEqualsToClearsRoomState covers the reconnect
// path: session.go publishes PlayerMoved with From == To so the
// evaluator forgets any stale per-room reaction state from the
// previous link-dead session.
func TestPlayerMoved_FromEqualsToClearsRoomState(t *testing.T) {
	tpl := &mob.Template{
		ID: "core:m", Name: "guard",
		DispositionRules: &mob.Definition{Default: mob.ReactionFriendly},
	}
	f := newFixture(t, tpl)
	p := PlayerView{ID: "p1", Name: "p1"}

	f.eval.Evaluate(context.Background(), f.inst, p, "core:r1", ModeFull)
	// Reconnect-style move: From == To. Should still clear state.
	f.bus.Publish(context.Background(), eventbus.PlayerMoved{
		PlayerID: "p1", From: "core:r1", To: "core:r1",
	})
	f.eval.ResetTick()
	f.eval.Evaluate(context.Background(), f.inst, p, "core:r1", ModeFull)

	if got := f.captureNames(); len(got) != 2 {
		t.Errorf("captured = %v, want 2 (From==To should still clear state)", got)
	}
}

// TestEvaluate_AtomicDedupUnderConcurrentCallers pins the H1 fix:
// two concurrent Evaluate calls on the same (mob, player) pair must
// produce exactly one dispatch. Pre-fix the lock-check was split
// from the lock-set so both could race past "seen?" and dispatch.
func TestEvaluate_AtomicDedupUnderConcurrentCallers(t *testing.T) {
	tpl := &mob.Template{
		ID: "core:m", Name: "boss", BaseDisposition: mob.ReactionHostile,
	}
	f := newFixture(t, tpl)
	p := PlayerView{ID: "p1", Name: "p1"}

	const goroutines = 32
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			<-start
			f.eval.Evaluate(context.Background(), f.inst, p, "core:r1", ModeFull)
		}()
	}
	close(start)
	wg.Wait()

	if got := f.captureNames(); len(got) != 1 {
		t.Errorf("captured = %d events, want exactly 1 (race protection)", len(got))
	}
}

func TestOnMobEntered_EvaluatesAgainstEveryPlayerInRoom(t *testing.T) {
	tpl := &mob.Template{
		ID: "core:m", Name: "guard", BaseDisposition: mob.ReactionHostile,
	}
	f := newFixture(t, tpl)
	f.players.byRoom["core:r1"] = []PlayerView{
		{ID: "p1", Name: "p1"},
		{ID: "p2", Name: "p2"},
	}

	f.eval.OnMobEntered(context.Background(), f.inst, "core:r1")

	if got := f.captureNames(); len(got) != 2 {
		t.Errorf("captured = %v, want 2 hostile (one per player)", got)
	}
}
