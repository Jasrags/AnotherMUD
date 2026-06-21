package session

import (
	"errors"
	"slices"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
)

// inviteAccept is the common "L invites X, X accepts" helper.
func inviteAccept(t *testing.T, m *Manager, leader, invitee string) {
	t.Helper()
	if err := m.Invite(leader, invitee); err != nil {
		t.Fatalf("Invite(%s,%s): %v", leader, invitee, err)
	}
	if err := m.Accept(invitee, leader); err != nil {
		t.Fatalf("Accept(%s,%s): %v", invitee, leader, err)
	}
}

func TestParty_InviteAcceptFormsParty(t *testing.T) {
	m := NewManager()
	inviteAccept(t, m, "L", "A")
	if l, ok := m.LeaderOf("A"); !ok || l != "L" {
		t.Fatalf("LeaderOf(A) = %q,%v; want L,true", l, ok)
	}
	if l, ok := m.LeaderOf("L"); !ok || l != "L" {
		t.Fatalf("LeaderOf(L) = %q,%v; want L,true (leader is its own leader)", l, ok)
	}
	if got := sortedCopy(m.Members("A")); !slices.Equal(got, []string{"A", "L"}) {
		t.Fatalf("Members = %v, want [A L]", got)
	}
}

func TestParty_SelfAndAlreadyGrouped(t *testing.T) {
	m := NewManager()
	if err := m.Invite("L", "L"); !errors.Is(err, command.ErrGroupSelf) {
		t.Errorf("self-invite = %v, want ErrGroupSelf", err)
	}
	inviteAccept(t, m, "L", "A")
	// A is grouped → inviting A elsewhere is refused.
	if err := m.Invite("L2", "A"); !errors.Is(err, command.ErrGroupHasParty) {
		t.Errorf("invite already-grouped = %v, want ErrGroupHasParty", err)
	}
	// A (a non-leader member) can't invite.
	if err := m.Invite("A", "B"); !errors.Is(err, command.ErrGroupInviterBad) {
		t.Errorf("member invite = %v, want ErrGroupInviterBad", err)
	}
}

func TestParty_AcceptWithoutInvite(t *testing.T) {
	m := NewManager()
	if err := m.Accept("A", "L"); !errors.Is(err, command.ErrGroupNoInvite) {
		t.Errorf("accept w/o invite = %v, want ErrGroupNoInvite", err)
	}
}

func TestParty_Cap(t *testing.T) {
	m := NewManager()
	m.SetPartyCap(2) // leader + 1
	inviteAccept(t, m, "L", "A")
	if err := m.Invite("L", "B"); !errors.Is(err, command.ErrGroupCapFull) {
		t.Errorf("invite past cap = %v, want ErrGroupCapFull", err)
	}
}

func TestParty_CapOneLeavesNoDanglingParty(t *testing.T) {
	m := NewManager()
	m.SetPartyCap(1) // pathological cap: the leader alone fills it
	if err := m.Invite("L", "A"); !errors.Is(err, command.ErrGroupCapFull) {
		t.Fatalf("invite at cap 1 = %v, want ErrGroupCapFull", err)
	}
	// The refused invite must NOT have formed a dangling 1-member party.
	if _, ok := m.LeaderOf("L"); ok {
		t.Error("a cap-rejected invite left the leader in a dangling party")
	}
	if got := m.Members("L"); got != nil {
		t.Errorf("Members(L) = %v, want nil (no party formed)", got)
	}
}

func TestParty_NonLeaderLeave(t *testing.T) {
	m := NewManager()
	inviteAccept(t, m, "L", "A")
	inviteAccept(t, m, "L", "B")
	disbanded, others, had := m.Leave("A")
	if !had || disbanded {
		t.Fatalf("A leave: disbanded=%v had=%v, want false,true", disbanded, had)
	}
	if got := sortedCopy(others); !slices.Equal(got, []string{"B", "L"}) {
		t.Fatalf("others = %v, want [B L]", got)
	}
	if _, ok := m.LeaderOf("A"); ok {
		t.Error("A should be ungrouped after leaving")
	}
}

func TestParty_LeaderLeaveDisbands(t *testing.T) {
	m := NewManager()
	inviteAccept(t, m, "L", "A")
	inviteAccept(t, m, "L", "B")
	disbanded, others, had := m.Leave("L")
	if !had || !disbanded {
		t.Fatalf("L leave: disbanded=%v had=%v, want true,true", disbanded, had)
	}
	if got := sortedCopy(others); !slices.Equal(got, []string{"A", "B"}) {
		t.Fatalf("others = %v, want [A B]", got)
	}
	for _, id := range []string{"L", "A", "B"} {
		if _, ok := m.LeaderOf(id); ok {
			t.Errorf("%s still grouped after leader disband", id)
		}
	}
}

func TestParty_DissolvesAtOne(t *testing.T) {
	m := NewManager()
	inviteAccept(t, m, "L", "A")
	// A leaves → only L remains → dissolve.
	disbanded, others, _ := m.Leave("A")
	if !disbanded {
		t.Fatalf("party should dissolve when reduced to one; disbanded=%v", disbanded)
	}
	if got := sortedCopy(others); !slices.Equal(got, []string{"L"}) {
		t.Fatalf("others = %v, want [L]", got)
	}
	if _, ok := m.LeaderOf("L"); ok {
		t.Error("L should be ungrouped after the party dissolves")
	}
}

func TestParty_Disband(t *testing.T) {
	m := NewManager()
	inviteAccept(t, m, "L", "A")
	if _, ok := m.Disband("A"); ok {
		t.Error("a non-leader cannot disband")
	}
	others, ok := m.Disband("L")
	if !ok || !slices.Equal(sortedCopy(others), []string{"A"}) {
		t.Fatalf("Disband(L) = %v,%v; want [A],true", others, ok)
	}
}

func TestParty_AcceptAfterDisbandFails(t *testing.T) {
	m := NewManager()
	if err := m.Invite("L", "A"); err != nil {
		t.Fatal(err)
	}
	m.Disband("L") // leader disbands before A accepts → stale invite cleared
	if err := m.Accept("A", "L"); !errors.Is(err, command.ErrGroupNoInvite) {
		t.Errorf("accept after disband = %v, want ErrGroupNoInvite", err)
	}
}
