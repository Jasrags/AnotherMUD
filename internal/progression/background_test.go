package progression

import "testing"

func TestBackgroundRegistry_RegisterAndGet(t *testing.T) {
	br := NewBackgroundRegistry()
	if err := br.Register(&Background{
		ID: "Soldier", DisplayName: "Soldier",
		Skills: []BackgroundSkill{{AbilityID: "Open-Lock", Proficiency: 10}},
		Items:  []string{"Short-Sword"}, Gold: 25,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	b, ok := br.Get("soldier") // case-insensitive
	if !ok {
		t.Fatal("soldier not found")
	}
	if b.ID != "soldier" {
		t.Errorf("ID = %q, want lowercased 'soldier'", b.ID)
	}
	if len(b.Skills) != 1 || b.Skills[0].AbilityID != "open-lock" || b.Skills[0].Proficiency != 10 {
		t.Errorf("Skills = %+v, want [{open-lock 10}] (ability id lowercased)", b.Skills)
	}
	if len(b.Items) != 1 || b.Items[0] != "short-sword" {
		t.Errorf("Items = %v, want [short-sword] (lowercased)", b.Items)
	}
	if b.Gold != 25 {
		t.Errorf("Gold = %d, want 25", b.Gold)
	}
}

func TestBackgroundRegistry_RejectsEmptyAndNil(t *testing.T) {
	br := NewBackgroundRegistry()
	if err := br.Register(nil); err == nil {
		t.Error("nil background should error")
	}
	if err := br.Register(&Background{ID: "  "}); err == nil {
		t.Error("empty id should error")
	}
}

func TestBackgroundRegistry_DeepCopyIsolatesCaller(t *testing.T) {
	br := NewBackgroundRegistry()
	skills := []BackgroundSkill{{AbilityID: "smithing", Proficiency: 5}}
	items := []string{"hammer"}
	cats := []string{"humanoid"}
	if err := br.Register(&Background{ID: "smith", Skills: skills, Items: items, AllowedCategories: cats}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// Mutate the caller's slices after Register; the registry copy must not change.
	skills[0].Proficiency = 999
	items[0] = "anvil"
	cats[0] = "dragon"
	b, _ := br.Get("smith")
	if b.Skills[0].Proficiency != 5 {
		t.Errorf("Skills leaked: %+v", b.Skills)
	}
	if b.Items[0] != "hammer" {
		t.Errorf("Items leaked: %v", b.Items)
	}
	if b.AllowedCategories[0] != "humanoid" {
		t.Errorf("AllowedCategories leaked: %v", b.AllowedCategories)
	}
}

func TestBackgroundRegistry_PriorityOverride(t *testing.T) {
	br := NewBackgroundRegistry()
	_ = br.Register(&Background{ID: "thief", Gold: 10, Priority: 0})
	// Equal priority no-ops (existing retained).
	_ = br.Register(&Background{ID: "thief", Gold: 99, Priority: 0})
	if b, _ := br.Get("thief"); b.Gold != 10 {
		t.Errorf("equal-priority should no-op: Gold = %d, want 10", b.Gold)
	}
	// Higher priority replaces.
	_ = br.Register(&Background{ID: "thief", Gold: 50, Priority: 1})
	if b, _ := br.Get("thief"); b.Gold != 50 {
		t.Errorf("higher priority should replace: Gold = %d, want 50", b.Gold)
	}
}

func TestBackgroundRegistry_AllSorted(t *testing.T) {
	br := NewBackgroundRegistry()
	_ = br.Register(&Background{ID: "wanderer"})
	_ = br.Register(&Background{ID: "smith"})
	_ = br.Register(&Background{ID: "soldier"})
	all := br.All()
	want := []string{"smith", "soldier", "wanderer"}
	if len(all) != 3 {
		t.Fatalf("All len = %d, want 3", len(all))
	}
	for i, b := range all {
		if b.ID != want[i] {
			t.Errorf("All[%d] = %q, want %q (id-sorted)", i, b.ID, want[i])
		}
	}
}

func TestBackgroundRegistry_GetEligible(t *testing.T) {
	br := NewBackgroundRegistry()
	_ = br.Register(&Background{ID: "open", AllowedCategories: nil})                          // unrestricted
	_ = br.Register(&Background{ID: "humanonly", AllowedCategories: []string{"humanoid"}})    // category-gated
	_ = br.Register(&Background{ID: "menonly", AllowedGenders: []string{"male"}})             // gender-gated
	_ = br.Register(&Background{ID: "dwarfonly", AllowedCategories: []string{"construct"}})   // excludes humanoid

	got := br.GetEligible("humanoid", "female")
	ids := map[string]bool{}
	for _, b := range got {
		ids[b.ID] = true
	}
	if !ids["open"] || !ids["humanonly"] {
		t.Errorf("unrestricted + matching-category should be eligible: %v", ids)
	}
	if ids["menonly"] {
		t.Error("a female character should not get the male-only background")
	}
	if ids["dwarfonly"] {
		t.Error("a humanoid should not get the construct-only background")
	}
}
