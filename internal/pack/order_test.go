package pack

import (
	"errors"
	"testing"
)

// mkPack is a tiny helper to build a Discovered with name and deps only.
func mkPack(name string, deps ...string) Discovered {
	depMap := map[string]string{}
	for _, d := range deps {
		depMap[d] = "*"
	}
	return Discovered{
		ManifestPath: "test://" + name,
		Manifest: &Manifest{
			Name:         name,
			Dependencies: depMap,
		},
	}
}

func TestOrderLinearChain(t *testing.T) {
	// charlie depends on bravo depends on alpha
	in := []Discovered{
		mkPack("charlie", "bravo"),
		mkPack("alpha"),
		mkPack("bravo", "alpha"),
	}
	got, err := Order(in)
	if err != nil {
		t.Fatalf("Order: %v", err)
	}
	want := []string{"alpha", "bravo", "charlie"}
	if !equalStrings(discoveredNS(got), want) {
		t.Errorf("got %v, want %v", discoveredNS(got), want)
	}
}

func TestOrderAlphabeticalTieBreak(t *testing.T) {
	// All independent — should come out alphabetically.
	in := []Discovered{
		mkPack("delta"),
		mkPack("alpha"),
		mkPack("charlie"),
		mkPack("bravo"),
	}
	got, err := Order(in)
	if err != nil {
		t.Fatalf("Order: %v", err)
	}
	want := []string{"alpha", "bravo", "charlie", "delta"}
	if !equalStrings(discoveredNS(got), want) {
		t.Errorf("got %v, want %v", discoveredNS(got), want)
	}
}

func TestOrderDiamond(t *testing.T) {
	// d depends on b and c, both depend on a.
	in := []Discovered{
		mkPack("d", "b", "c"),
		mkPack("c", "a"),
		mkPack("b", "a"),
		mkPack("a"),
	}
	got, err := Order(in)
	if err != nil {
		t.Fatalf("Order: %v", err)
	}
	want := []string{"a", "b", "c", "d"}
	if !equalStrings(discoveredNS(got), want) {
		t.Errorf("got %v, want %v", discoveredNS(got), want)
	}
}

func TestOrderScopedDepResolution(t *testing.T) {
	// foo's manifest names "@scope/dep" which derives to "scope-dep".
	in := []Discovered{
		mkPack("foo", "@scope/dep"),
		mkPack("@scope/dep"),
	}
	got, err := Order(in)
	if err != nil {
		t.Fatalf("Order: %v", err)
	}
	want := []string{"scope-dep", "foo"}
	if !equalStrings(discoveredNS(got), want) {
		t.Errorf("got %v, want %v", discoveredNS(got), want)
	}
}

func TestOrderCycle(t *testing.T) {
	in := []Discovered{
		mkPack("a", "b"),
		mkPack("b", "a"),
	}
	_, err := Order(in)
	if !errors.Is(err, ErrCycle) {
		t.Fatalf("err = %v, want ErrCycle", err)
	}
}

func TestOrderSelfCycle(t *testing.T) {
	in := []Discovered{mkPack("a", "a")}
	_, err := Order(in)
	if !errors.Is(err, ErrCycle) {
		t.Fatalf("err = %v, want ErrCycle", err)
	}
}

func TestOrderUnknownDep(t *testing.T) {
	in := []Discovered{mkPack("a", "ghost")}
	_, err := Order(in)
	if !errors.Is(err, ErrUnknownDep) {
		t.Fatalf("err = %v, want ErrUnknownDep", err)
	}
}

func TestOrderEmpty(t *testing.T) {
	got, err := Order(nil)
	if err != nil {
		t.Fatalf("Order(nil): %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestOrderDuplicateDepKey(t *testing.T) {
	// A pack listing the same target through two notations should not
	// double-increment the dependent's indegree.
	pkg := Discovered{
		ManifestPath: "test://consumer",
		Manifest: &Manifest{
			Name: "consumer",
			Dependencies: map[string]string{
				"@scope/dep": "*",
				"scope-dep":  "*",
			},
		},
	}
	in := []Discovered{pkg, mkPack("@scope/dep")}
	got, err := Order(in)
	if err != nil {
		t.Fatalf("Order: %v", err)
	}
	want := []string{"scope-dep", "consumer"}
	if !equalStrings(discoveredNS(got), want) {
		t.Errorf("got %v, want %v", discoveredNS(got), want)
	}
}

func TestOrderDuplicateNamespace(t *testing.T) {
	// Two packs deriving the same namespace — should error.
	in := []Discovered{
		mkPack("@scope/foo"),
		mkPack("scope-foo"),
	}
	_, err := Order(in)
	if err == nil {
		t.Fatal("expected duplicate-namespace error")
	}
}
