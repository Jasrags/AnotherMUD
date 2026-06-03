package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
)

func TestToConnCompletion(t *testing.T) {
	res := command.CompletionResult{
		Target: command.CompleteArgument,
		Candidates: []command.Candidate{
			{Completion: "sword", Display: "a short sword", Kind: command.CandItem},
			{Completion: "swap", Display: "swap", Kind: command.CandItem},
		},
	}
	out := toConnCompletion(res)

	if out.Common != "sw" { // lcp(sword, swap)
		t.Errorf("common = %q, want %q", out.Common, "sw")
	}
	if len(out.Candidates) != 2 {
		t.Fatalf("candidates = %d, want 2", len(out.Candidates))
	}
	if out.Candidates[0].Value != "sword" || out.Candidates[0].Display != "a short sword" {
		t.Errorf("candidate[0] = %+v", out.Candidates[0])
	}
}
