package progression

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// driverRig bundles the real managers + a recording sink so each
// test drives the §4.2 loop end-to-end through the production
// ValidationPipeline + AbilityResolver rather than fakes.
type driverRig struct {
	queue *ActionQueueManager
	prof  *profStub
	sink  *recordingSink
	phase combat.PhaseFunc
	src   *fakeSource
}

// newDriverRig wires a driver whose only ability-capable source is
// src, reachable under combatant id "player:p1". Abilities passed in
// are registered; the source's proficiency map is seeded so learned
// abilities pass the §4.3 proficiency gate.
func newDriverRig(t *testing.T, src *fakeSource, learned []string, abilities ...*Ability) *driverRig {
	t.Helper()
	reg := NewAbilityRegistry()
	for _, a := range abilities {
		if err := reg.Register(a); err != nil {
			t.Fatalf("register %q: %v", a.ID, err)
		}
	}
	prof := newProfStub()
	for _, id := range learned {
		prof.vals[id] = 1
	}
	queue := NewActionQueueManager(ActionQueueConfig{})
	pipeline := NewValidationPipeline(reg, prof, nil, nil, nil)
	sink := &recordingSink{}
	resolver := NewAbilityResolver(DefaultResolutionConfig(), prof, nil, nil, nil, nil, sink, nil)
	lookup := ResolutionSourceLookupFunc(func(id string) (ResolutionSource, bool) {
		if id == "player:p1" {
			return src, true
		}
		return nil, false
	})
	phase := NewAbilityPhaseDriver(queue, pipeline, resolver, lookup, sink, nil, nil, nil)
	return &driverRig{queue: queue, prof: prof, sink: sink, phase: phase, src: src}
}

// selfSpell is a non-offensive (spell, no effect) ability that
// resolves as a self-cast, so the §4.3 target/in-combat checks pass
// without a combat target wired into the fake source.
func selfSpell(id string) *Ability {
	return &Ability{ID: id, DisplayName: id, Type: AbilityActive, Category: AbilitySpell, Variance: 0}
}

// The driver invokes the overchannel handler — with the deficit captured by
// validation BEFORE the spend — when a flagged action resolves below the
// reserve. An ordinary (unflagged-sufficient) cast does not.
func TestDriver_OverchannelHandlerFires(t *testing.T) {
	reg := NewAbilityRegistry()
	weave := &Ability{ID: "weave", DisplayName: "Weave", Type: AbilityActive, Category: AbilitySpell, Cost: 20}
	if err := reg.Register(weave); err != nil {
		t.Fatalf("register: %v", err)
	}
	prof := newProfStub()
	prof.vals["weave"] = 1
	queue := NewActionQueueManager(ActionQueueConfig{})
	pipeline := NewValidationPipeline(reg, prof, nil, nil, nil) // reserveMultiple 1 → threshold = cost 20
	resolver := NewAbilityResolver(DefaultResolutionConfig(), prof, nil, nil, nil, nil, &recordingSink{}, nil)
	src := &fakeSource{id: "p1", mana: 8} // below the 20 threshold
	lookup := ResolutionSourceLookupFunc(func(id string) (ResolutionSource, bool) {
		return src, id == "player:p1"
	})
	var gotEntity, gotAbility string
	var gotDeficit int
	calls := 0
	phase := NewAbilityPhaseDriver(queue, pipeline, resolver, lookup, &recordingSink{},
		func(_ context.Context, entityID, abilityID string, deficit int) {
			calls++
			gotEntity, gotAbility, gotDeficit = entityID, abilityID, deficit
		}, nil, nil)

	queue.Push("p1", QueuedAction{AbilityID: "weave", Overchannel: true})
	phase(context.Background(), "player:p1", nil, 0)

	if calls != 1 {
		t.Fatalf("overchannel handler called %d times; want 1", calls)
	}
	if gotEntity != "p1" || gotAbility != "weave" || gotDeficit != 12 { // 20 − 8
		t.Fatalf("handler got (%q,%q,%d); want (p1,weave,12)", gotEntity, gotAbility, gotDeficit)
	}

	// A sufficient cast (mana ≥ threshold) does not fire the handler.
	src.mana = 20
	queue.Push("p1", QueuedAction{AbilityID: "weave", Overchannel: true})
	phase(context.Background(), "player:p1", nil, 0)
	if calls != 1 {
		t.Fatalf("handler fired on a sufficient cast; calls=%d want still 1", calls)
	}
}

// Under SpendOnSuccess, an overchanneled weave that MISSES draws no Power, so
// it carries no overchannel consequence — the handler must not fire (a miss
// cost tempo, not Power). Gated on ResolveOutcome.ResourceSpent > 0.
func TestDriver_OverchannelSkippedWhenMissDrewNothing(t *testing.T) {
	reg := NewAbilityRegistry()
	// variance > 0 so the roll can miss; prof 1 + high roll → miss.
	weave := &Ability{ID: "weave", DisplayName: "Weave", Type: AbilityActive, Category: AbilitySpell, Cost: 20, Variance: 50}
	if err := reg.Register(weave); err != nil {
		t.Fatalf("register: %v", err)
	}
	prof := newProfStub()
	prof.vals["weave"] = 1
	queue := NewActionQueueManager(ActionQueueConfig{})
	pipeline := NewValidationPipeline(reg, prof, nil, nil, nil)
	cfg := DefaultResolutionConfig()
	cfg.SpendOnSuccess = true
	roller := &seqRoller{t: t, seq: []int{99}} // forces a miss
	resolver := NewAbilityResolver(cfg, prof, nil, nil, nil, nil, &recordingSink{}, roller)
	src := &fakeSource{id: "p1", mana: 8} // below the 20 threshold → overchannel
	lookup := ResolutionSourceLookupFunc(func(id string) (ResolutionSource, bool) {
		return src, id == "player:p1"
	})
	calls := 0
	phase := NewAbilityPhaseDriver(queue, pipeline, resolver, lookup, &recordingSink{},
		func(context.Context, string, string, int) { calls++ }, nil, nil)

	queue.Push("p1", QueuedAction{AbilityID: "weave", Overchannel: true})
	phase(context.Background(), "player:p1", nil, 0)

	if calls != 0 {
		t.Fatalf("overchannel handler fired on a spend-on-success miss (no Power drawn); calls=%d want 0", calls)
	}
	if src.deductedMn != 0 {
		t.Fatalf("spend-on-success miss must draw no mana; deductedMn=%d", src.deductedMn)
	}
}

func TestDriver_EmptyQueueNoOp(t *testing.T) {
	rig := newDriverRig(t, &fakeSource{id: "p1"}, []string{"heal"}, selfSpell("heal"))
	rig.phase(context.Background(), "player:p1", nil, 0)
	if len(rig.sink.used)+len(rig.sink.missed)+len(rig.sink.fizzled) != 0 {
		t.Fatalf("empty queue must emit nothing")
	}
}

func TestDriver_UnknownSourceNoOp(t *testing.T) {
	rig := newDriverRig(t, &fakeSource{id: "p1"}, []string{"heal"}, selfSpell("heal"))
	// Push a valid entry, then drive an unrelated combatant id.
	rig.queue.Push("p1", QueuedAction{AbilityID: "heal"})
	rig.phase(context.Background(), "mob:wolf-1", nil, 0)
	if rig.queue.Len("p1") != 1 {
		t.Fatalf("unknown source must not touch p1's queue")
	}
	if len(rig.sink.used) != 0 {
		t.Fatalf("unknown source must resolve nothing")
	}
}

func TestDriver_ValidResolvesDropsStops(t *testing.T) {
	rig := newDriverRig(t, &fakeSource{id: "p1"}, []string{"heal"}, selfSpell("heal"))
	rig.queue.Push("p1", QueuedAction{AbilityID: "heal"})

	rig.phase(context.Background(), "player:p1", nil, 0)

	if rig.queue.Len("p1") != 0 {
		t.Fatalf("resolved entry must be dropped, queue len=%d", rig.queue.Len("p1"))
	}
	if len(rig.sink.used) != 1 || rig.sink.used[0].AbilityID != "heal" {
		t.Fatalf("want 1 used(heal), got %+v", rig.sink.used)
	}
}

func TestDriver_FizzleDropsAndContinuesToValid(t *testing.T) {
	src := &fakeSource{id: "p1"}
	// Only "heal" is learned; "cure" is registered but not learned, so
	// it fizzles no_proficiency. The fizzle must NOT consume the slot —
	// the loop should continue to "heal" and resolve it.
	rig := newDriverRig(t, src, []string{"heal"}, selfSpell("heal"), selfSpell("cure"))
	rig.queue.Push("p1", QueuedAction{AbilityID: "cure"})
	rig.queue.Push("p1", QueuedAction{AbilityID: "heal"})

	rig.phase(context.Background(), "player:p1", nil, 0)

	if rig.queue.Len("p1") != 0 {
		t.Fatalf("both entries should be consumed, queue len=%d", rig.queue.Len("p1"))
	}
	if len(rig.sink.fizzled) != 1 || rig.sink.fizzled[0].Reason != FizzleNoProficiency {
		t.Fatalf("want 1 no_proficiency fizzle, got %+v", rig.sink.fizzled)
	}
	if len(rig.sink.used) != 1 || rig.sink.used[0].AbilityID != "heal" {
		t.Fatalf("want heal resolved after fizzle, got %+v", rig.sink.used)
	}
}

func TestDriver_AtMostOneValidExecutionPerPulse(t *testing.T) {
	rig := newDriverRig(t, &fakeSource{id: "p1"}, []string{"heal", "bless"},
		selfSpell("heal"), selfSpell("bless"))
	rig.queue.Push("p1", QueuedAction{AbilityID: "heal"})
	rig.queue.Push("p1", QueuedAction{AbilityID: "bless"})

	rig.phase(context.Background(), "player:p1", nil, 0)

	// First valid entry resolves and stops; the second stays queued
	// for the next pulse (spec §4.2 "at most one valid execution per
	// entity per pulse").
	if rig.queue.Len("p1") != 1 {
		t.Fatalf("second valid entry should remain queued, len=%d", rig.queue.Len("p1"))
	}
	if len(rig.sink.used) != 1 || rig.sink.used[0].AbilityID != "heal" {
		t.Fatalf("want exactly heal resolved, got %+v", rig.sink.used)
	}
	remaining, _ := rig.queue.Peek("p1")
	if remaining.AbilityID != "bless" {
		t.Fatalf("want bless still at front, got %q", remaining.AbilityID)
	}
}

func TestDriver_UnknownAbilityFizzles(t *testing.T) {
	rig := newDriverRig(t, &fakeSource{id: "p1"}, nil, selfSpell("heal"))
	rig.queue.Push("p1", QueuedAction{AbilityID: "nonexistent"})

	rig.phase(context.Background(), "player:p1", nil, 0)

	if rig.queue.Len("p1") != 0 {
		t.Fatalf("unknown ability entry must be dropped")
	}
	if len(rig.sink.fizzled) != 1 || rig.sink.fizzled[0].Reason != FizzleUnknownAbility {
		t.Fatalf("want unknown_ability fizzle, got %+v", rig.sink.fizzled)
	}
	if rig.sink.fizzled[0].AbilityID != "nonexistent" {
		t.Fatalf("unknown-ability fizzle should carry the raw id, got %q", rig.sink.fizzled[0].AbilityID)
	}
}

// TestDriver_ThroughCombatHeartbeat is the M9.4b integration test:
// it drives a queued ability through the REAL combat.Heartbeat so
// the full PhaseFunc path (round snapshot → InCombat gate → ability
// phase → driver → resolver) is exercised, including the tick-count
// → pulse threading the heartbeat performs.
func TestDriver_ThroughCombatHeartbeat(t *testing.T) {
	mgr := combat.NewManager(nil, nil)
	ctx := context.Background()
	player := combat.NewPlayerCombatantID("p1")
	mob := combat.NewMobCombatantID("m1")
	mgr.Engage(ctx, player, mob, world.RoomID("room-1"))

	reg := NewAbilityRegistry()
	if err := reg.Register(selfSpell("heal")); err != nil {
		t.Fatal(err)
	}
	prof := newProfStub()
	prof.vals["heal"] = 1
	queue := NewActionQueueManager(ActionQueueConfig{})
	tracker := NewPulseDelayTracker()
	sink := &recordingSink{}
	pipeline := NewValidationPipeline(reg, prof, nil, tracker, nil)
	resolver := NewAbilityResolver(DefaultResolutionConfig(), prof, nil, tracker, nil, nil, sink, nil)
	src := &fakeSource{id: "p1"}
	sources := ResolutionSourceLookupFunc(func(id string) (ResolutionSource, bool) {
		if id == string(player) {
			return src, true
		}
		return nil, false // mob is not an ability source
	})
	phase := NewAbilityPhaseDriver(queue, pipeline, resolver, sources, sink, nil, nil, nil)

	hb := combat.NewHeartbeat(mgr, combat.Phases{Ability: phase})
	queue.Push("p1", QueuedAction{AbilityID: "heal"})

	hb.Tick(ctx, 9) // pulse 9

	if queue.Len("p1") != 0 {
		t.Fatalf("heal should resolve + drain through the heartbeat, len=%d", queue.Len("p1"))
	}
	if len(sink.used) != 1 || sink.used[0].AbilityID != "heal" {
		t.Fatalf("want heal used via heartbeat, got %+v", sink.used)
	}
}

func TestDriver_PulseThreadedToResolver(t *testing.T) {
	// A pulse-delay ability records next-ready = pulse + delay + 1.
	// Driving at pulse 7 with delay 2 must record readyAt 10, proving
	// the heartbeat's tick count threads through to the resolver.
	tracker := NewPulseDelayTracker()
	reg := NewAbilityRegistry()
	ab := &Ability{ID: "heal", DisplayName: "heal", Type: AbilityActive, Category: AbilitySpell, PulseDelay: 2}
	if err := reg.Register(ab); err != nil {
		t.Fatal(err)
	}
	prof := newProfStub()
	prof.vals["heal"] = 1
	queue := NewActionQueueManager(ActionQueueConfig{})
	pipeline := NewValidationPipeline(reg, prof, nil, tracker, nil)
	sink := &recordingSink{}
	resolver := NewAbilityResolver(DefaultResolutionConfig(), prof, nil, tracker, nil, nil, sink, nil)
	src := &fakeSource{id: "p1"}
	lookup := ResolutionSourceLookupFunc(func(id string) (ResolutionSource, bool) {
		return src, id == "player:p1"
	})
	phase := NewAbilityPhaseDriver(queue, pipeline, resolver, lookup, sink, nil, nil, nil)
	queue.Push("p1", QueuedAction{AbilityID: "heal"})

	phase(context.Background(), "player:p1", nil, 7)

	readyAt, ok := tracker.ReadyAt("p1", "heal")
	if !ok || readyAt != 10 {
		t.Fatalf("want readyAt 10 (pulse 7 + delay 2 + 1), got %d ok=%v", readyAt, ok)
	}
}

// --- WoT S2: the channel interrupt game (timed casts) -----------------------

// recordingCastNotifier captures cast-lifecycle emissions.
type recordingCastNotifier struct {
	began       []CastBeganEvent
	interrupted []CastInterruptedEvent
}

func (n *recordingCastNotifier) OnCastBegan(_ context.Context, ev CastBeganEvent) {
	n.began = append(n.began, ev)
}
func (n *recordingCastNotifier) OnCastInterrupted(_ context.Context, ev CastInterruptedEvent) {
	n.interrupted = append(n.interrupted, ev)
}

// newTimedDriverRig is newDriverRig with a CastTracker + recording notifier
// wired, so timed (cast_time > 0) weaves exercise the warmup state machine.
func newTimedDriverRig(t *testing.T, src *fakeSource, learned []string, abilities ...*Ability) (combat.PhaseFunc, *ActionQueueManager, *CastTracker, *recordingCastNotifier, *recordingSink) {
	t.Helper()
	reg := NewAbilityRegistry()
	for _, a := range abilities {
		if err := reg.Register(a); err != nil {
			t.Fatalf("register %q: %v", a.ID, err)
		}
	}
	prof := newProfStub()
	for _, id := range learned {
		prof.vals[id] = 1
	}
	queue := NewActionQueueManager(ActionQueueConfig{})
	pipeline := NewValidationPipeline(reg, prof, nil, nil, nil)
	sink := &recordingSink{}
	resolver := NewAbilityResolver(DefaultResolutionConfig(), prof, nil, nil, nil, nil, sink, nil)
	lookup := ResolutionSourceLookupFunc(func(id string) (ResolutionSource, bool) {
		return src, id == "player:p1"
	})
	casts := NewCastTracker()
	notifier := &recordingCastNotifier{}
	phase := NewAbilityPhaseDriver(queue, pipeline, resolver, lookup, sink, nil, casts, notifier)
	return phase, queue, casts, notifier, sink
}

// A weave with cast_time > 0 BEGINS a warmup instead of resolving in the pulse
// it is validated: the begin event fires, the queue drains into the tracker,
// and no used event is emitted yet.
func TestDriver_TimedCastBeginsNotResolveImmediately(t *testing.T) {
	src := &fakeSource{id: "p1", mana: 100}
	weave := &Ability{ID: "firebolt", DisplayName: "Firebolt", Type: AbilityActive, Category: AbilitySpell, Variance: 0, CastTime: 2}
	phase, queue, casts, notifier, sink := newTimedDriverRig(t, src, []string{"firebolt"}, weave)

	queue.Push("p1", QueuedAction{AbilityID: "firebolt"})
	phase(context.Background(), "player:p1", nil, 0)

	if len(notifier.began) != 1 || notifier.began[0].AbilityID != "firebolt" || notifier.began[0].Rounds != 2 {
		t.Fatalf("begin event = %+v; want one firebolt/2", notifier.began)
	}
	if len(sink.used) != 0 {
		t.Fatalf("a timed cast must NOT resolve on begin; got used=%+v", sink.used)
	}
	if !casts.IsCasting("p1") {
		t.Fatal("entity should be mid-cast after begin")
	}
	if queue.Len("p1") != 0 {
		t.Fatal("the queue entry should be consumed at begin")
	}
}

// Across rounds the warmup counts down and the weave resolves only when it
// elapses — one used event, at the end, and the tracker clears.
func TestDriver_TimedCastResolvesAfterWarmup(t *testing.T) {
	src := &fakeSource{id: "p1", mana: 100}
	weave := &Ability{ID: "firebolt", DisplayName: "Firebolt", Type: AbilityActive, Category: AbilitySpell, Variance: 0, CastTime: 2}
	phase, queue, casts, _, sink := newTimedDriverRig(t, src, []string{"firebolt"}, weave)

	queue.Push("p1", QueuedAction{AbilityID: "firebolt"})
	phase(context.Background(), "player:p1", nil, 0) // begin (remaining 2)
	phase(context.Background(), "player:p1", nil, 1) // warmup (remaining 1) — still no resolve
	if len(sink.used) != 0 {
		t.Fatalf("weave resolved mid-warmup; used=%+v", sink.used)
	}
	phase(context.Background(), "player:p1", nil, 2) // remaining 0 → resolve

	if len(sink.used) != 1 || sink.used[0].AbilityID != "firebolt" {
		t.Fatalf("weave should resolve once after warmup; used=%+v", sink.used)
	}
	if casts.IsCasting("p1") {
		t.Fatal("tracker should clear once the weave resolves")
	}
}

// An interrupted weave never resolves: clearing the tracker mid-warmup means
// the next ability phase finds nothing to advance and emits no used event.
func TestDriver_InterruptedTimedCastDoesNotResolve(t *testing.T) {
	src := &fakeSource{id: "p1", mana: 100}
	weave := &Ability{ID: "firebolt", DisplayName: "Firebolt", Type: AbilityActive, Category: AbilitySpell, Variance: 0, CastTime: 2}
	phase, queue, casts, _, sink := newTimedDriverRig(t, src, []string{"firebolt"}, weave)

	queue.Push("p1", QueuedAction{AbilityID: "firebolt"})
	phase(context.Background(), "player:p1", nil, 0) // begin
	if _, ok := casts.Interrupt("p1"); !ok {           // a hit lands (slice 2 trigger, simulated)
		t.Fatal("expected an in-flight cast to interrupt")
	}
	phase(context.Background(), "player:p1", nil, 1) // nothing in flight → no-op
	phase(context.Background(), "player:p1", nil, 2)

	if len(sink.used) != 0 {
		t.Fatalf("an interrupted weave must not resolve; used=%+v", sink.used)
	}
}

// An instant (cast_time 0) ability still resolves in the pulse it validates,
// even with the cast tracker wired — timed casts are opt-in per ability.
func TestDriver_InstantAbilityUnaffectedByCastTracker(t *testing.T) {
	src := &fakeSource{id: "p1", mana: 100}
	instant := selfSpell("bless") // CastTime 0
	phase, queue, casts, notifier, sink := newTimedDriverRig(t, src, []string{"bless"}, instant)

	queue.Push("p1", QueuedAction{AbilityID: "bless"})
	phase(context.Background(), "player:p1", nil, 0)

	if len(sink.used) != 1 {
		t.Fatalf("instant ability should resolve immediately; used=%+v", sink.used)
	}
	if len(notifier.began) != 0 {
		t.Fatalf("instant ability should not emit a begin event; began=%+v", notifier.began)
	}
	if casts.IsCasting("p1") {
		t.Fatal("instant ability should not enter the cast tracker")
	}
}
