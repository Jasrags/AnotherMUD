package light

import "testing"

// fakeSource is a minimal Source backed by a property map.
type fakeSource map[string]any

func (f fakeSource) Property(key string) (any, bool) {
	v, ok := f[key]
	return v, ok
}

func TestSourceLevel(t *testing.T) {
	cases := []struct {
		name string
		src  Source
		want Level
	}{
		{"nil source", nil, Black},
		{"no light prop", fakeSource{}, Black},
		{"valid level", fakeSource{PropItemLight: "dim"}, Dim},
		{"lit level", fakeSource{PropItemLight: "lit"}, Lit},
		{"non-string light", fakeSource{PropItemLight: 3}, Black},
		{"garbage level name", fakeSource{PropItemLight: "blazing"}, Black},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SourceLevel(tc.src); got != tc.want {
				t.Fatalf("SourceLevel = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsLit(t *testing.T) {
	if IsLit(nil) {
		t.Fatal("IsLit(nil) = true")
	}
	if IsLit(fakeSource{}) {
		t.Fatal("IsLit(no prop) = true")
	}
	if IsLit(fakeSource{PropItemLit: "yes"}) {
		t.Fatal("IsLit(non-bool) = true")
	}
	if IsLit(fakeSource{PropItemLit: false}) {
		t.Fatal("IsLit(false) = true")
	}
	if !IsLit(fakeSource{PropItemLit: true}) {
		t.Fatal("IsLit(true) = false")
	}
}

func TestIsSource(t *testing.T) {
	if IsSource(fakeSource{}) {
		t.Fatal("plain item is a source")
	}
	if !IsSource(fakeSource{PropItemLight: "gloom"}) {
		t.Fatal("item with light level is not a source")
	}
	// An unlit source is still a source.
	if !IsSource(fakeSource{PropItemLight: "dim", PropItemLit: false}) {
		t.Fatal("unlit source not recognized as a source")
	}
}

func TestContribution(t *testing.T) {
	// Unlit source contributes nothing even with a high level.
	if got := Contribution(fakeSource{PropItemLight: "lit", PropItemLit: false}); got != Black {
		t.Fatalf("unlit source contributes %v, want Black", got)
	}
	// Lit source contributes its level.
	if got := Contribution(fakeSource{PropItemLight: "dim", PropItemLit: true}); got != Dim {
		t.Fatalf("lit source contributes %v, want Dim", got)
	}
	// A lit non-source (no light prop) still contributes nothing.
	if got := Contribution(fakeSource{PropItemLit: true}); got != Black {
		t.Fatalf("lit non-source contributes %v, want Black", got)
	}
}

func TestBestContribution(t *testing.T) {
	if got := BestContribution(); got != Black {
		t.Fatalf("no sources = %v, want Black", got)
	}
	torch := fakeSource{PropItemLight: "gloom", PropItemLit: true}
	lantern := fakeSource{PropItemLight: "dim", PropItemLit: true}
	unlit := fakeSource{PropItemLight: "lit", PropItemLit: false}
	if got := BestContribution(torch, lantern, unlit); got != Dim {
		t.Fatalf("best of {gloom-lit, dim-lit, lit-unlit} = %v, want Dim", got)
	}
}
