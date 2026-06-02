# Session and Connection Lifecycle — Feature Specification

**Status:** Draft · **Scope:** The PlayerSession runtime object,
the SessionManager registry and broadcast surface, the async
connection adapter used by login, the per-session token-bucket
flood protection, idle timeouts (warn + disconnect), link-dead
state and reconnection, session takeover stale-event handling,
the bounded input queue, the per-entity notification queue, and
the prompt refresh state machine · **Audience:** Anyone
reimplementing or porting this feature in any language.

This document describes *what* the session feature must do, not
*how* to implement it. Specific rates, timeouts, queue depths,
and prompt formats are policy and live outside this spec.

---

## 1. Overview

A **session** is the engine's view of a player who has finished
login and is interacting with the world. Sessions sit between
the network transport (which produces IConnection values) and
the engine's per-tick simulation. They:

- Carry the bound entity and account identifiers.
- Buffer player input between connection arrival and the game
  loop's per-tick command drain.
- Enforce flood limits on input.
- Track idle time so non-admin players can be timed out.
- Survive a connection drop as a **link-dead** record that a
  later login can re-attach to.
- Hold prompt-refresh and input-mode state used by the UI
  layer.

The feature also owns the **pre-login pool** — the set of
connections that have been accepted by the network layer but
have not yet bound to an entity / account. Pre-login entries
are not sessions yet; they become sessions when login
completes.

### Core concepts

- **Pre-login context** — a `LoginContext` (see
  `docs/specs/login.md`) held in the session manager's
  pre-login map, keyed by connection id. Carries the
  connection, the login phase, a per-phase cancellation
  token, and bookkeeping.
- **PlayerSession** — the post-login runtime object. Holds
  the connection, the player entity, the account id, the
  current login phase (Playing, LinkDead, Creating), an
  input queue, flood-protection state, idle state, prompt
  state, and an optional active flow instance.
- **SessionManager** — concurrent maps indexing sessions by
  connection id, entity id, lowercased player name, and
  account id. Plus the pre-login map. Provides every
  feature-facing send / broadcast operation.
- **Flood context** — the per-session token-bucket parameters
  (config + tick clock + logger + metric counters) passed
  into the session constructor.
- **Disconnect event** — a system-event-queue entry that
  carries connection id, entity id, and reason. The game
  loop drains the queue and routes each entry to a
  registered handler.
- **Notification** — a `(type, priority, text, optional GMCP
  package + payload)` record enqueued for an entity and
  delivered at the end of the tick that wrote it.

### Goals

1. Make session lookup O(1) by every identifier callers
   actually use (connection, entity, name, account).
2. Bound per-session input rate so a single client cannot
   stall the loop.
3. Reclaim idle non-admin sessions on a configurable timeout
   without affecting active players.
4. Preserve session state across a brief connection drop so
   players can reconnect without losing combat, location,
   or quest state.
5. Recognize stale disconnects from a taken-over connection
   and ignore them.
6. Provide a single broadcast surface (to a player, to a
   room, to a tag, to the world) so features don't crawl the
   session map themselves.

### Non-goals

- The transport layer's connection acceptance and IAC
  negotiation (covered by `docs/specs/networking-
  protocols.md`).
- The login flow itself (covered by `docs/specs/login.md`).
- The actual idle warn / timeout messages and link-dead
  reconnect timing — those are configuration.
- The contents of a notification — schema is content-driven.
- Persistence. Sessions are runtime state only. The session
  manager doesn't save anything; what survives a restart
  lives on the player entity, which the persistence feature
  owns.

---

## 2. PlayerSession

### 2.1 Identity and binding

A PlayerSession carries:

- A reference to the bound `IConnection`.
- A reference to the bound player entity.
- The owning account id (may be `Guid.Empty` for legacy /
  testing paths).
- The current `LoginPhase` (typically `Playing` post-login;
  may be `Creating` during character creation, `LinkDead`
  during a connection drop, etc.).
- A `ConnectedAt` timestamp (UTC, set at construction).

The connection and entity references MAY be mutated during
the session's lifetime by two operations:

- **`ReplaceConnection(newConnection)`** — used by link-dead
  reconnect and session takeover. Unwires the input handler
  from the old connection, sets the new connection, wires
  the input handler to it.
- **`ReplaceEntity(newEntity)`** — used by the flow engine's
  restart path (see `docs/specs/character-creation.md` §7).
  Replaces the entity reference; callers MUST update any
  external indices keyed on the old entity id.

### 2.2 Input pipeline

Each session owns:

- A bounded **input queue** (today MaxQueueDepth = 100). An
  enqueue that would exceed the cap is rejected; the
  client's input is dropped silently.
- An **input handler** (`Action<string>`) wired to the
  connection's `OnInput`. The handler trims the line and
  routes it (§2.3).

The handler runs on the network thread (telnet read loop or
websocket read loop). It MUST NOT directly mutate world
state; it enqueues input for the game loop to process
on the next tick.

### 2.3 Input handler routing

When a line arrives:

1. **Prompt mode.** If `InputMode == Prompt`, the line goes
   straight to the registered `PromptHandler` callback. Used
   for confirmation prompts ("are you sure? (y/n)") where
   the next input needs special handling.
2. **Active flow.** If `CurrentFlow != null`:
   - When the flow is cancellable AND the trimmed input
     equals `quit` or `cancel` (case-insensitive), clear the
     flow, send "Cancelled.", and enqueue a `look` for the
     player.
   - Otherwise feed the input to `CurrentFlow.HandleInput`
     and return.
3. **Flood gate.** Call the flood-protection check (§4). If
   it returns false, drop this input.
4. **Enqueue.** Push the input onto the bounded queue
   (silently drop on overflow).

The actual command dispatch happens later, when the game
loop drains the queue. At most 10 inputs per session per
tick are dispatched (a separate game-loop cap; see
`docs/specs/commands-and-dispatch.md`).

### 2.4 Idle tracking

A session tracks the tick of its most recent input:

- `LastInputTick` — updated by the loop on every dequeued
  command.
- `IdleWarned` — flag flipped true after the warn message
  has been sent. Cleared when LastInputTick advances.

The idle handler (§5) reads both.

### 2.5 Prompt state

Three booleans drive the prompt refresh state machine:

- `PromptDisplayed` — the most recent send produced a
  prompt at the bottom of the screen.
- `ReceivedInput` — input has arrived since the prompt was
  displayed.
- `NeedsPromptRefresh` — content has been sent since the
  last prompt; refresh on the next opportunity.

The session manager's send helpers (§3.5) read and write
these flags so output that arrives between a prompt and a
keystroke is rendered with a leading CR-LF to keep the user's
visible line clean.

### 2.6 Auxiliary state

- `CurrentFlow` — the active flow instance (see
  `docs/specs/character-creation.md`). Null when not in a
  flow.
- `PendingPasswordHash` — used by character creation's
  password capture step.
- `CancelPreLoginTimeout` — a callback set by login that
  the spawn path invokes to disable the pre-login idle
  timer after the session reaches Playing.
- `LinkDeadSinceTick` — the tick at which the session
  entered link-dead. Used by the cleanup handler (§7.3).
- `InputMode` — `Normal` or `Prompt` (§2.3).

### 2.7 Send helpers

The session exposes `Send(text)` (raw passthrough) and
`SendLine(text)` (with CR-LF), both of which delegate to the
connection if it is still connected. Output to a disconnected
session is silently dropped (callers don't need to check
liveness).

The session-manager helpers wrap these with prompt-state
management; features SHOULD prefer the manager's
`SendToPlayer` and friends over calling `session.Send`
directly.

**Acceptance criteria**

- [ ] Constructing a session wires its input handler to the
      connection.
- [ ] ReplaceConnection swaps the handler subscription
      atomically (no double-dispatch and no missed input).
- [ ] Input handler routes Prompt mode → CurrentFlow →
      flood gate → input queue in that order.
- [ ] Input queue rejects beyond the cap and does not raise.
- [ ] LastInputTick advances on every dequeued command, and
      IdleWarned clears with it.

---

## 3. SessionManager

### 3.1 Indices

Four concurrent maps:

- **By connection id** (string) — primary index used by the
  game loop's disconnect handler and by network-layer
  callbacks.
- **By entity id** (Guid) — used by combat, abilities,
  movement, and every feature that has an entity reference.
- **By lowercased player name** — used by `tell`, login's
  duplicate-name check, and admin commands.
- **By account id** — used by per-account concurrency
  enforcement and by admin tooling. The value is a list
  (one account may have multiple characters online).

A separate map holds the **pre-login pool**, keyed by
connection id (§3.6).

All maps use concurrent-safe collections; the by-account
list is protected by a per-list lock.

### 3.2 Add and remove

`Add(session)`:

1. Resolve the lowercased name. If a stale entry exists for
   the same name with a *different* connection id, remove
   the stale connection mapping (legacy cleanup).
2. Insert the session into the connection / entity / name
   maps.
3. If account id is non-empty, append the session to the
   account's list under the lock (deduplicated).

`Remove(session)`:

1. Remove from connection / entity / name maps.
2. Remove from the account list under the lock; drop the
   account entry entirely when the list becomes empty.

`RemoveConnectionOnly(session)` removes only the connection
index entry, leaving the entity / name / account indices
intact. Used when transitioning to link-dead — the player
"is" still there, just unreachable by the current connection
id.

`ReRegisterConnectionForSession(session)` re-installs the
connection-id mapping for a session whose connection has
been swapped. Used by link-dead reconnect.

`UpdateEntityId(oldId, session)` updates the by-entity
index when the entity reference was replaced (used by flow
restart).

### 3.3 Queries

The manager exposes:

- `GetByConnectionId`, `GetByEntityId`, `GetByPlayerName`.
- `GetByAccountId` — returns a snapshot list (under lock).
- `ActiveCharacterCount(accountId, excludeEntityId?)` —
  count of currently-bound characters on this account,
  optionally excluding one (used by login's concurrency
  check, see `docs/specs/login.md` §4.3).
- `AllSessions` — every session indexed by connection id.
- `AllLinkDeadSessions` — every session whose phase is
  LinkDead.
- `Count` — total connection-mapped sessions.
- `ConnectionCount` — pre-login + sessions.

Snapshots returned to callers MUST be copies, not views into
internal lists; this lets callers iterate without holding
locks.

### 3.4 Broadcast surface

The manager owns every "send X to Y" operation in the engine
that involves more than one specific session:

- `SendToPlayer(entityId, text)` — addresses one session.
- `SendToAll(text, excludeId?)` — every active session.
- `SendToRoom(roomId, text, excludeEntityId?)` — every
  session whose player is in the room. A second overload
  takes a set of excluded ids.
- `SendToTag(tag, text)` — every session whose player carries
  the tag *as either a tag or a role*. Used by admin and
  channel-style broadcasts (the admin telemetry channel uses
  this).

The room broadcasts iterate the by-entity-id map; they
filter on the player entity's `LocationRoomId`. This is O(N
sessions). Future optimization could use a room-keyed
index.

### 3.5 Prompt-aware send

All broadcasts go through a single internal
`SendContentToSession` that:

1. If the session has a prompt displayed AND has not
   received input since, send a CR-LF first so the new
   content begins on a fresh line instead of running into
   the prompt.
2. Clear `PromptDisplayed` and `ReceivedInput`.
3. Send the rendered text.
4. Set `NeedsPromptRefresh` so the next prompt-flush will
   render.

`FlushPrompts(promptRenderer)`:

For each session in the entity-id map:

- Skip if the session is in `Creating` or `LinkDead`.
- Skip if `InputMode == Prompt`.
- Skip if a flow is active.
- If `NeedsPromptRefresh` is set, render the player's prompt
  template (or a default), send a leading CR-LF and the
  prompt text, and update the flags (`PromptDisplayed =
  true`, `NeedsPromptRefresh = false`).

Called at the end of every tick by the game loop's
notification-drain hook.

### 3.6 Pre-login pool

A separate map holds `LoginContext` entries keyed by
connection id. The login flow (see `docs/specs/login.md`)
inserts on connection accept and removes on a successful
Add (which represents the session being promoted to live)
or on a disconnect mid-login.

`RegisterPreLogin(ctx)`, `RemovePreLogin(connectionId)`,
`GetPreLogin(connectionId)`, `AllPreLoginConnections`,
`AllConnectionsByPhase(phase)` round out the surface.

**Acceptance criteria**

- [ ] Every Add updates all four indices; every Remove
      clears all four.
- [ ] RemoveConnectionOnly leaves entity / name / account
      mappings intact (used by link-dead transition).
- [ ] By-account list is dedup'd within a lock; empty lists
      are removed.
- [ ] All queries returning collections return snapshots.
- [ ] Broadcasts honor the prompt-state machine via the
      shared SendContentToSession path.
- [ ] FlushPrompts skips Creating / LinkDead / Prompt-mode
      sessions and sessions with active flows.

---

## 4. Flood protection

Per-session token-bucket rate limiter applied to input.
Config (`FloodProtection` section) carries four knobs:

- `CommandsPerSecond` (default 15) — token refill rate.
- `BurstSize` (default 30) — bucket capacity.
- `StrikeThreshold` (default 3) — strikes-to-disconnect.
- `StrikeDecaySeconds` (default 10) — strikes reset after
  this much time without offense.

### 4.1 Token bucket

Each session holds:

- `_tokens` (float) — current token count.
- `_lastReplenishTick` — last tick that refilled tokens.
- `_floodStrikes` — current strike count.
- `_lastStrikeTick` — last tick a strike was scored.
- `_floodWarned` — whether the "Slow down." reply was sent
  since the last decay.

The first call after construction initializes `_tokens` to
the burst size.

On each input that reaches the flood gate (§2.3 step 3):

1. **Refill.** Elapsed ticks since last replenish are
   converted to seconds (using the current tick rate);
   tokens grow by `seconds × CommandsPerSecond`, capped at
   `BurstSize`. `_lastReplenishTick` is set to the current
   tick.
2. **Strike decay.** If strikes are positive AND the
   elapsed since `_lastStrikeTick` exceeds the configured
   decay, reset strikes and clear `_floodWarned`.
3. **Token check.** If `_tokens >= 1.0`, decrement and
   return success.
4. **Warn.** Otherwise, if `_floodWarned` is false, send
   "Slow down." and set `_floodWarned = true`.
5. **Strike.** Increment strikes; update `_lastStrikeTick`;
   increment the dropped-input metric (tagged by player
   name).
6. **Disconnect.** If strikes reach the threshold, log a
   warning, increment the disconnect metric, send
   "Disconnected: command flooding.", and disconnect the
   connection with reason `command flooding`.

The token bucket is intentionally permissive: a player who
types quickly drains their burst and then refills at the
configured rate. The strike system protects against
malicious clients that hammer faster than the refill rate
can keep up.

### 4.2 What gets gated

The flood gate runs only for normal command input. It does
NOT run during:

- Prompt-mode input (the user is responding to an explicit
  prompt; a single response is expected).
- Flow input (the flow's HandleInput receives input
  directly).

This means a player in a flow cannot drain their bucket on
flow steps, and a player at a prompt does not lose tokens
on the y/n answer.

### 4.3 Disconnect under flood

When the threshold is reached, the session sets an internal
`_disconnected` flag so subsequent inputs are silently
dropped before another disconnect is issued. The flood gate
returns false for everything from that point until the
disconnect handler tears the session down.

**Acceptance criteria**

- [ ] First input refills tokens to BurstSize.
- [ ] Tokens cap at BurstSize on refill.
- [ ] Strikes decay after `StrikeDecaySeconds` of clean
      input.
- [ ] "Slow down." is sent at most once per decay cycle.
- [ ] Strike threshold disconnects with reason
      `command flooding` and increments the disconnect
      metric.
- [ ] Flood gate is bypassed for Prompt-mode and flow input.

---

## 5. Idle timeouts

### 5.1 Configuration

The `Idle` config carries:

- A **warn-seconds** threshold (send the warning at this
  much idle time).
- A **timeout-seconds** threshold (disconnect at this much
  idle time).
- The warn and timeout message strings.
- An admin-role tag (default `admin`) that exempts holders
  from idle handling.
- Per-login-phase timeout values (used by the login flow;
  see `docs/specs/login.md` §6.1) and a pre-login fallback.

If both thresholds are zero, idle handling is disabled.

### 5.2 Handler

The game loop registers an `idle-timeout` tick handler at
cadence 300 (~30 seconds at 100 ms/tick). On each fire,
iterate every active session:

1. **Dead connection check.** If the connection is no
   longer reported as connected, enqueue a DisconnectEvent
   with reason `dead connection` and continue (this catches
   sessions whose transport-level disconnect failed to
   produce an `OnDisconnected` callback for any reason).
2. **Admin exemption.** If the player carries the admin
   role, skip.
3. Compute idle ticks as `currentTick - LastInputTick`.
4. **Timeout.** If `idleTicks >= timeoutTicks`, enqueue a
   DisconnectEvent with reason `idle timeout`, send the
   timeout message, and disconnect the connection.
5. **Warn.** Else if `idleTicks >= warnTicks` AND
   `IdleWarned == false`, send the warn message and set
   `IdleWarned = true`.

The handler does NOT itself remove the session from the
manager; the DisconnectEvent flows through the loop's
disconnect path, which routes through the OnDisconnect
handler (§6).

### 5.3 Interaction with link-dead

Idle handling iterates `AllSessions`, which is keyed by
connection id. Link-dead sessions have been removed from
that index (§7.2) and are NOT touched by the idle handler.
A link-dead session can sit indefinitely as far as idle is
concerned; the link-dead cleanup handler (§7.3) handles its
expiration on a separate cadence.

**Acceptance criteria**

- [ ] The idle handler runs at a fixed cadence and iterates
      only active (connection-mapped) sessions.
- [x] Admin role exempts a session from both warn and
      timeout.
- [ ] Dead-connection detection enqueues a disconnect with
      the canonical `dead connection` reason.
- [ ] The warn message is sent at most once per idle period.
- [ ] Reaching the timeout enqueues a disconnect AND sends
      the timeout message AND closes the connection.

---

## 6. Disconnect handling

The game loop owns a single `OnDisconnect` callback that
fires for every DisconnectEvent drained from the system-
event queue. The session feature's role is to look up the
session, gate on its state, and route through one of two
paths: link-dead or full teardown.

### 6.1 Stale-event guard

When a session has been **taken over** (§9) the old
connection's eventual disconnect may still arrive as a
DisconnectEvent. The handler MUST compare the event's
session id to the *current* connection id on the session;
if they differ, the event is stale and ignored. Otherwise
a recently-taken-over player would be torn down by the
ghost of their previous connection.

### 6.2 Routing

After the stale guard:

1. Capture the player name and last room id (used for
   logging and "fades from existence" messages).
2. Determine `isIntentionalQuit` from the disconnect
   reason (case-insensitive equality with "Quit").
3. **Link-dead path.** If link-dead is enabled in config
   AND the disconnect is NOT a Quit AND the session is in
   `Playing` phase → §7.
4. **Full teardown.** Otherwise:
   - Save the player asynchronously (errors logged, not
     propagated).
   - Remove the session from the manager.
   - Untrack the entity from the world's account tracker.
   - Decrement the active-connections metric.
   - Remove the entity from its room and notify mob AI
     that the player left.
   - Untrack the entity from the world's tracked-entities
     map.
   - Emit a `player.logout` event.
   - If the disconnect was unintentional AND the player
     had a room, send "&lt;name&gt; fades from existence."
     to the room.
   - Log the disconnect with player name and reason.

The link-dead path explicitly skips Save (the entity stays
in the world; nothing to save yet) and skips room removal
(the player's body remains).

### 6.3 Logout vs link-dead

The two events `player.logout` and `player.linkdead` are
distinct. Both carry the entity id and a reason. Other
features (combat, mob AI, quests) MAY subscribe to either
or both. A clean quit fires only `player.logout`. A dropped
connection fires only `player.linkdead`. The eventual
link-dead expiration fires `player.logout` at that time.

**Acceptance criteria**

- [ ] The stale-event guard short-circuits handlers
      whose session has been taken over.
- [ ] An intentional Quit goes directly to full teardown
      regardless of link-dead config.
- [ ] Save is skipped on the link-dead path and run on
      the full-teardown path.
- [ ] `player.logout` and `player.linkdead` are mutually
      exclusive for a single disconnect event.

---

## 7. Link-dead

### 7.1 Configuration

`LinkDead` config:

- `Enabled` (default true).
- `TimeoutSeconds` (default 120) — how long a link-dead
  session survives before being cleaned up.

When disabled, every disconnect goes straight to full
teardown (§6.2).

### 7.2 Entering link-dead

When the disconnect handler routes to link-dead (§6.2):

1. Set `session.Phase = LinkDead`.
2. Record `session.LinkDeadSinceTick = currentTick`.
3. Add a `linkdead` tag to the player entity (visible to
   room descriptions, who lists, etc.).
4. **`RemoveConnectionOnly(session)`** — drop the
   connection-id index entry only. Entity / name / account
   mappings stay so a reconnect can find the session.
5. Increment the `LinkDeadActive` metric.
6. Send "&lt;name&gt; has lost their connection." to the
   room (excluding the player's own entity).
7. Emit a `player.linkdead` event with the disconnect
   reason.
8. Log at info level.

### 7.3 Cleanup handler

A tick handler `linkdead-cleanup` at cadence 300:

- Iterates `AllLinkDeadSessions`.
- For each session whose `(currentTick - LinkDeadSinceTick)
  ≥ timeoutTicks`:
  - Save the player asynchronously.
  - Run the same teardown as the full-teardown disconnect
    path: remove from manager, untrack from world,
    untrack the account, decrement the active-connections
    and link-dead-active metrics.
  - Remove from the room and notify mob AI.
  - Emit `player.logout` with reason `linkdead timeout`.
  - Log at info level.

The cleanup handler is only registered when link-dead is
enabled.

### 7.4 Reconnect

A returning player who logs in to a character with a
link-dead session triggers **link-dead reconnect** from the
login flow (see `docs/specs/login.md` §4.5):

1. Call `session.ReplaceConnection(newConnection)` —
   atomically detach the old input handler, attach the new
   one.
2. Call `sessions.ReRegisterConnectionForSession(session)`
   to re-install the connection-id index entry with the
   new id.
3. Wire a new `OnDisconnectedWithReason` callback to
   enqueue future DisconnectEvents.
4. Set `session.Phase = Playing`.
5. Remove the `linkdead` tag from the entity.
6. Reset `LastInputTick` to the current tick (so the idle
   timer starts fresh).
7. Clear the input queue (any commands queued at the
   moment of the disconnect are NOT replayed).
8. Remove the new connection's pre-login entry from the
   manager.
9. Decrement `LinkDeadActive`; increment
   `LinkDeadReconnected`.
10. Send "Reconnected." to the player.
11. Enqueue a `look` so the player sees their surroundings.

The old connection (if it still exists) was already closed
when its disconnect handler ran; no further action against
it is required.

**Acceptance criteria**

- [ ] Entering link-dead removes only the connection
      mapping, not entity / name / account mappings.
- [ ] The `linkdead` tag is set on entry and cleared on
      reconnect.
- [ ] The cleanup handler does not run when link-dead is
      disabled.
- [ ] Reconnect resets idle and clears the input queue.
- [ ] LinkDeadActive metric tracks the count: +1 on enter,
      −1 on reconnect, −1 on cleanup.

---

## 8. Session takeover

When login authenticates against an account that already
has an active session (not link-dead) for the same
character, the login flow asks the user to confirm
takeover and, on yes, calls `TakeOverSession(existing,
newConnection, preLoginContext)`. The session feature's
side of takeover:

1. Send "Another connection has taken over this character."
   to the existing session.
2. `sessions.Remove(existing)` — clear every index.
3. Disconnect the existing connection with reason
   `session takeover`.
4. Decrement `ActiveConnections` metric.
5. Run the post-login spawn path with the new connection
   on the same entity and account id (this re-adds the
   session under the new connection id).

The old connection's `OnDisconnect` will fire shortly
afterward, producing a stale DisconnectEvent. The §6.1
stale-event guard prevents it from tearing down the
already-promoted new session.

**Acceptance criteria**

- [ ] The existing session is fully removed before the
      new session is added.
- [ ] The disconnect reason on takeover is
      `session takeover`.
- [ ] The stale-event guard short-circuits the eventual
      DisconnectEvent for the old connection.

---

## 9. Async connection adapter

The login flow runs as an async sequence of `ReadLineAsync`
calls against a `IConnection`. The adapter bridges the
connection's callback-style input to a TaskCompletionSource:

- One pending `TaskCompletionSource<string>` at a time per
  adapter, guarded by an internal lock.
- `ReadLineAsync(ct)` registers a TCS, stores it, registers
  the cancellation token to clear and cancel the TCS, and
  returns the task.
- `OnInput` from the connection completes the pending TCS
  with the input string.
- `OnDisconnected` cancels the pending TCS.
- `Dispose` unsubscribes and cancels any pending TCS.

Only the login flow uses this adapter. After login completes,
the bound PlayerSession owns input directly via its
`OnInput` subscription; the adapter is disposed.

**Acceptance criteria**

- [ ] At most one pending read per adapter.
- [ ] Disconnect or cancellation cancels the pending read
      exactly once.
- [ ] Dispose is idempotent.

---

## 10. Notification queue

A separate concurrent service per entity, used to deliver
small queued messages at well-defined points in the tick.

### 10.1 The record

`Notification(Type, Priority, Text, GmcpPackage?,
GmcpPayload?)`. `Type` is a content-defined keyword (e.g.
`ability_unlock`). `Priority` is an integer (lower
arrives first when ordered).

### 10.2 Operations

- `Enqueue(entityId, notification)` — append to the per-
  entity queue.
- `DrainFor(entityId)` — drain the queue, sort by priority
  ascending (stable within equal priority), return the
  list.

Drain happens at the loop's notification-drain hook (today
fired at the end of every tick). The hook iterates active
sessions and renders each session's notifications via the
session's send helpers.

### 10.3 GMCP coupling

A notification may carry a GMCP package name and payload.
Clients that have GMCP active see both the text and the
structured data; clients without GMCP see only the text.
The text is the canonical user-facing message.

**Acceptance criteria**

- [ ] Queue is per-entity and concurrent-safe.
- [ ] Drain returns notifications in priority order (stable
      within equal priority).
- [ ] Drain leaves the queue empty.

---

## 11. Return address

A tiny helper service backed by a single `return_room`
property on the player entity:

- `SetReturn(player, roomId)` — store the room id and emit
  a `return.set` event.
- `GetReturn(player)` — read.
- `ClearReturn(player)` — null out.
- `HasReturn(player)` — typed presence check.

Used by teleport-like commands ("recall" / "return") that
want to remember where the player was before the teleport.
Persistence and broadcast are not the service's concern;
the property is marked transient (see `docs/specs/world-
rooms-movement.md` §2.2 / `RoomProperties`) so it does not
survive a server restart.

**Acceptance criteria**

- [ ] Set / Get / Clear / Has operate on the same property
      key.
- [ ] Set emits `return.set` with the entity id and room id.

---

## 12. Observable events

| Event | When |
|---|---|
| (system event) ConnectEvent | a new session connected with a spawn room |
| (system event) DisconnectEvent | a connection dropped or was closed |
| player.login | a session completed login and is Playing (§6 routing context) |
| player.linkdead | a Playing session entered link-dead (§7.2) |
| player.logout | a session terminated (clean quit, idle timeout, link-dead expiry) (§6.2, §7.3) |
| entity.rest_state.changed | combat-driven wake at engage (§10 of `docs/specs/economy-survival.md`) |
| return.set | a return room was recorded (§11) |

The session feature itself emits `player.linkdead`,
`player.logout`, and `return.set`. ConnectEvent /
DisconnectEvent are system-event-queue entries, not bus
events; they're the input plumbing for the disconnect
handler. `player.login` is fired by the spawn path that the
session feature drives but lives in the login spec.

**Acceptance criteria**

- [ ] Every transition in §6 - §11 emits exactly the
      listed event with the documented payload.
- [ ] DisconnectEvents are drained from the system-event
      queue at the top of every tick.

---

## 13. Configuration surface

The following are externally configurable and not fixed by
this spec.

| Policy | Where it applies |
|---|---|
| Idle warn / timeout thresholds and messages | §5.1 |
| Per-phase pre-login timeouts | §5.1 (see login spec) |
| Admin role / tag exempting from idle | §5.1 |
| Flood: commands-per-second, burst, strike threshold, strike decay | §4 |
| Link-dead enabled flag and timeout seconds | §7.1 |
| Per-session max input queue depth (today 100) | §2.2 |
| Per-tick command drain cap per session (today 10) | §2.3 |
| Linkdead tag name (today `linkdead`) | §7.2 |

---

## 14. Open questions / future work

- **Input queue cap is silent.** When the queue overflows,
  the user's input is dropped without any indication. A
  "you're typing too fast" notification (or a separate
  metric counter) would help diagnostics.
- **No per-room broadcast index.** SendToRoom iterates
  every session and filters on the player's room id. On
  servers with many sessions and many rooms, an inverse
  index would let the broadcast scope to room occupants.
- **Stale-event guard depends on string id parsing.** The
  comparison parses the connection's string id into a Guid
  every time. Caching the Guid on the session would avoid
  the parse cost in the hot path.
- **Link-dead is per-character, not per-account.** A player
  with two characters on one account who drops on one
  cannot reconnect the other to it. Multi-character
  link-dead semantics are out of scope.
- **Notification flush has no rate limiting.** A pathological
  feature emitting many notifications per tick floods the
  player without any throttle. A per-session per-tick cap
  would defend against the worst case.
- **Idle handler is O(N) every cadence.** Each fire walks
  every session. Replacing this with a priority queue
  keyed by next-warn / next-timeout tick would scale to
  thousands of sessions.
- **Pre-login pool grows unbounded.** A network-level abuse
  that accepts but never sends bytes accumulates pre-login
  entries until idle reaps them via the phase timeout.
  The network layer's per-connection budgets help, but a
  cap on pre-login pool size would be defense in depth.
- **InputMode is bistable.** Prompt mode is a single
  pending callback; nested prompts (a flow within a
  prompt) aren't supported. Today this doesn't matter
  because flows and prompts are mutually exclusive at
  call sites, but the constraint is implicit.
- **AsyncConnectionAdapter is single-use.** It pairs with
  one pending read; deeper login flows that want to
  multiplex reads (concurrent OAuth, side-channel
  confirmation) need a richer adapter.
- **PromptDisplayed state is per-session, not per-write.**
  Two writes from different features arriving on the same
  tick race on the flag and may produce one inserted
  CR-LF instead of the cleaner two-CR-LF behavior. Output
  composition is currently best-effort.

---

<!-- Generated: 2026-05-21 · Scope: PlayerSession + SessionManager + FloodContext + AsyncConnectionAdapter + NotificationQueue + ReturnAddressService + GameLoop idle/disconnect/linkdead handlers + GameLoopService disconnect routing + PlayerSpawner takeover/reconnect · Spec style: narrative + acceptance criteria · Detail level: behavior only -->
