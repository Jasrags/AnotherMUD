package entities

import (
	"errors"
	"sort"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
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

// M7.1: MobInstance must satisfy combat.Combatant. The compile-time
// assignment in this test pins the contract — a refactor that breaks
// the interface fails here rather than at the (currently absent)
// CombatManager call site.
func TestMobInstanceImplementsCombatant(t *testing.T) {
	s := NewStore()
	inst, err := s.SpawnMob(guardTpl())
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	var c combat.Combatant = inst
	if c.Name() != "a village guard" {
		t.Errorf("Name() = %q, want %q", c.Name(), "a village guard")
	}
	want := string(combat.NewMobCombatantID(string(inst.ID())))
	if got := string(c.CombatantID()); got != want {
		t.Errorf("CombatantID() = %q, want %q", got, want)
	}
	cur, max := c.Vitals().Snapshot()
	if cur != 40 || max != 40 {
		t.Errorf("Vitals at spawn = (%d, %d), want (40, 40) per template hp_max", cur, max)
	}
	st := c.Stats()
	if st.STR != 12 {
		t.Errorf("Stats.STR = %d, want 12 from template", st.STR)
	}
	// Engine defaults fill in keys the template omitted.
	if st.AC != combat.DefaultAC {
		t.Errorf("Stats.AC = %d, want default %d", st.AC, combat.DefaultAC)
	}
}

// Mobs spawned from a template with no Stats map at all must still
// produce a working Combatant — engine defaults are non-zero so the
// round loop has finite inputs.
func TestSpawnMobBareTemplateGetsCombatDefaults(t *testing.T) {
	s := NewStore()
	tpl := &mob.Template{
		ID:   "tapestry-core:silent-watcher",
		Name: "a silent watcher",
		Type: "npc",
	}
	inst, err := s.SpawnMob(tpl)
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	cur, max := inst.Vitals().Snapshot()
	if cur != combat.DefaultMobMaxHP || max != combat.DefaultMobMaxHP {
		t.Errorf("Vitals = (%d, %d), want (%d, %d)", cur, max, combat.DefaultMobMaxHP, combat.DefaultMobMaxHP)
	}
	st := inst.Stats()
	if st.AC != combat.DefaultAC || st.STR != combat.DefaultSTR {
		t.Errorf("Stats = %+v, want engine defaults", st)
	}
}

func TestWimpyThresholdAcceptsCommonYAMLNumericTypes(t *testing.T) {
	// YAML decoding produces int OR int64 OR float64 depending on the
	// magnitude and document context. WimpyThreshold must accept all
	// three so a content-pack author who writes `wimpy_threshold: 30`
	// gets the expected behavior regardless of which numeric type the
	// decoder picked.
	cases := []struct {
		name string
		raw  any
		want int
	}{
		{"int", int(30), 30},
		{"int64", int64(40), 40},
		{"float64", float64(50), 50},
		{"int out of range high", int(150), 0},
		{"int out of range low", int(-1), 0},
		{"string ignored", "30", 0},
		{"nil missing key", nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tpl := guardTpl()
			if tc.raw != nil {
				tpl.Properties["wimpy_threshold"] = tc.raw
			}
			s := NewStore()
			inst, err := s.SpawnMob(tpl)
			if err != nil {
				t.Fatalf("SpawnMob: %v", err)
			}
			if got := inst.WimpyThreshold(); got != tc.want {
				t.Errorf("WimpyThreshold() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestMobInstanceApplyRacialFlagsMergesTags(t *testing.T) {
	s := NewStore()
	tpl := &mob.Template{
		ID:       "test:orc-warrior",
		Name:     "an orc warrior",
		Type:     "npc",
		Behavior: "stationary",
		Tags:     []string{"hostile"},
		Race:     "orc",
	}
	inst, err := s.SpawnMob(tpl)
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	if inst.RaceID() != "orc" {
		t.Errorf("RaceID = %q, want %q", inst.RaceID(), "orc")
	}

	inst.ApplyRacialFlags([]string{"darkvision", "common-tongue"}, -150)

	tags := inst.Tags()
	want := map[string]bool{"hostile": false, "mob": false, "darkvision": false, "common-tongue": false}
	for _, tag := range tags {
		if _, ok := want[tag]; ok {
			want[tag] = true
		}
	}
	for k, found := range want {
		if !found {
			t.Errorf("missing tag %q after ApplyRacialFlags; got %v", k, tags)
		}
	}

	props := inst.Properties()
	if got, ok := props[PropAlignment].(int); !ok || got != -150 {
		t.Errorf("alignment property = %v, want -150", props[PropAlignment])
	}
}

func TestMobInstanceApplyRacialFlagsDedupes(t *testing.T) {
	s := NewStore()
	tpl := &mob.Template{
		ID:       "test:dwarf-soldier",
		Name:     "a dwarf soldier",
		Type:     "npc",
		Behavior: "stationary",
		Tags:     []string{"common-tongue"},
		Race:     "dwarf",
	}
	inst, err := s.SpawnMob(tpl)
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}

	inst.ApplyRacialFlags([]string{"common-tongue", "darkvision"}, 0)
	tags := inst.Tags()
	count := 0
	for _, t := range tags {
		if t == "common-tongue" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("common-tongue appears %d times, want 1 (dedup)", count)
	}
}

func TestMobInstanceApplyRacialFlagsZeroAlignmentSkipsProperty(t *testing.T) {
	s := NewStore()
	tpl := &mob.Template{
		ID:       "test:human",
		Name:     "a person",
		Type:     "npc",
		Behavior: "stationary",
		Race:     "human",
	}
	inst, _ := s.SpawnMob(tpl)
	inst.ApplyRacialFlags(nil, 0)

	if _, ok := inst.Properties()[PropAlignment]; ok {
		t.Error("alignment property set to zero unexpectedly; should be absent")
	}
}

func TestMobInstanceAlignmentRoundTrip(t *testing.T) {
	s := NewStore()
	tpl := &mob.Template{ID: "test:m", Name: "m", Type: "npc"}
	inst, _ := s.SpawnMob(tpl)
	if inst.Alignment() != 0 {
		t.Errorf("default alignment = %d, want 0", inst.Alignment())
	}
	inst.SetAlignment(-600)
	if inst.Alignment() != -600 {
		t.Errorf("after SetAlignment: got %d, want -600", inst.Alignment())
	}
	// Property bag should reflect the write.
	if v, _ := inst.Properties()[PropAlignment].(int); v != -600 {
		t.Errorf("Properties[alignment] = %v, want -600", inst.Properties()[PropAlignment])
	}
}

func TestMobInstanceSetAlignmentTagExclusive(t *testing.T) {
	s := NewStore()
	tpl := &mob.Template{ID: "test:m", Name: "m", Type: "npc", Tags: []string{"humanoid"}}
	inst, _ := s.SpawnMob(tpl)

	inst.SetAlignmentTag("alignment_evil")
	if !inst.HasTag("alignment_evil") {
		t.Error("alignment_evil not set")
	}
	if !inst.HasTag("humanoid") {
		t.Error("template tags lost during alignment tag set")
	}

	// Switching the bucket removes the old tag.
	inst.SetAlignmentTag("alignment_good")
	if inst.HasTag("alignment_evil") {
		t.Error("alignment_evil still present after switching to good")
	}
	if !inst.HasTag("alignment_good") {
		t.Error("alignment_good not set")
	}

	// Empty tag clears all.
	inst.SetAlignmentTag("")
	for _, b := range []string{"alignment_evil", "alignment_neutral", "alignment_good"} {
		if inst.HasTag(b) {
			t.Errorf("tag %q still present after empty SetAlignmentTag", b)
		}
	}
	// Non-bucket tags preserved across the clear.
	if !inst.HasTag("humanoid") {
		t.Error("template tag removed during alignment tag clear")
	}
}
