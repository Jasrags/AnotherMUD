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

// CreationFlowFor for the default/unknown worlds must produce the exact
// same flow as NewCreationFlow — the regression lock for "default flow
// preserved byte-for-byte".
func TestCreationFlowFor_DefaultMatchesNewCreationFlow(t *testing.T) {
	rr, cr := twoRaceOneClass(t)
	want := stepIDs(NewCreationFlow(rr, cr, nil))

	for _, world := range []string{"", "starter-world", "STARTER-WORLD", "nonsense", "  "} {
		got := stepIDs(CreationFlowFor(world, rr, cr, nil))
		if !equalStrings(got, want) {
			t.Errorf("CreationFlowFor(%q) step IDs = %v, want %v (default)", world, got, want)
		}
	}
}

// The nil-content path propagates through the selector for every branch.
func TestCreationFlowFor_NilWhenNoContent(t *testing.T) {
	for _, world := range []string{"", "starter-world", "wot"} {
		if f := CreationFlowFor(world, progression.NewRaceRegistry(), progression.NewClassRegistry(), progression.NewBackgroundRegistry()); f != nil {
			t.Errorf("CreationFlowFor(%q) with empty registries = non-nil, want nil", world)
		}
	}
}

// The WoT flow inserts the channeling step immediately after gender and
// before race/class, and the default flow has no such step.
func TestCreationFlowFor_WoTInsertsChannelingAfterGender(t *testing.T) {
	rr, cr := twoRaceOneClass(t)

	def := stepIDs(NewCreationFlow(rr, cr, nil))
	for _, id := range def {
		if id == "channeling" {
			t.Fatalf("default flow unexpectedly has a channeling step: %v", def)
		}
	}

	wot := stepIDs(CreationFlowFor("wot", rr, cr, nil))
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
	// channeling must precede race and class.
	if ri := indexOf(wot, "race"); ri >= 0 && ci > ri {
		t.Errorf("channeling (%d) should precede race (%d): %v", ci, ri, wot)
	}
	if cli := indexOf(wot, "class"); cli >= 0 && ci > cli {
		t.Errorf("channeling (%d) should precede class (%d): %v", ci, cli, wot)
	}
	// Stripping channeling out of the WoT flow yields the default flow —
	// proves every other step is reused unchanged.
	var stripped []string
	for _, id := range wot {
		if id != "channeling" {
			stripped = append(stripped, id)
		}
	}
	if !equalStrings(stripped, def) {
		t.Errorf("WoT flow minus channeling = %v, want default %v", stripped, def)
	}
}

func indexOf(ids []string, want string) int {
	for i, id := range ids {
		if id == want {
			return i
		}
	}
	return -1
}

// Driving the WoT channeling step records the choice on the entity (option
// (a): recorded, not persisted). Exercises the OnSelect wiring end-to-end.
func TestWoTCreationFlow_RecordsChannelingGift(t *testing.T) {
	rr, cr := twoRaceOneClass(t)
	flow := CreationFlowFor("wot", rr, cr, nil)
	if flow == nil {
		t.Fatal("WoT flow should not be nil with races+classes")
	}
	e := &creationEntity{}
	in := wizard.NewInstance(flow, e, &wizFakeIO{}, nil)
	in.Start(context.Background())
	in.Input(context.Background(), "male") // gender
	// channeling: option 1 = "Born with the spark" (Value "spark"). Selecting
	// by index — "spark" is the Value, not a label prefix, so a prefix match
	// would not resolve it.
	in.Input(context.Background(), "1")
	in.Input(context.Background(), "elf")     // race
	in.Input(context.Background(), "fighter") // class
	st, _ := in.Input(context.Background(), "yes")

	if st != wizard.StatusCompleted {
		t.Fatalf("status = %v, want Completed", st)
	}
	if e.channelingGift != "spark" {
		t.Errorf("channelingGift = %q, want spark", e.channelingGift)
	}
	// Sanity: the rest of the character still assembled.
	if e.gender != "male" || e.raceID != "elf" || e.classID != "fighter" {
		t.Errorf("entity = %+v, want gender=male race=elf class=fighter", e)
	}
}

// The recorded channeling gift is NOT stamped onto the player save (option
// (a) is non-persisted). runCreation commits race/class/gender but leaves no
// channeling trace on loaded.Player.
func TestRunCreation_WoTDoesNotPersistChanneling(t *testing.T) {
	rr, cr := twoRaceOneClass(t)
	cfg := Config{CreationFlow: CreationFlowFor("wot", rr, cr, nil)}
	loaded := newPlayerLoaded("Rand")
	// gender, channeling(index 1), race, class, confirm.
	conn := &scriptedConn{inputs: []string{"male", "1", "elf", "fighter", "yes"}}

	if err := runCreation(context.Background(), conn, cfg, loaded); err != nil {
		t.Fatalf("runCreation: %v", err)
	}
	if loaded.Player.Race != "elf" || loaded.Player.Gender != "male" {
		t.Errorf("save race/gender = %q/%q, want elf/male", loaded.Player.Race, loaded.Player.Gender)
	}
	// player.Save has no channeling field — the contract is "no save bump".
	// This test documents that the commit path is unchanged; if a future
	// change persists the gift, it must add a field + a save migration.
}
