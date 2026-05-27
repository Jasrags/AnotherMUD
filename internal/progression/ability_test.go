package progression_test

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
)

func mkAbility(id string, opts ...func(*progression.Ability)) *progression.Ability {
	a := &progression.Ability{
		ID:          id,
		DisplayName: id,
		Type:        progression.AbilityActive,
		Category:    progression.AbilitySkill,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

func withPriority(p int) func(*progression.Ability) {
	return func(a *progression.Ability) { a.Priority = p }
}

func withDisplay(s string) func(*progression.Ability) {
	return func(a *progression.Ability) { a.DisplayName = s }
}

func TestAbilityRegistry_Register_RejectsMalformed(t *testing.T) {
	r := progression.NewAbilityRegistry()
	cases := []struct {
		name string
		in   *progression.Ability
	}{
		{"nil", nil},
		{"empty id", &progression.Ability{Type: progression.AbilityActive, Category: progression.AbilitySkill}},
		{"whitespace id", &progression.Ability{ID: "   ", Type: progression.AbilityActive, Category: progression.AbilitySkill}},
		{"invalid type", &progression.Ability{ID: "kick", Type: "bogus", Category: progression.AbilitySkill}},
		{"invalid category", &progression.Ability{ID: "kick", Type: progression.AbilityActive, Category: "bogus"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := r.Register(tc.in); err == nil {
				t.Fatalf("Register(%v) = nil, want error", tc.in)
			}
		})
	}
}

func TestAbilityRegistry_Register_LowercasesAndStores(t *testing.T) {
	r := progression.NewAbilityRegistry()
	if err := r.Register(mkAbility("KICK", withDisplay("Kick"))); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, ok := r.Get("kick")
	if !ok {
		t.Fatalf("Get(kick) = false, want true")
	}
	if got.ID != "kick" {
		t.Errorf("ID = %q, want %q", got.ID, "kick")
	}
	if got.DisplayName != "Kick" {
		t.Errorf("DisplayName = %q, want %q", got.DisplayName, "Kick")
	}
	if !r.Has("Kick") {
		t.Errorf("Has(mixed-case) = false")
	}
}

func TestAbilityRegistry_Register_PriorityOverride(t *testing.T) {
	r := progression.NewAbilityRegistry()
	if err := r.Register(mkAbility("kick", withDisplay("Low"), withPriority(0))); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(mkAbility("kick", withDisplay("Mid"), withPriority(5))); err != nil {
		t.Fatal(err)
	}
	if a, _ := r.Get("kick"); a.DisplayName != "Mid" {
		t.Fatalf("after p=5 override, DisplayName = %q, want Mid", a.DisplayName)
	}
	if err := r.Register(mkAbility("kick", withDisplay("LowerAgain"), withPriority(1))); err != nil {
		t.Fatal(err)
	}
	if a, _ := r.Get("kick"); a.DisplayName != "Mid" {
		t.Fatalf("lower-priority Register changed entry: DisplayName = %q, want Mid", a.DisplayName)
	}
	if err := r.Register(mkAbility("kick", withDisplay("EqualNoop"), withPriority(5))); err != nil {
		t.Fatal(err)
	}
	if a, _ := r.Get("kick"); a.DisplayName != "Mid" {
		t.Fatalf("equal-priority Register changed entry: DisplayName = %q, want Mid", a.DisplayName)
	}
}

func TestAbilityRegistry_Register_DefaultCapClamped(t *testing.T) {
	r := progression.NewAbilityRegistry()
	r.Register(&progression.Ability{ID: "low", DisplayName: "Low", Type: progression.AbilityActive, Category: progression.AbilitySkill, DefaultCap: -5})
	r.Register(&progression.Ability{ID: "high", DisplayName: "High", Type: progression.AbilityActive, Category: progression.AbilitySkill, DefaultCap: 500})
	if a, _ := r.Get("low"); a.DefaultCap != 0 {
		t.Errorf("negative cap not clamped: %d", a.DefaultCap)
	}
	if a, _ := r.Get("high"); a.DefaultCap != 100 {
		t.Errorf(">100 cap not clamped: %d", a.DefaultCap)
	}
}

func TestAbilityRegistry_All_SortedById(t *testing.T) {
	r := progression.NewAbilityRegistry()
	for _, id := range []string{"charlie", "alpha", "bravo"} {
		r.Register(mkAbility(id))
	}
	got := r.All()
	want := []string{"alpha", "bravo", "charlie"}
	if len(got) != len(want) {
		t.Fatalf("All() len = %d, want %d", len(got), len(want))
	}
	for i, a := range got {
		if a.ID != want[i] {
			t.Errorf("All()[%d] = %q, want %q", i, a.ID, want[i])
		}
	}
}

func TestAbilityRegistry_ByType(t *testing.T) {
	r := progression.NewAbilityRegistry()
	r.Register(&progression.Ability{ID: "kick", DisplayName: "Kick", Type: progression.AbilityActive, Category: progression.AbilitySkill})
	r.Register(&progression.Ability{ID: "second-attack", DisplayName: "Second Attack", Type: progression.AbilityPassive, Category: progression.AbilitySkill})
	r.Register(&progression.Ability{ID: "heal", DisplayName: "Heal", Type: progression.AbilityActive, Category: progression.AbilitySpell})

	active := r.ByType(progression.AbilityActive)
	if len(active) != 2 || active[0].ID != "heal" || active[1].ID != "kick" {
		t.Errorf("ByType(active) = %+v, want [heal kick]", ids(active))
	}
	passive := r.ByType(progression.AbilityPassive)
	if len(passive) != 1 || passive[0].ID != "second-attack" {
		t.Errorf("ByType(passive) = %+v, want [second-attack]", ids(passive))
	}
}

func ids(as []*progression.Ability) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.ID
	}
	return out
}

func TestParseAbilityTypeAndCategory(t *testing.T) {
	if v, ok := progression.ParseAbilityType("ACTIVE"); !ok || v != progression.AbilityActive {
		t.Errorf("ParseAbilityType(ACTIVE) = (%v,%v)", v, ok)
	}
	if v, ok := progression.ParseAbilityType("passive"); !ok || v != progression.AbilityPassive {
		t.Errorf("ParseAbilityType(passive) = (%v,%v)", v, ok)
	}
	if _, ok := progression.ParseAbilityType("bogus"); ok {
		t.Errorf("ParseAbilityType(bogus) ok=true")
	}
	if v, ok := progression.ParseAbilityCategory("Spell"); !ok || v != progression.AbilitySpell {
		t.Errorf("ParseAbilityCategory(Spell) = (%v,%v)", v, ok)
	}
	if _, ok := progression.ParseAbilityCategory(""); ok {
		t.Errorf("ParseAbilityCategory(empty) ok=true")
	}
}
