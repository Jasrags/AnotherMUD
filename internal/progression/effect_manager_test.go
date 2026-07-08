package progression_test

import (
	"context"
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/stats"
)

// recTarget records every Add/Remove call so tests can assert
// source-key dedup behavior + reversal correctness.
type recTarget struct {
	id      string
	mu      sync.Mutex
	mods    map[entities.SourceKey][]stats.Modifier
	removed []entities.SourceKey
}

func newRecTarget(id string) *recTarget {
	return &recTarget{id: id, mods: make(map[entities.SourceKey][]stats.Modifier)}
}

func (t *recTarget) EntityID() string { return t.id }

func (t *recTarget) AddModifiers(src entities.SourceKey, mods []stats.Modifier) {
	t.mu.Lock()
	defer t.mu.Unlock()
	cp := make([]stats.Modifier, len(mods))
	copy(cp, mods)
	t.mods[src] = cp
}

func (t *recTarget) RemoveBySource(src entities.SourceKey) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.mods[src]; !ok {
		return false
	}
	delete(t.mods, src)
	t.removed = append(t.removed, src)
	return true
}

func (t *recTarget) hasSource(src entities.SourceKey) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.mods[src]
	return ok
}

// recSink captures every applied/removed/expired event so tests
// can assert payloads + emission discipline (single-instance
// refusals MUST NOT emit applied, etc.).
type recSink struct {
	mu      sync.Mutex
	applied []progression.EffectAppliedEvent
	removed []progression.EffectRemovedEvent
	expired []progression.EffectExpiredEvent
}

func (s *recSink) EffectApplied(_ context.Context, ev progression.EffectAppliedEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applied = append(s.applied, ev)
}

func (s *recSink) EffectRemoved(_ context.Context, ev progression.EffectRemovedEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removed = append(s.removed, ev)
}

func (s *recSink) EffectExpired(_ context.Context, ev progression.EffectExpiredEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expired = append(s.expired, ev)
}

func newManagerForTarget(tgt *recTarget, sink *recSink) *progression.EffectManager {
	// sink is taken as concrete *recSink so callers can pass nil
	// without minting an empty fake; we hoist the typed nil into a
	// nil interface here to avoid the classic typed-nil/non-nil-
	// interface trap that would make m.sink != nil true.
	var es progression.EffectSink
	if sink != nil {
		es = sink
	}
	return progression.NewEffectManager(progression.TargetResolverFunc(func(id string) (progression.EffectTarget, bool) {
		if id == tgt.EntityID() {
			return tgt, true
		}
		return nil, false
	}), es)
}

func TestEffectManager_ApplyInstallsModifiersAndEmits(t *testing.T) {
	tgt := newRecTarget("p-1")
	sink := &recSink{}
	m := newManagerForTarget(tgt, sink)

	ok := m.Apply(context.Background(), "P-1",
		progression.EffectTemplate{
			ID: "bless", Duration: 10,
			Modifiers: []stats.Modifier{{Stat: "STR", Value: 2}},
			Flags:     []string{"Blessed"},
		}, "caster", "spell.bless")
	if !ok {
		t.Fatalf("Apply returned false")
	}
	src := progression.EffectSourceKey("bless")
	if !tgt.hasSource(src) {
		t.Errorf("modifiers not installed under %s", src)
	}
	if got := tgt.mods[src]; len(got) != 1 || got[0].Stat != "str" || got[0].Value != 2 {
		t.Errorf("modifiers = %+v, want [{str 2}] (lowercased)", got)
	}
	if !m.Has("p-1", "BLESS") {
		t.Errorf("Has(BLESS) = false after Apply (case-insensitive)")
	}
	if !m.HasFlag("p-1", "blessed") {
		t.Errorf("HasFlag(blessed) = false (flag lowercased)")
	}
	if len(sink.applied) != 1 {
		t.Fatalf("applied events = %d, want 1", len(sink.applied))
	}
	ev := sink.applied[0]
	if ev.EntityID != "p-1" || ev.EffectID != "bless" || ev.SourceAbilityID != "spell.bless" || ev.Duration != 10 {
		t.Errorf("applied payload = %+v", ev)
	}
}

func TestEffectManager_SingleInstanceRefusesReapply(t *testing.T) {
	tgt := newRecTarget("p-1")
	sink := &recSink{}
	m := newManagerForTarget(tgt, sink)

	tpl := progression.EffectTemplate{
		ID: "bless", Duration: 10,
		Modifiers: []stats.Modifier{{Stat: "str", Value: 2}},
	}
	if !m.Apply(context.Background(), "p-1", tpl, "", "") {
		t.Fatal("first Apply returned false")
	}
	if m.Apply(context.Background(), "p-1", tpl, "", "") {
		t.Errorf("second Apply returned true; spec §5.2 requires refusal")
	}
	if len(sink.applied) != 1 {
		t.Errorf("applied events = %d, want 1 (refusal must not emit)", len(sink.applied))
	}
	// Stat mods unchanged — refusal is mutation-free.
	if got := tgt.mods[progression.EffectSourceKey("bless")]; len(got) != 1 || got[0].Value != 2 {
		t.Errorf("modifiers after refusal = %+v", got)
	}
}

func TestEffectManager_RefreshableReapplyResetsDuration(t *testing.T) {
	tgt := newRecTarget("p-1")
	m := newManagerForTarget(tgt, nil)

	tpl := progression.EffectTemplate{
		ID: "well-fed", Duration: 600, Refreshable: true,
		Modifiers: []stats.Modifier{{Stat: "str", Value: 2}},
	}
	if !m.Apply(context.Background(), "p-1", tpl, "", "") {
		t.Fatal("first Apply returned false")
	}
	// Burn the duration down so a refresh is observable.
	for range 100 {
		m.Tick(context.Background())
	}
	if got := m.Effects("p-1")[0].Remaining; got != 500 {
		t.Fatalf("remaining after 100 ticks = %d, want 500", got)
	}

	// Re-apply a refreshable effect: returns true and resets the timer.
	if !m.Apply(context.Background(), "p-1", tpl, "", "") {
		t.Error("refreshable re-apply returned false; want true (refreshed)")
	}
	if got := m.Effects("p-1")[0].Remaining; got != 600 {
		t.Errorf("remaining after refresh = %d, want 600 (reset to Duration)", got)
	}
	// Still a single instance — refresh must not stack.
	if n := len(m.Effects("p-1")); n != 1 {
		t.Errorf("effect count after refresh = %d, want 1 (no stacking)", n)
	}
	// The str modifier is unchanged (one source key, never doubled).
	if got := tgt.mods[progression.EffectSourceKey("well-fed")]; len(got) != 1 || got[0].Value != 2 {
		t.Errorf("modifiers after refresh = %+v, want a single {str 2}", got)
	}
}

func TestEffectManager_NonRefreshableReapplyStillIgnored(t *testing.T) {
	tgt := newRecTarget("p-1")
	m := newManagerForTarget(tgt, nil)

	tpl := progression.EffectTemplate{ID: "bless", Duration: 10} // Refreshable defaults false
	m.Apply(context.Background(), "p-1", tpl, "", "")
	m.Tick(context.Background()) // remaining 9
	if m.Apply(context.Background(), "p-1", tpl, "", "") {
		t.Error("non-refreshable re-apply returned true; want false (ignored)")
	}
	if got := m.Effects("p-1")[0].Remaining; got != 9 {
		t.Errorf("remaining = %d, want 9 (ignored re-apply must not reset)", got)
	}
}

func TestEffectManager_RemoveByIDReversesAndEmits(t *testing.T) {
	tgt := newRecTarget("p-1")
	sink := &recSink{}
	m := newManagerForTarget(tgt, sink)

	m.Apply(context.Background(), "p-1", progression.EffectTemplate{
		ID: "bless", Duration: 10,
		Modifiers: []stats.Modifier{{Stat: "str", Value: 2}},
	}, "", "spell.bless")

	if !m.RemoveByID(context.Background(), "p-1", "BLESS") {
		t.Fatalf("RemoveByID returned false")
	}
	if tgt.hasSource(progression.EffectSourceKey("bless")) {
		t.Errorf("modifiers not removed under effect source key")
	}
	if m.Has("p-1", "bless") {
		t.Errorf("Has(bless) = true after RemoveByID")
	}
	if len(sink.removed) != 1 {
		t.Fatalf("removed events = %d, want 1", len(sink.removed))
	}
	if sink.removed[0].SourceAbilityID != "spell.bless" {
		t.Errorf("removed.SourceAbilityID = %q, want spell.bless", sink.removed[0].SourceAbilityID)
	}
	// Removing an unknown id is a silent no-op (spec §5.3).
	if m.RemoveByID(context.Background(), "p-1", "ghost") {
		t.Errorf("RemoveByID(ghost) returned true; spec requires silent no-op")
	}
	if len(sink.removed) != 1 {
		t.Errorf("removed events grew on unknown id removal: %d", len(sink.removed))
	}
}

func TestEffectManager_RemoveByFlagBatchesEveryMatch(t *testing.T) {
	tgt := newRecTarget("p-1")
	sink := &recSink{}
	m := newManagerForTarget(tgt, sink)

	m.Apply(context.Background(), "p-1", progression.EffectTemplate{ID: "bless", Duration: 10, Flags: []string{"buff", "holy"}}, "", "")
	m.Apply(context.Background(), "p-1", progression.EffectTemplate{ID: "shield", Duration: 10, Flags: []string{"buff"}}, "", "")
	m.Apply(context.Background(), "p-1", progression.EffectTemplate{ID: "poison", Duration: 10, Flags: []string{"debuff"}}, "", "")

	n := m.RemoveByFlag(context.Background(), "p-1", "BUFF")
	if n != 2 {
		t.Errorf("RemoveByFlag(buff) = %d, want 2", n)
	}
	if m.Has("p-1", "bless") || m.Has("p-1", "shield") {
		t.Errorf("buff effects remained after RemoveByFlag")
	}
	if !m.Has("p-1", "poison") {
		t.Errorf("non-matching effect was removed")
	}
	if len(sink.removed) != 2 {
		t.Errorf("removed events = %d, want 2", len(sink.removed))
	}
	// Removing an absent flag is a no-op.
	if got := m.RemoveByFlag(context.Background(), "p-1", "ghost"); got != 0 {
		t.Errorf("RemoveByFlag(ghost) = %d, want 0", got)
	}
}

func TestEffectManager_TickDecrementsAndExpires(t *testing.T) {
	tgt := newRecTarget("p-1")
	sink := &recSink{}
	m := newManagerForTarget(tgt, sink)

	m.Apply(context.Background(), "p-1", progression.EffectTemplate{ID: "short", Duration: 2,
		Modifiers: []stats.Modifier{{Stat: "str", Value: 1}}}, "", "ability.short")
	m.Apply(context.Background(), "p-1", progression.EffectTemplate{ID: "long", Duration: 10}, "", "ability.long")
	m.Apply(context.Background(), "p-1", progression.EffectTemplate{ID: "permanent", Duration: -1,
		Modifiers: []stats.Modifier{{Stat: "dex", Value: 1}}}, "", "world.perm")

	ctx := context.Background()
	m.Tick(ctx) // short:1, long:9, perm:-1
	if len(sink.expired) != 0 {
		t.Errorf("expired after first tick: %d", len(sink.expired))
	}
	m.Tick(ctx) // short:0 -> expire
	if len(sink.expired) != 1 {
		t.Fatalf("expired = %d, want 1", len(sink.expired))
	}
	if sink.expired[0].EffectID != "short" {
		t.Errorf("expired effect = %q, want short", sink.expired[0].EffectID)
	}
	if tgt.hasSource(progression.EffectSourceKey("short")) {
		t.Errorf("short modifiers not reversed on expire")
	}
	if !tgt.hasSource(progression.EffectSourceKey("permanent")) {
		t.Errorf("permanent modifiers reversed unexpectedly")
	}
	// Permanent doesn't tick. After many ticks long expires
	// eventually; permanent stays.
	for range 100 {
		m.Tick(ctx)
	}
	if m.Has("p-1", "long") {
		t.Errorf("long survived %d ticks past duration", 100)
	}
	if !m.Has("p-1", "permanent") {
		t.Errorf("permanent expired after 100 ticks")
	}
	// EffectRemoved should never have fired — every removal in
	// this test was via Tick.
	if len(sink.removed) != 0 {
		t.Errorf("removed events = %d, want 0 (only expires fired)", len(sink.removed))
	}
}

func TestEffectManager_EffectsSnapshotIsDeepCopy(t *testing.T) {
	tgt := newRecTarget("p-1")
	m := newManagerForTarget(tgt, nil)
	m.Apply(context.Background(), "p-1", progression.EffectTemplate{
		ID: "bless", Duration: 5,
		Modifiers: []stats.Modifier{{Stat: "str", Value: 2}},
		Flags:     []string{"buff"},
	}, "", "spell.bless")

	snap := m.Effects("p-1")
	if len(snap) != 1 {
		t.Fatalf("Effects len = %d, want 1", len(snap))
	}
	snap[0].Modifiers[0].Value = 999
	snap[0].Flags[0] = "tampered"
	snap[0].Remaining = -42

	again := m.Effects("p-1")[0]
	if again.Modifiers[0].Value != 2 {
		t.Errorf("snapshot mutation leaked: modifier value = %d", again.Modifiers[0].Value)
	}
	if again.Flags[0] != "buff" {
		t.Errorf("snapshot mutation leaked: flag = %q", again.Flags[0])
	}
	if again.Remaining != 5 {
		t.Errorf("snapshot mutation leaked: remaining = %d", again.Remaining)
	}
}

func TestEffectManager_FlagsAggregatesAcrossEffects(t *testing.T) {
	tgt := newRecTarget("p-1")
	m := newManagerForTarget(tgt, nil)
	m.Apply(context.Background(), "p-1", progression.EffectTemplate{ID: "bless", Duration: 5, Flags: []string{"buff", "holy"}}, "", "")
	m.Apply(context.Background(), "p-1", progression.EffectTemplate{ID: "shield", Duration: 5, Flags: []string{"buff"}}, "", "")

	got := m.Flags("p-1")
	want := []string{"buff", "holy"}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Flags = %v, want %v (deduped + sorted)", got, want)
	}
}

func TestEffectManager_DropClearsWithoutEvents(t *testing.T) {
	tgt := newRecTarget("p-1")
	sink := &recSink{}
	m := newManagerForTarget(tgt, sink)
	m.Apply(context.Background(), "p-1", progression.EffectTemplate{ID: "bless", Duration: 5}, "", "")
	m.Apply(context.Background(), "p-1", progression.EffectTemplate{ID: "shield", Duration: 5}, "", "")

	n := m.Drop("p-1")
	if n != 2 {
		t.Errorf("Drop = %d, want 2", n)
	}
	if m.Has("p-1", "bless") {
		t.Errorf("effect survived Drop")
	}
	if len(sink.removed) != 0 || len(sink.expired) != 0 {
		t.Errorf("Drop emitted events: removed=%d expired=%d", len(sink.removed), len(sink.expired))
	}
}

func TestEffectManager_NilResolverApplyStillTracksMetadata(t *testing.T) {
	// Manager without a resolver: stat mods can't reach a target,
	// but the active-list bookkeeping + events still fire. Used in
	// tests that exercise effect identity without a stat block.
	sink := &recSink{}
	m := progression.NewEffectManager(nil, sink)
	if !m.Apply(context.Background(), "p-1", progression.EffectTemplate{ID: "bless", Duration: 5}, "", "") {
		t.Fatalf("Apply returned false with nil resolver")
	}
	if !m.Has("p-1", "bless") {
		t.Errorf("Has = false after nil-resolver Apply")
	}
	if len(sink.applied) != 1 {
		t.Errorf("applied events = %d, want 1", len(sink.applied))
	}
}

func TestEffectManager_EmptyAndUnknownInputsAreNoops(t *testing.T) {
	m := progression.NewEffectManager(nil, nil)
	if m.Apply(context.Background(), "", progression.EffectTemplate{ID: "bless", Duration: 5}, "", "") {
		t.Errorf("Apply with empty entity returned true")
	}
	if m.Apply(context.Background(), "p-1", progression.EffectTemplate{ID: "", Duration: 5}, "", "") {
		t.Errorf("Apply with empty effect id returned true")
	}
	if m.RemoveByID(context.Background(), "", "x") {
		t.Errorf("RemoveByID empty entity returned true")
	}
	if got := m.RemoveByFlag(context.Background(), "", "x"); got != 0 {
		t.Errorf("RemoveByFlag empty entity returned %d", got)
	}
	if got := m.Flags(""); got != nil {
		t.Errorf("Flags('') = %v, want nil", got)
	}
}

func TestEffectManager_TickConcurrentRemoveIsSafe(t *testing.T) {
	// Pin spec §5.4 last paragraph: "Tick MUST NOT mutate the
	// active-effect list during iteration; expirations are batched
	// and applied afterward." This test races Tick with RemoveByID
	// to drive the -race detector through both code paths.
	tgt := newRecTarget("p-1")
	m := newManagerForTarget(tgt, nil)
	for i := range 50 {
		m.Apply(context.Background(), "p-1", progression.EffectTemplate{
			ID:        "eff-" + idStr(i),
			Duration:  3,
			Modifiers: []stats.Modifier{{Stat: "str", Value: 1}},
		}, "", "")
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 50 {
			m.Tick(context.Background())
		}
	}()
	go func() {
		defer wg.Done()
		for i := range 50 {
			m.RemoveByID(context.Background(), "p-1", "eff-"+idStr(i))
		}
	}()
	wg.Wait()
	// No assertions on remaining state — the goal is just that the
	// race detector observes no concurrent map / slice mutation.
}

func idStr(i int) string {
	const digits = "0123456789"
	if i < 10 {
		return string(digits[i])
	}
	return string(digits[i/10]) + string(digits[i%10])
}

// --- conditions §4: per-tick shake-off (recurring) save -------------------

// scriptedSaveResolver returns a programmed pass/fail per call and records the
// (entityID, axis, dc, cause) it was asked to resolve.
type scriptedSaveResolver struct {
	mu      sync.Mutex
	results []bool // consumed in order; exhausted ⇒ false (never saves)
	idx     int
	calls   []saveCall
}

type saveCall struct {
	entityID string
	axis     progression.SaveType
	dc       int
	cause    string
}

func (r *scriptedSaveResolver) ResolveSave(_ context.Context, entityID string, axis progression.SaveType, dc int, cause string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, saveCall{entityID, axis, dc, cause})
	made := false
	if r.idx < len(r.results) {
		made = r.results[r.idx]
		r.idx++
	}
	return made
}

func (r *scriptedSaveResolver) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func stunTemplate(dur int) progression.EffectTemplate {
	return progression.EffectTemplate{
		ID:            "stunned",
		Duration:      dur,
		Flags:         []string{"condition:stunned"},
		Modifiers:     []stats.Modifier{{Stat: "dex", Value: -2}},
		RecurringSave: &progression.ConditionSave{Axis: progression.SaveFortitude, DC: 15},
	}
}

func TestEffectManager_RecurringSaveRemovesOnMadeSave(t *testing.T) {
	tgt := newRecTarget("p-1")
	sink := &recSink{}
	m := newManagerForTarget(tgt, sink)
	// First tick: save fails (effect persists). Second tick: save succeeds.
	res := &scriptedSaveResolver{results: []bool{false, true}}
	m.SetSaveResolver(res)

	ctx := context.Background()
	if !m.Apply(ctx, "p-1", stunTemplate(10), "mob:ogre", "") {
		t.Fatal("apply failed")
	}
	if !tgt.hasSource(progression.EffectSourceKey("stunned")) {
		t.Fatal("stun modifiers not installed")
	}

	m.Tick(ctx) // fails the save → still stunned
	if !m.Has("p-1", "stunned") {
		t.Fatal("stun removed on a FAILED shake-off save")
	}

	m.Tick(ctx) // makes the save → shaken off early
	if m.Has("p-1", "stunned") {
		t.Error("stun survived a MADE shake-off save")
	}
	if tgt.hasSource(progression.EffectSourceKey("stunned")) {
		t.Error("stun modifiers not reversed after shake-off")
	}
	// Removed via RemoveByID (not expiry) → an EffectRemoved, no EffectExpired.
	if len(sink.removed) != 1 || sink.removed[0].EffectID != "stunned" {
		t.Errorf("removed events = %+v, want one 'stunned'", sink.removed)
	}
	if len(sink.expired) != 0 {
		t.Errorf("expired events = %d, want 0 (shaken off, not expired)", len(sink.expired))
	}
	// The resolver saw exactly the two ticks' worth of saves, keyed right.
	if res.callCount() != 2 {
		t.Fatalf("resolver calls = %d, want 2", res.callCount())
	}
	c := res.calls[0]
	if c.entityID != "p-1" || c.axis != progression.SaveFortitude || c.dc != 15 || c.cause != "stunned" {
		t.Errorf("save call = %+v, want {p-1 fortitude 15 stunned}", c)
	}
}

func TestEffectManager_RecurringSaveNoResolverRunsFullDuration(t *testing.T) {
	tgt := newRecTarget("p-1")
	m := newManagerForTarget(tgt, nil)
	// No SetSaveResolver → the shake-off save can't roll; effect runs out.
	ctx := context.Background()
	m.Apply(ctx, "p-1", stunTemplate(3), "", "")
	for i := range 2 {
		m.Tick(ctx)
		if !m.Has("p-1", "stunned") {
			t.Fatalf("stun gone after %d ticks with no resolver (should run full duration)", i+1)
		}
	}
	m.Tick(ctx) // duration hits 0 → expires normally
	if m.Has("p-1", "stunned") {
		t.Error("stun should have expired at duration end")
	}
}

func TestEffectManager_RecurringSaveOnPermanentEffect(t *testing.T) {
	tgt := newRecTarget("p-1")
	m := newManagerForTarget(tgt, nil)
	res := &scriptedSaveResolver{results: []bool{false, false, true}} // make it on the 3rd tick
	m.SetSaveResolver(res)
	ctx := context.Background()
	m.Apply(ctx, "p-1", stunTemplate(-1), "", "") // permanent

	m.Tick(ctx)
	m.Tick(ctx)
	if !m.Has("p-1", "stunned") {
		t.Fatal("permanent stun removed before a made save")
	}
	m.Tick(ctx) // 3rd save succeeds
	if m.Has("p-1", "stunned") {
		t.Error("permanent stun survived a made shake-off save (recurring save must apply to permanents)")
	}
}

func TestEffectManager_NoRecurringSaveNeverRolls(t *testing.T) {
	tgt := newRecTarget("p-1")
	m := newManagerForTarget(tgt, nil)
	res := &scriptedSaveResolver{results: []bool{true, true}}
	m.SetSaveResolver(res)
	ctx := context.Background()
	// Plain effect, no RecurringSave → resolver must never be consulted.
	m.Apply(ctx, "p-1", progression.EffectTemplate{ID: "bless", Duration: 5}, "", "")
	m.Tick(ctx)
	m.Tick(ctx)
	if res.callCount() != 0 {
		t.Errorf("resolver called %d times for an effect with no recurring save, want 0", res.callCount())
	}
	if !m.Has("p-1", "bless") {
		t.Error("bless wrongly removed")
	}
}
