package entities

import (
	"errors"
	"sort"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/mob"
)

// guardTpl returns a mob template carrying a representative spread of
// fields: tags (one redundant with type), keywords, properties, stats,
// equipment (unused at spawn for now). Tests slice off whichever
// fields matter to a given case.
func guardTpl() *mob.Template {
	return &mob.Template{
		ID:          "tapestry-core:village-guard",
		Name:        "a village guard",
		Type:        "npc",
		Disposition: 0,
		Behavior:    "stationary",
		Tags:        []string{"npc", "humanoid", "guard"},
		Keywords:    []string{"guard", "villager"},
		Properties: map[string]any{
			"patrol_speed": 2,
		},
		Stats: map[string]int{
			"str":    12,
			"hp_max": 40,
		},
		Equipment: []string{"tapestry-core:short-sword"},
	}
}

func TestSpawnMobAssignsFreshIDAndCopiesFields(t *testing.T) {
	s := NewStore()
	tpl := guardTpl()
	inst, err := s.SpawnMob(tpl)
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	if inst.ID() == "" {
		t.Error("ID = empty, want fresh id")
	}
	if inst.Name() != "a village guard" {
		t.Errorf("Name = %q", inst.Name())
	}
	if inst.Type() != "npc" {
		t.Errorf("Type = %q", inst.Type())
	}
	if inst.TemplateID() != tpl.ID {
		t.Errorf("TemplateID = %q, want %q", inst.TemplateID(), tpl.ID)
	}
	// Tracked in the store.
	if got, ok := s.GetByID(inst.ID()); !ok || got != inst {
		t.Errorf("Store.GetByID = (%v, %v), want (inst, true)", got, ok)
	}
}

func TestSpawnMobDropsImplicitTypeTag(t *testing.T) {
	// Spec §2.3 step 2: the tag matching the entity's own type MUST
	// NOT be re-applied. Template carries "npc" as both type and tag;
	// the instance should NOT carry the "npc" tag.
	s := NewStore()
	inst, err := s.SpawnMob(guardTpl())
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	for _, tag := range inst.Tags() {
		if tag == "npc" {
			t.Errorf("instance carries implicit type tag %q", tag)
		}
	}
	// Other tags survived, plus the synthetic TagMob applied at
	// instantiation so the AI dispatcher can enumerate live mobs
	// via Store.GetByTag("mob").
	gotTags := append([]string(nil), inst.Tags()...)
	sort.Strings(gotTags)
	wantTags := []string{"guard", "humanoid", TagMob}
	sort.Strings(wantTags)
	if len(gotTags) != len(wantTags) {
		t.Fatalf("Tags = %v, want %v", gotTags, wantTags)
	}
	for i, want := range wantTags {
		if gotTags[i] != want {
			t.Errorf("Tags[%d] = %q, want %q", i, gotTags[i], want)
		}
	}
}

func TestSpawnMobAppliesSyntheticMobTag(t *testing.T) {
	// Explicit coverage of the synthetic-tag invariant. SwapTagIndex
	// to publish the write-side index into the read side, then
	// GetByTag(TagMob) must surface this mob.
	s := NewStore()
	inst, err := s.SpawnMob(guardTpl())
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	s.SwapTagIndex()
	got := s.GetByTag(TagMob)
	if len(got) != 1 || got[0].ID() != inst.ID() {
		t.Errorf("GetByTag(%q) = %v, want [%q]", TagMob, got, inst.ID())
	}
}

func TestSpawnMobCopiesStatsAndBehaviorIntoProperties(t *testing.T) {
	// Spec §2.3 step 3 + step 5: stats land in the per-instance bag,
	// and behavior is set as a property so AI dispatch can read it
	// without a typed accessor.
	s := NewStore()
	inst, err := s.SpawnMob(guardTpl())
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	props := inst.Properties()
	if got, _ := props["hp_max"].(int); got != 40 {
		t.Errorf("hp_max property = %v, want 40", props["hp_max"])
	}
	if got, _ := props["str"].(int); got != 12 {
		t.Errorf("str property = %v, want 12", props["str"])
	}
	if got, _ := props[PropBehavior].(string); got != "stationary" {
		t.Errorf("PropBehavior = %v, want stationary", props[PropBehavior])
	}
	if got, _ := props[PropTemplateID].(string); got != "tapestry-core:village-guard" {
		t.Errorf("PropTemplateID = %v", props[PropTemplateID])
	}
	// Free-form template properties also land.
	if got, _ := props["patrol_speed"].(int); got != 2 {
		t.Errorf("patrol_speed property = %v, want 2", props["patrol_speed"])
	}
}

func TestSpawnMobNilTemplateReturnsError(t *testing.T) {
	s := NewStore()
	_, err := s.SpawnMob(nil)
	if !errors.Is(err, ErrUnknownMobTemplate) {
		t.Errorf("SpawnMob(nil) err = %v, want ErrUnknownMobTemplate", err)
	}
}

func TestSpawnMobUniqueIDsAcrossCalls(t *testing.T) {
	s := NewStore()
	tpl := guardTpl()
	a, _ := s.SpawnMob(tpl)
	b, _ := s.SpawnMob(tpl)
	if a.ID() == b.ID() {
		t.Errorf("two SpawnMob calls returned same ID %q", a.ID())
	}
}

func TestSpawnMobTagsIndexedByStore(t *testing.T) {
	// Tag indexing is the §4.3 surface — the spawn pipeline must
	// register the mob's tags so GetByTag finds it. The write-side
	// index is consulted immediately (SwapTagIndex is a tick boundary;
	// the read side won't see this until then), so route through the
	// write side by triggering a swap.
	s := NewStore()
	inst, err := s.SpawnMob(guardTpl())
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	s.SwapTagIndex()
	got := s.GetByTag("guard")
	if len(got) != 1 || got[0].ID() != inst.ID() {
		t.Errorf("GetByTag(guard) = %v, want [%q]", got, inst.ID())
	}
}

func TestMobInstanceKeywordsReturnsCopy(t *testing.T) {
	// Mirror of TestMobInstanceTagsReturnsCopy. Pins the contract that
	// Keywords() does not alias the backing slice — symmetric with
	// Tags(). A caller mutating the returned slice must not corrupt
	// the entity's keyword list.
	s := NewStore()
	inst, _ := s.SpawnMob(guardTpl())
	first := inst.Keywords()
	first[0] = "MUTATED"
	second := inst.Keywords()
	if second[0] == "MUTATED" {
		t.Error("Keywords() aliased backing storage; mutation leaked across calls")
	}
}

func TestMobInstanceTagsReturnsCopy(t *testing.T) {
	s := NewStore()
	inst, _ := s.SpawnMob(guardTpl())
	first := inst.Tags()
	first[0] = "MUTATED"
	second := inst.Tags()
	if second[0] == "MUTATED" {
		t.Error("Tags() aliased backing storage; mutation leaked across calls")
	}
}
