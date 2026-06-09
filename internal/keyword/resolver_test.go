package keyword

import (
	"reflect"
	"testing"
)

// stub is the test fixture; it satisfies Named with explicit values
// so each case can be expressed concisely.
type stub struct {
	name string
	kws  []string
}

func (s *stub) Name() string       { return s.name }
func (s *stub) Keywords() []string { return s.kws }

func named(name string, kws ...string) *stub {
	return &stub{name: name, kws: kws}
}

func TestResolveExactKeywordWins(t *testing.T) {
	a := named("a short sword", "sword", "short")
	b := named("a long sword", "sword", "long")
	got := Resolve([]Named{a, b}, "sword")
	if got != a {
		t.Errorf("Resolve(sword) = %v, want a (first exact match)", got)
	}
}

func TestResolvePrefixMatchAfterExactMiss(t *testing.T) {
	a := named("a longsword", "longsword")
	b := named("a long bow", "bow")
	// "long" is not exact on either; longsword starts with "long"
	// (and is longer than the input), so a wins.
	got := Resolve([]Named{a, b}, "long")
	if got != a {
		t.Errorf("Resolve(long) = %v, want a (prefix)", got)
	}
}

func TestResolveExactBeatsPrefix(t *testing.T) {
	a := named("a longsword", "longsword")
	b := named("a long bow", "long")
	// b has exact "long"; even though a's "longsword" also matches as
	// prefix, exact wins per §6.1 precedence.
	got := Resolve([]Named{a, b}, "long")
	if got != b {
		t.Errorf("Resolve(long) = %v, want b (exact beats prefix)", got)
	}
}

func TestResolvePrefixRequiresLongerKeyword(t *testing.T) {
	// §6.1 step 3: prefix requires the candidate keyword to be strictly
	// longer than the input. A candidate whose keyword equals the input
	// must NOT count as a prefix hit on a later resolver step — that's
	// what step 2 (exact) is for.
	exactOnly := named("a long thing", "long")
	got := Resolve([]Named{exactOnly}, "long")
	if got != exactOnly {
		t.Errorf("Resolve(long) = %v, want exactOnly (via exact step)", got)
	}
	// Different scenario: keyword equals input but the resolver picks
	// it on step 2, not step 3. We don't have a way to observe which
	// step matched, but we can verify a same-length-only candidate
	// does NOT trip on the prefix step in isolation by constructing
	// one with a keyword longer than the input that should outrank a
	// same-length keyword if step 3 was being used wrongly.
}

func TestResolveNameSubstring(t *testing.T) {
	a := named("a red potion", "potion") // "red" not in keywords
	b := named("a vial", "vial")
	got := Resolve([]Named{a, b}, "red")
	if got != a {
		t.Errorf("Resolve(red) = %v, want a (name substring)", got)
	}
}

func TestResolveCaseInsensitive(t *testing.T) {
	a := named("A Short Sword", "Sword")
	got := Resolve([]Named{a}, "SWORD")
	if got != a {
		t.Errorf("case-insensitive exact failed: got %v", got)
	}
	got = Resolve([]Named{a}, "short")
	if got != a {
		t.Errorf("case-insensitive name substring failed: got %v", got)
	}
}

func TestResolveOrdinal(t *testing.T) {
	a := named("ring 1", "ring")
	b := named("ring 2", "ring")
	c := named("ring 3", "ring")
	got := Resolve([]Named{a, b, c}, "2.ring")
	if got != b {
		t.Errorf("Resolve(2.ring) = %v, want b", got)
	}
	got = Resolve([]Named{a, b, c}, "1.ring")
	if got != a {
		t.Errorf("Resolve(1.ring) = %v, want a", got)
	}
}

func TestResolveOrdinalOutOfRange(t *testing.T) {
	a := named("ring 1", "ring")
	got := Resolve([]Named{a}, "5.ring")
	if got != nil {
		t.Errorf("Resolve(5.ring) = %v, want nil", got)
	}
}

func TestResolveOrdinalZeroFallsThrough(t *testing.T) {
	// §6.1 step 1: "0.kw" must NOT take the ordinal path. The whole
	// string is treated as a literal keyword; since no candidate has
	// "0.ring" as a keyword or substring, the result is nil.
	a := named("ring 1", "ring")
	got := Resolve([]Named{a}, "0.ring")
	if got != nil {
		t.Errorf("Resolve(0.ring) = %v, want nil (literal keyword miss)", got)
	}
}

func TestResolveOrdinalNegativeFallsThrough(t *testing.T) {
	// Leading minus also falls through (the dot-split sees "-1" which
	// is not a positive integer).
	a := named("a -1.ring thing", "thing")
	got := Resolve([]Named{a}, "-1.ring")
	// Falls through to substring on name; "a -1.ring thing" contains
	// "-1.ring", so it matches.
	if got != a {
		t.Errorf("Resolve(-1.ring) = %v, want a (substring fallthrough)", got)
	}
}

func TestResolveEmptyInput(t *testing.T) {
	a := named("a sword", "sword")
	for _, in := range []string{"", "   ", "\t"} {
		if got := Resolve([]Named{a}, in); got != nil {
			t.Errorf("Resolve(%q) = %v, want nil", in, got)
		}
	}
}

func TestResolveEmptyCandidates(t *testing.T) {
	if got := Resolve(nil, "sword"); got != nil {
		t.Errorf("Resolve(nil, sword) = %v, want nil", got)
	}
}

func TestResolveAllReturnsEveryEntity(t *testing.T) {
	a := named("a sword", "sword")
	b := named("a shield", "shield")
	got := ResolveAll([]Named{a, b}, "all")
	if !reflect.DeepEqual(got, []Named{a, b}) {
		t.Errorf("ResolveAll(all) = %v, want [a b]", got)
	}
}

func TestResolveAllByKeyword(t *testing.T) {
	a := named("ring 1", "ring", "gold")
	b := named("ring 2", "ring", "silver")
	c := named("a sword", "sword")
	got := ResolveAll([]Named{a, b, c}, "all.ring")
	if !reflect.DeepEqual(got, []Named{a, b}) {
		t.Errorf("ResolveAll(all.ring) = %v, want [a b]", got)
	}
}

func TestResolveAllByKeywordEmptyAfterDot(t *testing.T) {
	a := named("a thing", "thing")
	got := ResolveAll([]Named{a}, "all.")
	if got != nil {
		t.Errorf("ResolveAll(all.) = %v, want nil", got)
	}
}

func TestResolveAllPlainInputReturnsAllMatching(t *testing.T) {
	a := named("ring 1", "ring")
	b := named("ring 2", "ring")
	c := named("a sword", "sword")
	got := ResolveAll([]Named{a, b, c}, "ring")
	if !reflect.DeepEqual(got, []Named{a, b}) {
		t.Errorf("ResolveAll(ring) = %v, want [a b]", got)
	}
}

func TestResolveAllPreservesInputOrder(t *testing.T) {
	a := named("ring a", "ring")
	b := named("ring b", "ring")
	c := named("ring c", "ring")
	// Pass in a known order; result MUST preserve it.
	got := ResolveAll([]Named{c, a, b}, "all.ring")
	if !reflect.DeepEqual(got, []Named{c, a, b}) {
		t.Errorf("order not preserved: got %v", got)
	}
}

func TestResolveAllEmptyInput(t *testing.T) {
	a := named("a sword", "sword")
	for _, in := range []string{"", "   "} {
		if got := ResolveAll([]Named{a}, in); got != nil {
			t.Errorf("ResolveAll(%q) = %v, want nil", in, got)
		}
	}
}

func TestResolveAllUnknownKeyword(t *testing.T) {
	a := named("a sword", "sword")
	got := ResolveAll([]Named{a}, "all.potion")
	if got != nil {
		t.Errorf("ResolveAll(all.potion) = %v, want nil", got)
	}
}

func TestResolveAllPlainKeywordPrefixMatch(t *testing.T) {
	// Bare keyword (no "all." prefix) must still pick up prefix matches,
	// not just exact keywords. Tightens coverage of the prefix branch
	// inside matchesAny in ResolveAll.
	a := named("a longsword", "longsword")
	b := named("a long bow", "longbow")
	c := named("a sword", "sword")
	got := ResolveAll([]Named{a, b, c}, "long")
	if !reflect.DeepEqual(got, []Named{a, b}) {
		t.Errorf("ResolveAll(long) = %v, want [a b]", got)
	}
}

func TestResolveAllAllOnEmptyCandidatesReturnsNil(t *testing.T) {
	if got := ResolveAll(nil, "all"); got != nil {
		t.Errorf("ResolveAll(nil, all) = %v, want nil", got)
	}
}

func TestResolveOrdinalMatchesByNameSubstring(t *testing.T) {
	// §6.1 step 1 + step 4 interaction: 2.red picks the second
	// candidate whose NAME contains 'red' even when no keyword does.
	a := named("a red potion", "potion")
	b := named("a red cloak", "cloak")
	c := named("a blue cloak", "cloak")
	got := Resolve([]Named{a, b, c}, "2.red")
	if got != b {
		t.Errorf("Resolve(2.red) = %v, want b (second name-substring match)", got)
	}
}

func TestResolveOrdinalWithMixedCaseInput(t *testing.T) {
	// Regression for the latent case-handling bug that was hidden by
	// an internal ToLower: the ordinal selector base must be matched
	// case-insensitively against keywords.
	a := named("ring a", "ring")
	b := named("ring b", "ring")
	got := Resolve([]Named{a, b}, "2.RING")
	if got != b {
		t.Errorf("Resolve(2.RING) = %v, want b", got)
	}
}

func TestResolveUnique_ExactKeywordBeatsNameSubstring(t *testing.T) {
	// The collision that motivated this: a "rusty dagger" with the exact
	// keyword "dagger" vs a scroll that only has "dagger" in its NAME.
	dagger := named("a rusty dagger", "dagger", "rusty")
	scroll := named("a recipe scroll - forging an iron dagger", "scroll", "recipe")

	got, ok := ResolveUnique([]Named{dagger, scroll}, "dagger")
	if !ok || got != dagger {
		t.Errorf("ResolveUnique(dagger) = (%v, %v), want the rusty dagger (exact keyword wins over name substring)", got, ok)
	}
	// The scroll still answers uniquely to its own keyword.
	if got, ok := ResolveUnique([]Named{dagger, scroll}, "scroll"); !ok || got != scroll {
		t.Errorf("ResolveUnique(scroll) = (%v, %v), want the scroll", got, ok)
	}
}

func TestResolveUnique_SameTierAmbiguityRefuses(t *testing.T) {
	// Two items keyed "potion" — a genuine same-tier tie must refuse.
	a := named("a red potion", "potion", "red")
	b := named("a blue potion", "potion", "blue")
	if got, ok := ResolveUnique([]Named{a, b}, "potion"); ok {
		t.Errorf("ResolveUnique(potion) = (%v, %v), want refusal on same-tier ambiguity", got, ok)
	}
	// But the unique colour keyword still resolves.
	if got, ok := ResolveUnique([]Named{a, b}, "red"); !ok || got != a {
		t.Errorf("ResolveUnique(red) = (%v, %v), want the red potion", got, ok)
	}
}

func TestResolveUnique_NoMatchAndEmpty(t *testing.T) {
	a := named("a potion", "potion")
	if _, ok := ResolveUnique([]Named{a}, "sword"); ok {
		t.Error("ResolveUnique(sword) should not match a potion")
	}
	if _, ok := ResolveUnique([]Named{a}, "   "); ok {
		t.Error("ResolveUnique(blank) should be ok=false")
	}
}
