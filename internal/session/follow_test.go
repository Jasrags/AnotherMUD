package session

import (
	"errors"
	"slices"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
)

func TestFollowGraph_BasicFollowAndReport(t *testing.T) {
	m := NewManager()
	if err := m.Follow("a", "b"); err != nil {
		t.Fatalf("Follow: %v", err)
	}
	if l, ok := m.Following("a"); !ok || l != "b" {
		t.Fatalf("Following(a) = %q, %v; want b, true", l, ok)
	}
	if _, ok := m.Following("b"); ok {
		t.Error("b follows no one")
	}
}

func TestFollowGraph_SelfRefused(t *testing.T) {
	m := NewManager()
	if err := m.Follow("a", "a"); !errors.Is(err, command.ErrFollowSelf) {
		t.Fatalf("self-follow err = %v, want ErrFollowSelf", err)
	}
	if _, ok := m.Following("a"); ok {
		t.Error("a self-follow should record nothing")
	}
}

func TestFollowGraph_CycleRefused(t *testing.T) {
	m := NewManager()
	// A→B→C is a legal chain.
	if err := m.Follow("b", "c"); err != nil {
		t.Fatal(err)
	}
	if err := m.Follow("a", "b"); err != nil {
		t.Fatal(err)
	}
	// C following A would close the loop A→B→C→A.
	if err := m.Follow("c", "a"); !errors.Is(err, command.ErrFollowCycle) {
		t.Fatalf("cycle err = %v, want ErrFollowCycle", err)
	}
	// Direct 2-cycle: B already follows C, so C following B closes it.
	if err := m.Follow("c", "b"); !errors.Is(err, command.ErrFollowCycle) {
		t.Fatalf("2-cycle err = %v, want ErrFollowCycle", err)
	}
}

func TestFollowGraph_ReplacesPriorLeader(t *testing.T) {
	m := NewManager()
	m.Follow("a", "b")
	m.Follow("a", "c") // re-target
	if l, _ := m.Following("a"); l != "c" {
		t.Fatalf("Following(a) = %q, want c after re-target", l)
	}
	// b should no longer count a as a follower.
	if got := m.Lose("b"); len(got) != 0 {
		t.Errorf("b's followers = %v, want none (a re-targeted to c)", got)
	}
	if got := m.Lose("c"); !slices.Contains(got, "a") {
		t.Errorf("c's followers = %v, want to include a", got)
	}
}

func TestFollowGraph_UnfollowAndLose(t *testing.T) {
	m := NewManager()
	m.Follow("a", "leader")
	m.Follow("b", "leader")

	if l, had := m.Unfollow("a"); !had || l != "leader" {
		t.Fatalf("Unfollow(a) = %q, %v; want leader, true", l, had)
	}
	if _, ok := m.Following("a"); ok {
		t.Error("a should follow no one after unfollow")
	}
	// Unfollow of a non-follower is a clean miss.
	if _, had := m.Unfollow("nobody"); had {
		t.Error("Unfollow(nobody) reported a relationship")
	}

	lost := m.Lose("leader")
	if !slices.Equal(sortedCopy(lost), []string{"b"}) {
		t.Fatalf("Lose(leader) = %v, want [b]", lost)
	}
	if _, ok := m.Following("b"); ok {
		t.Error("b should follow no one after the leader loses them")
	}
	if got := m.Lose("leader"); got != nil {
		t.Errorf("a second Lose returns %v, want nil", got)
	}
}

func sortedCopy(s []string) []string {
	out := append([]string(nil), s...)
	slices.Sort(out)
	return out
}
