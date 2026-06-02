package loot

import "testing"

func TestRollCoins_Nil(t *testing.T) {
	if got := RollCoins(nil, &seqRoller{t: t}); got != 0 {
		t.Fatalf("nil block: want 0, got %d", got)
	}
}

func TestRollCoins_FixedWhenMaxNotAboveMin(t *testing.T) {
	// Max == Min and Max < Min both yield a fixed Min without rolling.
	if got := RollCoins(&CoinBlock{Min: 7, Max: 7}, &seqRoller{t: t}); got != 7 {
		t.Fatalf("fixed: want 7, got %d", got)
	}
	if got := RollCoins(&CoinBlock{Min: 5, Max: 1}, &seqRoller{t: t}); got != 5 {
		t.Fatalf("max<min: want 5, got %d", got)
	}
}

func TestRollCoins_NegativeMinClampedToZero(t *testing.T) {
	// lo clamps to 0; range becomes [0, 3] → IntN(4). roll 2 → 2.
	if got := RollCoins(&CoinBlock{Min: -10, Max: 3}, &seqRoller{t: t, seq: []int{2}}); got != 2 {
		t.Fatalf("neg min: want 2, got %d", got)
	}
}

func TestRollCoins_Range(t *testing.T) {
	// [2, 6] → IntN(5); roll 0 → 2, roll 4 → 6.
	if got := RollCoins(&CoinBlock{Min: 2, Max: 6}, &seqRoller{t: t, seq: []int{0}}); got != 2 {
		t.Fatalf("low end: want 2, got %d", got)
	}
	if got := RollCoins(&CoinBlock{Min: 2, Max: 6}, &seqRoller{t: t, seq: []int{4}}); got != 6 {
		t.Fatalf("high end: want 6, got %d", got)
	}
}

func TestRegistry_DeepCopyCoin(t *testing.T) {
	orig := &Table{ID: "z", Coin: &CoinBlock{Min: 1, Max: 9}}
	r := NewRegistry()
	_ = r.Register(orig)
	orig.Coin.Max = 999 // mutate caller's block after register
	got, _ := r.Get("z")
	if got.Coin == nil || got.Coin.Max != 9 {
		t.Fatalf("coin not deep-copied: %+v", got.Coin)
	}
}
