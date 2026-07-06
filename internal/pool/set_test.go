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
	crossings, _, _ := s.ApplyDamage("physical", 13)

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

	crossings, _, _ := s.ApplyDamage("physical", 12)
	want := []Crossing{{Kind: "overflow"}}
	if !reflect.DeepEqual(crossings, want) {
		t.Fatalf("crossings = %+v; want %+v", crossings, want)
	}
}

func TestSet_NoOverflowWhenAbsorbed(t *testing.T) {
	s := NewSet()
	s.Add(New("hp", 20, Rules{Floor: 0, OverflowTo: "overflow", DepletionEvent: true}))
	s.Add(New("overflow", 5, Rules{Floor: 0, DepletionEvent: true}))

	crossings, _, _ := s.ApplyDamage("hp", 8)
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

	crossings, _, _ := s.ApplyDamage("a", 100) // would loop without the visited guard
	// a crosses, spills to b, b crosses, spills back to a (already visited) → stop.
	want := []Crossing{{Kind: "a"}, {Kind: "b"}}
	if !reflect.DeepEqual(crossings, want) {
		t.Fatalf("crossings = %+v; want %+v", crossings, want)
	}
}

func TestSet_ApplyDamageUnknownKindNoop(t *testing.T) {
	s := NewSet()
	s.Add(New("hp", 20, Rules{Floor: 0}))
	// An initial kind that isn't a pool crosses nothing — but it is NOT a total
	// no-op: the full amount escapes (destined for the unknown kind) so a caller
	// can route or ignore it rather than have it silently vanish. Combat only
	// spills escapedTo==hp onto Vitals, so a "ghost" target_pool is inert there.
	c, escaped, escapedTo := s.ApplyDamage("ghost", 10)
	if len(c) != 0 {
		t.Fatalf("unknown kind crosses nothing; got %+v", c)
	}
	if escaped != 10 || escapedTo != "ghost" {
		t.Fatalf("unknown initial kind should escape the full amount; got (%d,%q), want (10,ghost)", escaped, escapedTo)
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

// Fill restores every pool to its max — the "start full" used at character
// creation after a pool's max is endowed. A pool whose max was raised from
// 0 (a fresh channeler) but whose current stayed 0 ends full.
func TestSet_Fill(t *testing.T) {
	s := NewSet()
	drained := NewAt(Kind("mana"), 0, 30, Rules{Floor: 0}) // max raised, current 0
	half := NewAt(Kind("movement"), 5, 20, Rules{Floor: 0})
	s.Add(drained)
	s.Add(half)

	s.Fill()

	if c := drained.Current(); c != 30 {
		t.Fatalf("mana after Fill = %d; want 30 (full)", c)
	}
	if c := half.Current(); c != 20 {
		t.Fatalf("movement after Fill = %d; want 20 (full)", c)
	}
}

// TestSet_ApplyDamageOverflowEscapesToNonPool proves the SR-M3c stun→Physical
// surfacing: a Stun monitor overflowing to `hp` (which is the Vitals track, not
// a pool in this Set) crosses its own floor (a nonlethal KO) AND returns the
// unrouted excess as escaped, so combat can apply it to Vitals rather than drop it.
func TestSet_ApplyDamageOverflowEscapesToNonPool(t *testing.T) {
	s := NewSet()
	s.Add(New("stun", 5, Rules{Floor: 0, DepletionEvent: true, Nonlethal: true, OverflowTo: "hp"}))
	crossings, escaped, escapedTo := s.ApplyDamage("stun", 8) // 5 floors stun (+cross), 3 escapes to hp
	if len(crossings) != 1 || crossings[0].Kind != "stun" || !crossings[0].Nonlethal {
		t.Fatalf("crossings = %+v; want one nonlethal stun crossing", crossings)
	}
	if escaped != 3 || escapedTo != "hp" {
		t.Fatalf("escaped = (%d,%q); want (3, hp) — the unrouted overflow surfaced", escaped, escapedTo)
	}
}
