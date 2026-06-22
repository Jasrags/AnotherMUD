# Grouping (parties — shared XP, loot, and chat)

A **party**: a consensual collective of players who **share the rewards of
killing together** — experience and loot — and have a private channel. It is the
reward/social layer that sits beside the **follow** movement primitive
(`follow.md`): follow gets the party to travel together; grouping decides who
gets the XP and who may loot the corpse.

Grouping is the **consensual** counterpart to follow. `follow.md` parked the
consent question here: follow stays **consent-free** (you may trail anyone you
can see), and *grouping* — where rewards are shared — is **invite/accept**. You
can trail a stranger; you cannot share their kills without both agreeing.

It also **introduces combat kill-experience**, which the engine lacks today (XP
comes only from quests + the admin verb). Kill-XP is defined **party-aware from
the start**: a solo killer is simply a party of one, so the same grant path
serves both. *Greenfield — spec ahead of code; sliced (roster → kill-XP → loot).*

## 1. Overview

A party has a **leader** (the creator) and **members**. Membership is by
**invitation**: the leader invites, the invitee accepts. While grouped, members:

- **Share kill-XP** — a mob's experience is split among the party members present
  at the kill (§4).
- **Share loot rights** — the corpse's looting-rights window admits the whole
  party, not just the killer (§5).
- **Share a channel** — a party-only `gtell` (§6).

Travel together is **not** grouping's job — that's `follow` (members `follow` the
leader). Grouping and follow are **independent**: you may follow without
grouping, or group without following. (A future convenience could auto-follow on
join; v1 keeps them decoupled.)

### Kill-XP, introduced here

The engine has **no combat kill-XP** today. This spec adds it, defined through the
party from the start so there is one grant path:

- A mob carries an **experience value** (content metadata, §4).
- On a lethal kill, that value is granted to the **killer's party members present
  in the room**, split among them (§4). A killer with no party is a **party of
  one** and receives the full value.
- XP lands on each recipient's **default progression track** (the same default
  the rest of the engine uses), so no per-kill track choice is needed.

### Goals / non-goals

**Goals.** A party roster (invite / accept / leave / disband / list); kill-XP
introduced party-aware (mob XP value, the split, solo = party of one); shared
loot rights (the corpse owner-set filled with the party); a party channel
(`gtell`); transient (never persisted); clean teardown on logout.

**Non-goals.** Combat **assist** / auto-engage (a party member's fight pulling
the others) — a follow-up slice. **Shared quest credit** (quest objectives are
per-character) — deferred. The *auto-distribution* **loot policies**
(round-robin / need-greed) — deferred (the manual-loot model needs new machinery
for them); the **master-looter** policy shipped (§5/§9), beside the default
free-for-all rights-sharing. A **mob-XP balance pass** across all content — v1 reads
whatever `xp_value` content declares (0 when absent); tuning the curve is content
work, not this spec. Persisted parties across logout.

## 2. Forming and leaving a party

- **`group <player>`** — invite a player you can **see** in your room to your
  party. If you are in no party, this **creates** one with you as leader. The
  target receives an invitation; nothing is shared until they accept.
- **`join <leader>`** (alias `group accept`) — accept a pending party invitation
  from that leader.
- **`group`** (no argument) — list your party: leader first, then members, with
  who is present in your room.
- **`leave`** (alias `ungroup`) — leave your current party. The **leader**
  leaving **disbands** the party (§3).
- **`disband`** — the leader dissolves the party; every member is notified.

An invitation is **transient** and **single-target** (the latest invite from a
given leader stands); accepting one you no longer have, or that the leader
rescinded by disbanding, fails cleanly. A player is in **at most one** party.

### Membership rules

- The party has a **size cap** (§7); an invite past the cap is refused.
- You cannot invite someone already in a party (yours or another's); they must
  `leave` first.
- A self-invite, or inviting someone you can't see, is refused.

### Acceptance criteria

- [ ] `group <visible player>` with no current party creates a party (inviter =
      leader) and sends an invitation; the target is not yet a member.
- [ ] `join <leader>` / `group accept` on a standing invitation adds the invitee;
      both the new member and the existing party are notified.
- [ ] Accepting a non-existent / rescinded invitation fails cleanly.
- [ ] `group` lists the party with room-presence; `leave` removes a member;
      the leader's `leave`/`disband` dissolves the party and notifies all.
- [ ] Inviting past the cap, inviting an already-grouped player, self-invite, or
      an unseen target are each refused.

## 3. Leadership and dissolution

The **leader** is the party's owner: they hold the invite power. Two ways a
leader can step away, with deliberately different outcomes:

- **`leave` (or logout/link-loss)** is the *graceful* exit. If at least two
  members would remain, leadership **passes** to the longest-tenured remaining
  member — **succession** — and the party survives. If only one member would
  remain, the "party of one" **dissolves** (an ungrouped player). This is the
  friendly default: an involuntary or routine departure does not punish the
  rest of the party.
- **`disband`** is the *deliberate* end. It dissolves the whole party outright,
  even when succession would have been possible. This is the leader's explicit
  "we're done" — the distinct counterpart to a graceful `leave`.

A non-leader leaving removes only themselves; the party persists for the rest.
A party reduced to **one member** always dissolves.

**Succession order.** The successor is the member who has been in the party the
longest (excluding the departing leader) — join order, not arrival-in-room or
any other signal. Pending invites the old leader had sent transfer to the new
leader.

### Acceptance criteria

- [ ] A leader's `leave`/logout with ≥2 members remaining passes leadership to
      the longest-tenured remaining member; the party persists, the new leader
      is told they now lead, and the rest are told who leads.
- [ ] A leader's `leave`/logout that would leave only one member dissolves the
      party; the survivor ends ungrouped.
- [ ] A leader's `disband` dissolves the party outright regardless of size; every
      remaining member is notified and ends ungrouped.
- [ ] A non-leader's `leave`/logout removes only them; the party persists for the
      rest.
- [ ] A party reduced to one member dissolves.

## 4. Shared experience (and kill-XP itself)

On a **lethal** mob kill (`combat.md` §10 kill credit — the attacker id; subdual
knock-outs grant nothing, `subdual-damage.md`):

1. Resolve the **killer**.
2. Read the mob's **experience value** (content metadata; 0 → no XP, the silent
   default for unvalued content).
3. Determine the **recipients**: the killer's party members who are **in the room
   where the kill happened** (proximity-gated). A killer with no party is the
   sole recipient (party of one).
4. **Split** the value **evenly** among the recipients (`value / count`, integer;
   the remainder is dropped — a small rounding loss, acceptable for v1). Grant
   each recipient their share on their **default track**, emitting the normal
   XP-gained/level-up path (`progression`).

Proximity-gating (same room) is the anti-abuse rule: a grouped player AFK across
the world earns nothing from a kill they weren't present for. The split makes
grouping a **trade-off** — faster kills, shared spoils — not free multiplication.

### Acceptance criteria

- [x] A solo killer of a mob with XP value `V` gains `V` on their default track;
      a mob with no XP value grants nothing. **SHIPPED 2026-06-21.**
- [x] A party of `N` all present at the kill each gain `V/N` (integer split);
      a member in another room gains nothing. **SHIPPED** (the recipient set is
      same-room party members; share = `V/len`, rounded-to-zero grants nothing).
- [x] A subdual knock-out grants no XP (no kill) — subdual publishes no
      `MobKilled`, so the XP hook never fires. **SHIPPED.**
- [x] XP lands on the default track and drives the normal level-up path
      (`GrantXP` → the existing `LevelUp` render). **SHIPPED 2026-06-21.**

## 5. Shared loot

A corpse records an **owner set** that governs the looting-rights window
(`loot-and-corpses.md` §4: only owners may loot until the window expires, then
it opens to all). Today that set is just the killer. Grouping **fills it with the
killer's party** so any party member may loot the kill during the window.

- On corpse creation for a party kill, the owner set = the killer **plus their
  party members** (all members, not only those in the room — a member who arrives
  within the window may still loot).
- A solo killer's owner set is unchanged (just them).
- The window, expiry, and open-to-all-after behavior are unchanged
  (`loot-and-corpses.md` §4).

### Loot distribution policy (§9 resolved)

A party's owner set follows its **loot mode**, set by the leader with
`lootmode` (`loot` is the corpse-take verb, so the policy lives under its own
keyword):

- **Free-for-all** (`lootmode ffa`, the default): the owner set is the whole
  party, as above — any member may loot the kill.
- **Master-looter** (`lootmode master [<member>]`): the owner set is **just the
  designated member** — loot funnels to them and they distribute it (e.g. via
  `give`). The **killer is deliberately excluded** unless they are the master.
  Naming no member designates the leader; a master who later leaves the party
  falls back to the leader (so a corpse is never owner-locked to a departed
  member). The leader-only mutation is announced to the whole party; any member
  may read the current policy with a bare `lootmode`.

The corpse owner set is the single seam both modes write through (the
`OwnerSet` hook returns the complete set; killer-inclusion is the policy's call).
Richer policies (round-robin / need-greed) would build on the same seam.

### Acceptance criteria

- [x] A party kill's corpse admits every party member to loot during the rights
      window; a non-member is refused until the window expires. **SHIPPED
      2026-06-21** (the corpse owner set = killer + party; `MayLoot` admits them).
- [x] A solo killer's corpse is owned by the killer alone (unchanged).
      **SHIPPED** (no party → `OwnerSet` returns nil → owner set is just the
      killer).
- [x] Under master-looter, only the designated member owns the kill — the killer
      is locked out (refused during the window), the master may loot. **SHIPPED
      2026-06-22** (`lootmode master`; `Manager.LootOwners` returns the master
      alone; live `TestLive_MasterLooter`).
- [x] `lootmode` (no arg) reports the current policy; `lootmode ffa|master` is
      leader-only and announced to the party; a non-member master is refused.
      **SHIPPED 2026-06-22.**

## 6. Party channel

**`gtell <message>`** sends to every **online** member of your party (including
yourself, as an echo). It is the party's private channel — distinct from room
`say`, global channels, and one-to-one `tell`. A player in no party is told they
have no party to talk to.

### Acceptance criteria

- [ ] `gtell <msg>` delivers to all online party members and echoes to the
      sender; the message is attributed to the speaker.
- [ ] `gtell` with no party reports there's no one to tell.

## 7. Configuration surface

| Setting | Meaning | Default |
|---|---|---|
| Party size cap | Max members per party (§2). | a small cap (e.g. 6) |
| Mob XP value | Per-mob experience awarded on a lethal kill (§4). | per-mob content metadata; 0 when absent |
| XP default track | The progression track kill-XP lands on (§4). | the engine's existing default XP track |

The XP **split rule** (even, proximity-gated) and the loot-rights window are
behavior, not knobs; the window itself is `loot-and-corpses.md`'s.

## 8. Interaction with existing systems

- **Follow** (`follow.md`): the movement counterpart; grouping resolves follow's
  parked consent question (follow stays consent-free; grouping is invite/accept).
  Independent relationships — neither requires the other.
- **Combat** (`combat.md` §10): kill credit (the attacker id) is the input to §4
  and §5; subdual knock-outs (`subdual-damage.md`) are not kills and grant
  nothing.
- **Progression** (`progression`): §4 is the first **combat** consumer of the XP
  grant (quests + the admin verb were the only callers); it uses the existing
  default-track grant + level-up path.
- **Loot & corpses** (`loot-and-corpses.md` §4): §5 fills the corpse owner set —
  the seam that doc reserved for grouping.
- **Mobs** (`mobs-ai-spawning`): the per-mob **XP value** is new mob metadata.
- **Chat** (`chat-channels-and-tells`): `gtell` is a party-scoped fan-out beside
  the global channels and `tell`.
- **Session lifecycle** (`session-lifecycle`): logout teardown (§3) — a member
  leaves, a leader disbands.

## 9. Open questions

- **Assist / auto-engage.** Manual **`assist <ally>`** SHIPPED 2026-06-21 — you
  engage whatever an ally in your room is fighting (any visible combatant, not
  party-gated; kill credit + kill-XP/loot sharing already flow through the party).
  **Auto-assist** SHIPPED 2026-06-22 — a per-player opt-in (`autoassist on|off`,
  off by default and persisted) that, when a party member becomes engaged with an
  enemy, automatically pulls that member's idle, in-room, opted-in party-mates
  into the same fight. It fires on the engagement event (the combat sink's
  `OnEngagement` hook) for **both** sides of the engagement, so it covers the
  offensive case (you attack, your party joins) and the defensive case (you are
  attacked, your party defends you). Guards: a mate already in combat is left on
  their current target (this is also the recursion terminator — an engaged mate
  is no longer idle, so the engage→event→engage chain bottoms out within one
  pass over the capped party); and a party is never auto-pulled against one of
  its own members (no friendly-duel snowball). Still deferred: an opt-in to
  auto-assist *non-party* allies, and a "only assist the leader" narrowing.
- **Leadership succession.** SHIPPED 2026-06-22 — see §3. A leader's `leave` or
  logout now passes leadership to the longest-tenured remaining member (when ≥2
  remain); an explicit `disband` still hard-dissolves. (Still open: letting the
  leader *name* a successor, rather than always the longest-tenured.)
- **Loot distribution policy.** SHIPPED 2026-06-22 — see §5. The leader chooses a
  per-party **loot mode** with `lootmode`: free-for-all (the v1 default — the
  whole party owns the kill) or **master-looter** (only a designated member owns
  it; loot funnels through them). Both write the corpse owner set through one
  `OwnerSet` seam. (Still open: the *auto-distribution* policies — round-robin and
  need/greed — which the engine's **manual** loot model would need new machinery
  for: turn tracking, or a per-item roll/contention window. Also open: `lootmode
  master <member>` resolves the member by their online actor name, so a link-dead
  member can't be *named* as master — the `lootMasterLocked` fallback keeps this
  safe; naming an offline member is a spec clarification for later.)
- **XP split shape.** v1 is an even split among present members. Level-weighted
  (lower-level members get more, or a flat tax) and a small **group bonus** (the
  party earns slightly more total, rewarding cooperation) are common; deferred
  until the kill-XP curve is tuned as content.
- **Auto-follow on join.** Should joining a party auto-`follow` the leader for
  cohesion? Convenient, but couples the two systems; v1 keeps them decoupled.
- **Shared quest credit.** Party-wide quest objective advancement is desirable
  but quest objectives are per-character; needs a design pass on the quest side.
