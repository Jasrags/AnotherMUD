package progression

import (
	"sort"
	"testing"
)

func TestCastTracker_BeginActiveAdvance(t *testing.T) {
	tr := NewCastTracker()

	if tr.IsCasting("p1") {
		t.Fatal("fresh tracker reports a cast in progress")
	}
	if _, _, active := tr.Advance("p1"); active {
		t.Fatal("Advance on an idle entity reported active")
	}

	tr.Begin("p1", Cast{AbilityID: "firebolt", AbilityName: "Firebolt", Remaining: 2})
	if !tr.IsCasting("p1") {
		t.Fatal("entity should be casting after Begin")
	}
	got, ok := tr.Active("p1")
	if !ok || got.AbilityID != "firebolt" || got.Remaining != 2 {
		t.Fatalf("Active = %+v, %v; want firebolt/2", got, ok)
	}

	// Round 1: 2 → 1, still warming up.
	c, ready, active := tr.Advance("p1")
	if !active || ready || c.Remaining != 1 {
		t.Fatalf("round 1: active=%v ready=%v remaining=%d; want active,!ready,1", active, ready, c.Remaining)
	}
	// Round 2: 1 → 0, ready; tracker cleared.
	c, ready, active = tr.Advance("p1")
	if !active || !ready || c.AbilityID != "firebolt" {
		t.Fatalf("round 2: active=%v ready=%v ability=%q; want active,ready,firebolt", active, ready, c.AbilityID)
	}
	if tr.IsCasting("p1") {
		t.Fatal("tracker must clear the cast once it is ready")
	}
}

func TestCastTracker_BeginClampsRemaining(t *testing.T) {
	tr := NewCastTracker()
	tr.Begin("p1", Cast{AbilityID: "x", Remaining: 0}) // clamped to 1
	c, ready, active := tr.Advance("p1")
	if !active || !ready || c.Remaining != 0 {
		t.Fatalf("a clamped 1-round cast should be ready after one Advance: active=%v ready=%v rem=%d", active, ready, c.Remaining)
	}
}

func TestCastTracker_Interrupt(t *testing.T) {
	tr := NewCastTracker()
	if _, ok := tr.Interrupt("p1"); ok {
		t.Fatal("Interrupt on an idle entity reported a cast")
	}
	tr.Begin("p1", Cast{AbilityID: "bonds-of-air", AbilityName: "Bonds of Air", Remaining: 3})
	c, ok := tr.Interrupt("p1")
	if !ok || c.AbilityID != "bonds-of-air" {
		t.Fatalf("Interrupt = %+v, %v; want the in-flight cast", c, ok)
	}
	if tr.IsCasting("p1") {
		t.Fatal("Interrupt must clear the cast")
	}
}

func TestCastTracker_CastingEntities(t *testing.T) {
	tr := NewCastTracker()
	if got := tr.CastingEntities(); len(got) != 0 {
		t.Fatalf("empty tracker should list nobody, got %v", got)
	}
	tr.Begin("p1", Cast{AbilityID: "a", Remaining: 1})
	tr.Begin("p2", Cast{AbilityID: "b", Remaining: 1})
	got := tr.CastingEntities()
	sort.Strings(got)
	if len(got) != 2 || got[0] != "p1" || got[1] != "p2" {
		t.Fatalf("CastingEntities = %v; want [p1 p2]", got)
	}
}

func TestCastTracker_StoresOverchannelDeficit(t *testing.T) {
	tr := NewCastTracker()
	tr.Begin("p1", Cast{AbilityID: "firebolt", Overchannel: true, OverchannelDeficit: 7, Remaining: 2})
	c, ok := tr.Active("p1")
	if !ok || !c.Overchannel || c.OverchannelDeficit != 7 {
		t.Fatalf("Active = %+v, %v; want Overchannel/deficit 7 preserved", c, ok)
	}
	// Survives the warmup so resolve sees the begin-time reach.
	c, _, _ = tr.Advance("p1")
	if c.OverchannelDeficit != 7 {
		t.Fatalf("deficit not carried through Advance: %d", c.OverchannelDeficit)
	}
}

func TestCastTracker_DropAndKeyNormalization(t *testing.T) {
	tr := NewCastTracker()
	tr.Begin("  P1  ", Cast{AbilityID: "a", Remaining: 2}) // trimmed + lowercased
	if !tr.IsCasting("p1") {
		t.Fatal("key should be trimmed + lowercased on Begin and lookups")
	}
	tr.Drop("P1")
	if tr.IsCasting("p1") {
		t.Fatal("Drop should clear the cast")
	}
}
