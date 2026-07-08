package loot

import (
	"math/rand/v2"
	"reflect"
	"testing"
)

// seqRoller returns IntN results from a fixed sequence, asserting the
// caller never overruns it. Each value is interpreted modulo n so a
// test can supply a small literal and still land inside the requested
// range; tests that care about exact selection size their n's so the
// modulo is the identity.
type seqRoller struct {
	t   *testing.T
	seq []int
	i   int
}

func (r *seqRoller) IntN(n int) int {
	if n <= 0 {
		r.t.Fatalf("IntN called with n=%d (must be > 0)", n)
	}
	if r.i >= len(r.seq) {
		r.t.Fatalf("seqRoller overrun: wanted %d values, got %d", r.i+1, len(r.seq))
	}
	v := r.seq[r.i]
	r.i++
	return v % n
}

func TestRollItems_NilTable(t *testing.T) {
	if got := RollItems(nil, &seqRoller{t: t}); got != nil {
		t.Fatalf("nil table: want nil, got %v", got)
	}
}

func TestRollItems_GuaranteedOnly(t *testing.T) {
	tbl := &Table{
		Guaranteed: []GuaranteedEntry{
			{ItemID: "copper-coin", Count: 3},
			{ItemID: "rusty-dagger", Count: 1},
		},
	}
	// No pool, no rare bonus → roller must never be touched.
	got := RollItems(tbl, &seqRoller{t: t})
	want := []string{"copper-coin", "copper-coin", "copper-coin", "rusty-dagger"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("guaranteed: want %v, got %v", want, got)
	}
}

func TestRollItems_GuaranteedSkipsZeroAndEmpty(t *testing.T) {
	tbl := &Table{
		Guaranteed: []GuaranteedEntry{
			{ItemID: "skip-me", Count: 0},
			{ItemID: "", Count: 5},
			{ItemID: "keep", Count: 2},
		},
	}
	got := RollItems(tbl, &seqRoller{t: t})
	want := []string{"keep", "keep"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("zero/empty skip: want %v, got %v", want, got)
	}
}

func TestRollItems_WeightedPoolRolls(t *testing.T) {
	tbl := &Table{
		Weighted: []WeightedEntry{
			{ItemID: "common", Weight: 3}, // covers rolls 0,1,2
			{ItemID: "rare", Weight: 1},   // covers roll 3
		},
		PoolRolls: 3,
	}
	// total weight = 4. roll values: 0→common, 3→rare, 2→common.
	got := RollItems(tbl, &seqRoller{t: t, seq: []int{0, 3, 2}})
	want := []string{"common", "rare", "common"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("weighted pool: want %v, got %v", want, got)
	}
}

func TestRollItems_GuaranteedBeforePoolBeforeRare(t *testing.T) {
	tbl := &Table{
		Guaranteed: []GuaranteedEntry{{ItemID: "g", Count: 1}},
		Weighted:   []WeightedEntry{{ItemID: "p", Weight: 1}},
		PoolRolls:  1,
		RareBonus: &RareBonus{
			Chance:  100, // always fires
			Entries: []WeightedEntry{{ItemID: "boss-drop", Weight: 1}},
		},
	}
	// pool roll: IntN(1)=0→p. rare chance: IntN(100)→success. rare select: IntN(1)=0→boss-drop.
	got := RollItems(tbl, &seqRoller{t: t, seq: []int{0, 0, 0}})
	want := []string{"g", "p", "boss-drop"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ordering: want %v, got %v", want, got)
	}
}

func TestRollItems_RareBonusMisses(t *testing.T) {
	tbl := &Table{
		RareBonus: &RareBonus{
			Chance:  25,
			Entries: []WeightedEntry{{ItemID: "boss-drop", Weight: 1}},
		},
	}
	// IntN(100)=50 → 50 < 25 is false → no rare drop, no select roll consumed.
	got := RollItems(tbl, &seqRoller{t: t, seq: []int{50}})
	if len(got) != 0 {
		t.Fatalf("rare miss: want empty, got %v", got)
	}
}

func TestRollItems_RareBonusZeroChanceSkipsRoller(t *testing.T) {
	tbl := &Table{
		RareBonus: &RareBonus{
			Chance:  0,
			Entries: []WeightedEntry{{ItemID: "boss-drop", Weight: 1}},
		},
	}
	// Chance 0 must short-circuit before touching the roller.
	if got := RollItems(tbl, &seqRoller{t: t}); len(got) != 0 {
		t.Fatalf("rare zero chance: want empty, got %v", got)
	}
}

func TestRollItems_WeightedAllZeroWeightsSkips(t *testing.T) {
	tbl := &Table{
		Weighted:  []WeightedEntry{{ItemID: "a", Weight: 0}, {ItemID: "b", Weight: -2}},
		PoolRolls: 5,
	}
	// total weight 0 → every roll selects nothing, roller never called.
	if got := RollItems(tbl, &seqRoller{t: t}); len(got) != 0 {
		t.Fatalf("zero weights: want empty, got %v", got)
	}
}

func TestRollItems_EmptyTable(t *testing.T) {
	if got := RollItems(&Table{}, &seqRoller{t: t}); len(got) != 0 {
		t.Fatalf("empty table: want empty, got %v", got)
	}
}

// Stochastic smoke: with a real RNG, weighted selection stays in-range
// and respects the rough weight ratio over many rolls.
func TestRollItems_WeightedDistribution(t *testing.T) {
	tbl := &Table{
		Weighted:  []WeightedEntry{{ItemID: "heavy", Weight: 9}, {ItemID: "light", Weight: 1}},
		PoolRolls: 1,
	}
	r := rand.New(rand.NewPCG(1, 2))
	counts := map[string]int{}
	const n = 10000
	for range n {
		got := RollItems(tbl, r)
		if len(got) != 1 {
			t.Fatalf("want exactly one drop per roll, got %v", got)
		}
		counts[got[0]]++
	}
	if counts["heavy"] <= counts["light"] {
		t.Fatalf("heavy (w=9) should dominate light (w=1): %v", counts)
	}
	if counts["heavy"]+counts["light"] != n {
		t.Fatalf("unexpected ids in %v", counts)
	}
}
