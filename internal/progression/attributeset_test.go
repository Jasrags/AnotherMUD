package progression

import (
	"maps"
	"testing"
)

// classicish is a small well-formed set used across the tests.
func classicish() *AttributeSet {
	return &AttributeSet{
		ID:   "classic",
		Name: "Classic Six",
		Attributes: []Attribute{
			{ID: "str", Name: "Strength", Abbrev: "STR", Default: 10, Cap: 22, Trainable: true, Category: "physical"},
			{ID: "int", Name: "Intelligence", Abbrev: "INT", Default: 10, Cap: 22, Trainable: true, Category: "mental"},
			{ID: "luck", Name: "Luck", Abbrev: "LCK", Default: 10, Trainable: false, Category: "special"},
		},
	}
}

func TestAttributeSetRegister_LowercasesAndClones(t *testing.T) {
	// Arrange
	r := NewAttributeSetRegistry()
	src := &AttributeSet{
		ID: "  Classic  ",
		Attributes: []Attribute{
			{ID: "STR", Category: "Physical", Default: 10},
		},
	}

	// Act
	if err := r.Register(src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// Mutating the source after Register must not affect the stored copy.
	src.Attributes[0].Default = 999

	// Assert
	got, ok := r.Get("classic")
	if !ok {
		t.Fatal("Get(classic): not found (id not lowercased/trimmed?)")
	}
	if got.ID != "classic" {
		t.Errorf("set ID = %q, want classic", got.ID)
	}
	a := got.Attributes[0]
	if a.ID != "str" {
		t.Errorf("attr ID = %q, want str (not lowercased)", a.ID)
	}
	if a.Category != "physical" {
		t.Errorf("attr Category = %q, want physical (not lowercased)", a.Category)
	}
	if a.Default != 10 {
		t.Errorf("attr Default = %d, want 10 (registry stored a reference, not a clone)", a.Default)
	}
}

func TestAttributeSetRegister_Rejects(t *testing.T) {
	tests := []struct {
		name string
		set  *AttributeSet
	}{
		{"nil set", nil},
		{"empty id", &AttributeSet{ID: "  ", Attributes: []Attribute{{ID: "str"}}}},
		{"no attributes", &AttributeSet{ID: "empty"}},
		{"empty attr id", &AttributeSet{ID: "x", Attributes: []Attribute{{ID: "  "}}}},
		{"duplicate attr id", &AttributeSet{ID: "x", Attributes: []Attribute{{ID: "str"}, {ID: "STR"}}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewAttributeSetRegistry()
			if err := r.Register(tc.set); err == nil {
				t.Fatalf("Register(%s): expected error, got nil", tc.name)
			}
		})
	}
}

func TestAttributeSetRegister_PriorityWins(t *testing.T) {
	r := NewAttributeSetRegistry()
	low := &AttributeSet{ID: "s", Priority: 1, Name: "low", Attributes: []Attribute{{ID: "a", Default: 1}}}
	high := &AttributeSet{ID: "s", Priority: 2, Name: "high", Attributes: []Attribute{{ID: "a", Default: 2}}}
	equal := &AttributeSet{ID: "s", Priority: 2, Name: "equal-noop", Attributes: []Attribute{{ID: "a", Default: 3}}}

	if err := r.Register(low); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(high); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(equal); err != nil {
		t.Fatal(err)
	}

	got, _ := r.Get("s")
	if got.Name != "high" {
		t.Errorf("Name = %q, want high (higher priority should win, equal should no-op)", got.Name)
	}
}

func TestAttributeSetAccessors(t *testing.T) {
	s := classicish()

	// Keys preserve declared order.
	keys := s.Keys()
	want := []StatType{"str", "int", "luck"}
	if len(keys) != len(want) {
		t.Fatalf("Keys len = %d, want %d", len(keys), len(want))
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Errorf("Keys[%d] = %q, want %q (order not preserved)", i, keys[i], want[i])
		}
	}

	// Defaults maps id → seed.
	defs := s.Defaults()
	if defs["str"] != 10 || defs["luck"] != 10 {
		t.Errorf("Defaults = %v, want str/luck = 10", defs)
	}
	if _, ok := defs["hp_max"]; ok {
		t.Error("Defaults leaked an engine-vital key (hp_max) — it should only carry declared attributes")
	}

	// Caps only includes positive caps (luck declares none).
	caps := s.Caps()
	if caps["str"] != 22 {
		t.Errorf("Caps[str] = %d, want 22", caps["str"])
	}
	if _, ok := caps["luck"]; ok {
		t.Error("Caps included luck, which declares no positive cap")
	}

	// TrainableSet only carries trainable attrs, string-keyed.
	tr := s.TrainableSet()
	if !tr["str"] || !tr["int"] {
		t.Errorf("TrainableSet = %v, want str+int trainable", tr)
	}
	if tr["luck"] {
		t.Error("TrainableSet marked luck trainable; it is not")
	}

	// Get is case-insensitive.
	if a, ok := s.Get("STR"); !ok || a.Name != "Strength" {
		t.Errorf("Get(STR) = (%+v, %v), want Strength", a, ok)
	}
	if _, ok := s.Get("nope"); ok {
		t.Error("Get(nope) reported found")
	}
}

// classicSet mirrors content/core/attributes/classic.yaml — the six at 10.
func classicSet() *AttributeSet {
	return &AttributeSet{
		ID: ClassicAttributeSetID,
		Attributes: []Attribute{
			{ID: StatSTR, Default: 10}, {ID: StatINT, Default: 10}, {ID: StatWIS, Default: 10},
			{ID: StatDEX, Default: 10}, {ID: StatCON, Default: 10}, {ID: StatLUCK, Default: 10},
		},
	}
}

// The regression invariant (SR-M1 step 3): seeding from the `classic` set must
// produce byte-identical output to the DefaultPlayerBase hardcode, so a world
// resolving to `classic` (every world today) seeds exactly as before.
func TestSeedBaseFromSet_ClassicEqualsDefaultPlayerBase(t *testing.T) {
	got := SeedBaseFromSet(classicSet())
	want := DefaultPlayerBase()
	if !maps.Equal(got, want) {
		t.Errorf("SeedBaseFromSet(classic) = %v, want DefaultPlayerBase() = %v", got, want)
	}
}

// SeedBaseFromSet layers a set's attribute defaults over the engine-vital keys,
// and a non-classic set yields DIFFERENT attributes but the SAME vital keys.
func TestSeedBaseFromSet_ComposesVitalsAndAttributes(t *testing.T) {
	set := &AttributeSet{
		ID: "sr",
		Attributes: []Attribute{
			{ID: "body", Default: 3},
			{ID: "agility", Default: 4},
		},
	}
	got := SeedBaseFromSet(set)

	if got["body"] != 3 || got["agility"] != 4 {
		t.Errorf("attributes not seeded: %v", got)
	}
	// Engine-vital keys always present regardless of set.
	if got[StatHPMax] != 20 || got[StatMovementMax] != DefaultMovementMax || got[StatAC] != 10 {
		t.Errorf("engine-vital keys missing/wrong: %v", got)
	}
	// The classic six are NOT present — a Shadowrun character carries only its
	// own attribute keys (the whole point of the fix).
	if _, ok := got[StatSTR]; ok {
		t.Error("SR seed leaked the classic 'str' key — the carries-both-sets bug")
	}
}

// A nil set yields the vital base alone (callers resolve the classic fallback
// before reaching here).
func TestSeedBaseFromSet_NilSet(t *testing.T) {
	got := SeedBaseFromSet(nil)
	if got[StatHPMax] != 20 {
		t.Errorf("nil set should still carry vital keys: %v", got)
	}
	if _, ok := got[StatSTR]; ok {
		t.Error("nil set should carry no attribute keys")
	}
}
