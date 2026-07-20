package session

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/feat"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/wizard"
)

// The review sheet must sit immediately before the confirm prompt in every
// creation flow, so the player always sees the recap before committing. Guards
// both the default and WoT builders.
func TestCreationFlow_SummaryImmediatelyPrecedesConfirm(t *testing.T) {
	rr := progression.NewRaceRegistry()
	must(t, rr.Register(&progression.Race{ID: "human", DisplayName: "Human"}))
	cr := progression.NewClassRegistry()
	must(t, cr.Register(&progression.Class{ID: "face", DisplayName: "Face"}))

	for _, world := range []string{"shadowrun", "wot", "starter-world", ""} {
		flow := CreationFlowFor(world, rr, cr, nil, nil)
		if flow == nil {
			t.Fatalf("world %q: nil flow", world)
		}
		si, ci := -1, -1
		for i, s := range flow.Steps {
			switch s.StepID() {
			case "summary":
				si = i
			case "confirm":
				ci = i
			}
		}
		if si < 0 {
			t.Errorf("world %q: no summary step", world)
			continue
		}
		if ci != si+1 {
			t.Errorf("world %q: summary(%d) not immediately before confirm(%d)", world, si, ci)
		}
		if flow.Steps[si].(*wizard.InfoStep).Interactive() {
			t.Errorf("world %q: summary should be non-interactive", world)
		}
	}
}

// srSummaryFixtures builds registries mirroring the shadowrun Human/Ork
// metatypes, the Face role, and the Corporate Dropout origin so the review
// sheet can be asserted against real content shapes.
func srSummaryFixtures(t *testing.T) (*progression.RaceRegistry, *progression.ClassRegistry, *progression.BackgroundRegistry, *feat.Registry) {
	t.Helper()
	rr := progression.NewRaceRegistry()
	must(t, rr.Register(&progression.Race{ID: "human", DisplayName: "Human", RacialFlags: []string{"common-tongue"}}))
	must(t, rr.Register(&progression.Race{ID: "ork", DisplayName: "Ork", RacialFlags: []string{"common-tongue", "low-light"},
		StatBonuses: map[progression.StatType]int{"body": 3, "strength": 2, "hp_max": 3}}))

	cr := progression.NewClassRegistry()
	must(t, cr.Register(&progression.Class{ID: "face", DisplayName: "Face",
		ProficiencyTiers: []string{"simple"}, ArmorProficiencyTiers: []string{"light"},
		SaveProgressions: map[progression.SaveType]progression.SaveProgression{
			"will": progression.SaveStrong, "fortitude": progression.SaveWeak, "reflex": progression.SaveWeak},
		StartingItems: []string{"shadowrun:streetline-special"},
		Path: []progression.ClassPathEntry{
			{Level: 1, AbilityID: "negotiation"}, {Level: 1, AbilityID: "con"},
			{Level: 1, AbilityID: "intimidation"}, {Level: 1, AbilityID: "perception"}}}))

	br := progression.NewBackgroundRegistry()
	must(t, br.Register(&progression.Background{ID: "corporate-dropout", DisplayName: "Corporate Dropout",
		Skills:      []progression.BackgroundSkill{{AbilityID: "negotiation", Proficiency: 10}, {AbilityID: "perception", Proficiency: 10}},
		FeatOptions: []string{"alertness", "iron-will"}, Gold: 2500,
		Items: []string{"shadowrun:corporate-sin", "shadowrun:hermes-ikon", "shadowrun:armor-vest"}}))
	// A second origin that offers a pick-one equipment package, to exercise the
	// chosen-package branch of creationGear.
	must(t, br.Register(&progression.Background{ID: "street-kid", DisplayName: "Street Kid",
		Items: []string{"shadowrun:meta-link"}, Gold: 500,
		EquipmentPackages: [][]string{
			{"shadowrun:ares-predator-v", "shadowrun:armored-jacket"},
			{"shadowrun:katana", "shadowrun:armored-jacket"}}}))

	fr := feat.NewRegistry()
	must(t, fr.Register(&feat.Feat{ID: "iron-will", DisplayName: "Iron Will"}))
	return rr, cr, br, fr
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("register: %v", err)
	}
}

func TestRenderCreationSummary_FullSheet(t *testing.T) {
	rr, cr, br, fr := srSummaryFixtures(t)
	ce := &creationEntity{gender: "female", raceID: "human", classID: "face",
		backgroundID: "corporate-dropout", backgroundFeat: "iron-will"}

	got := renderCreationSummary(ce, rr, cr, br, fr)

	wants := []string{
		"Gender    Female",
		"Metatype  Human",
		"no attribute skew · vision: normal · size: medium",
		"Role      Face",
		"weapons: simple · armor: light · strong Will save",
		"Origin    Corporate Dropout",
		"Talent    Iron Will", // resolved via feat registry, not raw id
		"Funds     2500",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("summary missing %q\n--- got ---\n%s", w, got)
		}
	}
}

// The merged skill sheet dedups a skill granted by BOTH the role and the origin
// (negotiation/perception) — origin skills merge over the class floor rather
// than stacking, so each name appears once.
func TestCreationSkills_DedupsRoleAndOriginOverlap(t *testing.T) {
	_, cr, br, _ := srSummaryFixtures(t)
	c, _ := cr.Get("face")
	b, _ := br.Get("corporate-dropout")

	got := creationSkills(c, b)
	if strings.Count(strings.Join(got, ","), "negotiation") != 1 {
		t.Errorf("negotiation should appear once, got %v", got)
	}
	if strings.Count(strings.Join(got, ","), "perception") != 1 {
		t.Errorf("perception should appear once, got %v", got)
	}
	// con + intimidation are role-only; both present.
	joined := strings.Join(got, ",")
	for _, s := range []string{"con", "intimidation"} {
		if !strings.Contains(joined, s) {
			t.Errorf("skills missing %q, got %v", s, got)
		}
	}
}

// Gear is the union of the role floor, the origin's always-granted items, and
// the chosen equipment package.
func TestCreationGear_IncludesChosenPackage(t *testing.T) {
	_, cr, br, _ := srSummaryFixtures(t)
	c, _ := cr.Get("face")
	b, _ := br.Get("street-kid")

	// Pick package index 1 (katana + armored-jacket).
	ce := &creationEntity{backgroundEquipment: 1}
	got := strings.Join(creationGear(ce, c, b), ",")
	for _, w := range []string{"shadowrun:streetline-special", "shadowrun:meta-link", "shadowrun:katana"} {
		if !strings.Contains(got, w) {
			t.Errorf("gear missing %q, got %s", w, got)
		}
	}
	if strings.Contains(got, "ares-predator-v") {
		t.Errorf("gear should not include the unchosen package 0 item, got %s", got)
	}
}

// An out-of-range package index falls back to package 0 (the granter's default)
// rather than panicking or dropping the gear.
func TestCreationGear_OutOfRangePackageFallsBackToDefault(t *testing.T) {
	_, _, br, _ := srSummaryFixtures(t)
	b, _ := br.Get("street-kid")
	ce := &creationEntity{backgroundEquipment: 99}
	got := strings.Join(creationGear(ce, nil, b), ",")
	if !strings.Contains(got, "ares-predator-v") {
		t.Errorf("out-of-range index should fall back to package 0, got %s", got)
	}
}

func TestRaceBenefit_SkewVisionSize(t *testing.T) {
	ork := &progression.Race{RacialFlags: []string{"low-light"},
		StatBonuses: map[progression.StatType]int{"body": 3, "hp_max": 3}}
	got := raceBenefit(ork)
	for _, w := range []string{"+3 Body", "+3 HP", "vision: low-light", "size: medium"} {
		if !strings.Contains(got, w) {
			t.Errorf("raceBenefit missing %q, got %q", w, got)
		}
	}

	human := &progression.Race{RacialFlags: []string{"common-tongue"}}
	if got := raceBenefit(human); !strings.Contains(got, "no attribute skew") {
		t.Errorf("baseline metatype should read 'no attribute skew', got %q", got)
	}
}

// A background with exactly ONE feat option auto-grants it (no pick step runs,
// so ce.backgroundFeat stays empty). The review sheet must still show it,
// resolved from the origin — mirroring the equipment package-0 fallback.
func TestResolvedFeatID_SingleOptionAutoGrant(t *testing.T) {
	solo := &progression.Background{ID: "ex-security", FeatOptions: []string{"toughness"}}
	if got := resolvedFeatID(&creationEntity{}, solo); got != "toughness" {
		t.Errorf("single-option origin: got %q, want the auto-granted feat", got)
	}
	// An explicit pick always wins over the fallback.
	if got := resolvedFeatID(&creationEntity{backgroundFeat: "iron-will"}, solo); got != "iron-will" {
		t.Errorf("explicit pick should win: got %q", got)
	}
	// Zero options → no talent.
	none := &progression.Background{ID: "blank"}
	if got := resolvedFeatID(&creationEntity{}, none); got != "" {
		t.Errorf("no options: got %q, want empty", got)
	}

	// End-to-end: the summary row appears for the auto-granted talent even
	// though backgroundFeat was never set by a (skipped) pick step.
	br := progression.NewBackgroundRegistry()
	must(t, br.Register(solo))
	fr := feat.NewRegistry()
	must(t, fr.Register(&feat.Feat{ID: "toughness", DisplayName: "Toughness"}))
	got := renderCreationSummary(&creationEntity{backgroundID: "ex-security"}, nil, nil, br, fr)
	if !strings.Contains(got, "Talent") || !strings.Contains(got, "Toughness") {
		t.Errorf("auto-granted talent missing from summary:\n%s", got)
	}
}

// featDisplay falls back to the raw id when the registry lacks the feat (or is
// nil), so the review never renders a blank talent line for a real pick.
func TestFeatDisplay_FallsBackToID(t *testing.T) {
	if got := featDisplay(nil, "iron-will"); got != "iron-will" {
		t.Errorf("nil registry: got %q, want raw id", got)
	}
	if got := featDisplay(nil, ""); got != "" {
		t.Errorf("empty id: got %q, want empty", got)
	}
}
