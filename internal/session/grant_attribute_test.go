package session

import (
	"slices"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
)

// wireGrantActor builds a connActor with every grantable store wired: feats
// (from newFeatActor), an ability registry + proficiency manager, a recipe
// registry + known-manager, and a language registry — each seeded with one
// real entry to grant.
func wireGrantActor(t *testing.T) *connActor {
	t.Helper()
	a := newFeatActor(t, 0) // feats registry (incl. iron-will) + a save

	abilities := progression.NewAbilityRegistry()
	if err := abilities.Register(&progression.Ability{
		ID: "pistols", Type: progression.AbilityPassive, Category: progression.AbilitySkill, DefaultCap: 100,
	}); err != nil {
		t.Fatalf("register ability: %v", err)
	}
	a.abilities = abilities
	a.prof = progression.NewProficiencyManager(abilities, progression.DefaultProficiencyConfig())

	recipes := recipe.NewRegistry()
	recipes.Add(&recipe.Recipe{ID: "steel-blade", DisplayName: "Steel Blade", Discipline: "smithing"})
	a.known = recipe.NewKnownManager(recipes)

	langs := progression.NewLanguageRegistry()
	if err := langs.Register(&progression.Language{ID: "ogier", Name: "Ogier"}); err != nil {
		t.Fatalf("register language: %v", err)
	}
	a.languages = langs
	return a
}

// TestGrantAttribute_AllKinds exercises the generalized admin grant/revoke seam
// on a real connActor: each kind grants (changed), is idempotent on re-grant,
// and revokes (changed) — the full add/remove lifecycle.
func TestGrantAttribute_AllKinds(t *testing.T) {
	a := wireGrantActor(t)

	cases := []struct {
		kind, value string
		has         func() bool
	}{
		{"role", "builder", func() bool { return a.HasRole("builder") }},
		{"feat", "iron-will", func() bool { return a.HasFeat("iron-will") }},
		{"ability", "pistols", func() bool { return a.prof.Has(a.playerID, "pistols") }},
		{"recipe", "steel-blade", func() bool { return a.known.Knows(a.playerID, "steel-blade") }},
		{"language", "ogier", func() bool { return slices.Contains(a.save.KnownLanguages, "ogier") }},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			changed, err := a.GrantAttribute(c.kind, c.value)
			if err != nil || !changed {
				t.Fatalf("grant %s %q = (%v, %v), want (true, nil)", c.kind, c.value, changed, err)
			}
			if !c.has() {
				t.Errorf("target lacks %s %q after grant", c.kind, c.value)
			}
			if again, _ := a.GrantAttribute(c.kind, c.value); again {
				t.Errorf("re-granting %s should be an idempotent no-op", c.kind)
			}
			changed, err = a.RevokeAttribute(c.kind, c.value)
			if err != nil || !changed {
				t.Fatalf("revoke %s = (%v, %v), want (true, nil)", c.kind, changed, err)
			}
			if c.has() {
				t.Errorf("target still has %s %q after revoke", c.kind, c.value)
			}
		})
	}
}

// TestGrantAttribute_ValidationErrors — a value that doesn't name a real thing
// errors (per kind), and an unknown kind errors.
func TestGrantAttribute_ValidationErrors(t *testing.T) {
	a := wireGrantActor(t)
	for _, kind := range []string{"feat", "ability", "recipe", "language"} {
		if _, err := a.GrantAttribute(kind, "does-not-exist"); err == nil {
			t.Errorf("granting a bogus %s should error", kind)
		}
	}
	if _, err := a.GrantAttribute("bogus", "x"); err == nil {
		t.Error("an unknown kind should error")
	}
}
