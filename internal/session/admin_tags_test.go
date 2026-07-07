package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/player"
)

// AddTag records a free-form admin tag in the save bag, folds it into Tags(),
// and marks the save dirty so autosave persists it (admin-verbs §4 — a player
// tag survives relog).
func TestConnActor_AddTagFoldsAndPersists(t *testing.T) {
	a := &connActor{save: &player.Save{}}

	if !a.AddTag("cursed") {
		t.Fatal("AddTag(cursed) = false, want true")
	}
	if !a.dirty {
		t.Error("save not marked dirty after AddTag")
	}
	if len(a.save.AdminTags) != 1 || a.save.AdminTags[0] != "cursed" {
		t.Errorf("AdminTags = %v, want [cursed]", a.save.AdminTags)
	}
	if !containsTag(a.Tags(), "cursed") {
		t.Errorf("Tags() = %v, want it to include cursed", a.Tags())
	}
}

// A second add of the same tag is idempotent: no change, and it does not
// re-dirty the save (avoids a spurious autosave write).
func TestConnActor_AddTagIdempotent(t *testing.T) {
	a := &connActor{save: &player.Save{AdminTags: []string{"cursed"}}}

	if a.AddTag("cursed") {
		t.Error("AddTag(cursed) = true, want false (already present)")
	}
	if a.dirty {
		t.Error("save re-dirtied on a no-op AddTag")
	}
}

// AddTag never duplicates a manager-derived tag (here a racial flag): the
// admin bag stays empty and Tags() lists the tag once.
func TestConnActor_AddTagSkipsDerivedTag(t *testing.T) {
	a := &connActor{save: &player.Save{}, racialTags: []string{"human"}}

	if a.AddTag("human") {
		t.Error("AddTag(human) = true, want false (racial flag already derived)")
	}
	if len(a.save.AdminTags) != 0 {
		t.Errorf("AdminTags = %v, want empty (derived tag must not enter the admin bag)", a.save.AdminTags)
	}
	if n := countTag(a.Tags(), "human"); n != 1 {
		t.Errorf("Tags() lists human %d times, want 1", n)
	}
}

// RemoveTag drops an admin tag and re-dirties; removing an absent tag is a
// no-op that does not dirty. Manager-derived tags are not in the bag, so they
// are untouched by RemoveTag.
func TestConnActor_RemoveTag(t *testing.T) {
	a := &connActor{save: &player.Save{AdminTags: []string{"cursed"}}, alignmentTag: "alignment_evil"}

	if !a.RemoveTag("cursed") {
		t.Fatal("RemoveTag(cursed) = false, want true")
	}
	if !a.dirty {
		t.Error("save not marked dirty after a real RemoveTag")
	}
	if containsTag(a.Tags(), "cursed") {
		t.Error("Tags() still lists cursed after RemoveTag")
	}

	a.dirty = false
	if a.RemoveTag("nonesuch") {
		t.Error("RemoveTag(nonesuch) = true, want false")
	}
	if a.dirty {
		t.Error("save dirtied on a no-op RemoveTag")
	}
	// The derived alignment tag is unaffected by the admin-bag removal path.
	if !containsTag(a.Tags(), "alignment_evil") {
		t.Error("derived alignment tag lost through RemoveTag")
	}
}

// Tags() lists the admin bag alongside every manager-derived tag.
func TestConnActor_TagsComposesAllSources(t *testing.T) {
	a := &connActor{
		save:              &player.Save{AdminTags: []string{"watch"}},
		racialTags:        []string{"human"},
		alignmentTag:      "alignment_good",
		reputationTierTag: "renown:known-locally",
	}
	got := a.Tags()
	for _, want := range []string{"human", "alignment_good", "renown:known-locally", "watch"} {
		if !containsTag(got, want) {
			t.Errorf("Tags() = %v, missing %q", got, want)
		}
	}
}

func containsTag(tags []string, want string) bool {
	return countTag(tags, want) > 0
}

func countTag(tags []string, want string) int {
	n := 0
	for _, t := range tags {
		if t == want {
			n++
		}
	}
	return n
}
