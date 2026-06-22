package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
)

func TestPromote_NoArg(t *testing.T) {
	g := &fakeGroup{leader: "L", members: []string{"L", "A"}}
	c, a := lootModeCtx(g) // no args
	if err := command.PromoteHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if got := a.lastLine(); !strings.Contains(got, "Promote whom") {
		t.Errorf("no-arg promote = %q, want a usage prompt", got)
	}
}

func TestPromote_ByName(t *testing.T) {
	g := &fakeGroup{leader: "L", members: []string{"L", "A"}}
	c, a := lootModeCtx(g, "Alice")
	if err := command.PromoteHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if g.promoted != "A" {
		t.Fatalf("promoted = %q, want A (resolved from the name Alice)", g.promoted)
	}
	if got := a.lastLine(); !strings.Contains(got, "hand leadership") || !strings.Contains(got, "Alice") {
		t.Errorf("leader announce = %q, want a handoff naming Alice", got)
	}
}

func TestPromote_UnknownMember(t *testing.T) {
	g := &fakeGroup{leader: "L", members: []string{"L", "A"}}
	c, a := lootModeCtx(g, "Stranger")
	if err := command.PromoteHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if got := a.lastLine(); !strings.Contains(got, "isn't in your party") {
		t.Errorf("unknown target = %q, want isn't-in-your-party", got)
	}
	if g.promoted != "" {
		t.Error("an unknown target should not have promoted anyone")
	}
}

func TestPromote_NotLeader(t *testing.T) {
	g := &fakeGroup{leader: "L", members: []string{"L", "A"}, promoteErr: command.ErrGroupNotLeader}
	c, a := lootModeCtx(g, "Alice")
	if err := command.PromoteHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if got := a.lastLine(); !strings.Contains(got, "leader") {
		t.Errorf("non-leader promote = %q, want a leader-only refusal", got)
	}
}
