package progression

import "testing"

func TestSaveTypeGoverningStat(t *testing.T) {
	cases := map[SaveType]StatType{
		SaveFortitude: StatCON,
		SaveReflex:    StatDEX,
		SaveWill:      StatWIS,
		SaveType("?"): "",
	}
	for st, want := range cases {
		if got := st.GoverningStat(); got != want {
			t.Errorf("%s.GoverningStat() = %q, want %q", st, got, want)
		}
	}
}

func TestAbilityModifier(t *testing.T) {
	// Go integer division truncates toward zero — the same convention the
	// engine's combat.STRBonus uses and the spec cites (saves §2). Odd
	// sub-10 scores therefore round toward zero, not floor.
	cases := []struct {
		score int
		want  int
	}{
		{10, 0}, {11, 0}, {12, 1}, {9, 0}, {8, -1}, {7, -1}, {20, 5}, {1, -4},
	}
	for _, c := range cases {
		if got := AbilityModifier(c.score); got != c.want {
			t.Errorf("AbilityModifier(%d) = %d, want %d", c.score, got, c.want)
		}
	}
}

func TestSaveProgressionCurveBonusAt(t *testing.T) {
	cfg := DefaultSaveConfig()
	// d20 good save: 2 + level/2 → 2,3,3,4,4,5 ; poor save: level/3 → 0,0,1,1,1,2
	strong := []int{2, 3, 3, 4, 4, 5}
	weak := []int{0, 0, 1, 1, 1, 2}
	for i := range strong {
		level := i + 1
		if got := cfg.Strong.BonusAt(level); got != strong[i] {
			t.Errorf("strong.BonusAt(%d) = %d, want %d", level, got, strong[i])
		}
		if got := cfg.Weak.BonusAt(level); got != weak[i] {
			t.Errorf("weak.BonusAt(%d) = %d, want %d", level, got, weak[i])
		}
	}
	// Level below 1 clamps to 1 (keeps the base term).
	if got := cfg.Strong.BonusAt(0); got != 2 {
		t.Errorf("strong.BonusAt(0) = %d, want 2", got)
	}
	// Non-positive divisor degrades to a flat base.
	flat := SaveProgressionCurve{Base: 3, Divisor: 0}
	if got := flat.BonusAt(99); got != 3 {
		t.Errorf("flat.BonusAt(99) = %d, want 3", got)
	}
}

func TestClassBaseSaves_StrongWeakDefault(t *testing.T) {
	cfg := DefaultSaveConfig()
	// Fortitude strong, Reflex weak, Will undeclared (defaults to weak).
	c := &Class{
		ID: "warder",
		SaveProgressions: map[SaveType]SaveProgression{
			SaveFortitude: SaveStrong,
			SaveReflex:    SaveWeak,
		},
	}
	got := ClassBaseSaves([]ClassSaveInput{{Class: c, Level: 4}}, cfg)
	want := Saves{
		Fortitude: cfg.Strong.BonusAt(4), // 4
		Reflex:    cfg.Weak.BonusAt(4),    // 1
		Will:      cfg.Weak.BonusAt(4),    // 1 — undeclared → weak
	}
	if got != want {
		t.Errorf("ClassBaseSaves = %+v, want %+v", got, want)
	}
}

func TestClassBaseSaves_NoInputsZero(t *testing.T) {
	got := ClassBaseSaves(nil, DefaultSaveConfig())
	if (got != Saves{}) {
		t.Errorf("ClassBaseSaves(nil) = %+v, want zero", got)
	}
}

// The mob path passes a non-nil but empty slice (a mob with no class
// entries); base saves must be zero so the modifier-only derivation in
// DeriveSaves carries the whole value (saves §2 mob rule).
func TestClassBaseSaves_EmptySliceIsZero(t *testing.T) {
	got := ClassBaseSaves([]ClassSaveInput{}, DefaultSaveConfig())
	if (got != Saves{}) {
		t.Errorf("ClassBaseSaves(empty) = %+v, want zero", got)
	}
}

func TestClassBaseSaves_MulticlassBestPerAxis(t *testing.T) {
	cfg := DefaultSaveConfig()
	// Class A: strong Fort, weak Reflex/Will. Class B: strong Reflex, weak rest.
	// Best-per-axis: Fort from A, Reflex from B, Will weak from either.
	a := &Class{ID: "a", SaveProgressions: map[SaveType]SaveProgression{SaveFortitude: SaveStrong}}
	b := &Class{ID: "b", SaveProgressions: map[SaveType]SaveProgression{SaveReflex: SaveStrong}}
	got := ClassBaseSaves([]ClassSaveInput{{Class: a, Level: 6}, {Class: b, Level: 6}}, cfg)
	want := Saves{
		Fortitude: cfg.Strong.BonusAt(6), // 5 (from A)
		Reflex:    cfg.Strong.BonusAt(6), // 5 (from B)
		Will:      cfg.Weak.BonusAt(6),   // 2 (weak in both)
	}
	if got != want {
		t.Errorf("multiclass ClassBaseSaves = %+v, want %+v", got, want)
	}
}

func TestClassBaseSaves_NilClassSkipped(t *testing.T) {
	cfg := DefaultSaveConfig()
	c := &Class{ID: "a", SaveProgressions: map[SaveType]SaveProgression{SaveWill: SaveStrong}}
	got := ClassBaseSaves([]ClassSaveInput{{Class: nil, Level: 3}, {Class: c, Level: 3}}, cfg)
	if got.Will != cfg.Strong.BonusAt(3) {
		t.Errorf("Will = %d, want %d (nil class must be skipped)", got.Will, cfg.Strong.BonusAt(3))
	}
}

func TestDeriveSaves_AddsAbilityModifier(t *testing.T) {
	base := Saves{Fortitude: 4, Reflex: 1, Will: 1}
	scores := map[StatType]int{StatCON: 14, StatDEX: 8, StatWIS: 10}
	got := DeriveSaves(base, func(s StatType) int { return scores[s] })
	want := Saves{
		Fortitude: 4 + AbilityModifier(14), // 4 + 2 = 6
		Reflex:    1 + AbilityModifier(8),  // 1 + (-1) = 0
		Will:      1 + AbilityModifier(10), // 1 + 0 = 1
	}
	if got != want {
		t.Errorf("DeriveSaves = %+v, want %+v", got, want)
	}
}

func TestDeriveSaves_NilScoreIsBaseOnly(t *testing.T) {
	base := Saves{Fortitude: 4, Reflex: 1, Will: 2}
	if got := DeriveSaves(base, nil); got != base {
		t.Errorf("DeriveSaves(base, nil) = %+v, want base %+v", got, base)
	}
}

func TestSavesGet(t *testing.T) {
	s := Saves{Fortitude: 1, Reflex: 2, Will: 3}
	if s.Get(SaveFortitude) != 1 || s.Get(SaveReflex) != 2 || s.Get(SaveWill) != 3 {
		t.Errorf("Get mismatch: %+v", s)
	}
	if s.Get(SaveType("?")) != 0 {
		t.Errorf("Get(unknown) = %d, want 0", s.Get(SaveType("?")))
	}
}

func TestClassRegisterDeepCopiesSaveProgressions(t *testing.T) {
	reg := NewClassRegistry()
	src := map[SaveType]SaveProgression{SaveFortitude: SaveStrong}
	if err := reg.Register(&Class{ID: "fighter", SaveProgressions: src}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// Mutate the caller's map after Register; the registry copy must not change.
	src[SaveFortitude] = SaveWeak
	src[SaveReflex] = SaveStrong
	got, ok := reg.Get("fighter")
	if !ok {
		t.Fatal("class not registered")
	}
	if got.SaveProgressions[SaveFortitude] != SaveStrong {
		t.Errorf("Fortitude = %q, want strong (registry copy mutated)", got.SaveProgressions[SaveFortitude])
	}
	if _, present := got.SaveProgressions[SaveReflex]; present {
		t.Error("Reflex leaked into registry copy")
	}
}

// Lowercasing on Register: an axis or progression declared in mixed case
// resolves to the canonical lowercase form the lookups use.
func TestClassRegisterLowercasesSaveProgressions(t *testing.T) {
	reg := NewClassRegistry()
	if err := reg.Register(&Class{
		ID:               "wilder",
		SaveProgressions: map[SaveType]SaveProgression{SaveType("WILL"): SaveProgression("STRONG")},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, _ := reg.Get("wilder")
	if got.SaveProgressions[SaveWill] != SaveStrong {
		t.Errorf("Will = %q, want strong after lowercasing", got.SaveProgressions[SaveWill])
	}
}
