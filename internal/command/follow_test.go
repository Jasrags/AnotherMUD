package command_test

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// stubFollow is a scriptable command.FollowService for the verb tests.
type stubFollow struct {
	followErr           error
	leader              string
	leaderOK            bool
	lost                []string
	gotFollower, gotLed string
	unfollowCalled      bool
}

func (s *stubFollow) Follow(follower, leader string) error {
	s.gotFollower, s.gotLed = follower, leader
	return s.followErr
}
func (s *stubFollow) Unfollow(string) (string, bool) {
	s.unfollowCalled = true
	return s.leader, s.leaderOK
}
func (s *stubFollow) Lose(string) []string            { return s.lost }
func (s *stubFollow) Following(string) (string, bool) { return s.leader, s.leaderOK }

func TestFollowHandler_BeginsFollowing(t *testing.T) {
	room := &world.Room{ID: "z:a", Name: "Road"}
	follower := newNamedTestActor("Alice", "p-1", room)
	target := newNamedTestActor("Bob", "p-2", room)
	stub := &stubFollow{}
	c := &command.Context{
		Actor:    follower,
		Follow:   stub,
		Locator:  locatorFunc(func(world.RoomID, string) command.Actor { return target }),
		Resolved: map[string]any{"target": command.EntityRef{ID: "p-2", Name: "Bob"}},
	}
	if err := command.FollowHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if stub.gotFollower != "p-1" || stub.gotLed != "p-2" {
		t.Errorf("Follow(%q,%q), want (p-1,p-2)", stub.gotFollower, stub.gotLed)
	}
	if follower.lastLine() != "You begin following Bob." {
		t.Errorf("follower msg = %q", follower.lastLine())
	}
	if target.lastLine() != "Alice begins following you." {
		t.Errorf("target msg = %q", target.lastLine())
	}
}

func TestFollowHandler_SelfAndCycleMessages(t *testing.T) {
	room := &world.Room{ID: "z:a"}
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"self", command.ErrFollowSelf, "You can't follow yourself."},
		{"cycle", command.ErrFollowCycle, "You can't follow Bob — they're already following you."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			follower := newNamedTestActor("Alice", "p-1", room)
			target := newNamedTestActor("Bob", "p-2", room)
			c := &command.Context{
				Actor:    follower,
				Follow:   &stubFollow{followErr: tc.err},
				Locator:  locatorFunc(func(world.RoomID, string) command.Actor { return target }),
				Resolved: map[string]any{"target": command.EntityRef{ID: "p-2", Name: "Bob"}},
			}
			if err := command.FollowHandler(context.Background(), c); err != nil {
				t.Fatal(err)
			}
			if follower.lastLine() != tc.want {
				t.Errorf("msg = %q, want %q", follower.lastLine(), tc.want)
			}
		})
	}
}

func TestFollowHandler_NoArgReports(t *testing.T) {
	room := &world.Room{ID: "z:a"}
	a := newNamedTestActor("Alice", "p-1", room)
	// Following someone.
	c := &command.Context{
		Actor:     a,
		Follow:    &stubFollow{leader: "p-2", leaderOK: true},
		ActorByID: func(string) (command.Actor, bool) { return newNamedTestActor("Bob", "p-2", room), true },
	}
	if err := command.FollowHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if a.lastLine() != "You are following Bob." {
		t.Errorf("report = %q", a.lastLine())
	}
	// Following no one.
	b := newNamedTestActor("Cyd", "p-3", room)
	c2 := &command.Context{Actor: b, Follow: &stubFollow{}}
	if err := command.FollowHandler(context.Background(), c2); err != nil {
		t.Fatal(err)
	}
	if b.lastLine() != "You aren't following anyone." {
		t.Errorf("report = %q", b.lastLine())
	}
}

func TestUnfollowHandler(t *testing.T) {
	room := &world.Room{ID: "z:a"}
	a := newNamedTestActor("Alice", "p-1", room)
	leader := newNamedTestActor("Bob", "p-2", room)
	c := &command.Context{
		Actor:     a,
		Follow:    &stubFollow{leader: "p-2", leaderOK: true},
		ActorByID: func(string) (command.Actor, bool) { return leader, true },
	}
	if err := command.UnfollowHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if a.lastLine() != "You stop following Bob." {
		t.Errorf("follower msg = %q", a.lastLine())
	}
	if leader.lastLine() != "Alice stops following you." {
		t.Errorf("leader msg = %q", leader.lastLine())
	}
}

func TestLoseHandler(t *testing.T) {
	room := &world.Room{ID: "z:a"}
	leader := newNamedTestActor("Alice", "p-1", room)
	f1 := newNamedTestActor("Bob", "p-2", room)
	byID := map[string]command.Actor{"p-2": f1}
	c := &command.Context{
		Actor:     leader,
		Follow:    &stubFollow{lost: []string{"p-2"}},
		ActorByID: func(id string) (command.Actor, bool) { a, ok := byID[id]; return a, ok },
	}
	if err := command.LoseHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if leader.lastLine() != "You shake off your follower." {
		t.Errorf("leader msg = %q", leader.lastLine())
	}
	if f1.lastLine() != "Alice slips away, and you lose the trail." {
		t.Errorf("shaken follower msg = %q", f1.lastLine())
	}
}
