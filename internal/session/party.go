package session

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/command"
)

// defaultPartyCap is the fallback party size cap (grouping.md §7) when
// ANOTHERMUD_PARTY_CAP is unset. Includes the leader.
const defaultPartyCap = 6

// Manager implements command.GroupService (grouping.md). The roster is guarded
// by partyMu (separate from m.mu so notification lookups via GetByPlayerID don't
// reenter). A party is a leader id keyed in partyMembers with a member set that
// includes the leader; every member maps back to the leader in partyLeader.

// Invite records a pending invitation from leaderID to inviteeID, creating
// leaderID's party if they have none.
func (m *Manager) Invite(leaderID, inviteeID string) error {
	if m == nil || leaderID == "" || inviteeID == "" || leaderID == inviteeID {
		return command.ErrGroupSelf
	}
	m.partyMu.Lock()
	defer m.partyMu.Unlock()
	if _, grouped := m.partyLeader[inviteeID]; grouped {
		return command.ErrGroupHasParty
	}
	// The inviter must be a leader or ungrouped (a mere member can't invite).
	if l, grouped := m.partyLeader[leaderID]; grouped && l != leaderID {
		return command.ErrGroupInviterBad
	}
	// Check the cap BEFORE forming the party, so a too-small cap can't leave a
	// dangling 1-member party behind a rejected invite. A fresh party would be
	// just the leader (size 1).
	size := 1
	if existing := m.partyMembers[leaderID]; existing != nil {
		size = len(existing)
	}
	if size >= m.partyCap {
		return command.ErrGroupCapFull
	}
	if m.partyMembers[leaderID] == nil {
		m.partyLeader[leaderID] = leaderID
		m.partyMembers[leaderID] = map[string]bool{leaderID: true}
	}
	m.partyInvite[inviteeID] = leaderID
	return nil
}

// Accept consumes inviteeID's pending invite from leaderID, adding them.
func (m *Manager) Accept(inviteeID, leaderID string) error {
	if m == nil {
		return command.ErrGroupNoInvite
	}
	m.partyMu.Lock()
	defer m.partyMu.Unlock()
	if m.partyInvite[inviteeID] != leaderID || m.partyMembers[leaderID] == nil {
		return command.ErrGroupNoInvite // no invite, or the party is gone
	}
	if _, grouped := m.partyLeader[inviteeID]; grouped {
		return command.ErrGroupHasParty
	}
	if len(m.partyMembers[leaderID]) >= m.partyCap {
		return command.ErrGroupCapFull
	}
	m.partyMembers[leaderID][inviteeID] = true
	m.partyLeader[inviteeID] = leaderID
	delete(m.partyInvite, inviteeID)
	return nil
}

// Leave removes memberID from their party (grouping.md §3): the leader leaving
// disbands it; a party reduced to one dissolves.
func (m *Manager) Leave(memberID string) (disbanded bool, others []string, had bool) {
	if m == nil {
		return false, nil, false
	}
	m.partyMu.Lock()
	defer m.partyMu.Unlock()
	leaderID, ok := m.partyLeader[memberID]
	if !ok {
		return false, nil, false
	}
	if memberID == leaderID {
		others = m.disbandLocked(leaderID, memberID)
		return true, others, true
	}
	// A non-leader leaves: drop just them.
	delete(m.partyMembers[leaderID], memberID)
	delete(m.partyLeader, memberID)
	// A lone leader left → dissolve.
	if len(m.partyMembers[leaderID]) <= 1 {
		others = m.disbandLocked(leaderID, "")
		return true, others, true
	}
	return false, m.othersLocked(leaderID, memberID), true
}

// Disband dissolves leaderID's party (only if they lead it).
func (m *Manager) Disband(leaderID string) (others []string, ok bool) {
	if m == nil {
		return nil, false
	}
	m.partyMu.Lock()
	defer m.partyMu.Unlock()
	if l := m.partyLeader[leaderID]; l != leaderID || m.partyMembers[leaderID] == nil {
		return nil, false
	}
	return m.disbandLocked(leaderID, leaderID), true
}

// Members returns the party member ids (including playerID), or nil when
// ungrouped.
func (m *Manager) Members(playerID string) []string {
	if m == nil {
		return nil
	}
	m.partyMu.Lock()
	defer m.partyMu.Unlock()
	leaderID, ok := m.partyLeader[playerID]
	if !ok {
		return nil
	}
	set := m.partyMembers[leaderID]
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	return ids
}

// LeaderOf returns playerID's party leader and whether they're grouped.
func (m *Manager) LeaderOf(playerID string) (string, bool) {
	if m == nil {
		return "", false
	}
	m.partyMu.Lock()
	defer m.partyMu.Unlock()
	l, ok := m.partyLeader[playerID]
	return l, ok
}

// disbandLocked tears down leaderID's whole party (partyMu held), returning the
// member ids to notify EXCEPT exclude (the actor who triggered it, messaged
// separately). Also clears any pending invites from this leader.
func (m *Manager) disbandLocked(leaderID, exclude string) []string {
	set := m.partyMembers[leaderID]
	others := make([]string, 0, len(set))
	for id := range set {
		delete(m.partyLeader, id)
		if id != exclude {
			others = append(others, id)
		}
	}
	delete(m.partyMembers, leaderID)
	for invitee, l := range m.partyInvite {
		if l == leaderID {
			delete(m.partyInvite, invitee)
		}
	}
	return others
}

// othersLocked returns leaderID's members except exclude (partyMu held).
func (m *Manager) othersLocked(leaderID, exclude string) []string {
	set := m.partyMembers[leaderID]
	others := make([]string, 0, len(set))
	for id := range set {
		if id != exclude {
			others = append(others, id)
		}
	}
	return others
}

// dropParty removes id from any party on logout/teardown (grouping.md §3),
// returning the members to notify and whether the party disbanded. Mirrors
// Leave; the caller messages survivors.
func (m *Manager) dropParty(ctx context.Context, id, name string) {
	disbanded, others, had := m.Leave(id)
	if !had {
		return
	}
	notice := name + " leaves the party."
	if disbanded {
		notice = "The party disbands."
	}
	for _, oid := range others {
		if a, ok := m.GetByPlayerID(oid); ok {
			_ = a.Write(ctx, notice)
		}
	}
}
