# Tag-Change Notifications — Feature Specification

**Status:** Draft · **Scope:** A reactive notification fired when an
entity gains or loses a tag, so systems other than the tag index can
respond to tag changes; the idempotency and payload contract; the timing
relationship to the double-buffered tag index · **Audience:** Anyone
reimplementing or porting this feature in any language.

This document describes *what* tag-change notification must do, not *how*
to implement it. Event names and defaults live in the
configuration-surface table at §6.

This is **substrate ahead of a consumer.** The prior incarnation ships a
per-entity observer interface (`ITagObserver` with `OnTagAdded` /
`OnTagRemoved`), but its only observer is the world's own tag index. This
spec captures the reaction contract so that when a real consumer appears
(a quest objective tracking a status tag, AI reacting to `feared`, an
effect keyed on a tag), the behavior is already defined — and it adapts
the pattern to this engine's bus instead of porting a per-entity callback
list (§3). If no consumer materializes, this stays a one-paragraph
substrate; it should not grow speculative machinery (the
"runtime in search of a use case" trap).

---

## 1. Overview

When an entity's tag set changes — a tag is added that was not present,
or removed that was present — the change is announced so interested
systems can react. That is the whole feature: *tag changed → reaction
opportunity*.

- The tag index that powers `get-by-tag` queries
  (`world-rooms-movement.md` §4) does **not** depend on this surface. The
  index is maintained directly by the entity store's tracking operations
  and its tick-boundary buffer swap. Tag-change notification is for
  *other* reactions, not for keeping the index current.
- Notifications are a decoupling seam: the code that mutates a tag does not
  know or care who reacts. Reactors subscribe; the mutating operation
  publishes — from a bus-holding caller layer, **not** from inside the
  entity store (§3 explains why the publish cannot live at the mutation
  method itself).

### 1.1 What this is *not*

- Not the tag index. The index is independent (§1). A consumer that just
  wants "all entities with tag X" queries the index; it does not subscribe
  here.
- Not a gate. Tag changes are facts, announced after they happen. A
  subscriber cannot veto a tag change by handling its notification (a
  *cancellable* tag-change would be a different feature with a different
  contract — §7).
- Not per-tag filtering machinery. The notification carries the entity and
  the tag; a subscriber that cares about one tag checks the payload. The
  engine does not maintain per-tag subscriber lists unless a consumer's
  volume proves it necessary.

---

## 2. The notification

- A tag **addition** that actually changes the set (the tag was absent)
  fires an added-notification. A **removal** that actually changes the set
  (the tag was present) fires a removed-notification. Adding a
  already-present tag or removing an absent one fires **nothing** — the
  notification follows the real state change, idempotently. This mirrors
  the reference's `if (set.Add(tag))` guard.
- The payload carries the **entity identity** and the **tag** (normalized
  the same way tags are normalized elsewhere). It carries enough to let a
  subscriber resolve the entity and decide relevance; it does not carry a
  snapshot of the whole tag set.
- **Single-tag** add/remove carry the real state change — the mutator's
  `if set.Add(tag)` guard already reports whether the set changed — so their
  notifications are exact. **Bulk re-tag is harder:** the store's
  `Retag`-style path rebuilds an entity's buckets **wholesale** and keeps
  **no snapshot of the prior set**, so it cannot emit a per-tag diff for
  free. Emitting "observably the diff, not a blanket 'something changed'"
  for a bulk re-tag requires new machinery (snapshot the old set before the
  rewrite, or have the bulk mutators — `SetAlignmentTag`, racial-flag
  application — return what changed). Which of those, and whether bulk
  re-tag emits the exact diff or a coarser signal, is a **build-time design
  task** (§7), not a free consequence of the existing path.

**Acceptance — the notification**

- [ ] Adding an absent tag fires an added-notification; removing a present
      tag fires a removed-notification.
- [ ] Adding a present tag or removing an absent tag fires nothing.
- [ ] The payload carries the entity identity and the changed tag.
- [ ] Single-tag add/remove fire exact add/remove notifications (the change
      guard already exists on the mutator).
- [ ] Bulk re-tag's notification granularity (exact diff vs. a coarser
      signal) is resolved as a build-time design task — the existing
      `Retag` holds no prior-set snapshot to diff against (§7).

---

## 3. Mechanism: bus, not per-entity callbacks

- This engine already has a typed event bus for decoupled reactions
  (mob death, player movement, ability use). Tag-change notification rides
  it: a tag change **publishes a bus event** (`entity.tag_added` /
  `entity.tag_removed`), and reactors subscribe through the normal bus
  surface.
- This is a deliberate adaptation of the reference's per-entity observer
  list. A per-entity callback list couples reactors to entity lifetime
  (register on spawn, unregister on despawn) and scatters subscription
  across the codebase; the bus centralizes it and matches every other
  reactive seam in this engine.
- **Where the publish lives — the import-cycle constraint (load-bearing).**
  Tags mutate inside `internal/entities`, but `eventbus` **imports**
  `entities`, so `entities` **cannot** import `eventbus` — the store
  documents this exact cycle. The publish therefore is **not** emitted at
  the mutation method itself. It is emitted by the **caller layer that
  already holds the bus** (the layers that call the mutators today — the
  command layer, the decay/scrap path, the progression/alignment path), or
  via a small **injected publisher interface** handed to the store (the
  `srckey`-style leaf precedent for breaking the cycle). This mirrors how
  spawning emits its "spawned" event from the spawn caller, not from inside
  the entity constructor. **There is no single mutation chokepoint:**
  `Retag` has several callers and the alignment path does not route through
  it at all today, so the publish points must be **enumerated per caller**
  when this is built — the main reason this spec is not yet a drop-in build.
- Because it rides the bus, the bus's dispatch guarantees apply —
  publication is synchronous to the publishing call, and a slow subscriber
  is the subscriber's problem, not the mutator's. **Re-entrancy is the
  publisher's/handler's responsibility, not the bus's:** the bus permits a
  handler to publish again, so a tag-add handler that itself adds a tag will
  recurse with **no engine-enforced limit** (the repo convention: re-entrant
  dispatch is used deliberately but must be guarded against unbounded
  loops). A tag-mutating reactor must guard its own recursion.

**Acceptance — mechanism**

- [ ] A tag change publishes the corresponding bus event; a bus subscriber
      receives it.
- [ ] No per-entity registration/unregistration is required to receive
      tag-change events.
- [ ] The publish is emitted from a bus-holding caller layer (or an injected
      publisher), **never** from inside `internal/entities` (the import-cycle
      constraint); every mutation caller that must notify is enumerated.

---

## 4. Timing versus the index

This is the subtle part. The tag index is **double-buffered**: a tag
added now becomes visible to `get-by-tag` readers only after the next
tick-boundary buffer swap (`world-rooms-movement.md` §3.4). The
notification, however, fires **immediately** at mutation time.

- Therefore a subscriber reacting to `entity.tag_added` and immediately
  querying `get-by-tag` for that same tag **may not see the entity that
  triggered the event** until the next swap. This is not a bug; it is the
  read/write-buffer contract surfacing.
- A subscriber that needs the post-change truth about a single entity
  should consult the **entity directly** (`has-tag`), which reflects the
  change immediately, rather than the index, which reflects it at the
  swap. The notification payload carries the entity precisely so the
  subscriber can do this.
- Conversely, a subscriber that wants the settled set of all tagged
  entities should react at or after the swap, not on the raw event.

**Acceptance — timing**

- [ ] `has-tag` on the notified entity reflects the change immediately
      inside the notification handler.
- [ ] `get-by-tag` may not yet reflect the change inside the handler; it
      does after the next index swap.

---

## 5. Persistence

Nothing here persists. Notifications are runtime events; subscriptions are
process-lifetime wiring. Tag *state* persists wherever tags persist
(racial flags, alignment, etc., per the owning specs); this feature adds
no save surface.

**Acceptance — persistence**

- [ ] No notification or subscription state is written to disk.

---

## 6. Configuration surface

| Setting | Description |
|---|---|
| Tag-change event names | The bus events published on add/remove (`entity.tag_added` / `entity.tag_removed`) (§3). |
| Payload shape | The entity identity + tag fields the events carry (§2). |

---

## 7. Open questions

- **Cancellable tag changes.** A *pre*-change, vetoable hook ("refuse to
  apply `silenced`") is a distinct feature with a cancellable-event
  contract. Not in scope here (this surface is post-fact). Add only if a
  concrete gate need appears.
- **Per-tag subscription.** If a high-volume consumer makes "every
  subscriber inspects every tag event" costly, a per-tag subscription
  index could be added. Deferred until volume proves it — premature
  otherwise.
- **Bulk re-tag diff granularity (build-time design task).** Single-tag
  add/remove notify exactly (§2), but the store's `Retag` rebuilds buckets
  with no prior-set snapshot, so a bulk re-tag cannot emit a per-tag diff
  for free. Decide, when built, whether to snapshot-and-diff in `Retag`,
  make the bulk mutators (`SetAlignmentTag`, racial-flag application) return
  what changed, or accept a coarser "re-tagged" signal for bulk paths.
- **Generic vs. entity-specific events.** Alignment bucket changes already
  emit a dedicated `alignment.bucket.changed` event, and `SetAlignmentTag`
  does not re-index through `Retag` today. If the alignment path also
  published generic `entity.tag_*` events, a subscriber could **double-fire**.
  The relationship between the generic tag events and existing
  entity-specific events must be settled so a mutation is announced once.
- **Mob vs player scope.** Whether tag-change events fire for all tracked
  entities (mobs **and items**, not only players) or a narrower set. The
  store re-tags items too (the decay and set-command paths drive `Retag` on
  item instances), so "all tracked entities" is the conservative default;
  the payload lets a subscriber filter cheaply.
- **Consumer-driven shape.** The exact payload and whether old/new
  adjacency matters (e.g. "replaced tag A with B") should be confirmed
  against the first real consumer rather than guessed now.

---

## Cross-references

- `world-rooms-movement.md` §3.4 — the tag index and its double-buffer
  swap, whose timing §4 here depends on.
- `eventbus` (the typed bus the notifications ride) — see the cancellable-
  events and registry tables in this README.
- `mobs-ai-spawning.md`, `progression.md` — owners of the persisted tags
  (racial flags, alignment) whose mutations would publish these events.
