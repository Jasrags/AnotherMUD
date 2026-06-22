package session

import (
	"errors"
	"slices"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// TestKillXPRecipients covers the grouping-specific XP recipient selection
// (grouping.md §4): a solo killer is a party of one; a party shares only with
// members present in the kill room.
func TestKillXPRecipients(t *testing.T) {
	mgr := NewManager()
	roomA, roomB := world.RoomID("z:a"), world.RoomID("z:b")
	add := func(pid string, r world.RoomID) *connActor {
		a := &connActor{id: "c-" + pid, playerID: pid, room: &world.Room{ID: r}}
		mgr.Add(a)
		return a
	}
	killer := add("K", roomA)

	if got := mgr.killXPRecipients("K", roomA); len(got) != 1 || got[0] != killer {
		t.Fatalf("solo recipients = %v, want just the killer", got)
	}
	if got := mgr.killXPRecipients("K", roomB); len(got) != 0 {
		t.Fatalf("a killer not in the kill room yields no recipients, got %v", got)
	}

	add("A", roomA) // same room as the kill
	add("B", roomB) // a different room
	for _, id := range []string{"A", "B"} {
		if err := mgr.Invite("K", id); err != nil {
			t.Fatal(err)
		}
		if err := mgr.Accept(id, "K"); err != nil {
			t.Fatal(err)
		}
	}
	got := mgr.killXPRecipients("K", roomA)
	ids := make([]string, 0, len(got))
	for _, a := range got {
		ids = append(ids, a.playerID)
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []string{"A", "K"}) {
		t.Fatalf("party recipients = %v, want [A K] (B is in another room)", ids)
	}
}

// TestAutoAssistCandidates covers grouping.md §9 candidate selection: only
// party-mates (not the engager), present in the engager's room, with
// auto-assist enabled, qualify. An ungrouped engager has no candidates.
func TestAutoAssistCandidates(t *testing.T) {
	mgr := NewManager()
	roomA, roomB := world.RoomID("z:a"), world.RoomID("z:b")
	add := func(pid string, r world.RoomID, assist bool) *connActor {
		a := &connActor{id: "c-" + pid, playerID: pid, room: &world.Room{ID: r}}
		a.autoAssist.Store(assist)
		mgr.Add(a)
		return a
	}

	// Engager E, ungrouped → no candidates even with willing players around.
	add("E", roomA, true)
	if got := mgr.AutoAssistCandidates("E", "", roomA); len(got) != 0 {
		t.Fatalf("ungrouped engager yields no candidates, got %v", got)
	}

	add("A", roomA, true)  // same room, assist on  → included
	add("B", roomB, true)  // different room        → excluded
	add("C", roomA, false) // same room, assist off → excluded
	for _, id := range []string{"A", "B", "C"} {
		inviteAccept(t, mgr, "E", id)
	}

	got := mgr.AutoAssistCandidates("E", "", roomA)
	ids := make([]string, 0, len(got))
	for _, a := range got {
		ids = append(ids, a.playerID)
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []string{"A"}) {
		t.Fatalf("candidates = %v, want [A] (E is the engager, B elsewhere, C opted out)", ids)
	}

	// PvP safety: if the opponent is a party-mate (a friendly duel), the whole
	// party is withheld — no candidates, regardless of who is willing/present.
	if got := mgr.AutoAssistCandidates("E", "A", roomA); len(got) != 0 {
		t.Fatalf("opponent is a party-mate, want no candidates, got %v", got)
	}
}

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
