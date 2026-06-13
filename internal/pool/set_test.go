package pool

import (
	"reflect"
	"testing"
)

func TestSet_AddGet(t *testing.T) {
	s := NewSet()
	s.Add(New("hp", 20, Rules{Floor: 0}))
	if _, ok := s.Get("hp"); !ok {
		t.Fatal("hp should be present after Add")
	}
	if _, ok := s.Get("mana"); ok {
		t.Fatal("mana should be absent")
	}
	s.Add(nil) // tolerated no-op
}

// TestSet_OverflowRoutesToDeathTrack models Shadowrun's Physical monitor
// spilling its excess into a death/overflow track, and asserts both
// crossings are reported when both pools cross to floor.
func TestSet_OverflowRoutesToDeathTrack(t *testing.T) {
	s := NewSet()
	s.Add(New("physical", 10, Rules{Floor: 0, OverflowTo: "overflow", DepletionEvent: true}))
	s.Add(NewAt("overflow", 3, 3, Rules{Floor: 0, DepletionEvent: true}))

	// 13 damage: 10 floors physical (+crosses), 3 overflow routed into the
	// 3-box death track, which itself floors (+crosses).
	crossings := s.ApplyDamage("physical", 13)

	phys, _ := s.Get("physical")
	over, _ := s.Get("overflow")
	if c := phys.Current(); c != 0 {
		t.Fatalf("physical current = %d; want 0", c)
	}
	if c := over.Current(); c != 0 {
		t.Fatalf("overflow current = %d; want 0", c)
	}
	want := []Crossing{{Kind: "physical"}, {Kind: "overflow"}}
	if !reflect.DeepEqual(crossings, want) {
		t.Fatalf("crossings = %+v; want %+v", crossings, want)
	}
}

func TestSet_OverflowOnlyReportsDepletionEventPools(t *testing.T) {
	s := NewSet()
	// physical advertises no depletion event; it should not appear in crossings.
	// A 2-box overflow track so the 2 points of spill actually deplete it.
	s.Add(New("physical", 10, Rules{Floor: 0, OverflowTo: "overflow"}))
	s.Add(NewAt("overflow", 2, 2, Rules{Floor: 0, DepletionEvent: true}))

	crossings := s.ApplyDamage("physical", 12)
	want := []Crossing{{Kind: "overflow"}}
	if !reflect.DeepEqual(crossings, want) {
		t.Fatalf("crossings = %+v; want %+v", crossings, want)
	}
}

func TestSet_NoOverflowWhenAbsorbed(t *testing.T) {
	s := NewSet()
	s.Add(New("hp", 20, Rules{Floor: 0, OverflowTo: "overflow", DepletionEvent: true}))
	s.Add(New("overflow", 5, Rules{Floor: 0, DepletionEvent: true}))

	crossings := s.ApplyDamage("hp", 8)
	if len(crossings) != 0 {
		t.Fatalf("crossings = %+v; want none", crossings)
	}
	if c, _ := s.Get("hp"); c.Current() != 12 {
		t.Fatalf("hp current = %d; want 12", c.Current())
	}
	if o, _ := s.Get("overflow"); o.Current() != 5 {
		t.Fatalf("overflow should be untouched, got %d", o.Current())
	}
}

func TestSet_OverflowCycleTerminates(t *testing.T) {
	// Misconfigured A→B→A overflow cycle must not loop forever.
	s := NewSet()
	s.Add(New("a", 5, Rules{Floor: 0, OverflowTo: "b", DepletionEvent: true}))
	s.Add(New("b", 5, Rules{Floor: 0, OverflowTo: "a", DepletionEvent: true}))

	crossings := s.ApplyDamage("a", 100) // would loop without the visited guard
	// a crosses, spills to b, b crosses, spills back to a (already visited) → stop.
	want := []Crossing{{Kind: "a"}, {Kind: "b"}}
	if !reflect.DeepEqual(crossings, want) {
		t.Fatalf("crossings = %+v; want %+v", crossings, want)
	}
}

func TestSet_ApplyDamageUnknownKindNoop(t *testing.T) {
	s := NewSet()
	s.Add(New("hp", 20, Rules{Floor: 0}))
	if c := s.ApplyDamage("ghost", 10); len(c) != 0 {
		t.Fatalf("unknown kind should be a no-op; got %+v", c)
	}
}

func TestSet_SnapshotRestoreRoundTrip(t *testing.T) {
	s := NewSet()
	s.Add(NewAt("hp", 12, 20, Rules{Floor: 0, DepletionEvent: true}))
	s.Add(NewAt("mana", 5, 30, Rules{Floor: 0}))

	snap := s.Snapshot()
	// Deterministic order: sorted by kind ("hp" < "mana").
	want := Snapshot{
		{Kind: "hp", Current: 12, Max: 20},
		{Kind: "mana", Current: 5, Max: 30},
	}
	if !reflect.DeepEqual(snap, want) {
		t.Fatalf("snapshot = %+v; want %+v", snap, want)
	}

	// Rules are re-derived from content at restore, not persisted.
	rulesFor := func(k Kind) Rules {
		if k == "hp" {
			return Rules{Floor: 0, DepletionEvent: true}
		}
		return Rules{Floor: 0}
	}
	restored := RestoreSet(snap, rulesFor)
	if !reflect.DeepEqual(restored.Snapshot(), want) {
		t.Fatalf("restored snapshot = %+v; want %+v", restored.Snapshot(), want)
	}
	hp, _ := restored.Get("hp")
	if !hp.Rules().DepletionEvent {
		t.Fatal("restored hp should have re-derived DepletionEvent rule")
	}
}

func TestSet_EmptySnapshotIsNil(t *testing.T) {
	if s := NewSet().Snapshot(); s != nil {
		t.Fatalf("empty set snapshot = %+v; want nil", s)
	}
}
