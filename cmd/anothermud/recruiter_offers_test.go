package main

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/mob"
)

// RecruiterOffers returns the hirelings offered by recruiter templates present in
// a room (hireable-mobs.md §3.1): offer entries resolve to hireable templates (by
// id or name/keyword), non-recruiters contribute nothing, deduped + name-ordered.
func TestRecruiterOffers(t *testing.T) {
	reg := mob.NewTemplates()
	reg.Add(&mob.Template{ID: "sw:sellsword", Name: "a grizzled sellsword", Hireling: &mob.HirelingSpec{HireCost: 50}})
	reg.Add(&mob.Template{ID: "sw:archer", Name: "an archer", Hireling: &mob.HirelingSpec{HireCost: 80}})
	reg.Add(&mob.Template{ID: "sw:captain", Name: "a captain",
		Recruiter: &mob.RecruiterSpec{Offers: []string{"sellsword", "sw:archer"}}}) // bare + exact
	reg.Add(&mob.Template{ID: "sw:guard", Name: "a guard"}) // ordinary mob, not a recruiter
	h := &hirelingService{spawner: &bootSpawner{mobTemplates: reg}}

	// A recruiter + a non-recruiter present → offers from the recruiter only,
	// sorted by name ("a grizzled sellsword" < "an archer").
	offers := h.RecruiterOffers([]string{"sw:guard", "sw:captain"})
	if len(offers) != 2 {
		t.Fatalf("offers = %+v, want 2 (sellsword + archer)", offers)
	}
	if offers[0].TemplateID != "sw:sellsword" || offers[0].HireCost != 50 {
		t.Errorf("offer[0] = %+v, want the sellsword at 50", offers[0])
	}
	if offers[1].TemplateID != "sw:archer" || offers[1].HireCost != 80 {
		t.Errorf("offer[1] = %+v, want the archer at 80", offers[1])
	}

	// No recruiter present → no offers.
	if got := h.RecruiterOffers([]string{"sw:guard"}); got != nil {
		t.Errorf("non-recruiter offers = %+v, want nil", got)
	}

	// Two recruiters offering the same hireling → deduped.
	reg.Add(&mob.Template{ID: "sw:captain2", Name: "another captain",
		Recruiter: &mob.RecruiterSpec{Offers: []string{"sw:sellsword"}}})
	if got := h.RecruiterOffers([]string{"sw:captain", "sw:captain2"}); len(got) != 2 {
		t.Errorf("deduped offers = %+v, want 2 distinct (sellsword once)", got)
	}
}
