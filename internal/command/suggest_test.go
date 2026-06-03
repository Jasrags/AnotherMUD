package command

import (
	"strings"
	"testing"
)

func TestRenderSuggest_VerbList(t *testing.T) {
	res := CompletionResult{Target: CompleteVerb, Candidates: []Candidate{
		{Completion: "look", Kind: CandVerb}, {Completion: "loot", Kind: CandVerb}, {Completion: "lock", Kind: CandVerb},
	}}
	if got := renderSuggest("lo", res); got != "Commands: look, loot, lock" {
		t.Errorf("verb list = %q", got)
	}
	res.Truncated = true
	if got := renderSuggest("", res); !strings.HasSuffix(got, ", …") {
		t.Errorf("truncated verb list = %q, want trailing ', …'", got)
	}
}

func TestRenderSuggest_SingleArg(t *testing.T) {
	res := CompletionResult{Target: CompleteArgument, Verb: "kill", Candidates: []Candidate{
		{Completion: "guard", Display: "a village guard", Kind: CandEntity},
	}}
	got := renderSuggest("kill gu", res)
	if !strings.Contains(got, "→ kill guard") || !strings.Contains(got, "a village guard") {
		t.Errorf("single arg = %q", got)
	}
}

func TestRenderSuggest_MultiArgWithLCPHint(t *testing.T) {
	res := CompletionResult{Target: CompleteArgument, Verb: "kill", Candidates: []Candidate{
		{Completion: "rose", Display: "a rose", Kind: CandEntity},
		{Completion: "rosemary", Display: "Rosemary", Kind: CandEntity},
	}}
	got := renderSuggest("kill ro", res)
	if !strings.Contains(got, "kill — 2 matches") {
		t.Errorf("multi header = %q", got)
	}
	if !strings.Contains(got, "(try kill rose…)") {
		t.Errorf("LCP hint missing (lcp 'rose' > typed 'ro'): %q", got)
	}
	for _, want := range []string{"rose", "rosemary"} {
		if !strings.Contains(got, want) {
			t.Errorf("multi list missing %q in %q", want, got)
		}
	}
}

func TestRenderSuggest_MultiNoHintWhenNoExtension(t *testing.T) {
	res := CompletionResult{Target: CompleteArgument, Verb: "drop", Candidates: []Candidate{
		{Completion: "ruby", Display: "a ruby", Kind: CandItem},
		{Completion: "rose", Display: "a rose", Kind: CandItem},
	}}
	// lcp("ruby","rose") = "r"; typed "r" → no extension → no hint.
	if got := renderSuggest("drop r", res); strings.Contains(got, "try") {
		t.Errorf("unexpected LCP hint when lcp doesn't extend: %q", got)
	}
}

func TestRenderSuggest_None(t *testing.T) {
	res := CompletionResult{Target: CompleteNone}
	if got := renderSuggest("frobnicate x", res); got != `No suggestions for "frobnicate x".` {
		t.Errorf("none = %q", got)
	}
}

func TestLongestCommonPrefix(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"look", "loot", "lock"}, "lo"},
		{[]string{"sword"}, "sword"},
		{[]string{"rose", "rosemary"}, "rose"},
		{[]string{"ruby", "rose"}, "r"},
		{[]string{"alpha", "beta"}, ""},
		{nil, ""},
	}
	for _, c := range cases {
		if got := longestCommonPrefix(c.in); got != c.want {
			t.Errorf("longestCommonPrefix(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
