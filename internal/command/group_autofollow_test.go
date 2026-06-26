package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// joinCtx wires a Context for the `join` verb: the actor (Alice/"A") joins the
// leader (Lead/"L"), resolved in-room via the locator, with both a party service
// and a follow service. Returns the context, the joining actor, and the follow stub.
func joinCtx(g *fakeGroup, fol *stubFollow) (*command.Context, *testActor) {
	room := &world.Room{ID: "z:a"}
	joiner := newNamedTestActor("Alice", "A", room)
	leader := newNamedTestActor("Lead", "L", room)
	byID := map[string]command.Actor{"A": joiner, "L": leader}
	return &command.Context{
		Actor:     joiner,
		Args:      []string{"Lead"},
		Group:     g,
		Follow:    fol,
		Locator:   locatorFunc(func(world.RoomID, string) command.Actor { return leader }),
		ActorByID: func(id string) (command.Actor, bool) { a, ok := byID[id]; return a, ok },
	}, joiner
}

// grouping.md §9: joining a party auto-follows the leader.
func TestJoin_AutoFollowsLeader(t *testing.T) {
	g := &fakeGroup{leader: "L", members: []string{"L", "A"}}
	fol := &stubFollow{}
	c, a := joinCtx(g, fol)
	if err := command.JoinHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if fol.gotFollower != "A" || fol.gotLed != "L" {
		t.Fatalf("auto-follow called with (%q→%q), want (A→L)", fol.gotFollower, fol.gotLed)
	}
	if got := a.lastLine(); !strings.Contains(got, "begin following") {
		t.Errorf("join message = %q, want it to mention following", got)
	}
}

// A nil follow service must not break the join (best-effort auto-follow).
func TestJoin_NoFollowServiceStillJoins(t *testing.T) {
	g := &fakeGroup{leader: "L", members: []string{"L", "A"}}
	c, a := joinCtx(g, nil)
	c.Follow = nil
	if err := command.JoinHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if got := a.lastLine(); !strings.Contains(got, "join") || strings.Contains(got, "following") {
		t.Errorf("join-without-follow message = %q, want a plain join line", got)
	}
}

// leaveCtx wires a Context for `leave`: the actor is in a party led by "L" and
// (per the follow stub) currently following `following`.
func leaveCtx(following string) (*command.Context, *stubFollow) {
	room := &world.Room{ID: "z:a"}
	a := newNamedTestActor("Alice", "A", room)
	fol := &stubFollow{leader: following, leaderOK: following != ""}
	return &command.Context{
		Actor:     a,
		Group:     &fakeGroup{leader: "L", members: []string{"L", "A"}},
		Follow:    fol,
		ActorByID: func(string) (command.Actor, bool) { return nil, false },
	}, fol
}

// grouping.md §9: leaving auto-unfollows the leader you were trailing.
func TestLeave_AutoUnfollowsPartyLeader(t *testing.T) {
	c, fol := leaveCtx("L") // following the party leader
	if err := command.LeaveHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if !fol.unfollowCalled {
		t.Error("leaving while trailing the leader should auto-unfollow")
	}
}

// A manual follow of someone OTHER than the leader survives leaving the party.
func TestLeave_KeepsManualFollow(t *testing.T) {
	c, fol := leaveCtx("X") // following a non-leader
	if err := command.LeaveHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if fol.unfollowCalled {
		t.Error("leaving should NOT end a manual follow of a non-leader")
	}
}
