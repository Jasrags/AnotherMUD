package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
)

func TestBuildCompleteResponse_Argument(t *testing.T) {
	res := command.CompletionResult{
		Target: command.CompleteArgument,
		Verb:   "get",
		Candidates: []command.Candidate{
			{Completion: "sword", Display: "a short sword", Kind: command.CandItem},
			{Completion: "shield", Display: "a shield", Kind: command.CandItem},
		},
	}
	out := buildCompleteResponse("get s", res)

	if out.Line != "get s" || out.Target != "argument" || out.Verb != "get" {
		t.Errorf("line/target/verb = %q/%q/%q", out.Line, out.Target, out.Verb)
	}
	if out.Common != "s" { // lcp("sword","shield")
		t.Errorf("common = %q, want %q", out.Common, "s")
	}
	if len(out.Candidates) != 2 {
		t.Fatalf("candidates = %d, want 2", len(out.Candidates))
	}
	c0 := out.Candidates[0]
	if c0.Value != "sword" || c0.Display != "a short sword" || c0.Kind != "item" {
		t.Errorf("candidate[0] = %+v", c0)
	}
}

func TestBuildCompleteResponse_VerbAndNone(t *testing.T) {
	verb := buildCompleteResponse("lo", command.CompletionResult{
		Target: command.CompleteVerb,
		Candidates: []command.Candidate{
			{Completion: "look", Kind: command.CandVerb},
			{Completion: "loot", Kind: command.CandVerb},
		},
	})
	if verb.Target != "verb" || verb.Verb != "" || verb.Common != "loo" { // lcp("look","loot")
		t.Errorf("verb target/verb/common = %q/%q/%q", verb.Target, verb.Verb, verb.Common)
	}

	none := buildCompleteResponse("xyzzy", command.CompletionResult{Target: command.CompleteNone})
	if none.Target != "none" || len(none.Candidates) != 0 {
		t.Errorf("none target=%q candidates=%d", none.Target, len(none.Candidates))
	}
}
