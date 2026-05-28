package render

import "testing"

func newTestTheme() *ThemeRegistry {
	r := NewThemeRegistry()
	r.Register("highlight", ThemeEntry{FG: "bright-yellow"})
	r.Register("danger", ThemeEntry{FG: "red", BG: "black"})
	r.Register("item.rare", ThemeEntry{FG: "cyan", HTML: "#00FFFF"})
	r.Register("note", ThemeEntry{HTML: "#888888"}) // declared but color-less
	r.Compile()
	return r
}

func TestThemeResolve(t *testing.T) {
	r := newTestTheme()

	pair, ok := r.Resolve("highlight")
	if !ok || pair.Open != "\x1b[93m" || pair.Close != Reset {
		t.Errorf("highlight resolve = %+v, ok=%v", pair, ok)
	}

	pair, ok = r.Resolve("danger")
	if !ok || pair.Open != "\x1b[31m\x1b[40m" {
		t.Errorf("danger resolve = %+v, ok=%v", pair, ok)
	}

	// case-insensitive
	if _, ok := r.Resolve("HIGHLIGHT"); !ok {
		t.Error("expected case-insensitive resolve")
	}

	// declared-but-color-less: IsKnown true, Resolve false
	if !r.IsKnown("note") {
		t.Error("note should be IsKnown")
	}
	if _, ok := r.Resolve("note"); ok {
		t.Error("note should not Resolve (no fg/bg)")
	}

	// unknown
	if r.IsKnown("bogus") {
		t.Error("bogus should not be IsKnown")
	}
}

func TestThemeCompileIdempotent(t *testing.T) {
	r := newTestTheme()
	p1, _ := r.Resolve("highlight")
	r.Compile()
	r.Compile()
	p2, ok := r.Resolve("highlight")
	if !ok || p1 != p2 {
		t.Errorf("compile not idempotent: %+v vs %+v", p1, p2)
	}
}

func TestThemeRegisterOverride(t *testing.T) {
	r := NewThemeRegistry()
	r.Register("x", ThemeEntry{FG: "red"})
	r.Register("x", ThemeEntry{FG: "green"})
	r.Compile()
	pair, _ := r.Resolve("x")
	if pair.Open != "\x1b[32m" {
		t.Errorf("override = %q, want green", pair.Open)
	}
}

func TestThemeHtmlMap(t *testing.T) {
	r := newTestTheme()
	m := r.GetHtmlMap()
	if m["item.rare"] != "#00FFFF" || m["note"] != "#888888" {
		t.Errorf("html map = %v", m)
	}
	if _, ok := m["highlight"]; ok {
		t.Error("highlight has no html, should be absent")
	}
}
