package light

import "testing"

func TestLevel_StringParseRoundTrip(t *testing.T) {
	for _, l := range []Level{Black, Gloom, Dim, Lit} {
		got, ok := ParseLevel(l.String())
		if !ok {
			t.Fatalf("ParseLevel(%q) ok=false", l.String())
		}
		if got != l {
			t.Fatalf("round trip: %v -> %q -> %v", l, l.String(), got)
		}
	}
}

func TestLevel_NamesAreFixedVocabulary(t *testing.T) {
	cases := map[Level]string{Black: "black", Gloom: "gloom", Dim: "dim", Lit: "lit"}
	for l, want := range cases {
		if got := l.String(); got != want {
			t.Fatalf("%d.String() = %q, want %q", l, got, want)
		}
	}
}

func TestParseLevel_RejectsUnknown(t *testing.T) {
	for _, s := range []string{"", "bright", "LIT", "Dim", "0", "dark"} {
		if lvl, ok := ParseLevel(s); ok {
			t.Fatalf("ParseLevel(%q) = (%v, true), want ok=false", s, lvl)
		}
	}
}

func TestParseLevel_FailsSafeToNoOverride(t *testing.T) {
	// A garbage value must report ok=false (treated as "no override")
	// rather than ok=true with a black pin.
	if lvl, ok := ParseLevel("pitch-black"); ok || lvl != Black {
		t.Fatalf("ParseLevel(garbage) = (%v, %v), want (Black, false)", lvl, ok)
	}
}

func TestClampBounds(t *testing.T) {
	if got := clamp(Level(-5)); got != Black {
		t.Fatalf("clamp(-5) = %v, want Black", got)
	}
	if got := clamp(Level(99)); got != Lit {
		t.Fatalf("clamp(99) = %v, want Lit", got)
	}
	if got := clamp(Dim); got != Dim {
		t.Fatalf("clamp(Dim) = %v, want Dim", got)
	}
}

func TestOrdering(t *testing.T) {
	if !(Black < Gloom && Gloom < Dim && Dim < Lit) {
		t.Fatal("levels are not strictly ordered black<gloom<dim<lit")
	}
}
