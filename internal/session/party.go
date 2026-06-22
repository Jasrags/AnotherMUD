package session

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/world"
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
		m.partyJoinSeq[leaderID] = m.nextSeqLocked() // founder is the most-tenured
	}
	m.partyInvite[inviteeID] = leaderID
	return nil
}

// nextSeqLocked hands out the next monotonic join stamp (partyMu held). Used to
// order members by tenure for leadership succession.
func (m *Manager) nextSeqLocked() uint64 {
	m.partySeq++
	return m.partySeq
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
	m.partyJoinSeq[inviteeID] = m.nextSeqLocked()
	delete(m.partyInvite, inviteeID)
	return nil
}

// Leave removes memberID from their party (grouping.md §3). Outcomes:
//   - A non-leader leaves → only they are removed; the party persists.
//   - The leader leaves and ≥2 members remain → leadership PASSES to the
//     longest-tenured remaining member (succession); newLeaderID names them and
//     others lists the surviving party (the new leader included) to notify.
//   - The leader leaves and only one would remain → the party of one dissolves.
//
// Succession is the graceful path for an involuntary departure (a `leave` or a
// logout). An explicit `Disband` still hard-dissolves — see Disband.
func (m *Manager) Leave(memberID string) (disbanded bool, newLeaderID string, others []string, had bool) {
	if m == nil {
		return false, "", nil, false
	}
	m.partyMu.Lock()
	defer m.partyMu.Unlock()
	leaderID, ok := m.partyLeader[memberID]
	if !ok {
		return false, "", nil, false
	}
	if memberID == leaderID {
		// Leader leaving. If ≥2 members would remain, pass leadership rather
		// than punish the party; otherwise the lone survivor dissolves.
		if len(m.partyMembers[leaderID]) > 2 {
			newLeaderID = m.succeedLocked(leaderID)
			return false, newLeaderID, m.othersLocked(newLeaderID, ""), true
		}
		others = m.disbandLocked(leaderID, memberID)
		return true, "", others, true
	}
	// A non-leader leaves: drop just them.
	delete(m.partyMembers[leaderID], memberID)
	delete(m.partyLeader, memberID)
	delete(m.partyJoinSeq, memberID)
	// A lone leader left → dissolve.
	if len(m.partyMembers[leaderID]) <= 1 {
		others = m.disbandLocked(leaderID, "")
		return true, "", others, true
	}
	return false, "", m.othersLocked(leaderID, memberID), true
}

// succeedLocked passes leadership of oldLeaderID's party to its longest-tenured
// remaining member and removes the old leader (grouping.md §3 succession;
// partyMu held). The party is keyed by leader id, so this RE-KEYS the member
// set, every member's leader pointer, and any pending invites from the old
// leader onto the new one. Returns the new leader's id. Precondition: the set
// has >2 members, so at least two remain after the old leader is removed (a
// successor and at least one other — else the caller dissolves the party).
func (m *Manager) succeedLocked(oldLeaderID string) string {
	set := m.partyMembers[oldLeaderID]
	// The successor is the remaining member with the smallest join stamp (in
	// the party the longest). Map iteration is unordered, so the seq compare —
	// not iteration order — is what makes the choice deterministic.
	newLeaderID := ""
	var best uint64
	for id := range set {
		if id == oldLeaderID {
			continue
		}
		if seq := m.partyJoinSeq[id]; newLeaderID == "" || seq < best {
			newLeaderID, best = id, seq
		}
	}
	// Drop the departing leader from the set + the per-member maps, then re-key
	// the (now leaderless) party onto the successor.
	delete(set, oldLeaderID)
	delete(m.partyLeader, oldLeaderID)
	delete(m.partyJoinSeq, oldLeaderID)
	m.rekeyLeaderLocked(oldLeaderID, newLeaderID)
	return newLeaderID
}

// rekeyLeaderLocked moves the party currently keyed by oldLeaderID to be keyed by
// newLeaderID (partyMu held): the member set, every member's leader pointer, any
// pending invites from the old leader, and the loot policy all follow. The caller
// has already shaped the member set — succession removes the departing leader;
// promotion keeps them as a regular member. A master pointing at a now-absent
// member is handled by lootMasterLocked's fallback, so no master fix-up is needed.
func (m *Manager) rekeyLeaderLocked(oldLeaderID, newLeaderID string) {
	set := m.partyMembers[oldLeaderID]
	delete(m.partyMembers, oldLeaderID)
	m.partyMembers[newLeaderID] = set
	for id := range set {
		m.partyLeader[id] = newLeaderID
	}
	for invitee, l := range m.partyInvite {
		if l == oldLeaderID {
			m.partyInvite[invitee] = newLeaderID
		}
	}
	if mode, ok := m.partyLootMode[oldLeaderID]; ok {
		m.partyLootMode[newLeaderID] = mode
		delete(m.partyLootMode, oldLeaderID)
	}
	if master, ok := m.partyLootMaster[oldLeaderID]; ok {
		m.partyLootMaster[newLeaderID] = master
		delete(m.partyLootMaster, oldLeaderID)
	}
}

// Promote hands leadership of leaderID's party to targetID (grouping.md §3),
// leader only. Unlike succession (an unplanned departure → longest-tenured), this
// is the leader's deliberate choice: the old leader REMAINS in the party as a
// regular member. targetID must be a current member other than the leader.
func (m *Manager) Promote(leaderID, targetID string) ([]string, error) {
	if m == nil {
		return nil, command.ErrGroupNotLeader
	}
	m.partyMu.Lock()
	defer m.partyMu.Unlock()
	if l := m.partyLeader[leaderID]; l != leaderID || m.partyMembers[leaderID] == nil {
		return nil, command.ErrGroupNotLeader
	}
	if targetID == leaderID || !m.partyMembers[leaderID][targetID] {
		return nil, command.ErrGroupPromoteTarget
	}
	m.rekeyLeaderLocked(leaderID, targetID)
	// Snapshot the (now targetID-keyed) member set under the lock so the caller
	// announces the handoff without a second, racy read.
	set := m.partyMembers[targetID]
	members := make([]string, 0, len(set))
	for id := range set {
		members = append(members, id)
	}
	return members, nil
}

// LootPolicy returns the party's loot mode and (master-looter only) the
// designated master's pid for playerID's party (grouping.md §9). A master that
// is unset or no longer a member falls back to the leader. inParty is false when
// ungrouped.
func (m *Manager) LootPolicy(playerID string) (command.LootMode, string, bool) {
	if m == nil {
		return command.LootFFA, "", false
	}
	m.partyMu.Lock()
	defer m.partyMu.Unlock()
	leaderID, ok := m.partyLeader[playerID]
	if !ok {
		return command.LootFFA, "", false
	}
	if m.partyLootMode[leaderID] == command.LootMaster {
		return command.LootMaster, m.lootMasterLocked(leaderID), true
	}
	return command.LootFFA, "", true
}

// SetLootMode sets the loot policy for leaderID's party (grouping.md §9), leader
// only. For LootMaster, masterID names the designated member; "" defaults to the
// leader. A non-member masterID is refused. On success it returns the resolved
// policy (mode + effective master) under the same lock, so the caller announces
// it without a second read.
func (m *Manager) SetLootMode(leaderID string, mode command.LootMode, masterID string) (command.LootMode, string, error) {
	if m == nil {
		return command.LootFFA, "", command.ErrGroupNotLeader
	}
	m.partyMu.Lock()
	defer m.partyMu.Unlock()
	if l := m.partyLeader[leaderID]; l != leaderID || m.partyMembers[leaderID] == nil {
		return command.LootFFA, "", command.ErrGroupNotLeader
	}
	if mode == command.LootMaster {
		if masterID == "" {
			masterID = leaderID
		} else if !m.partyMembers[leaderID][masterID] {
			return command.LootFFA, "", command.ErrLootMasterNotMember
		}
		m.partyLootMode[leaderID] = command.LootMaster
		m.partyLootMaster[leaderID] = masterID
		return command.LootMaster, masterID, nil
	}
	m.partyLootMode[leaderID] = command.LootFFA
	delete(m.partyLootMaster, leaderID)
	return command.LootFFA, "", nil
}

// LootOwners returns the bare player ids that own a corpse from killerPID's kill
// per the party's loot policy (grouping.md §5/§9), for the corpse owner-set hook:
//   - ungrouped → nil (the solo killer owns it; corpse creation falls back)
//   - free-for-all → every party member (killer included)
//   - master-looter → just the designated master (the leader if unset/departed)
func (m *Manager) LootOwners(killerPID string) []string {
	if m == nil {
		return nil
	}
	m.partyMu.Lock()
	defer m.partyMu.Unlock()
	leaderID, ok := m.partyLeader[killerPID]
	if !ok {
		return nil
	}
	if m.partyLootMode[leaderID] == command.LootMaster {
		return []string{m.lootMasterLocked(leaderID)}
	}
	set := m.partyMembers[leaderID]
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	return ids
}

// lootMasterLocked returns the effective master-looter pid for leaderID's party
// (partyMu held): the designated member, or the leader when none is set or the
// designated master has since left the party.
func (m *Manager) lootMasterLocked(leaderID string) string {
	master := m.partyLootMaster[leaderID]
	if master == "" || !m.partyMembers[leaderID][master] {
		return leaderID
	}
	return master
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
		delete(m.partyJoinSeq, id)
		if id != exclude {
			others = append(others, id)
		}
	}
	delete(m.partyMembers, leaderID)
	delete(m.partyLootMode, leaderID)
	delete(m.partyLootMaster, leaderID)
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

// GrantKillXP awards a lethal kill's experience (grouping.md §4): recipients are
// the killer's party members present in the kill room (proximity-gated), or just
// the killer when ungrouped (a party of one). The total is split EVENLY; each
// share lands on the recipient's default track and is announced. A total or a
// rounded-to-zero share grants nothing. Called from the mob-killed reward hook.
func (m *Manager) GrantKillXP(ctx context.Context, prog *progression.Manager, track, killerPID string, room world.RoomID, total int64) {
	if m == nil || prog == nil || total <= 0 || track == "" || killerPID == "" {
		return
	}
	recipients := m.killXPRecipients(killerPID, room)
	if len(recipients) == 0 {
		return
	}
	share := total / int64(len(recipients))
	if share <= 0 {
		// The split rounds to nothing — a tiny-XP mob in a large party. Logged so
		// content authors tuning xp_value can see why nobody gained.
		logging.From(ctx).Debug("kill xp rounded to zero",
			slog.String("event", "kill_xp.zero_share"),
			slog.Int64("total", total), slog.Int("recipients", len(recipients)))
		return
	}
	for _, a := range recipients {
		a.GrantXP(ctx, prog, track, "kill", share)
		_ = a.Write(ctx, fmt.Sprintf("You gain %d experience.", share))
	}
}

// killXPRecipients resolves the present, in-room XP recipients for a kill: the
// killer's party (or the lone killer when ungrouped), filtered to those online
// and standing in the kill room.
func (m *Manager) killXPRecipients(killerPID string, room world.RoomID) []*connActor {
	members := m.Members(killerPID)
	if len(members) == 0 {
		members = []string{killerPID} // ungrouped → a party of one
	}
	out := make([]*connActor, 0, len(members))
	for _, id := range members {
		a, ok := m.GetByPlayerID(id)
		if !ok {
			continue
		}
		// Snapshot the room once (each Room() call takes a.mu) to avoid a
		// check-then-use window on the proximity gate.
		if r := a.Room(); r == nil || r.ID != room {
			continue
		}
		out = append(out, a)
	}
	return out
}

// AutoAssistCandidates resolves the party-mates who could be auto-pulled into
// engagerID's fight (grouping.md §9): the engager's party members who are
// online, standing in the given room (proximity-gated, mirroring kill-XP), have
// auto-assist enabled, and are not the engager themselves. The combat-side
// filter (already-in-combat) is applied by the caller, which holds the combat
// manager; this method stays free of any combat dependency.
//
// oppID is the bare player id of the opponent, or "" when the opponent is not a
// player (a mob can never be a party member). The PvP-safety guard — never
// auto-pull a party against one of its own members (a friendly duel must not
// snowball) — is resolved HERE, off the SAME membership snapshot used to build
// the candidate list, so the guard and the list can't disagree under a
// concurrent leave/disband. Returns nil when the engager is ungrouped or the
// opponent is a party-mate.
func (m *Manager) AutoAssistCandidates(engagerID, oppID string, room world.RoomID) []*connActor {
	if m == nil || engagerID == "" {
		return nil
	}
	members := m.Members(engagerID)
	if len(members) == 0 {
		return nil // ungrouped — no party to pull from
	}
	if oppID != "" && slices.Contains(members, oppID) {
		return nil // PvP between party-mates — don't gang up on a friend
	}
	out := make([]*connActor, 0, len(members))
	for _, id := range members {
		if id == engagerID {
			continue
		}
		a, ok := m.GetByPlayerID(id)
		if !ok || !a.AutoAssistEnabled() {
			continue
		}
		// Snapshot the room once (each Room() call takes a.mu) to avoid a
		// check-then-use window on the proximity gate.
		if r := a.Room(); r == nil || r.ID != room {
			continue
		}
		out = append(out, a)
	}
	return out
}

// dropParty removes id from any party on logout/teardown (grouping.md §3) and
// messages the survivors. A departing leader passes leadership (succession)
// rather than disbanding the party — the same graceful path as `leave`.
func (m *Manager) dropParty(ctx context.Context, id, name string) {
	disbanded, newLeaderID, others, had := m.Leave(id)
	if !had {
		return
	}
	newLeaderName := ""
	if newLeaderID != "" {
		if a, ok := m.GetByPlayerID(newLeaderID); ok {
			newLeaderName = a.Name()
		}
	}
	for _, oid := range others {
		a, ok := m.GetByPlayerID(oid)
		if !ok {
			continue
		}
		switch {
		case disbanded:
			_ = a.Write(ctx, "The party disbands.")
		case newLeaderID != "" && oid == newLeaderID:
			_ = a.Write(ctx, fmt.Sprintf("%s has left; you now lead the party.", name))
		case newLeaderID != "":
			_ = a.Write(ctx, fmt.Sprintf("%s has left; %s now leads the party.", name, newLeaderName))
		default:
			_ = a.Write(ctx, name+" leaves the party.")
		}
	}
}
