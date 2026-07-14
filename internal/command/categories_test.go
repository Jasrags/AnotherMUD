package command

import "testing"

// TestBuiltinsAreCategorized guards the help taxonomy: every discoverable
// builtin must resolve to a category in categoryOrder, so a newly added verb
// can't silently fall into the "commands" default and vanish from the grouped
// index. Admin verbs resolve to catAdmin via categoryFor's fallback.
func TestBuiltinsAreCategorized(t *testing.T) {
	known := make(map[string]bool, len(categoryOrder))
	for _, m := range categoryOrder {
		known[m.Key] = true
	}

	r := New()
	if err := RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	for _, ci := range r.Commands() {
		if !known[ci.Category] {
			t.Errorf("command %q has category %q, which is not in categoryOrder — add it to commandCategories (categories.go)", ci.Keyword, ci.Category)
		}
	}
}

// TestCategoryForAdminFallbackGated pins the invariant that a bare admin verb
// (keyword+handler only, no listing metadata) is NOT categorized — so it stays
// out of the help grid — while an admin verb that carries metadata falls back to
// the admin group. Guards against the fallback re-coupling to topic registration.
func TestCategoryForAdminFallbackGated(t *testing.T) {
	bare := Command{Keyword: "secretprobe", Admin: true}
	if got := categoryFor(bare); got != "" {
		t.Errorf("bare admin verb categorized as %q, want \"\" (stays undiscoverable)", got)
	}
	listed := Command{Keyword: "secretprobe", Admin: true, Brief: "A probe."}
	if got := categoryFor(listed); got != catAdmin {
		t.Errorf("admin verb with metadata categorized as %q, want %q", got, catAdmin)
	}
}

// TestCategoryOrderTitlesUnique catches an accidental duplicate key/title in the
// canonical list (a copy-paste risk as categories are added).
func TestCategoryOrderTitlesUnique(t *testing.T) {
	keys := make(map[string]bool)
	titles := make(map[string]bool)
	for _, m := range categoryOrder {
		if keys[m.Key] {
			t.Errorf("duplicate category key %q", m.Key)
		}
		if titles[m.Title] {
			t.Errorf("duplicate category title %q", m.Title)
		}
		keys[m.Key] = true
		titles[m.Title] = true
	}
}
