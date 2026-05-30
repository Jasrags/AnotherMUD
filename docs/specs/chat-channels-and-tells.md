# Chat Channels and Tells — Feature Specification

**Status:** Draft · **Scope:** Multi-recipient out-of-character
chat channels (engine baseline + pack-defined); one-to-one
private tells with an offline inbox; per-channel global
scrollback persisted to disk; verbs that drive both; the
contract between this feature and the
[notifications](notifications.md) substrate it publishes
through · **Audience:** Anyone reimplementing or porting this
feature in any language.

This document describes *what* the chat surface must do, not
*how* to implement it. Specific channel names, ring buffer
sizes, inbox caps, history-verb shapes, and role-gate
identifiers are policy and live in the configuration-surface
table at §10.

This spec is the consumer layer on top of `notifications`. The
notification queue owns delivery; this spec owns *what gets
sent through it* (channels, tells), the persisted scrollback
that survives restart, and the verbs that publish.

The third sibling — emotes — is specced separately
(`emotes.md`) because it does not publish through the
notification queue (emotes are room-scoped, not addressed).

---

## 1. Overview

The chat surface has two distinct flavors that share substrate:

1. **Channels** — fan-out to many recipients (everyone tuned
   in to that channel). Multi-recipient by design. The
   transcript is shared across all players (one canonical
   scrollback per channel).
2. **Tells** — one-to-one private message between two players.
   The transcript is per-recipient (Bob's tell inbox is
   private to Bob). Offline tells deliver on next login.

Both publish through the [notifications](notifications.md)
queue. They differ in priority tier (tells > channels), in
persistence surface (global ring buffer vs. per-player inbox),
and in addressing (set of subscribers vs. one named recipient).

### 1.1 Out of v1 scope

- **Ignore / block.** Per-player suppression of messages from
  named senders. Deferred to a follow-up after the M13 slice
  lands. See `docs/themes/social-mud-plan.md`.
- **GMCP `Comm.Channel` routing.** All chat in v1 is plain MUD
  text. Mudlet and similar clients see chat the same as any
  other server output. GMCP is Theme B's work.
- **Cross-pack channel federation.** Each channel is owned by
  one pack (or the engine). Two packs cannot share a channel
  id. See §3.2 namespacing.
- **Channel ops / moderation verbs.** No `kick from channel`,
  no temporary mutes. Admins can use system tools and the
  deferred ignore primitive when it lands.
- **Rich media.** No links, no images, no MXP. Text only.

---

## 2. Channels

A channel is a named multi-recipient pub-sub topic with
shared scrollback. Examples: `ooc`, `admin`, a pack-defined
`newbie` or `trade`.

### 2.1 Ownership

Channels are owned in one of two modes:

- **Engine baseline.** A small fixed set that always exists,
  regardless of which packs are loaded. The engine guarantees
  these channels are present and that core verbs (`ooc`,
  admin announce) work. The exact baseline set is
  configurable (see §10).
- **Pack-defined.** A pack declares additional channels in
  its content. Pack channels do not exist if their owning
  pack is not loaded. Pack channels can be role-gated and
  decorated the same as engine channels.

Engine and pack channels are observably identical at the
verb layer. The distinction matters only at load time
(engine channels boot before any pack; pack channels load
during pack discovery) and at the namespacing layer (§3.2).

### 2.2 Channel shape

Each channel carries:

- **id** — short stable identifier. Engine-baseline channels
  use bare ids (`ooc`); pack-defined channels are namespaced
  (`<pack>:<id>`) per §3.2.
- **display name** — the verb players type and the prefix in
  rendered output. Usually matches the bare id.
- **kind** — `public` (anyone tuned in), `gated` (a tag/role
  filter applies), `private` (membership-only; not used in
  v1 baseline but the model accommodates it).
- **role gate** — optional. If set, only players carrying a
  matching role tag can speak on the channel; non-matching
  players may or may not be able to listen, depending on
  the gate's `listen` policy (see §3.3).
- **color / decoration** — optional ANSI color or theme tag
  used to render `[ooc] Alice: …`. Plain-text fallback if
  no color is set.
- **default-on** — whether a brand-new character is auto-
  tuned-in. Engine `ooc` is `true`; engine `admin` is
  `false` (and gated).
- **persisted** — whether messages on this channel are
  written to the global ring buffer (§4). Defaults `true`;
  ephemeral channels (admin shoptalk, system notices) may
  opt out.

### Acceptance — channels

- [ ] Engine baseline channels exist regardless of which
      packs are loaded.
- [ ] A pack channel exists if and only if its declaring
      pack is loaded.
- [ ] Each channel carries id, display name, kind, optional
      role gate, optional decoration, default-on flag,
      persisted flag.

---

## 3. Channel declaration

### 3.1 Engine baseline

The engine ships a minimum baseline set so chat works out of
the box. The baseline set lives in the **engine content pack**
(`tapestry-core`, per `internal/pack/manifest.go`'s
`EngineNamespace`), not in engine code. This matches the
existing convention for every other engine-baseline content
type (rooms, areas, mobs, items, races, classes, abilities).

The default baseline is `tapestry-core:ooc` (public,
default-on, persisted) and `tapestry-core:admin` (gated,
default-off, ephemeral). The exact baseline is configurable
in §10.

Baseline channels load through the same `pack.Load` pipeline
as every other registry — they are not a special case. Verb
dispatch uses the channel's display name (`ooc`, `admin`),
not the namespaced id, so the player-facing surface is
unchanged.

> **Implication:** removing the `tapestry-core` pack removes
> the baseline channels along with every other piece of
> engine content (rooms, mobs, etc.). This is the same
> constraint that already applies to every other registry;
> no new forking concern.

### 3.2 Pack-defined channels

Packs declare channels in a content file (parallel to mob
files, item files, etc. — exact filename in §10). Each
declaration carries the same fields as §2.2 plus an
explicit `pack` field that matches the declaring pack id.

Channel ids in pack YAML may be written bare (`trade`) or
qualified (`mypack:trade`). Bare ids resolve to the
declaring pack's namespace; qualified ids must match the
declaring pack (a pack cannot declare a channel in another
pack's namespace).

Verb resolution uses the display name. If two loaded packs
both ship `trade` channels, the second-loaded pack's
declaration fails with a structured error
(`event=channel.collision`, `id`, `pack`, `colliding_pack`)
and that pack is rejected at load time. There is no
silent override.

### 3.3 Role gates

A role-gated channel has:

- **speak gate** — set of role tags that can publish to the
  channel. Empty set means "no one can speak" (listen-only
  broadcasts).
- **listen gate** — set of role tags that can subscribe.
  Empty set means "no listen restriction" (anyone can hear).

Both gates are evaluated against the publisher's / listener's
current role tag set, not their account permissions. (Role
elevation is a separate concern; see
`commands-and-dispatch`.)

Publishing without the speak role returns a structured
failure to the verb (`NotPermitted`); subscribing without
the listen role omits the channel from the player's `tune-
in` results and from the channels-list verb.

### Acceptance — declaration

- [ ] Engine baseline channels load before pack discovery.
- [ ] Pack channels load with their declaring pack and
      unload when the pack unloads.
- [ ] Bare ids in pack YAML resolve to the declaring
      pack's namespace.
- [ ] A pack cannot declare a channel in another pack's
      namespace.
- [ ] Two packs declaring the same channel id is a load-
      time error (the second pack is rejected).
- [ ] Role gates are evaluated against current role tags
      at publish/subscribe time, not cached at login.

---

## 4. Channel scrollback

Persisted channels keep a global ring buffer of the last N
messages. The buffer is *one canonical transcript per
channel*, shared across all players — every player who runs
`history ooc` (or whatever the configured verb is) sees the
same scrollback.

### 4.1 Storage layout

```
saves/
  channels/
    <channel-id>.yaml      ← engine baseline channels (bare id)
    <pack>__<id>.yaml      ← pack channels (filesystem-safe split)
```

Filename rules:
- Engine baseline: `<id>.yaml` (e.g., `ooc.yaml`).
- Pack channels: `<pack>__<id>.yaml` (double-underscore as
  the separator because `:` is unsafe on some filesystems).
- The on-disk content stores the *full* qualified id; the
  filename is just an addressing convention.

### 4.2 Buffer shape

The file holds an ordered list of recent messages, oldest
first. Each message carries:

- **published_at** — engine `Clock` time at publish.
- **sender_id** — publisher's entity id.
- **sender_name** — publisher's display name at publish time
  (snapshotted because rename or deletion shouldn't blank
  out scrollback).
- **text** — the rendered message body.

Pruning: when a new message would push the buffer past the
configured cap, the oldest message is evicted. Cap is per-
channel (see §10) so noisy channels can be larger than
quiet ones.

### 4.3 Write cadence

Channel scrollback is mutated on every publish but flushed
to disk on a cadence (not on every publish), batched per
channel, using the atomic tmp→bak→rename rotation from the
`persistence` spec. A crash loses at most one cadence-
interval of trailing messages.

### 4.4 Ephemeral channels

A channel with `persisted: false` does *not* write to disk.
It still has an in-memory ring buffer (for live `history`
verb output) but the buffer is empty on every restart. The
admin baseline channel is ephemeral by default.

### Acceptance — scrollback

- [ ] Two players running the channel-history verb see
      byte-identical scrollback for the same channel at the
      same tick.
- [ ] Scrollback survives process restart for `persisted:
      true` channels.
- [ ] Scrollback for `persisted: false` channels is empty
      after restart.
- [ ] Channel files use the atomic tmp→bak→rename rotation.
- [ ] Renaming or deleting a player does not blank their
      historical entries.
- [ ] The buffer is capped per-channel; oldest messages
      evict first.

---

## 5. Subscriptions

A player is either **tuned in** to a channel or not. Tuned-in
means: they hear messages on the channel and they appear in
the channel's `who` (if/when that verb lands).

### 5.1 Default subscriptions

- New characters auto-tune to every channel where the
  channel's `default-on` is `true` and the listen gate
  permits.
- Gated channels (`default-on: false` or listen gate fails)
  require an explicit tune-in.
- The subscription set is stored on the player file
  (under a new `chat.subscriptions` key) so it survives
  logout.

### 5.2 Tune / untune

Two verbs:

- `chat tune <channel>` — opt in. Fails if listen gate denies.
- `chat untune <channel>` — opt out. Player no longer
  receives messages on that channel.

A player can untune any channel including baseline `ooc`
(no force-on). Admins re-subscribe via the same verb.

### 5.3 Auto-subscribe on new pack

When a pack loads at runtime (post-boot reload, future
scope), already-online players are auto-tuned to any new
`default-on: true` channels their listen gate permits. Future-
compat statement; v1 packs only load at boot.

### Acceptance — subscriptions

- [ ] A new character is tuned to all default-on, listen-
      permitted channels and no others.
- [ ] Tune/untune persist across logout.
- [ ] Tuning into a gate-denied channel returns
      `NotPermitted` and does not mutate the subscription
      set.
- [ ] A player can untune `ooc`.

---

## 6. Publishing

### 6.1 Channel publish

A player types `<channel> <message>` (verb dispatch routes
the channel id; see §8). The flow:

1. Resolve the channel id from the verb.
2. Check the speak gate against the publisher's current
   role tags. Deny on fail.
3. Build a notification with:
   - `priority: channel`
   - `kind: channel`
   - `recipients`: every player currently tuned in
     (excluding the publisher? — see open question §11)
   - `text`: the rendered line, including channel prefix
     and sender name
   - `sender`: publisher's entity id
4. Publish through the [notifications](notifications.md)
   queue (§4 of that spec).
5. Append the message to the channel's ring buffer (§4),
   in parallel with the publish.

Failure to publish to one recipient (e.g., a session writer
error) does not stop the others, per the notifications
substrate.

### 6.2 Tell publish

A player types `tell <name> <message>`. The flow:

1. Resolve the recipient by name. Returns `NoSuchPlayer`
   if no character exists with that name (case-insensitive).
2. Build a notification with:
   - `priority: tell`
   - `kind: tell`
   - `recipients: [<recipient_entity_id>]`
   - `text`: the rendered line (`Alice tells you: hi`)
   - `sender`: publisher's entity id
3. Publish through the notifications queue.
4. Send a confirmation line back to the publisher
   (`You tell Alice: hi`).
5. Record the tell in the *publisher's* `last-told` slot
   (for `reply`) and in the *recipient's* `last-tell-from`
   slot (also for `reply`).

The `reply` verb publishes a tell back to the most recent
sender of a tell received. The slot survives login (see
§7.2). If no sender is recorded, `reply` returns
`NoReplyTarget`.

### Acceptance — publish

- [ ] A channel publish reaches every tuned-in subscriber
      with a live session (immediate) plus any offline
      tuned-in subscribers (enqueued).
- [ ] A channel publish appends one entry to the ring
      buffer regardless of recipient count.
- [ ] A tell publish reaches one specific recipient (online
      or offline) and writes one confirmation line back to
      the publisher.
- [ ] `reply` resolves the last sender at the publish
      moment; it does not cache the target between sessions
      beyond what §7.2 specifies.
- [ ] Failed publish to one channel subscriber does not
      stop publish to the others.

---

## 7. Tells inbox

A tell that arrives while the recipient is online is
delivered immediately by the notifications substrate.
A tell that arrives while the recipient is offline is
enqueued.

### 7.1 Storage layout

**Resolved 2026-05-30 (M13 open-Q pass):** the substrate's
`notifications.yaml` (see [notifications](notifications.md)
§6.3) is the single source of truth for the offline
backlog. There is *no* separate `tells.yaml` inbox file in
v1 — tells, channel posts, and any future addressed message
share one priority-ordered queue per recipient. The earlier
spec wording that called for a dedicated `tells.yaml` inbox
was written before M13.1 landed; it would have been
double-persistence.

The `last_tell_from` slot (the `reply` target) lives **in-
memory on the session actor** for v1: set when a tell is
delivered, cleared on logout. A server restart drops it; a
reconnecting player gets `NoReplyTarget` until a new tell
arrives. Persistence can be added later (as a tiny per-
player file, or as a field on player.yaml) if the UX hit
proves real.

```
saves/
  players/
    <name>/
      player.yaml
      notifications.yaml   ← substrate queue, holds offline tells
                              alongside any channel/system posts
```

### 7.2 Drain on reconnect

When a player session enters the active phase, the
notifications substrate drains the queue (priority order).
Tell-priority notifications drain first. Each drained tell:

1. Is written to the session writer with framing copy owned
   by this spec (e.g., `--- Tells while you were away ---`).
2. Is removed from the substrate queue once the writer
   accepts the line. (The notifications substrate
   guarantees this.)

The `last_tell_from` slot is set to the *most recent* tell's
sender after drain, so `reply` works immediately on login
within the same process. After a server restart the slot is
empty until a fresh tell arrives.

### 7.3 Inbox cap

The tell inbox has a per-player cap (see §10). At cap, a new
tell evicts the oldest tell, **not** the new one:

- The eviction is logged at the notifications substrate
  level (`notify.queue.evicted`).
- The sender is *not* notified that their old tell was
  evicted (no read receipts, no NACKs in v1).

### Acceptance — tells inbox

- [ ] Tells received while offline appear in
      `notifications.yaml` (substrate queue) and are
      delivered on the next login in publish order.
- [ ] The substrate queue survives process restart and
      atomic-write crashes the same way `player.yaml` does.
- [ ] Substrate queue cap evicts oldest lowest-priority
      entry, not newest (already enforced by notifications
      §6.1).
- [ ] `last_tell_from` (in-memory on the session actor) is
      set after drain so `reply` works on the freshly-
      logged-in player within the current process lifetime.
- [ ] An online player receives a tell with no on-disk
      writes (immediate delivery only).

---

## 8. Verbs

The exact verb names are policy; the canonical lean set in
this section is the suggested baseline.

### 8.1 Per-channel verbs

Each loaded channel registers a verb matching its display
name (`ooc`, `admin`, pack channel names). The verb takes
one argument: the message body. No flags in v1.

`ooc hello everyone` → publishes "hello everyone" on the
`ooc` channel.

Channel-named verbs are case-insensitive on lookup but
echo the channel's canonical display name in output.

### 8.2 Tell verbs

- `tell <name> <message>` — publish a tell.
- `reply <message>` — publish a tell to `last_tell_from`.

### 8.3 Subscription verbs

- `chat list` — show available channels and the player's
  subscription state for each. Listen-gate-denied channels
  are omitted from the list.
- `chat tune <channel>` — opt in.
- `chat untune <channel>` — opt out.

### 8.4 History verb

- `chat history <channel> [n]` — render the most recent
  N messages from the channel's ring buffer (cap N at the
  channel's buffer size; default N is configurable).

History always renders the canonical global scrollback (per
§4) — there is no per-player history filter in v1.

### 8.5 Tells review

- `tells` — render the recipient's tell session-history
  (in-memory only; what *this* session has received, not
  the persisted backlog). Helps players catch up on what
  scrolled off-screen during combat.

### Acceptance — verbs

- [ ] Every channel currently in the registry registers its
      display-name verb at boot.
- [ ] Channel-named verbs route to the channel publish flow
      with the message body as the sole argument.
- [ ] `tell`, `reply`, `chat list`, `chat tune`, `chat
      untune`, `chat history`, `tells` all exist as
      distinct verbs.
- [ ] `chat list` omits listen-gate-denied channels.
- [ ] `chat history` respects the channel's ring buffer
      cap and the requested N (whichever is smaller).

---

## 9. Observability

Beyond the notifications substrate's own logging, this
layer emits:

| Event | Fields | When |
|---|---|---|
| `channel.publish.ok` | channel, sender, recipient_count | publish completes |
| `channel.publish.gated` | channel, sender, gate | speak gate denies |
| `channel.collision` | id, pack, colliding_pack | duplicate id at pack load |
| `channel.subscribe` | channel, player, default | tune-in or auto-on |
| `channel.unsubscribe` | channel, player | untune |
| `tell.publish.ok` | sender, recipient | tell sent |
| `tell.no_such_player` | sender, attempted_name | tell to nobody |
| `reply.no_target` | sender | reply with empty slot |

### Acceptance — observability

- [ ] Every observable transition emits one structured log
      line at appropriate severity.
- [ ] Routine publish/subscribe events log at `debug`;
      gates and errors log at `warn`.

---

## 10. Configuration surface

| Setting | Default | Meaning |
|---|---|---|
| Engine baseline channels | `tapestry-core:ooc` (public, default-on, persisted), `tapestry-core:admin` (gated, default-off, ephemeral) | Ship in the `content/core/channels/` directory of the engine content pack. |
| Pack channel filename | `channels/*.yaml` under pack root | Loader glob (same pattern as `rooms/*.yaml`, `mobs/*.yaml`, etc.). |
| Channel ring buffer cap (default) | 50 | Per-channel scrollback size when channel doesn't override. |
| Channel ring buffer cap (override) | per-channel field | Channels may set their own (admin = 20, ooc = 100, etc.). |
| Channel save cadence | autosave cadence | When to flush ring buffer to disk. |
| Tell inbox cap | n/a — uses notifications queue cap (50) | Tells share the substrate's per-entity priority queue. |
| Tell inbox TTL | n/a — uses notifications policy (none in v1) | Same. |
| `last_tell_from` storage | in-memory on session actor (v1) | Survives within process lifetime; reset on server restart. Persistence can land later if UX demands it. |
| Channel history default N | 20 | Default count for `chat history <channel>`. |
| Tells session-history cap | 50 | In-memory recent-tells for `tells` verb. |
| Subscriptions storage key | `chat.subscriptions` on player | Player-file location. |
| Admin role tag | `role:admin` | Default speak/listen gate for `tapestry-core:admin`. Pin in M13.6 impl if the role-tag convention differs. |
| Publisher self-echo | confirmation copy only | Channel publisher sees `You ooc: hi`; they are NOT in the recipient set (mirrors `tell` flow). |
| Tell-name resolution | exact match, case-insensitive | No prefix/substring matching on player names; avoids spoofing-via-collision. |
| History authorization | current listen gate | Re-checked at history-verb invocation against current role tags; demotion revokes access. |
| Channel mute | not in v1 | Only tune/untune in v1; mute as a third subscription state is a follow-up. |

---

## 11. Open questions

Resolved during the M13 open-Q pass (2026-05-30): publisher
self-echo (confirmation-only), tell-name resolution (exact
match, case-insensitive), engine-baseline namespace
(`tapestry-core` content), history authorization (current
listen gate), tell to offline-known vs. unknown (offline
succeeds, unknown fails), per-player channel mute (deferred
to follow-up). Those decisions live in §3.1, §6, §8, and §10.

Remaining items, all noted for during-impl decisions:

- **Channel publish ordering within a tick.** If two players
  publish on `ooc` in the same tick, the ring buffer needs
  a stable order. Spec posture: notifications substrate
  stamps `published_at` from the engine `Clock`; ties
  broken by publish arrival order (FIFO within a tier).
  Confirm during M13.1 implementation that arrival order is
  actually deterministic under the writer lock.
- **Pack-channel filename collision.** Two packs each ship
  `channels/trade.yaml` with bare id `trade` — both qualify
  the bare id against their own pack, so the resulting
  qualified ids are `<pack1>:trade` and `<pack2>:trade`. No
  collision on qualified ids; but both packs register a
  display-name verb `trade`, and *that* collides. Pin
  during M13.6: either require packs to namespace their
  display names too, or first-wins with a load-warn.

---

## Cross-references

- `notifications` — the queue substrate this spec
  publishes through; tells and channels are consumers.
- `persistence` — atomic-write rotation, save directory
  layout, player save shape (`chat.subscriptions` key).
- `commands-and-dispatch` — verb registration, argument
  resolution, role tag evaluation, keyword resolver
  (player name lookup for `tell`).
- `session-lifecycle` — when a session enters active
  phase (drain trigger), link-dead/reconnect transitions.
- `scripting-and-packs` — pack manifest format, content
  globs, dependency ordering, the `channels/*.yaml` glob
  this spec proposes.
- `docs/themes/social-mud-plan.md` — Theme A live plan,
  pre-decisions, current step.
- `docs/specs/README.md` — substrate-layer placement,
  indexes to update when this spec lands.
- `emotes` (forthcoming) — sibling presentation feature;
  *does not* publish through the notifications queue.
