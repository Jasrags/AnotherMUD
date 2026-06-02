package decoration

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/render"
)

// A registered essence resolves case-insensitively.
func TestEssence_RegisterAndGetCaseInsensitive(t *testing.T) {
	r := NewEssenceRegistry()
	if !r.Register(Essence{Key: "Fire", Glyph: "✦", Color: render.ThemeEntry{FG: "red"}}) {
		t.Fatal("Register returned false for a valid essence")
	}
	for _, k := range []string{"fire", "FIRE", " Fire "} {
		if _, ok := r.Get(k); !ok {
			t.Errorf("Get(%q) miss; want hit (keys are case-insensitive)", k)
		}
	}
}

// An empty key is rejected and stores nothing.
func TestEssence_EmptyKeyRejected(t *testing.T) {
	r := NewEssenceRegistry()
	if r.Register(Essence{Key: "  ", Glyph: "✦"}) {
		t.Error("Register accepted an empty key")
	}
	if r.Len() != 0 {
		t.Errorf("Len = %d, want 0", r.Len())
	}
}

// Re-registering a key replaces the prior definition (later wins).
func TestEssence_RegisterLaterWins(t *testing.T) {
	r := NewEssenceRegistry()
	r.Register(Essence{Key: "fire", Glyph: "✦", Color: render.ThemeEntry{FG: "red"}})
	r.Register(Essence{Key: "fire", Glyph: "★", Color: render.ThemeEntry{FG: "orange"}})

	got, ok := r.Get("fire")
	if !ok {
		t.Fatal("Get(fire) miss after re-register")
	}
	if got.Glyph != "★" || got.Color.FG != "orange" {
		t.Errorf("after re-register: %+v, want Glyph=★ FG=orange", got)
	}
	if r.Len() != 1 {
		t.Errorf("Len = %d, want 1 (re-register replaces, not appends)", r.Len())
	}
}

// VisibleText wraps a glyph in parens; an empty glyph renders nothing.
func TestEssence_VisibleText(t *testing.T) {
	if got := (Essence{Key: "fire", Glyph: "✦"}).VisibleText(); got != "(✦)" {
		t.Errorf("VisibleText() = %q, want %q", got, "(✦)")
	}
	if got := (Essence{Key: "void", Glyph: ""}).VisibleText(); got != "" {
		t.Errorf("empty-glyph VisibleText() = %q, want \"\"", got)
	}
}

// All returns essences sorted by key.
func TestEssence_AllSortedByKey(t *testing.T) {
	r := NewEssenceRegistry()
	r.Register(Essence{Key: "water", Glyph: "≈"})
	r.Register(Essence{Key: "air", Glyph: "~"})
	r.Register(Essence{Key: "fire", Glyph: "✦"})

	all := r.All()
	want := []string{"air", "fire", "water"}
	if len(all) != len(want) {
		t.Fatalf("All len = %d, want %d", len(all), len(want))
	}
	for i, w := range want {
		if all[i].Key != w {
			t.Errorf("All[%d].Key = %q, want %q", i, all[i].Key, w)
		}
	}
}

// An unknown key resolves to (zero, false).
func TestEssence_GetUnknown(t *testing.T) {
	r := NewEssenceRegistry()
	r.Register(Essence{Key: "fire", Glyph: "✦"})
	if _, ok := r.Get("earth"); ok {
		t.Error("Get(earth) hit; want miss for an unregistered key")
	}
	if _, ok := r.Get(""); ok {
		t.Error("Get(\"\") hit; want miss")
	}
}
