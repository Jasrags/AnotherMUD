package progression_test

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

func mustDice(t *testing.T, s string) combat.DiceExpr {
	t.Helper()
	d, err := combat.ParseDice(s)
	if err != nil {
		t.Fatalf("ParseDice %q: %v", s, err)
	}
	return d
}

func TestApplyMobClassGrowth_AppliesAveragedDeltas(t *testing.T) {
	sb := progression.NewWithBase(map[progression.StatType]int{
		progression.StatHPMax: 40,
		progression.StatSTR:   12,
	})
	cls := &progression.Class{
		ID: "fighter",
		StatGrowth: map[progression.StatType]combat.DiceExpr{
			progression.StatHPMax: mustDice(t, "1d10"), // avg 5
			progression.StatSTR:   mustDice(t, "1d2"),  // avg 1
		},
	}

	if !progression.ApplyMobClassGrowth(sb, cls, 5) {
		t.Fatal("ApplyMobClassGrowth returned false, want true")
	}

	// Expected: hp_max grew by 5*5=25 → effective 65; STR grew by 1*5=5 → effective 17
	if got := sb.Effective(progression.StatHPMax); got != 65 {
		t.Errorf("hp_max = %d, want 65 (base 40 + 5×5)", got)
	}
	if got := sb.Effective(progression.StatSTR); got != 17 {
		t.Errorf("str = %d, want 17 (base 12 + 1×5)", got)
	}
}

func TestApplyMobClassGrowth_ZeroLevelNoop(t *testing.T) {
	sb := progression.NewWithBase(map[progression.StatType]int{
		progression.StatHPMax: 40,
	})
	cls := &progression.Class{
		ID: "fighter",
		StatGrowth: map[progression.StatType]combat.DiceExpr{
			progression.StatHPMax: mustDice(t, "1d10"),
		},
	}
	if progression.ApplyMobClassGrowth(sb, cls, 0) {
		t.Error("Level 0: want false")
	}
	if got := sb.Effective(progression.StatHPMax); got != 40 {
		t.Errorf("hp_max = %d, want 40 (untouched)", got)
	}
}

func TestApplyMobClassGrowth_EmptyGrowthNoop(t *testing.T) {
	sb := progression.NewWithBase(map[progression.StatType]int{
		progression.StatHPMax: 40,
	})
	cls := &progression.Class{ID: "scholar"} // no StatGrowth
	if progression.ApplyMobClassGrowth(sb, cls, 5) {
		t.Error("empty growth: want false")
	}
}

func TestApplyMobClassGrowth_NilArgs(t *testing.T) {
	sb := progression.NewWithBase(nil)
	if progression.ApplyMobClassGrowth(nil, &progression.Class{ID: "x"}, 5) {
		t.Error("nil sb: want false")
	}
	if progression.ApplyMobClassGrowth(sb, nil, 5) {
		t.Error("nil cls: want false")
	}
}

func TestApplyMobClassGrowth_NegativeLevelNoop(t *testing.T) {
	sb := progression.NewWithBase(map[progression.StatType]int{
		progression.StatHPMax: 40,
	})
	cls := &progression.Class{
		ID: "fighter",
		StatGrowth: map[progression.StatType]combat.DiceExpr{
			progression.StatHPMax: mustDice(t, "1d10"),
		},
	}
	if progression.ApplyMobClassGrowth(sb, cls, -3) {
		t.Error("negative level: want false")
	}
}

func TestApplyMobClassGrowth_FiresMaxChangeListener(t *testing.T) {
	sb := progression.NewWithBase(map[progression.StatType]int{
		progression.StatHPMax: 40,
	})
	var lastNew int
	sb.OnMaxChange(progression.StatHPMax, func(_, newMax int) { lastNew = newMax })

	cls := &progression.Class{
		ID: "fighter",
		StatGrowth: map[progression.StatType]combat.DiceExpr{
			progression.StatHPMax: mustDice(t, "1d10"), // avg 5
		},
	}
	progression.ApplyMobClassGrowth(sb, cls, 3) // +15

	if lastNew != 55 {
		t.Errorf("listener observed newMax=%d, want 55", lastNew)
	}
}
