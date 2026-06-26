package session

import (
	"errors"
	"slices"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// TestHirelingHoldsNoPartySeat locks the resolved hireable-mobs.md §11 decision:
// a hireling is the owner's ASSET, not a party seat. A live hireling standing in
// the kill room never becomes an XP recipient (so it can't dilute the human
// split) nor a loot owner nor a party-roster member — its kills flow through the
// owner's seat instead. The party graph is keyed on player ids throughout, so the
// exclusion is structural; this is the executable guard against a future change
// that wires co-located hirelings into the split.
func TestHirelingHoldsNoPartySeat(t *testing.T) {
	mgr := NewManager()
	place := entities.NewPlacement()
	mgr.actionEnv = command.Env{Placement: place}
	roomA := world.RoomID("z:a")
	add := func(pid string) *connActor {
		a := &connActor{id: "c-" + pid, playerID: pid, room: &world.Room{ID: roomA}}
		mgr.Add(a)
		return a
	}
	owner := add("K")
	add("A")
	if err := mgr.Invite("K", "A"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Accept("A", "K"); err != nil {
		t.Fatal(err)
	}

	// The owner has a live hireling standing in the kill room.
	const hid = entities.EntityID("entity-h")
	place.Place(hid, roomA)
	owner.TrackLiveHireling(hid, "sw:sellsword")

	// XP recipients are the two humans only — the hireling adds no seat.
	got := mgr.killXPRecipients("K", roomA)
	ids := make([]string, 0, len(got))
	for _, a := range got {
		ids = append(ids, a.playerID)
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []string{"A", "K"}) {
		t.Fatalf("XP recipients = %v, want [A K] (a hireling holds no seat)", ids)
	}

	// Loot owners are the two humans only.
	owners := mgr.LootOwners("K")
	slices.Sort(owners)
	if !slices.Equal(owners, []string{"A", "K"}) {
		t.Fatalf("loot owners = %v, want [A K] (a hireling holds no seat)", owners)
	}

	// And the hireling id never appears in the party roster.
	if slices.Contains(mgr.Members("K"), string(hid)) {
		t.Error("hireling id leaked into the party roster")
	}
}

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
	disbanded, newLeaderID, others, had := m.Leave("A")
	if !had || disbanded || newLeaderID != "" {
		t.Fatalf("A leave: disbanded=%v newLeader=%q had=%v, want false,\"\",true", disbanded, newLeaderID, had)
	}
	if got := sortedCopy(others); !slices.Equal(got, []string{"B", "L"}) {
		t.Fatalf("others = %v, want [B L]", got)
	}
	if _, ok := m.LeaderOf("A"); ok {
		t.Error("A should be ungrouped after leaving")
	}
}

// TestParty_LeaderLeaveSucceeds: a leader leaving a party of three passes
// leadership to the longest-tenured remaining member (grouping.md §3) rather
// than disbanding. A joined before B, so A succeeds.
func TestParty_LeaderLeaveSucceeds(t *testing.T) {
	m := NewManager()
	inviteAccept(t, m, "L", "A")
	inviteAccept(t, m, "L", "B")
	disbanded, newLeaderID, others, had := m.Leave("L")
	if !had || disbanded {
		t.Fatalf("L leave: disbanded=%v had=%v, want false,true", disbanded, had)
	}
	if newLeaderID != "A" {
		t.Fatalf("newLeaderID = %q, want A (the longest-tenured remaining member)", newLeaderID)
	}
	// The survivors (new leader included) are notified; the old leader is gone.
	if got := sortedCopy(others); !slices.Equal(got, []string{"A", "B"}) {
		t.Fatalf("others = %v, want [A B]", got)
	}
	if _, ok := m.LeaderOf("L"); ok {
		t.Error("the departed leader L should be ungrouped")
	}
	// The party is re-keyed onto A: both survivors point at A, the roster holds.
	if l, ok := m.LeaderOf("A"); !ok || l != "A" {
		t.Fatalf("LeaderOf(A) = %q,%v; want A,true (A now leads itself)", l, ok)
	}
	if l, ok := m.LeaderOf("B"); !ok || l != "A" {
		t.Fatalf("LeaderOf(B) = %q,%v; want A,true", l, ok)
	}
	if got := sortedCopy(m.Members("B")); !slices.Equal(got, []string{"A", "B"}) {
		t.Fatalf("Members after succession = %v, want [A B]", got)
	}
}

// TestParty_LeaderLeaveOfTwoDissolves: with only one member left after the
// leader goes, there is no one to lead — the party of one dissolves.
func TestParty_LeaderLeaveOfTwoDissolves(t *testing.T) {
	m := NewManager()
	inviteAccept(t, m, "L", "A")
	disbanded, newLeaderID, others, had := m.Leave("L")
	if !had || !disbanded || newLeaderID != "" {
		t.Fatalf("L leave (party of 2): disbanded=%v newLeader=%q had=%v, want true,\"\",true", disbanded, newLeaderID, had)
	}
	if got := sortedCopy(others); !slices.Equal(got, []string{"A"}) {
		t.Fatalf("others = %v, want [A]", got)
	}
	if _, ok := m.LeaderOf("A"); ok {
		t.Error("A should be ungrouped after the party dissolves")
	}
}

func TestParty_DissolvesAtOne(t *testing.T) {
	m := NewManager()
	inviteAccept(t, m, "L", "A")
	// A leaves → only L remains → dissolve.
	disbanded, _, others, _ := m.Leave("A")
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

// TestParty_SuccessionPicksLongestTenured: succession follows join order, not
// map iteration. With B joining before C, B succeeds when L leaves.
func TestParty_SuccessionPicksLongestTenured(t *testing.T) {
	m := NewManager()
	inviteAccept(t, m, "L", "B")
	inviteAccept(t, m, "L", "C")
	_, newLeaderID, _, _ := m.Leave("L")
	if newLeaderID != "B" {
		t.Fatalf("newLeaderID = %q, want B (joined before C)", newLeaderID)
	}
	// A second succession: B leaves → C is the only remaining non-leader.
	_, newLeaderID2, _, _ := m.Leave("B")
	if newLeaderID2 != "" {
		t.Fatalf("party of two leader-leave should dissolve, got newLeader=%q", newLeaderID2)
	}
}

// TestParty_SuccessionTransfersPendingInvite: an invite the old leader had sent
// but that wasn't accepted yet must re-target the new leader, so the invitee can
// still `Accept` after succession (grouping.md §3 — succeedLocked re-keys invites).
func TestParty_SuccessionTransfersPendingInvite(t *testing.T) {
	m := NewManager()
	inviteAccept(t, m, "L", "A")
	inviteAccept(t, m, "L", "B")
	// C is invited by L but hasn't joined when L leaves.
	if err := m.Invite("L", "C"); err != nil {
		t.Fatalf("Invite C: %v", err)
	}
	_, newLeaderID, _, _ := m.Leave("L")
	if newLeaderID != "A" {
		t.Fatalf("newLeaderID = %q, want A", newLeaderID)
	}
	// The stale invite (was against L) must now resolve against A.
	if err := m.Accept("C", "L"); err == nil {
		t.Fatal("Accept against the departed leader L should fail")
	}
	if err := m.Accept("C", "A"); err != nil {
		t.Fatalf("Accept against the new leader A should succeed, got %v", err)
	}
	if got := sortedCopy(m.Members("C")); !slices.Equal(got, []string{"A", "B", "C"}) {
		t.Fatalf("Members after invite-transfer = %v, want [A B C]", got)
	}
}

// TestParty_DisbandStillHardDissolves: an explicit `disband` ends the party
// outright even when succession would have been possible — the distinct,
// deliberate counterpart to a leader's graceful `leave`.
func TestParty_DisbandStillHardDissolves(t *testing.T) {
	m := NewManager()
	inviteAccept(t, m, "L", "A")
	inviteAccept(t, m, "L", "B")
	others, ok := m.Disband("L")
	if !ok {
		t.Fatal("Disband(L) should succeed")
	}
	if got := sortedCopy(others); !slices.Equal(got, []string{"A", "B"}) {
		t.Fatalf("others = %v, want [A B]", got)
	}
	for _, id := range []string{"L", "A", "B"} {
		if _, ok := m.LeaderOf(id); ok {
			t.Errorf("%s still grouped after explicit disband", id)
		}
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

// --- Loot distribution policy (grouping.md §9) ---

// TestLootOwners_FreeForAllReturnsWholeParty: the default policy owns the kill
// for every member (killer included).
func TestLootOwners_FreeForAllReturnsWholeParty(t *testing.T) {
	m := NewManager()
	inviteAccept(t, m, "L", "A")
	inviteAccept(t, m, "L", "B")
	got := m.LootOwners("L")
	slices.Sort(got)
	if !slices.Equal(got, []string{"A", "B", "L"}) {
		t.Fatalf("free-for-all owners = %v, want the whole party", got)
	}
}

// TestLootOwners_UngroupedReturnsNil: a solo killer has no party policy, so the
// hook returns nil and corpse creation falls back to the killer alone.
func TestLootOwners_UngroupedReturnsNil(t *testing.T) {
	m := NewManager()
	if got := m.LootOwners("solo"); got != nil {
		t.Fatalf("ungrouped owners = %v, want nil", got)
	}
}

// TestLootOwners_MasterReturnsMasterOnly: under master-looter only the
// designated member owns the kill — the killer is deliberately excluded.
func TestLootOwners_MasterReturnsMasterOnly(t *testing.T) {
	m := NewManager()
	inviteAccept(t, m, "L", "A")
	inviteAccept(t, m, "L", "B")
	if _, _, err := m.SetLootMode("L", command.LootMaster, "A"); err != nil {
		t.Fatal(err)
	}
	// The killer (L) is not the master, yet the corpse owner set is just {A}.
	if got := m.LootOwners("L"); !slices.Equal(got, []string{"A"}) {
		t.Fatalf("master-looter owners = %v, want [A] only", got)
	}
}

// TestSetLootMode_DefaultsMasterToLeader: master mode with no member named
// designates the leader.
func TestSetLootMode_DefaultsMasterToLeader(t *testing.T) {
	m := NewManager()
	inviteAccept(t, m, "L", "A")
	if _, _, err := m.SetLootMode("L", command.LootMaster, ""); err != nil {
		t.Fatal(err)
	}
	mode, master, in := m.LootPolicy("A")
	if !in || mode != command.LootMaster || master != "L" {
		t.Fatalf("policy = (%v,%q,%v), want (master, L, true)", mode, master, in)
	}
}

// TestSetLootMode_LeaderOnly: a non-leader may not change the policy.
func TestSetLootMode_LeaderOnly(t *testing.T) {
	m := NewManager()
	inviteAccept(t, m, "L", "A")
	if _, _, err := m.SetLootMode("A", command.LootMaster, ""); !errors.Is(err, command.ErrGroupNotLeader) {
		t.Fatalf("non-leader SetLootMode = %v, want ErrGroupNotLeader", err)
	}
	if _, _, err := m.SetLootMode("nobody", command.LootFFA, ""); !errors.Is(err, command.ErrGroupNotLeader) {
		t.Fatalf("ungrouped SetLootMode = %v, want ErrGroupNotLeader", err)
	}
}

// TestSetLootMode_MasterMustBeMember: naming a non-member as master is refused.
func TestSetLootMode_MasterMustBeMember(t *testing.T) {
	m := NewManager()
	inviteAccept(t, m, "L", "A")
	if _, _, err := m.SetLootMode("L", command.LootMaster, "stranger"); !errors.Is(err, command.ErrLootMasterNotMember) {
		t.Fatalf("non-member master = %v, want ErrLootMasterNotMember", err)
	}
}

// TestLootPolicy_MasterFallsBackWhenMasterLeaves: a master who leaves the party
// falls back to the leader (membership-checked), so the corpse is never owned by
// a departed member.
func TestLootPolicy_MasterFallsBackWhenMasterLeaves(t *testing.T) {
	m := NewManager()
	inviteAccept(t, m, "L", "A")
	inviteAccept(t, m, "L", "B")
	if _, _, err := m.SetLootMode("L", command.LootMaster, "A"); err != nil {
		t.Fatal(err)
	}
	m.Leave("A") // the designated master departs
	mode, master, _ := m.LootPolicy("L")
	if mode != command.LootMaster || master != "L" {
		t.Fatalf("policy after master left = (%v,%q), want (master, L)", mode, master)
	}
	if got := m.LootOwners("L"); !slices.Equal(got, []string{"L"}) {
		t.Fatalf("owners after master left = %v, want [L]", got)
	}
}

// TestSuccession_CarriesLootPolicy: when leadership passes, the loot policy
// travels with the party onto the new leader.
func TestSuccession_CarriesLootPolicy(t *testing.T) {
	m := NewManager()
	inviteAccept(t, m, "L", "A") // A is the longest-tenured member
	inviteAccept(t, m, "L", "B")
	if _, _, err := m.SetLootMode("L", command.LootMaster, "B"); err != nil {
		t.Fatal(err)
	}
	disbanded, newLeader, _, had := m.Leave("L") // leader leaves → A succeeds
	if !had || disbanded || newLeader != "A" {
		t.Fatalf("succession = (disbanded %v, new %q, had %v), want (false, A, true)", disbanded, newLeader, had)
	}
	mode, master, _ := m.LootPolicy("A")
	if mode != command.LootMaster || master != "B" {
		t.Fatalf("policy after succession = (%v,%q), want (master, B)", mode, master)
	}
}

// --- Leader-named successor / promote (grouping.md §3) ---

// TestPromote_TransfersLeadershipKeepsOldLeader: promotion makes the target the
// leader while the old leader stays in the party as a regular member.
func TestPromote_TransfersLeadershipKeepsOldLeader(t *testing.T) {
	m := NewManager()
	inviteAccept(t, m, "L", "A")
	inviteAccept(t, m, "L", "B")
	if _, err := m.Promote("L", "A"); err != nil {
		t.Fatal(err)
	}
	if l, ok := m.LeaderOf("A"); !ok || l != "A" {
		t.Fatalf("A leader = (%q,%v), want A as its own leader", l, ok)
	}
	// The old leader is still in the party, now pointing at A.
	if l, ok := m.LeaderOf("L"); !ok || l != "A" {
		t.Fatalf("old leader L = (%q,%v), want still grouped under A", l, ok)
	}
	got := m.Members("A")
	slices.Sort(got)
	if !slices.Equal(got, []string{"A", "B", "L"}) {
		t.Fatalf("party after promote = %v, want all three (old leader retained)", got)
	}
}

// TestPromote_LeaderOnly: a non-leader can't promote.
func TestPromote_LeaderOnly(t *testing.T) {
	m := NewManager()
	inviteAccept(t, m, "L", "A")
	if _, err := m.Promote("A", "L"); !errors.Is(err, command.ErrGroupNotLeader) {
		t.Fatalf("non-leader promote = %v, want ErrGroupNotLeader", err)
	}
}

// TestPromote_TargetMustBeMember: promoting a non-member or yourself is refused.
func TestPromote_TargetMustBeMember(t *testing.T) {
	m := NewManager()
	inviteAccept(t, m, "L", "A")
	if _, err := m.Promote("L", "stranger"); !errors.Is(err, command.ErrGroupPromoteTarget) {
		t.Fatalf("promote non-member = %v, want ErrGroupPromoteTarget", err)
	}
	if _, err := m.Promote("L", "L"); !errors.Is(err, command.ErrGroupPromoteTarget) {
		t.Fatalf("promote self = %v, want ErrGroupPromoteTarget", err)
	}
}

// TestPromote_CarriesLootPolicy: the loot policy follows the leadership handoff.
func TestPromote_CarriesLootPolicy(t *testing.T) {
	m := NewManager()
	inviteAccept(t, m, "L", "A")
	inviteAccept(t, m, "L", "B")
	if _, _, err := m.SetLootMode("L", command.LootMaster, "B"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Promote("L", "A"); err != nil {
		t.Fatal(err)
	}
	mode, master, _ := m.LootPolicy("A")
	if mode != command.LootMaster || master != "B" {
		t.Fatalf("policy after promote = (%v,%q), want (master, B)", mode, master)
	}
}

// TestPromote_ThenSuccessionStillWorks: after a promote, an unplanned departure
// of the new leader still falls back to longest-tenured succession (the re-key
// left the join-order stamps intact).
func TestPromote_ThenSuccessionStillWorks(t *testing.T) {
	m := NewManager()
	inviteAccept(t, m, "L", "A") // L founder (oldest), then A, then B
	inviteAccept(t, m, "L", "B")
	if _, err := m.Promote("L", "B"); err != nil { // B leads, L + A remain members
		t.Fatal(err)
	}
	disbanded, newLeader, _, had := m.Leave("B") // B (leader) departs unplanned
	if !had || disbanded {
		t.Fatalf("leave = (disbanded %v, had %v), want a succession", disbanded, had)
	}
	if newLeader != "L" {
		t.Fatalf("successor = %q, want L (the longest-tenured remaining)", newLeader)
	}
}
