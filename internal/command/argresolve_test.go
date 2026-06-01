package command

import (
	"errors"
	"strings"
	"testing"
)

// resolveDefault is the registry used by most tests — engine
// baseline only, no pack extensions.
func resolveDefault() *ArgResolverRegistry { return NewArgResolverRegistry() }

func TestEngineArgTypes_LookupHits(t *testing.T) {
	r := resolveDefault()
	for _, name := range []ArgType{ArgKeyword, ArgText, ArgNumber} {
		if _, ok := r.Lookup(name); !ok {
			t.Errorf("Lookup(%q) miss, want hit", name)
		}
	}
}

func TestEngineArgTypes_AreImmutable(t *testing.T) {
	// Spec §5.3: packs MUST NOT override engine types.
	r := resolveDefault()
	custom := func(in ResolverInput) (ResolverOutput, error) {
		return ResolverOutput{Value: "OVERRIDE", Consumed: 1}, nil
	}
	for _, name := range []ArgType{ArgKeyword, ArgText, ArgNumber, ArgInventory, ArgDoor} {
		err := r.Register(name, custom)
		if err == nil {
			t.Errorf("Register(%q) should reject engine override", name)
		}
		if !errors.Is(err, ErrEngineTypeImmutable) {
			t.Errorf("Register(%q) err = %v, want ErrEngineTypeImmutable", name, err)
		}
	}
}

func TestIsEngineArgType(t *testing.T) {
	cases := map[ArgType]bool{
		ArgKeyword:        true,
		ArgText:           true,
		ArgNumber:         true,
		ArgInventory:      true,
		ArgDoor:           true,
		ArgType("custom"): false,
		"":                false,
	}
	for name, want := range cases {
		if got := IsEngineArgType(name); got != want {
			t.Errorf("IsEngineArgType(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestRegister_RejectsNilResolver(t *testing.T) {
	r := resolveDefault()
	if err := r.Register("custom", nil); err == nil {
		t.Error("Register(nil) should fail")
	}
}

func TestRegister_PackTypeWinsLast(t *testing.T) {
	r := resolveDefault()
	first := func(in ResolverInput) (ResolverOutput, error) {
		return ResolverOutput{Value: "first", Consumed: 1}, nil
	}
	second := func(in ResolverInput) (ResolverOutput, error) {
		return ResolverOutput{Value: "second", Consumed: 1}, nil
	}
	if err := r.Register("color", first); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := r.Register("color", second); err != nil {
		t.Fatalf("second register: %v", err)
	}
	res, _, _, err := r.ResolveArgs(
		[]ArgDefinition{{Name: "c", Type: "color"}},
		[]string{"x"},
	)
	if err != nil {
		t.Fatalf("ResolveArgs: %v", err)
	}
	if res["c"] != "second" {
		t.Errorf("got %v, want second", res["c"])
	}
}

// --- §5.4 driver semantics ---

func TestResolve_KeywordPassesTokenVerbatim(t *testing.T) {
	r := resolveDefault()
	res, _, rest, err := r.ResolveArgs(
		[]ArgDefinition{{Name: "verb", Type: ArgKeyword}},
		[]string{"jump", "extra"},
	)
	if err != nil {
		t.Fatalf("ResolveArgs: %v", err)
	}
	if res["verb"] != "jump" {
		t.Errorf("verb = %v, want jump", res["verb"])
	}
	if len(rest) != 1 || rest[0] != "extra" {
		t.Errorf("rest = %v, want [extra]", rest)
	}
}

func TestResolve_TextSlurpsRemainder(t *testing.T) {
	r := resolveDefault()
	res, _, rest, err := r.ResolveArgs(
		[]ArgDefinition{{Name: "msg", Type: ArgText}},
		[]string{"hello", "world", "lots"},
	)
	if err != nil {
		t.Fatalf("ResolveArgs: %v", err)
	}
	if res["msg"] != "hello world lots" {
		t.Errorf("msg = %v", res["msg"])
	}
	if len(rest) != 0 {
		t.Errorf("rest = %v, want empty", rest)
	}
}

func TestResolve_NumberFailsOnNonInteger(t *testing.T) {
	r := resolveDefault()
	_, _, _, err := r.ResolveArgs(
		[]ArgDefinition{{Name: "n", Type: ArgNumber}},
		[]string{"three"},
	)
	if err == nil {
		t.Fatal("expected ErrNotANumber")
	}
	if !errors.Is(err, ErrNotANumber) {
		t.Errorf("err = %v, want ErrNotANumber", err)
	}
	// Surface message goes straight to the player verbatim.
	if !strings.Contains(err.Error(), "not a number") {
		t.Errorf("err.Error() = %q, want player-readable", err.Error())
	}
}

func TestResolve_NumberAcceptsNegatives(t *testing.T) {
	r := resolveDefault()
	res, _, _, err := r.ResolveArgs(
		[]ArgDefinition{{Name: "n", Type: ArgNumber}},
		[]string{"-7"},
	)
	if err != nil {
		t.Fatalf("ResolveArgs: %v", err)
	}
	if res["n"] != -7 {
		t.Errorf("n = %v, want -7", res["n"])
	}
}

func TestResolve_PrepositionConsumed(t *testing.T) {
	r := resolveDefault()
	// `put <item> in <container>` — `in` is a preposition before
	// the container arg.
	res, _, _, err := r.ResolveArgs(
		[]ArgDefinition{
			{Name: "item", Type: ArgKeyword},
			{Name: "container", Type: ArgKeyword, Prepositions: []string{"in"}},
		},
		[]string{"gem", "in", "chest"},
	)
	if err != nil {
		t.Fatalf("ResolveArgs: %v", err)
	}
	if res["item"] != "gem" || res["container"] != "chest" {
		t.Errorf("got %v", res)
	}
}

func TestResolve_PrepositionCaseInsensitive(t *testing.T) {
	r := resolveDefault()
	res, _, _, err := r.ResolveArgs(
		[]ArgDefinition{
			{Name: "item", Type: ArgKeyword},
			{Name: "container", Type: ArgKeyword, Prepositions: []string{"in"}},
		},
		[]string{"gem", "IN", "chest"},
	)
	if err != nil {
		t.Fatalf("ResolveArgs: %v", err)
	}
	if res["container"] != "chest" {
		t.Errorf("container = %v, want chest", res["container"])
	}
}

func TestResolve_PrepositionOnlySkippedImmediatelyBefore(t *testing.T) {
	// `in` here precedes the FIRST arg, not the container arg,
	// so it should NOT be consumed — the resolver would then
	// take "in" as the first keyword and "gem" as the container,
	// which is the §5.4 step-1 behavior.
	r := resolveDefault()
	res, _, _, err := r.ResolveArgs(
		[]ArgDefinition{
			{Name: "item", Type: ArgKeyword},
			{Name: "container", Type: ArgKeyword, Prepositions: []string{"in"}},
		},
		[]string{"in", "gem", "chest"},
	)
	if err != nil {
		t.Fatalf("ResolveArgs: %v", err)
	}
	if res["item"] != "in" || res["container"] != "gem" {
		t.Errorf("preposition consumed at wrong position: %v", res)
	}
}

// --- §5.4 short-circuit behavior ---

func TestResolve_MissingRequired_ShortCircuits(t *testing.T) {
	r := resolveDefault()
	_, _, _, err := r.ResolveArgs(
		[]ArgDefinition{
			{Name: "first", Type: ArgKeyword},
			{Name: "second", Type: ArgKeyword},
		},
		[]string{"only"},
	)
	if err == nil {
		t.Fatal("expected MissingRequired error")
	}
	var ae *ArgResolveError
	if !errors.As(err, &ae) {
		t.Fatalf("err type = %T, want *ArgResolveError", err)
	}
	if ae.ArgName != "second" {
		t.Errorf("ArgName = %q, want second", ae.ArgName)
	}
	if ae.Error() != "What second?" {
		t.Errorf("player text = %q, want What second?", ae.Error())
	}
}

func TestResolve_MissingOptional_ProducesNil(t *testing.T) {
	r := resolveDefault()
	res, _, _, err := r.ResolveArgs(
		[]ArgDefinition{
			{Name: "first", Type: ArgKeyword},
			{Name: "second", Type: ArgKeyword, Optional: true},
		},
		[]string{"only"},
	)
	if err != nil {
		t.Fatalf("ResolveArgs: %v", err)
	}
	if v, ok := res["second"]; !ok || v != nil {
		t.Errorf("optional miss: got (%v, ok=%v), want (nil, ok=true)", v, ok)
	}
}

func TestResolve_MissingRequiredText(t *testing.T) {
	r := resolveDefault()
	_, _, _, err := r.ResolveArgs(
		[]ArgDefinition{{Name: "msg", Type: ArgText}},
		[]string{},
	)
	if err == nil {
		t.Fatal("expected MissingRequired for text")
	}
	if err.Error() != "What msg?" {
		t.Errorf("err = %q", err.Error())
	}
}

func TestResolve_UnknownTypeFallsBackToKeyword_WithWarning(t *testing.T) {
	r := resolveDefault()
	res, warnings, _, err := r.ResolveArgs(
		[]ArgDefinition{{Name: "thing", Type: ArgType("nonesuch")}},
		[]string{"raw"},
	)
	if err != nil {
		t.Fatalf("ResolveArgs: %v", err)
	}
	if res["thing"] != "raw" {
		t.Errorf("fallback should pass token verbatim: %v", res)
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "nonesuch") {
		t.Errorf("warnings = %v, want one mentioning the unknown type", warnings)
	}
}

func TestResolve_EmptyDefinitions_NoOp(t *testing.T) {
	r := resolveDefault()
	res, warnings, rest, err := r.ResolveArgs(nil, []string{"a", "b"})
	if err != nil {
		t.Fatalf("ResolveArgs: %v", err)
	}
	if len(res) != 0 || len(warnings) != 0 {
		t.Errorf("empty defs should produce empty result + no warnings")
	}
	if len(rest) != 2 {
		t.Errorf("rest should hold both tokens, got %v", rest)
	}
}

func TestResolve_NumberThenKeyword(t *testing.T) {
	r := resolveDefault()
	res, _, _, err := r.ResolveArgs(
		[]ArgDefinition{
			{Name: "count", Type: ArgNumber},
			{Name: "item", Type: ArgKeyword},
		},
		[]string{"3", "potions"},
	)
	if err != nil {
		t.Fatalf("ResolveArgs: %v", err)
	}
	if res["count"] != 3 {
		t.Errorf("count = %v, want 3", res["count"])
	}
	if res["item"] != "potions" {
		t.Errorf("item = %v, want potions", res["item"])
	}
}

func TestResolve_TextAfterKeyword_SlurpsRest(t *testing.T) {
	r := resolveDefault()
	res, _, rest, err := r.ResolveArgs(
		[]ArgDefinition{
			{Name: "channel", Type: ArgKeyword},
			{Name: "msg", Type: ArgText},
		},
		[]string{"ooc", "hello", "there"},
	)
	if err != nil {
		t.Fatalf("ResolveArgs: %v", err)
	}
	if res["channel"] != "ooc" || res["msg"] != "hello there" {
		t.Errorf("got %v", res)
	}
	if len(rest) != 0 {
		t.Errorf("text should slurp all remaining tokens")
	}
}

// --- ArgResolveError surface ---

func TestArgResolveError_UnwrapTraversesToCause(t *testing.T) {
	e := &ArgResolveError{ArgName: "n", Cause: ErrNotANumber}
	if !errors.Is(e, ErrNotANumber) {
		t.Error("errors.Is should match the cause")
	}
}

func TestArgResolveError_MissingRequiredFormatsPlayerString(t *testing.T) {
	e := &ArgResolveError{ArgName: "target", Cause: ErrMissingRequired}
	if e.Error() != "What target?" {
		t.Errorf("Error() = %q, want What target?", e.Error())
	}
}

func TestArgResolveError_OtherCausesSurfaceVerbatim(t *testing.T) {
	e := &ArgResolveError{ArgName: "x", Cause: ErrNotANumber}
	if e.Error() != ErrNotANumber.Error() {
		t.Errorf("Error() = %q, want %q", e.Error(), ErrNotANumber.Error())
	}
}
