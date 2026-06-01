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
- Notifications are a decoupling seam: the entity (or the store) that
  mutates a tag does not know or care who reacts. Reactors subscribe; the
  mutator publishes.

### 1.1 What this is *not*

- Not the tag index. The index is independent (§1). A consumer that just
  wants "all entities with tag X" queries the index; it does not subscribe
  here.
- Not a gate. Tag changes are facts, announced after they happen. A
  subscriber cannot veto a tag change by handling its notification (a
  *cancellable* tag-change would be a different feature with a different
  contract — §6).
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
- In-place tag mutations that re-tag an entity (the store's `Retag`-style
  path) fire the corresponding add/remove notifications for the tags that
  actually changed, so a re-tag is observably the diff, not a blanket
  "something changed".

**Acceptance — the notification**

- [ ] Adding an absent tag fires an added-notification; removing a present
      tag fires a removed-notification.
- [ ] Adding a present tag or removing an absent tag fires nothing.
- [ ] The payload carries the entity identity and the changed tag.
- [ ] A re-tag fires add/remove notifications for exactly the tags that
      changed.

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
- Because it is the bus, the existing bus guarantees apply: publication is
  synchronous to the mutation, re-entrancy is bounded by the bus's own
  rules, and a slow subscriber is the subscriber's problem, not the
  mutator's.

**Acceptance — mechanism**

- [ ] A tag change publishes the corresponding bus event; a bus subscriber
      receives it.
- [ ] No per-entity registration/unregistration is required to receive
      tag-change events.

---

## 4. Timing versus the index

This is the subtle part. The tag index is **double-buffered**: a tag
added now becomes visible to `get-by-tag` readers only after the next
tick-boundary buffer swap (`world-rooms-movement.md` §4). The
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
- **Mob vs player scope.** Whether tag-change events fire for all tracked
  entities (mobs included) or only players. The reference fires for any
  entity; the conservative default here is all tracked entities, since the
  payload lets a subscriber filter cheaply.
- **Consumer-driven shape.** The exact payload and whether old/new
  adjacency matters (e.g. "replaced tag A with B") should be confirmed
  against the first real consumer rather than guessed now.

---

## Cross-references

- `world-rooms-movement.md` §4 — the tag index and its double-buffer
  swap, whose timing §4 here depends on.
- `eventbus` (the typed bus the notifications ride) — see the cancellable-
  events and registry tables in this README.
- `mobs-ai-spawning.md`, `progression.md` — owners of the persisted tags
  (racial flags, alignment) whose mutations would publish these events.
