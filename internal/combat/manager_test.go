package combat

import (
	"context"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// recordingSink captures every dispatched event for assertion. Safe
// for concurrent use so the lock-stress test can record from multiple
// goroutines.
type recordingSink struct {
	mu      sync.Mutex
	engaged []Engagement
	ended   []CombatEnded
}

func (r *recordingSink) OnEngagement(_ context.Context, e Engagement) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.engaged = append(r.engaged, e)
}

func (r *recordingSink) OnCombatEnded(_ context.Context, e CombatEnded) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ended = append(r.ended, e)
}

func (r *recordingSink) engagedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.engaged)
}

func (r *recordingSink) endedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.ended)
}

// staticCombatant satisfies Combatant with fixed Name + ID, enough
// for Manager tests that don't touch Vitals/Stats.
type staticCombatant struct {
	id   CombatantID
	name string
}

func (s staticCombatant) CombatantID() CombatantID { return s.id }
func (s staticCombatant) Name() string             { return s.name }
func (s staticCombatant) Vitals() *Vitals          { return nil }
func (s staticCombatant) Stats() Stats             { return Stats{} }

func makeRig(t *testing.T, names ...string) (*Manager, *recordingSink, []CombatantID) {
	t.Helper()
	locator := MapLocator{}
	ids := make([]CombatantID, len(names))
	for i, n := range names {
		// Mix prefixes deliberately so tests exercise both spaces.
		var id CombatantID
		if i%2 == 0 {
			id = NewMobCombatantID(n)
		} else {
			id = NewPlayerCombatantID(n)
		}
		ids[i] = id
		locator[id] = staticCombatant{id: id, name: n}
	}
	sink := &recordingSink{}
	return NewManager(locator, sink), sink, ids
}

const testRoom world.RoomID = "tapestry-core:town-square"

func TestEngageSymmetricAndIdempotent(t *testing.T) {
	mgr, sink, ids := makeRig(t, "a", "b")
	if !mgr.Engage(context.Background(), ids[0], ids[1], testRoom) {
		t.Fatal("first Engage returned false")
	}
	// Symmetric: both sides hold the other.
	if got := mgr.OpponentsOf(ids[0]); len(got) != 1 || got[0] != ids[1] {
		t.Errorf("a opponents = %v, want [b]", got)
	}
	if got := mgr.OpponentsOf(ids[1]); len(got) != 1 || got[0] != ids[0] {
		t.Errorf("b opponents = %v, want [a]", got)
	}
	// Idempotent — second Engage is a no-op (spec §2.1).
	if mgr.Engage(context.Background(), ids[0], ids[1], testRoom) {
		t.Error("second Engage returned true (should be a no-op)")
	}
	// Exactly one Engagement event fired across both calls.
	if got := sink.engagedCount(); got != 1 {
		t.Errorf("Engagement events = %d, want 1", got)
	}
}

func TestEngageRefusesSelfAndEmpty(t *testing.T) {
	mgr, sink, ids := makeRig(t, "a")
	if mgr.Engage(context.Background(), ids[0], ids[0], testRoom) {
		t.Error("self-engage returned true, want false")
	}
	if mgr.Engage(context.Background(), "", ids[0], testRoom) {
		t.Error("empty-attacker engage returned true")
	}
	if mgr.Engage(context.Background(), ids[0], "", testRoom) {
		t.Error("empty-target engage returned true")
	}
	if sink.engagedCount() != 0 {
		t.Errorf("Engagement events = %d on refused engages, want 0", sink.engagedCount())
	}
}

func TestEngagementPayload(t *testing.T) {
	mgr, sink, ids := makeRig(t, "alpha", "bravo")
	mgr.Engage(context.Background(), ids[0], ids[1], testRoom)
	if len(sink.engaged) != 1 {
		t.Fatalf("engaged len = %d, want 1", len(sink.engaged))
	}
	e := sink.engaged[0]
	if e.AttackerID != ids[0] || e.TargetID != ids[1] {
		t.Errorf("ids = (%s, %s), want (%s, %s)", e.AttackerID, e.TargetID, ids[0], ids[1])
	}
	if e.AttackerName != "alpha" || e.TargetName != "bravo" {
		t.Errorf("names = (%q, %q), want (alpha, bravo)", e.AttackerName, e.TargetName)
	}
	if e.RoomID != testRoom {
		t.Errorf("room = %s, want %s", e.RoomID, testRoom)
	}
}

func TestDisengagePairwiseEmitsCombatEndedOnEmpty(t *testing.T) {
	mgr, sink, ids := makeRig(t, "a", "b")
	mgr.Engage(context.Background(), ids[0], ids[1], testRoom)
	if !mgr.Disengage(context.Background(), ids[0], ids[1], testRoom) {
		t.Fatal("Disengage returned false on engaged pair")
	}
	// Both sides emptied, so two CombatEnded events.
	if sink.endedCount() != 2 {
		t.Errorf("CombatEnded count = %d, want 2", sink.endedCount())
	}
	if mgr.InCombat(ids[0]) || mgr.InCombat(ids[1]) {
		t.Error("InCombat true after pairwise disengage")
	}
}

func TestDisengageDoesNotEmitWhenOtherOpponentsRemain(t *testing.T) {
	mgr, sink, ids := makeRig(t, "a", "b", "c")
	// a engages both b and c
	mgr.Engage(context.Background(), ids[0], ids[1], testRoom)
	mgr.Engage(context.Background(), ids[0], ids[2], testRoom)
	// Disengage a from b. b's list empties (only had a); a still has c.
	mgr.Disengage(context.Background(), ids[0], ids[1], testRoom)
	// Exactly one CombatEnded (for b).
	if sink.endedCount() != 1 {
		t.Fatalf("CombatEnded count = %d, want 1", sink.endedCount())
	}
	if sink.ended[0].CombatantID != ids[1] {
		t.Errorf("CombatEnded id = %s, want b (%s)", sink.ended[0].CombatantID, ids[1])
	}
	if !mgr.InCombat(ids[0]) {
		t.Error("a should still be in combat with c")
	}
}

func TestDisengageReturnsFalseWhenNotEngaged(t *testing.T) {
	mgr, _, ids := makeRig(t, "a", "b")
	if mgr.Disengage(context.Background(), ids[0], ids[1], testRoom) {
		t.Error("Disengage on non-engaged pair returned true")
	}
}

func TestDisengageAllCleansBothSidesAndEmits(t *testing.T) {
	mgr, sink, ids := makeRig(t, "a", "b", "c", "d")
	// a is engaged with b, c, d
	for _, opp := range ids[1:] {
		mgr.Engage(context.Background(), ids[0], opp, testRoom)
	}
	mgr.DisengageAll(context.Background(), ids[0], testRoom)
	// b, c, d each emptied (their only opponent was a); a always emits.
	if got := sink.endedCount(); got != 4 {
		t.Errorf("CombatEnded events = %d, want 4 (a + b + c + d)", got)
	}
	// Every combatant out of combat now.
	for _, id := range ids {
		if mgr.InCombat(id) {
			t.Errorf("%s still in combat after DisengageAll", id)
		}
	}
}

func TestDisengageAllOnEmptyEntityStillEmitsForSelf(t *testing.T) {
	mgr, sink, ids := makeRig(t, "a")
	mgr.DisengageAll(context.Background(), ids[0], testRoom)
	// Spec §2.3: "emit combat ended for the entity itself" —
	// unconditional. A not-in-combat caller still gets one event.
	if got := sink.endedCount(); got != 1 {
		t.Errorf("CombatEnded events = %d, want 1 (self emission)", got)
	}
}

func TestPrimaryTargetIsHeadOfList(t *testing.T) {
	mgr, _, ids := makeRig(t, "a", "b", "c")
	mgr.Engage(context.Background(), ids[0], ids[1], testRoom)
	mgr.Engage(context.Background(), ids[0], ids[2], testRoom)
	pt, ok := mgr.PrimaryTargetOf(ids[0])
	if !ok || pt != ids[1] {
		t.Errorf("primary = %s (ok=%v), want b first-engaged", pt, ok)
	}
}

func TestPromoteTargetMovesToHead(t *testing.T) {
	mgr, _, ids := makeRig(t, "a", "b", "c", "d")
	mgr.Engage(context.Background(), ids[0], ids[1], testRoom) // a→b
	mgr.Engage(context.Background(), ids[0], ids[2], testRoom) // a→c
	mgr.Engage(context.Background(), ids[0], ids[3], testRoom) // a→d
	if !mgr.PromoteTarget(ids[0], ids[3]) {
		t.Fatal("PromoteTarget(d) returned false")
	}
	got := mgr.OpponentsOf(ids[0])
	want := []CombatantID{ids[3], ids[1], ids[2]}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("after promote: %v, want %v", got, want)
	}
}

func TestPromoteTargetRefusesUnknownOpponent(t *testing.T) {
	mgr, _, ids := makeRig(t, "a", "b", "c")
	mgr.Engage(context.Background(), ids[0], ids[1], testRoom)
	// c is not in a's list — promotion must fail, not silently insert.
	if mgr.PromoteTarget(ids[0], ids[2]) {
		t.Error("promote of non-opponent returned true (should refuse, no silent insert)")
	}
	if got := mgr.OpponentsOf(ids[0]); len(got) != 1 {
		t.Errorf("a opponents = %v, want [b] (promote should not insert)", got)
	}
}

func TestPromoteTargetIdempotentOnHead(t *testing.T) {
	mgr, _, ids := makeRig(t, "a", "b")
	mgr.Engage(context.Background(), ids[0], ids[1], testRoom)
	if !mgr.PromoteTarget(ids[0], ids[1]) {
		t.Error("promoting already-primary returned false")
	}
}

func TestOpponentsSnapshotIsCopy(t *testing.T) {
	mgr, _, ids := makeRig(t, "a", "b")
	mgr.Engage(context.Background(), ids[0], ids[1], testRoom)
	snap := mgr.OpponentsOf(ids[0])
	snap[0] = "MUTATED"
	if got := mgr.OpponentsOf(ids[0]); got[0] == "MUTATED" {
		t.Error("OpponentsOf returned aliased slice; mutation leaked into Manager state")
	}
}

func TestAllCombatantsListsEveryEngagedEntity(t *testing.T) {
	mgr, _, ids := makeRig(t, "a", "b", "c", "d")
	mgr.Engage(context.Background(), ids[0], ids[1], testRoom)
	mgr.Engage(context.Background(), ids[2], ids[3], testRoom)
	got := mgr.AllCombatants()
	if len(got) != 4 {
		t.Errorf("AllCombatants len = %d, want 4", len(got))
	}
	seen := make(map[CombatantID]bool, 4)
	for _, id := range got {
		seen[id] = true
	}
	for _, id := range ids {
		if !seen[id] {
			t.Errorf("missing %s in AllCombatants", id)
		}
	}
}

// TestManagerConcurrentEngageDisengage stresses the lock. With -race
// the harness fails fast if the mutex were missing. The assertion
// is loose (state correctness, not a specific final value) — the
// point is to exercise concurrent mutation.
func TestManagerConcurrentEngageDisengage(t *testing.T) {
	mgr, _, ids := makeRig(t, "a", "b", "c", "d", "e", "f", "g", "h")
	var wg sync.WaitGroup
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			wg.Add(2)
			go func(a, b CombatantID) {
				defer wg.Done()
				mgr.Engage(context.Background(), a, b, testRoom)
			}(ids[i], ids[j])
			go func(a, b CombatantID) {
				defer wg.Done()
				mgr.Disengage(context.Background(), a, b, testRoom)
			}(ids[i], ids[j])
		}
	}
	wg.Wait()
	// State invariant: any combatant in someone's list also has that
	// someone in their own list. Verifies symmetry survived the race.
	for _, a := range ids {
		for _, opp := range mgr.OpponentsOf(a) {
			oppList := mgr.OpponentsOf(opp)
			found := false
			for _, x := range oppList {
				if x == a {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("symmetry broken: %s has %s but not vice versa", a, opp)
			}
		}
	}
}

// TestNewManagerNilSinkDoesNotPanic confirms the nil-sink shortcut
// substitutes a no-op so engage / disengage don't nil-deref.
func TestNewManagerNilSinkDoesNotPanic(t *testing.T) {
	mgr := NewManager(nil, nil)
	a := NewMobCombatantID("a")
	b := NewMobCombatantID("b")
	if !mgr.Engage(context.Background(), a, b, testRoom) {
		t.Error("Engage on nil-sink Manager returned false")
	}
	mgr.DisengageAll(context.Background(), a, testRoom)
}
