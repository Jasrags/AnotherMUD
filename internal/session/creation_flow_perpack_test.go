package session

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/wizard"
)

// stepIDs returns the ordered step IDs of a flow (nil flow → nil) for
// structural comparison.
func stepIDs(flow *wizard.Flow) []string {
	if flow == nil {
		return nil
	}
	ids := make([]string, 0, len(flow.Steps))
	for _, s := range flow.Steps {
		ids = append(ids, s.StepID())
	}
	return ids
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func indexOf(ids []string, want string) int {
	for i, id := range ids {
		if id == want {
			return i
		}
	}
	return -1
}

// giftTaggedClasses registers three classes mirroring the WoT pack's gift
// eligibility: two channeler classes (spark|learn) and one non-channeler
// (none). Sorted-by-id, the channeler set is [initiate, wilder] and the
// non-channeler set is [armsman].
func giftTaggedClasses(t *testing.T) *progression.ClassRegistry {
	t.Helper()
	cr := progression.NewClassRegistry()
	must := func(c *progression.Class) {
		t.Helper()
		if err := cr.Register(c); err != nil {
			t.Fatalf("register %s: %v", c.ID, err)
		}
	}
	must(&progression.Class{ID: "initiate", DisplayName: "Initiate", AllowedGifts: []string{"spark", "learn"}})
	must(&progression.Class{ID: "wilder", DisplayName: "Wilder", AllowedGifts: []string{"spark", "learn"}})
	must(&progression.Class{ID: "armsman", DisplayName: "Armsman", AllowedGifts: []string{"none"}})
	return cr
}

// activeClassOptions returns the class-step option labels offered to an entity
// carrying the given gift — the decoupled capability gate as the player sees
// it. The class step is now ONE dynamic step whose OptionsFn filters by the
// gift (and eligibility); an empty raceID yields an unrestricted category, so
// the gift is the only filter for the test's unrestricted classes.
func activeClassOptions(flow *wizard.Flow, gift string) []string {
	e := &creationEntity{channelingGift: gift}
	for _, s := range flow.Steps {
		cs, ok := s.(*wizard.ChoiceStep)
		if !ok || cs.ID != "class" || cs.OptionsFn == nil {
			continue
		}
		labels := make([]string, 0)
		for _, o := range cs.OptionsFn(e) {
			labels = append(labels, o.Label)
		}
		return labels
	}
	return nil
}

// CreationFlowFor for the default/unknown worlds must produce the exact
// same flow as NewCreationFlow — the regression lock for "default flow
// preserved byte-for-byte".
func TestCreationFlowFor_DefaultMatchesNewCreationFlow(t *testing.T) {
	rr, cr := twoRaceOneClass(t)
	want := stepIDs(NewCreationFlow(rr, cr, nil, nil))

	for _, world := range []string{"", "starter-world", "STARTER-WORLD", "nonsense", "  "} {
		got := stepIDs(CreationFlowFor(world, rr, cr, nil, nil))
		if !equalStrings(got, want) {
			t.Errorf("CreationFlowFor(%q) step IDs = %v, want %v (default)", world, got, want)
		}
	}
}

// The nil-content path propagates through the selector for every branch.
func TestCreationFlowFor_NilWhenNoContent(t *testing.T) {
	for _, world := range []string{"", "starter-world", "wot"} {
		if f := CreationFlowFor(world, progression.NewRaceRegistry(), progression.NewClassRegistry(), progression.NewBackgroundRegistry(), nil); f != nil {
			t.Errorf("CreationFlowFor(%q) with empty registries = non-nil, want nil", world)
		}
	}
}

// The WoT flow inserts the channeling step immediately after gender and
// before the class step(s); the default flow has no channeling step.
func TestCreationFlowFor_WoTInsertsChannelingAfterGender(t *testing.T) {
	rr := progression.NewRaceRegistry()
	if err := rr.Register(&progression.Race{ID: "human", DisplayName: "Human"}); err != nil {
		t.Fatalf("register human: %v", err)
	}
	cr := giftTaggedClasses(t)

	def := stepIDs(NewCreationFlow(rr, cr, nil, nil))
	if indexOf(def, "channeling") >= 0 {
		t.Fatalf("default flow unexpectedly has a channeling step: %v", def)
	}

	wot := stepIDs(CreationFlowFor("wot", rr, cr, nil, nil))
	gi, ci := indexOf(wot, "gender"), indexOf(wot, "channeling")
	if gi < 0 {
		t.Fatalf("WoT flow missing gender step: %v", wot)
	}
	if ci < 0 {
		t.Fatalf("WoT flow missing channeling step: %v", wot)
	}
	if ci != gi+1 {
		t.Errorf("channeling at %d, gender at %d; want channeling immediately after gender (%v)", ci, gi, wot)
	}
	if firstClass := indexOf(wot, "class"); firstClass >= 0 && ci > firstClass {
		t.Errorf("channeling (%d) should precede the class step (%d): %v", ci, firstClass, wot)
	}
}

// The decoupled capability gate: a "cannot channel" character is offered only
// non-channeler classes; a spark/learn character only channeler classes.
func TestCreationFlowFor_WoTGiftGatesClassOptions(t *testing.T) {
	rr := progression.NewRaceRegistry()
	if err := rr.Register(&progression.Race{ID: "human", DisplayName: "Human"}); err != nil {
		t.Fatalf("register human: %v", err)
	}
	cr := giftTaggedClasses(t)
	flow := CreationFlowFor("wot", rr, cr, nil, nil)

	cases := []struct {
		gift string
		want []string
	}{
		{"none", []string{"Armsman"}},
		{"spark", []string{"Initiate", "Wilder"}},
		{"learn", []string{"Initiate", "Wilder"}},
	}
	for _, tc := range cases {
		if got := activeClassOptions(flow, tc.gift); !equalStrings(got, tc.want) {
			t.Errorf("gift %q → class options %v, want %v", tc.gift, got, tc.want)
		}
	}
}

// Driving the WoT flow for a "cannot channel" character lands on the
// non-channeler class step (the channeler step is skipped) and records the
// gift. Exercises the Skip-gated gate end-to-end through the instance.
func TestWoTCreationFlow_NoneSelectsNonChanneler(t *testing.T) {
	rr := progression.NewRaceRegistry()
	if err := rr.Register(&progression.Race{ID: "human", DisplayName: "Human"}); err != nil {
		t.Fatalf("register human: %v", err)
	}
	cr := giftTaggedClasses(t)
	flow := CreationFlowFor("wot", rr, cr, nil, nil)
	e := &creationEntity{}
	in := wizard.NewInstance(flow, e, &wizFakeIO{}, nil)
	in.Start(context.Background())
	in.Input(context.Background(), "male")    // gender
	in.Input(context.Background(), "cannot")  // channeling: "Cannot channel" → none
	in.Input(context.Background(), "human")   // race
	in.Input(context.Background(), "armsman") // the non-channeler class step
	st, _ := in.Input(context.Background(), "yes")

	if st != wizard.StatusCompleted {
		t.Fatalf("status = %v, want Completed", st)
	}
	if e.channelingGift != "none" {
		t.Errorf("channelingGift = %q, want none", e.channelingGift)
	}
	if e.classID != "armsman" {
		t.Errorf("classID = %q, want armsman", e.classID)
	}
}

// Driving the WoT channeling step records the choice on the entity and lands
// on a channeler class. Exercises the OnSelect wiring end-to-end.
func TestWoTCreationFlow_SparkSelectsChanneler(t *testing.T) {
	rr := progression.NewRaceRegistry()
	if err := rr.Register(&progression.Race{ID: "human", DisplayName: "Human"}); err != nil {
		t.Fatalf("register human: %v", err)
	}
	cr := giftTaggedClasses(t)
	flow := CreationFlowFor("wot", rr, cr, nil, nil)
	if flow == nil {
		t.Fatal("WoT flow should not be nil with races+classes")
	}
	e := &creationEntity{}
	in := wizard.NewInstance(flow, e, &wizFakeIO{}, nil)
	in.Start(context.Background())
	in.Input(context.Background(), "male")   // gender
	in.Input(context.Background(), "born")   // channeling: "Born with the spark" → spark
	in.Input(context.Background(), "human")  // race
	in.Input(context.Background(), "wilder") // channeler class step
	st, _ := in.Input(context.Background(), "yes")

	if st != wizard.StatusCompleted {
		t.Fatalf("status = %v, want Completed", st)
	}
	if e.channelingGift != "spark" {
		t.Errorf("channelingGift = %q, want spark", e.channelingGift)
	}
	if e.classID != "wilder" {
		t.Errorf("classID = %q, want wilder", e.classID)
	}
}

// The recorded channeling gift IS stamped onto the player save (v28) by
// runCreation, alongside race/class/gender.
func TestRunCreation_WoTPersistsChanneling(t *testing.T) {
	rr := progression.NewRaceRegistry()
	if err := rr.Register(&progression.Race{ID: "human", DisplayName: "Human"}); err != nil {
		t.Fatalf("register human: %v", err)
	}
	cr := giftTaggedClasses(t)
	cfg := Config{CreationFlow: CreationFlowFor("wot", rr, cr, nil, nil)}
	loaded := newPlayerLoaded("Rand")
	// gender, channeling("born"→spark), race, channeler class, confirm.
	conn := &scriptedConn{inputs: []string{"male", "born", "human", "initiate", "yes"}}

	if err := runCreation(context.Background(), conn, cfg, loaded); err != nil {
		t.Fatalf("runCreation: %v", err)
	}
	if loaded.Player.ChannelingGift != "spark" {
		t.Errorf("save ChannelingGift = %q, want spark", loaded.Player.ChannelingGift)
	}
	if loaded.Player.Gender != "male" || loaded.Player.Race != "human" {
		t.Errorf("save gender/race = %q/%q, want male/human", loaded.Player.Gender, loaded.Player.Race)
	}
	if len(loaded.Player.Class) != 1 || loaded.Player.Class[0] != "initiate" {
		t.Errorf("save class = %v, want [initiate]", loaded.Player.Class)
	}
}

// The default (non-WoT) flow never sets a channeling gift, so the save's
// ChannelingGift stays empty (no key written).
func TestRunCreation_DefaultLeavesChannelingUnset(t *testing.T) {
	rr, cr := twoRaceOneClass(t)
	cfg := Config{CreationFlow: NewCreationFlow(rr, cr, nil, nil)}
	loaded := newPlayerLoaded("Bob")
	conn := &scriptedConn{inputs: []string{"male", "elf", "fighter", "yes"}}

	if err := runCreation(context.Background(), conn, cfg, loaded); err != nil {
		t.Fatalf("runCreation: %v", err)
	}
	if loaded.Player.ChannelingGift != "" {
		t.Errorf("default flow set ChannelingGift = %q, want empty", loaded.Player.ChannelingGift)
	}
}
