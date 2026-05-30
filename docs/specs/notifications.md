# Notifications — Feature Specification

**Status:** Draft · **Scope:** The per-entity priority queue that
delivers asynchronous messages (tells, channel posts, system
notices, future shop receipts, quest updates, etc.) to players;
online vs. offline routing; bounded growth; the cross-restart
persistence contract for messages addressed to disconnected
players · **Audience:** Anyone reimplementing or porting this
feature in any language.

This document describes *what* the notification substrate must
do, not *how* to implement it. Specific queue caps, eviction
counts, priority labels, and persistence cadences are policy and
live in the configuration-surface table at the bottom.

This spec is the substrate for the social-MUD theme (see
`docs/THEME-AXIS-PLAN.md` Theme A and `docs/themes/social-mud-plan.md`).
Tells, channels, emotes, and any future asynchronous message all
publish through the surface described here.

---

## 1. Overview

A notification is a small, structured message addressed to one or
more entities. It is *not* a chat-domain concept — it is the
delivery primitive that channels and tells use, and the same
primitive future systems (quest grants, shop confirmations,
mail) will reuse.

Three behaviors distinguish notifications from generic room
output:

1. **Addressed.** A notification has one or more recipient entity
   ids, not a room id. It is delivered to those entities
   regardless of which room they are in.
2. **Survives logout.** If a recipient is offline, the
   notification is held until they reconnect (subject to caps
   and TTLs in §6).
3. **Prioritized.** Higher-priority notifications drain before
   lower-priority ones when a session reconnects with a backlog.

Notifications are **fire-and-forget**: once published, they
cannot be cancelled or recalled. There is no censorship hook,
no "unsend tell" surface, no editorial review. Cancellation is
explicitly out of scope for v1.

### 1.1 What notifications are not

- Not room-scoped output (`Hello!` said in a room → that's the
  existing per-room broadcast path, unchanged).
- Not a transport. Notifications produce text (or future
  structured payloads) that the existing session writer
  delivers; they do not own the bytes-on-wire.
- Not events on the cancellable event bus. Notifications cannot
  be cancelled, vetoed, or rewritten by subscribers.
- Not a transcript or audit log. Channel scrollback (see the
  channels spec) is a separate persistence surface; the
  notification queue holds *undelivered* messages, not delivered
  history.

---

## 2. The notification

A notification has the following observable shape:

- **id** — opaque, unique per-process. Used for de-duplication
  if the same notification is offered to multiple deliveries
  for the same recipient (e.g., a redelivery on reconnect).
- **recipients** — one or more entity ids. The substrate fans
  out to each.
- **priority** — one of the priority levels defined in §3.
- **kind** — a short stable string identifying the category
  (`tell`, `channel`, `system`, …). Used for filtering on
  delivery and for future GMCP routing.
- **text** — the rendered message body. Substrate does not own
  rendering; the publisher delivers the final string.
- **published_at** — server time the notification entered the
  queue. Used for ordering within a priority tier and for
  TTL expiry.
- **sender** — optional. Identifies the publishing entity (for
  tells: the speaking player). May be absent for system
  notifications.

Notifications are immutable after publish. The substrate never
rewrites text, recipients, or priority.

### Acceptance — notification shape

- [ ] Every notification carries id, recipients, priority, kind,
      text, published_at; sender is optional.
- [ ] No publish path can mutate a notification after it enters
      the queue.
- [ ] Same notification id offered twice to the same recipient is
      de-duplicated (delivered once).

---

## 3. Priority

Priority is a small ordered enumeration. The exact tier names
are policy (see configuration surface), but the substrate
guarantees:

- A strictly higher-priority notification drains before any
  strictly lower-priority notification.
- Within the same priority tier, notifications drain in
  published_at order (FIFO).
- Priority is set at publish time and never changes.

A minimal viable tier set (subject to confirmation in
configuration):

| Tier | Examples | Drain order |
|---|---|---|
| `system` | Maintenance notices, admin broadcasts, error replies | First |
| `tell` | Direct one-to-one chat | Second |
| `channel` | Multi-recipient chat | Third |

Future tiers (`quest`, `combat-summary`, …) may be added without
changing the substrate; they just slot into the ordering.

### Acceptance — priority

- [ ] Higher-priority notifications drain before lower-priority
      ones regardless of arrival order.
- [ ] Within a priority tier, drain order matches
      `published_at` (FIFO).
- [ ] Adding a new tier does not require changing already-
      published notifications.

---

## 4. Publish

The publish surface accepts a notification and a list of
recipient entity ids. The substrate:

1. Assigns an id if the caller did not supply one.
2. Stamps `published_at` from the engine `Clock` (never wall
   time — see `time-and-clock` spec).
3. For each recipient:
   - If the recipient has a live online session, attempts
     **immediate delivery** (§5).
   - Otherwise, **enqueues** the notification on the
     recipient's persisted queue (§6).
4. Returns success once all recipients have been routed
   (delivered or enqueued). Failure to deliver to one recipient
   does not abort the others; the substrate logs per-recipient
   failures with structured fields (`event=notify.publish.failed`,
   `recipient`, `kind`, `err`).

Publish is **idempotent on id**: republishing a notification
with an already-published id is a no-op for any recipient who
has already seen it.

### Acceptance — publish

- [ ] Publish stamps `published_at` from the engine `Clock`,
      never from wall time.
- [ ] Failure to deliver to one recipient never blocks delivery
      to siblings.
- [ ] Publishing the same id twice never delivers twice to the
      same recipient.
- [ ] Publish succeeds whether the recipient is online or
      offline; the caller does not need to know which.

---

## 5. Immediate delivery (online recipient)

When the recipient has a live session, the notification is
handed to the session writer in priority order against any
backlog the session already holds. The substrate guarantees:

- If the session has no backlog, the notification is written
  immediately (no tick delay).
- If the session has a backlog (because it just reconnected),
  the notification is inserted into the backlog at its priority
  position, not appended at the end. (The drain in §7 then
  delivers it in correct order.)
- Delivery to the session writer cannot fail in a way that
  blocks the publisher. Writer-side failures (link-dead, full
  buffer) cause the notification to be re-enqueued at its
  original priority and `published_at`, as if the recipient had
  been offline at publish time.

### Acceptance — immediate delivery

- [ ] An online recipient with no backlog receives a notification
      in the same tick it was published.
- [ ] An online recipient with a backlog has the new
      notification inserted in priority order, not appended.
- [ ] If immediate delivery fails (writer error, link-dead), the
      notification is enqueued with its original
      `published_at` and priority.

---

## 6. Enqueue (offline recipient)

When the recipient is offline (or immediate delivery failed),
the notification is appended to the recipient's persisted
queue.

The queue is **per-entity** (not per-session), so it survives
logout/login and link-dead/reconnect cycles. Today the only
entities with queues are players; the substrate does not
foreclose mob/object queues in the future.

### 6.1 Bounded growth

Each per-entity queue has a configurable cap. When publishing
would push the queue over the cap:

- The substrate **drops the oldest** notification at the
  **lowest priority tier present**, freeing one slot.
- The drop is logged with structured fields
  (`event=notify.queue.evicted`, `recipient`, `kind`,
  `published_at`, `priority`).
- If every notification in the queue is at the highest priority
  (i.e., no lower tier exists to drop), the new publish is
  refused and the publisher is told (so it can decide whether
  to retry or surface a "mailbox full" message).

### 6.2 TTL

Notifications older than a configurable per-tier TTL are
discarded on next queue inspection (publish, drain, or a
periodic sweep — see configuration surface). Discarded
notifications are logged the same way as evictions.

### 6.3 Persistence

Per-entity queues persist alongside player data (see
`persistence` spec). Specifically:

- Queues live under the per-player save directory in a
  dedicated file (separate from `player.yaml`) so the player
  schema bump cadence is decoupled from the notification
  schema.
- Writes follow the same tmp→bak→rename rotation used by
  `internal/persistence`.
- The file is written when the queue mutates and on the
  autosave cadence (whichever is sooner), not on every publish
  — batching by tick is acceptable as long as a crash
  loses at most one tick of queued notifications.
- An empty queue may either be an empty file or no file at
  all; the loader treats both as "no backlog".

### Acceptance — enqueue

- [ ] Offline recipients accumulate notifications in a per-
      entity queue that survives process restart.
- [ ] A queue at cap evicts the oldest lowest-priority
      notification (not the oldest overall) and logs the
      eviction.
- [ ] If every entry is at the highest tier, a publish over
      cap is refused and the refusal is observable to the
      publisher.
- [ ] Notifications past their TTL are discarded on next
      inspection.
- [ ] Queue persistence uses the same atomic-write rotation as
      other persisted state.
- [ ] Crash mid-batch loses at most one tick's worth of
      enqueues, not the whole queue.

---

## 7. Drain (recipient reconnects)

When a recipient session enters the active phase (after login,
or after link-dead → reconnect):

1. The substrate loads the recipient's persisted queue.
2. It walks notifications in **priority order**, then
   `published_at` order within a tier, and writes each to
   the session writer.
3. As each notification is successfully written, it is removed
   from the persisted queue (atomic with the next persisted
   flush — see §6.3).
4. Newly-arriving notifications during the drain interleave
   into the priority-ordered stream (see §5).
5. The drain rate is bounded so a large backlog does not
   freeze the session writer (see configuration surface).
   Specifically: at most N notifications per tick, configurable
   per priority tier.

### 7.1 Cosmetic framing

The substrate does not own framing prose ("--- Messages while
you were away ---" or similar). That is a presentation concern
owned by the channels/tells layer. The substrate guarantees
priority-correct ordering; the consumer renders.

### Acceptance — drain

- [ ] A reconnecting player with a backlog sees notifications
      in priority order, FIFO within tier.
- [ ] A drained notification is removed from the persisted
      queue and does not reappear on subsequent reconnect.
- [ ] Drain rate is bounded by configuration so a huge backlog
      cannot stall the session writer.
- [ ] Notifications published *during* a drain interleave by
      priority; they do not all queue behind the backlog.

---

## 8. Observability

Every observable transition emits a structured log line at
`info` or finer, using the field names below:

| Field | Meaning |
|---|---|
| `event` | One of `notify.publish.ok`, `notify.publish.failed`, `notify.deliver.immediate`, `notify.enqueued`, `notify.queue.evicted`, `notify.ttl.expired`, `notify.drained`, `notify.refused.cap` |
| `id` | Notification id |
| `recipient` | Recipient entity id |
| `kind` | Notification kind string |
| `priority` | Priority tier |
| `sender` | Publishing entity id (if any) |
| `queue_size` | Recipient queue size after the transition |
| `tick` | Engine tick at the transition |

Drops, refusals, and failures log at `warn`; routine deliveries
at `debug` (so a live MUD does not flood its logs).

### Acceptance — observability

- [ ] Every state transition (publish, deliver, enqueue, evict,
      expire, drain, refuse) emits exactly one log line with
      the fields above.
- [ ] Routine deliveries log at `debug`; failures and drops
      log at `warn`.

---

## 9. Concurrency

- The publish surface is safe to call from any goroutine.
- A single recipient's queue is serialized internally; concurrent
  publishes to the same recipient cannot interleave a partial
  write.
- Drains and publishes to the same recipient never deadlock
  with the session writer: the substrate holds the queue lock
  only long enough to mutate the queue, never while writing
  to the session.
- The substrate never holds the queue lock while calling out to
  publisher-supplied callbacks. (None exist in v1; this is a
  forward-compatibility statement.)

### Acceptance — concurrency

- [ ] Race detector clean under concurrent publish/drain
      stress across multiple recipients.
- [ ] No deadlock between queue lock and session writer.
- [ ] Two simultaneous publishes to the same recipient produce
      two ordered notifications with distinct ids — neither is
      lost.

---

## 10. Configuration surface

Everything externally policy-driven lives here. The substrate
must read these from configuration; it must not hardcode them.

| Setting | Default (suggested) | Meaning |
|---|---|---|
| Priority tier names | `system, tell, channel` | The enumeration. Adding a tier appends; removing a tier is a content-breaking change. |
| Per-entity queue cap | (open — see §11) | Max notifications held per recipient |
| Per-tier TTL | (open — see §11) | Max wall-time-equivalent age before discard |
| Drain rate per tick | (open — see §11) | Max notifications to deliver to a single session per engine tick |
| Persistence cadence | autosave cadence | When to flush mutated queues to disk |
| Queue file name | `notifications.yaml` (within per-player save dir) | On-disk filename, parallel to `player.yaml` |
| TTL sweep cadence | (open — see §11) | How often to walk all queues to discard expired entries (or "on next inspection only") |

---

## 11. Open questions

- **Default queue cap.** 100? 500? A function of priority tier
  (more system-tier headroom)? Pin before M13.5 (tells impl).
- **Default TTLs.** Tells expire? Channel notifications expire
  faster than tells? Or no TTL in v1 (rely solely on cap)?
- **Drain rate.** Bound per session, per tick, per tier? A
  single number across all tiers risks starving low-priority;
  per-tier means three numbers and more knobs. Lean: one
  number, the cap is reached so rarely it doesn't matter.
- **Periodic sweep vs. lazy expiry.** Lazy expiry (TTL checked
  only on publish/drain) is cheap but a player who never
  reconnects accumulates expired ghosts on disk. A periodic
  sweep is correct but adds a tick handler. Default: lazy in
  v1, add sweep if it becomes a real problem.
- **Cross-restart immediate-delivery.** A player who is online
  at publish time, link-deads before the writer flushes, and
  reconnects after a server restart — does their notification
  survive? Spec says yes (because writer failure re-enqueues
  per §5). Confirm the implementation actually does this.
- **Future cancellation seam.** Out of v1 scope explicitly. If
  added later (admin recall, content takedown), the cancellable
  event bus is the right surface — not a hole in the publish
  API.
- **Multi-recipient publish atomicity.** If publish to recipient
  A succeeds and to recipient B fails, the notification was
  partially delivered. Acceptable for v1 (tells/channels
  re-publish on retry). Confirm that no caller needs all-or-
  nothing semantics.
- **Mob/NPC queues.** Spec says queues are per-entity, not
  per-player, to keep the door open. No mob actually consumes
  notifications today. Confirm we want to keep the door open
  (vs. simplifying to player-only and burning the bridge until
  there's a real consumer).

---

## Cross-references

- `time-and-clock` — engine `Clock` used for `published_at`
  stamps.
- `persistence` — atomic-write rotation, per-player save dir
  layout.
- `session-lifecycle` — when a session enters the active phase
  (drain trigger), and link-dead/reconnect transitions.
- `commands-and-dispatch` — the eventual tell/channel verbs
  that publish through this substrate.
- `docs/themes/social-mud-plan.md` — Theme A live plan.
- `docs/specs/README.md` — substrate-layer placement, indexes
  to update when this spec lands.
