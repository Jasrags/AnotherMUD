package progression

import "testing"

// constRoller returns the same IntN value every call (mod n). Handy
// for the binary-check cases that roll at most once; for multi-roll
// resolver tests use seqRoller (resolution_test.go).
type constRoller int

func (c constRoller) IntN(n int) int {
	v := int(c)
	if v < 0 {
		v = 0
	}
	return v % n
}

func TestPassiveBinaryCheck(t *testing.T) {
	cases := []struct {
		name      string
		prof      int
		variance  int
		maxChance int
		roll      int // constRoller value; IntN+1 is the rolled number
		want      bool
	}{
		{"unlearned never fires", 0, 100, 100, 0, false},
		{"variance<100 success", 50, 40, 0, 0, true},           // chance 20, roll 1
		{"variance<100 miss", 50, 40, 0, 49, false},            // chance 20, roll 50
		{"variance>=100 uses maxChance", 50, 100, 80, 0, true}, // chance 40, roll 1
		{"variance>=100 maxChance 0 never", 50, 100, 0, 0, false},
		{"variance>=100 miss", 50, 100, 80, 99, false},         // chance 40, roll 100
		{"full chance clamps to 100", 100, 100, 100, 99, true}, // chance 100, roll 100
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PassiveBinaryCheck(tc.prof, tc.variance, tc.maxChance, constRoller(tc.roll))
			if got != tc.want {
				t.Errorf("PassiveBinaryCheck(prof=%d var=%d max=%d roll=%d) = %v, want %v",
					tc.prof, tc.variance, tc.maxChance, tc.roll+1, got, tc.want)
			}
		})
	}
}

func TestPassiveScalingBonus(t *testing.T) {
	cases := []struct {
		maxBonus, prof, want int
	}{
		{0, 50, 0},
		{10, 0, 0},
		{10, 50, 5},
		{3, 50, 1}, // 150/100 = 1 (truncation)
		{2, 100, 2},
		{5, 100, 5},
	}
	for _, tc := range cases {
		if got := PassiveScalingBonus(tc.maxBonus, tc.prof); got != tc.want {
			t.Errorf("PassiveScalingBonus(%d, %d) = %d, want %d", tc.maxBonus, tc.prof, got, tc.want)
		}
	}
}

func TestByHook(t *testing.T) {
	reg := NewAbilityRegistry()
	mustReg := func(a *Ability) {
		if err := reg.Register(a); err != nil {
			t.Fatalf("register %s: %v", a.ID, err)
		}
	}
	mustReg(&Ability{ID: "parry", Type: AbilityPassive, Category: AbilitySkill, Hook: "defensive"})
	mustReg(&Ability{ID: "dodge", Type: AbilityPassive, Category: AbilitySkill, Hook: "defensive"})
	mustReg(&Ability{ID: "second-attack", Type: AbilityPassive, Category: AbilitySkill, Hook: "extra_attack"})
	// An active ability with a (meaningless) hook must never be returned.
	mustReg(&Ability{ID: "kick", Type: AbilityActive, Category: AbilitySkill, Hook: "defensive"})

	def := reg.ByHook("defensive")
	if len(def) != 2 || def[0].ID != "dodge" || def[1].ID != "parry" {
		t.Fatalf("ByHook(defensive) = %v, want [dodge parry] sorted", ids(def))
	}
	if ea := reg.ByHook("extra_attack"); len(ea) != 1 || ea[0].ID != "second-attack" {
		t.Errorf("ByHook(extra_attack) = %v", ids(ea))
	}
	if reg.ByHook("") != nil {
		t.Error("empty hook must return nil")
	}
	if reg.ByHook("nonexistent") != nil {
		t.Error("unknown hook must return nil")
	}
}

func ids(abs []*Ability) []string {
	out := make([]string, len(abs))
	for i, a := range abs {
		out[i] = a.ID
	}
	return out
}

func TestPassiveResolver_ExtraAttacks(t *testing.T) {
	reg := NewAbilityRegistry()
	// Two extra-attack passives, variance 100 + maxChance 100 ⇒ binary
	// chance == proficiency. GainBaseChance 100 so a fired passive can
	// roll a gain.
	for _, id := range []string{"a-strike", "b-strike"} {
		if err := reg.Register(&Ability{
			ID: id, Type: AbilityPassive, Category: AbilitySkill, Hook: HookExtraAttack,
			Variance: 100, MaxHitChance: 100, GainBaseChance: 100,
		}); err != nil {
			t.Fatal(err)
		}
	}
	prof := newProfStub()
	prof.vals["a-strike"] = 50
	prof.vals["b-strike"] = 50
	// ByHook id-order: a-strike, b-strike. Rolls per passive: binary,
	// then gain only if it fired.
	//   a-strike binary roll 1 (≤50) → fire; gain roll 1 (≤50) → gain.
	//   b-strike binary roll 100 (>50) → miss; no gain roll.
	roller := &seqRoller{t: t, seq: []int{0, 0, 99}}
	pr := NewPassiveResolver(reg, prof, prof, roller)

	if got := pr.ExtraAttacks("p1"); got != 1 {
		t.Fatalf("ExtraAttacks = %d, want 1", got)
	}
	if prof.gains["a-strike"] != 1 {
		t.Errorf("a-strike gain = %d, want 1 (fired passive rolls §6.3 gain)", prof.gains["a-strike"])
	}
	if prof.gains["b-strike"] != 0 {
		t.Errorf("b-strike gain = %d, want 0 (did not fire)", prof.gains["b-strike"])
	}
}

func TestPassiveResolver_UnlearnedNeverFiresOrRolls(t *testing.T) {
	reg := NewAbilityRegistry()
	if err := reg.Register(&Ability{
		ID: "second-attack", Type: AbilityPassive, Category: AbilitySkill,
		Hook: HookExtraAttack, Variance: 100, MaxHitChance: 100,
	}); err != nil {
		t.Fatal(err)
	}
	prof := newProfStub() // entity has NOT learned it
	// Empty seq: seqRoller fatals if any roll is attempted. The prof-0
	// short-circuit in PassiveBinaryCheck must avoid rolling at all.
	roller := &seqRoller{t: t, seq: nil}
	pr := NewPassiveResolver(reg, prof, prof, roller)

	if got := pr.ExtraAttacks("p1"); got != 0 {
		t.Fatalf("ExtraAttacks for unlearned = %d, want 0", got)
	}
}

func TestPassiveResolver_DefensiveEvade(t *testing.T) {
	reg := NewAbilityRegistry()
	for _, id := range []string{"dodge", "parry"} {
		if err := reg.Register(&Ability{
			ID: id, DisplayName: id, Type: AbilityPassive, Category: AbilitySkill,
			Hook: HookDefensive, Variance: 100, MaxHitChance: 100,
		}); err != nil {
			t.Fatal(err)
		}
	}
	prof := newProfStub()
	prof.vals["dodge"] = 50
	prof.vals["parry"] = 50

	// First-in-sorted-order (dodge) fires on roll 1 → evade, returns it.
	// Only one binary roll consumed; parry is never tried.
	t.Run("first firing passive evades", func(t *testing.T) {
		roller := &seqRoller{t: t, seq: []int{0}} // dodge binary fires; no gain (GainBaseChance 0)
		pr := NewPassiveResolver(reg, prof, prof, roller)
		name, evaded := pr.DefensiveEvade("p1")
		if !evaded || name != "dodge" {
			t.Fatalf("DefensiveEvade = (%q, %v), want (dodge, true)", name, evaded)
		}
	})

	// Both miss → no evade. Two binary rolls (dodge, parry), both >50.
	t.Run("none fire", func(t *testing.T) {
		roller := &seqRoller{t: t, seq: []int{99, 99}}
		pr := NewPassiveResolver(reg, prof, prof, roller)
		if name, evaded := pr.DefensiveEvade("p1"); evaded {
			t.Errorf("DefensiveEvade = (%q, true), want no evade", name)
		}
	})
}
