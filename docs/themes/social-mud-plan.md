# Theme A — Social MUD (plan)

**Hook:** Players can talk to each other across the world, not just in their
current room.

**Source:** `docs/THEME-AXIS-PLAN.md` §"Theme A — Social MUD".
**Roadmap milestone:** M13 (see `docs/ROADMAP.md`).
**Status:** spec phase — no code yet.

---

## Pre-decisions (locked 2026-05-30)

| Question | Decision |
|---|---|
| Channel ownership | **Hybrid** — engine baseline (`ooc`, `admin`) + pack-defined additions |
| History — channels | **Per-channel global ring buffer** (last N messages, shared across players) |
| History — tells | **Per-player persisted tell inbox** (offline tells deliver on next login) |
| Ignore / block | **Deferred** — ship channels + tells first; add `ignore <name>` as follow-up |
| GMCP routing | **Plain telnet only** — GMCP `Comm.Channel` is Theme B's job |

### Implications

- **On-disk shape:**
  ```
  saves/
    channels/
      ooc.yaml        ← shared ring buffer, last N messages
      admin.yaml
      <pack-channel>.yaml
    players/
      <name>/
        player.yaml
        tells.yaml    ← per-player inbox (offline + recent delivered)
  ```
- **Channel scrollback is shared.** Every player sees the same recent N
  messages on `chan last ooc` (verb TBD). No per-player read pointers in v1.
- **Tell inbox is per-player.** Survives restart. Delivered tells stay in
  the inbox up to a cap (decision in spec phase).
- **No GMCP** means Mudlet/MUSHclient users see chat as normal MUD text
  in v1. Theme B will retrofit `Comm.Channel.Text` etc. on top of the
  same publish surface.
- **No ignore** means the publish path doesn't filter recipients in v1.
  Adding ignore later only needs a filter step at fan-out — no schema
  change to channel files (the messages were published regardless).

---

## Internal sequence

1. **Spec the notification queue** (M13.1). Per-entity priority queue —
   the substrate everything else publishes through. Smallest, isolated.
2. **Spec channels + tells** on top of the queue (M13.2). Hybrid ownership
   model; global ring buffer on disk; per-player inbox; verbs (`ooc`,
   `tell`, `reply`, channel listing).
3. **Spec emotes** (M13.3). Registry shape + actor/target/room pronoun
   substitution. Independent of channels — can land last or in parallel.
4. **Implement queue → tells → channels → emotes** (M13.4-M13.7). One
   shippable slice per step; demo target moves forward at each.

### Why this order

The notification queue is the substrate everything else publishes through.
Tells are the simplest channel (1:1) and exercise the whole pipeline —
publish, deliver, persist, redeliver on login. Multi-recipient channels
follow once the substrate is real. Emotes are independent and can land
last or in parallel without blocking anyone.

---

## Current step

**Step 1 — Spec the notification queue (M13.1).**

The queue is the substrate. Pin its shape before anything else is written.

Open questions for the spec:
- Per-entity or per-session ownership? (Lean: per-entity, because the
  tell inbox is per-player and the queue must survive logout/login
  cycles for offline delivery.)
- Cancellable or fire-and-forget? (Lean: fire-and-forget. Cancellation
  would mean a tell could be censored mid-publish; out of v1 scope.)
- Priority levels — what does "priority" mean for chat? (Probably
  three tiers: `system` > `tell` > `channel`. Pin in the spec.)
- Tick-driven drain vs. immediate delivery? (Lean: immediate when the
  recipient session is live; queued when offline; drained on login.)
- Bounded queue — what's the per-entity cap, and what's the eviction
  policy when full? (Open. Probably FIFO drop-oldest with a logged
  warning. Pin in spec.)

---

## Open pre-decisions still in spec phase

- **Ring buffer size per channel.** Default `last 50` is a placeholder.
  Pin in M13.2 spec.
- **Tell inbox cap and eviction policy.** When inbox is at cap, drop
  oldest or refuse new? Pin in M13.2 spec.
- **Channel pack-declaration schema.** YAML shape, namespacing
  (`tapestry-core:ooc` vs. bare `ooc`), role-gate field. Pin in M13.2.
- **Engine baseline channel set.** Just `ooc` + `admin`, or also
  `newbie` and `gossip`? Lean small: `ooc` + `admin` only. Pin in M13.2.
- **Emote substitution grammar.** `$n` actor, `$N` target, possessive
  forms — match a known MUD convention or design our own. Pin in M13.3.

---

## Shape estimate

4-6 weeks total per the theme plan.

| Step | Estimate |
|---|---|
| M13.1 notification queue spec + impl | ~1 week |
| M13.2 channels + tells spec | ~3-5 days |
| M13.3 emotes spec | ~2-3 days |
| M13.4 queue impl | already in M13.1 estimate |
| M13.5 tells impl | ~1 week (touches player save schema bump) |
| M13.6 channels impl | ~1 week (channels/ save dir + ring buffer + verbs) |
| M13.7 emotes impl | ~3-5 days (registry + substitution + verbs) |

Pure engine work, no protocol-level changes required (works on bare
telnet). Player save shape bumps to add tell-inbox version; channels
save dir is new — no migration needed (empty on first boot).

---

## Demo target

Two players in different rooms chat over `ooc`; one tells the other
privately; one emotes (`smile bob`); both see channel history on
reconnect; the offline tell delivers when the recipient logs back in.

---

## Tracking

- This file owns the live sequence + current step.
- `docs/ROADMAP.md` M13 heading owns the standard `[ ]/[x]` exit
  criteria once spec writing produces them.
- `docs/TAPESTRY-GAP-MATRIX.md` §1.6 (chat/tells/notifications), §1.7
  (emotes), and §3 (notifications queue, channels/tells/who) entries
  get struck or shrunk when M13 closes.

When M13 ends:
1. Move closed items out of `docs/TAPESTRY-GAP-MATRIX.md`.
2. Either archive this file or leave it for history.
3. Pick the next theme via the rubric in `docs/THEME-AXIS-PLAN.md`
   §"Picking the next theme".
