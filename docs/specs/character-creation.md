# Character Creation — Feature Specification

**Status:** Draft · **Scope:** Creating phase entry (from login) → Playing
handoff with a persisted, spawned character · **Audience:** Anyone
reimplementing or porting this feature in any language.

This document describes *what* the system must do, not *how* to
implement it. The specific steps, prompts, options, and validation
rules of any one creation flow live in content packs and are out of
scope. The mechanics by which any such flow runs *are* in scope.

---

## 1. Overview

Character creation is the interactive wizard that runs between the
moment login hands a new connection to the Creating phase and the
moment that character is persisted, placed in the world, and switched
to the Playing phase. It builds the character's starting properties
(race, class, alignment seed, anything else a pack defines), validates
the result, and commits.

The creation flow itself — its steps, options, and per-step logic — is
defined by content (packs), not by the engine. The engine provides:

1. A **wizard primitive** that runs an ordered sequence of typed steps
   against an entity, accepting input from the connection.
2. A **completion pipeline** that validates the assembled entity,
   seeds derived starting values, persists the new character, places
   it in the world, and transitions to Playing.
3. A **restart pipeline** that discards the in-progress entity and
   replays the flow when post-assembly validation fails.

### Core concepts

- **Flow** — an ordered, named sequence of steps with a single
  completion handler. Identified by a stable id. Bound to a trigger
  name.
- **Flow instance** — the live execution of a flow against one
  entity in one session. The session holds at most one instance.
- **Step** — a single interactive (or non-interactive) unit of a
  flow. Step types are defined in §3.
- **Wizard step (label)** — optional UI metadata for clients that
  render a progress indicator across major flow milestones. Distinct
  from execution steps; some execution steps may be grouped under one
  wizard label and others may have none.
- **Trigger** — a named event that causes the engine to look up and
  start a registered flow. Character creation uses the trigger raised
  when login finishes the New-Player flow.
- **Pending entity** — an in-memory entity that exists for the
  duration of creation but has not yet been persisted or placed in
  the world.

### Goals

1. Run a content-defined sequence of prompts against a new entity.
2. Render those prompts to both plain text clients and clients with
   structured-data support.
3. Validate the assembled entity once all steps complete; on failure,
   discard the entity and restart the flow with a fresh entity.
4. Atomically persist, spawn, and switch to Playing on success.
5. Emit observable signals so rich clients can render appropriate UI.
6. Clean up cleanly if the connection drops mid-creation.

### Non-goals

- The specific steps, options, prompts, or validation logic of any
  one character-creation flow (content responsibility).
- Cancellable in-game wizards (the wizard primitive supports them,
  but creation itself is not cancellable — see §6.2).
- Character deletion, rename, or post-creation editing.
- The login feature that delivers the connection to the Creating
  phase (see `docs/specs/login.md`).
- The world's spawn-room selection logic beyond "use a configured
  default if the flow did not set one".

---

## 2. Entry conditions and handoff

Character creation begins when the login feature:

- Has reserved a canonical character name for the session.
- Has bound the session to an account id.
- Has constructed an initial entity using the world's new-player
  baseline (name, starting stats, vitals, regen, prompt template).
- Has set the session phase to Creating.
- Raises the "new player connected" trigger.

The engine looks up flows registered against that trigger. If at
least one flow is registered, it starts the most-recently-registered
match against the session's entity. If no flow is registered, the
engine MUST treat creation as immediately complete and proceed to the
commit pipeline (§6) — i.e. a deployment with no content-defined
creation still produces a valid playable character.

**Acceptance criteria**

- [ ] Creation begins from a session already in the Creating phase
      with a baseline entity, a reserved canonical name, and a
      known account id.
- [ ] The system selects a flow by trigger name, not by hardcoded
      id, so packs can override or extend.
- [ ] If multiple flows match a trigger, a deterministic resolution
      rule is applied (most-recently-registered wins is acceptable).
- [ ] A trigger with no matching flow advances directly to commit.
- [ ] While creation is running, the session holds a reference to
      the live flow instance.

---

## 3. Step model

A flow is an ordered list of steps. Each step has a stable id and an
optional skip predicate. The engine MUST support at least these step
types:

### 3.1 Info step

Non-interactive. Renders text to the connection (and emits a
structured "info" event), then auto-advances to the next step.

### 3.2 Choice step

Interactive. Renders a prompt and an ordered list of options. The
user selects either by 1-based index or by a unique prefix of an
option's label (case-insensitive). On a valid selection the step's
selection handler is invoked with the chosen option's value and the
flow advances. On invalid input the prompt is repeated.

Each option carries a label and may carry a description and a short
tag line for richer rendering.

A choice step also supports **non-committal inspection**: input of the
form `? <token>` (where `<token>` is an option index or unique label
prefix) shows that option's tag line + description and **re-renders the
menu without spending the choice** (prior art: NukeFire's `? <letter>`
class inspection). A `? <token>` that matches no option falls through to
the §4 help passthrough, so `? <help-topic>` still reaches help. When any
option carries a description, the menu advertises the affordance. (A bare
`?` and the `help` keyword remain help, not inspection.)

### 3.3 Text step

Interactive. Renders a prompt and accepts free-form input. The step
may declare a validation predicate; on rejection the configured
invalid-input message is shown and the prompt is repeated. On
acceptance the input is passed to the step's input handler and the
flow advances.

A text step MAY declare itself **secret**, in which case:

- Client echo MUST be suppressed before the prompt is rendered.
- Client echo MUST be restored before any subsequent output
  (including the next prompt on validation failure).

### 3.4 Confirm step

Interactive. Renders a yes/no prompt. Affirmative input runs the
yes-handler, negative input runs the no-handler; either way the flow
advances. Any other input is rejected and the prompt is repeated.

### 3.5 Skip predicate

Any step MAY declare a predicate evaluated against the in-progress
entity at the moment the step would be entered. If the predicate is
true the step is skipped (handlers do not run) and the engine
proceeds to the next step. Skipping happens *before* rendering.

### 3.6 Step extensibility

These four types cover everything in scope. The wizard primitive
SHOULD be extensible so packs can introduce new step types without
modifying the engine, but extensibility itself is not specified
here.

**Acceptance criteria**

- [ ] Info steps render once and auto-advance.
- [ ] Choice steps accept either a 1-based index or a unique
      prefix; invalid input repeats the prompt.
- [ ] Text steps run validation if declared; rejected input does
      not advance.
- [ ] Secret text steps suppress echo before rendering and restore
      echo before any subsequent output.
- [ ] Confirm steps treat affirmative/negative variants
      consistently and reject everything else.
- [ ] Skip predicates evaluate against the in-progress entity and
      bypass both rendering and handlers.

---

## 4. Input handling during a flow

While the session holds a live flow instance, all user input MUST be
routed to the flow first, with the following exceptions:

- **Help requests.** Input that begins with `?` or with a help
  keyword MUST be forwarded to the command system as a help command
  (with any argument preserved) without advancing the flow. Help
  passthrough lets users get context on options without losing their
  place.
- **Cancellation.** A flow that is declared cancellable accepts a
  configured cancel keyword as input. Character creation is
  **not** cancellable (§6.2), so this exception does not apply to it.

Any other input is delivered to the current step's input handler as
described in §3.

Input MUST NOT be forwarded to the normal command router while a
non-cancellable flow is active. The flood-control token bucket SHOULD
NOT consume tokens during flow input, since flow steps are paced by
the user not the script.

**Acceptance criteria**

- [ ] Help passthrough works for `?`, `? <topic>`, `help`, and
      `help <topic>` from inside a flow.
- [ ] Help passthrough does not advance the current step.
- [ ] Non-help input never reaches the normal command router during
      character creation.

---

## 5. Observable transitions

For every step that is rendered (info, choice, text, confirm), the
engine MUST emit a structured "flow step" event to the connection.
The event MUST identify the step type, carry the prompt, and — for
choice steps — carry the list of option labels (and tag lines where
present). The event format itself is not specified here.

Clients that render structured UI can rely on the event; clients
that only render text rely on the text written to the connection.
Both paths MUST be produced for every interactive step.

For clients that have negotiated rich rendering AND the flow defines
a wizard-step (progress) list, the engine SHOULD render an in-place
panel showing progress markers across the wizard steps, the current
prompt, and any choice options. Plain clients receive the same
information as a simple prompt followed by a numbered option list.

**Acceptance criteria**

- [ ] Every rendered step produces both a text rendering and a
      structured event.
- [ ] Choice events include enough data for a client to render the
      options without parsing the text rendering.
- [ ] Wizard-progress rendering is gated on client capability and on
      the flow declaring a wizard-step list.

---

## 6. Completion pipeline

When the final non-skipped step's handler returns, the engine enters
the completion pipeline.

### 6.1 Alignment seeding (creation-specific)

Before validation, the engine SHOULD derive starting alignment from
the entity's chosen race and class (when both are set) and record it
via the alignment subsystem. The exact derivation is policy (e.g.
sum of race and class starting alignment) and lives outside this
spec. If race or class is not set on the entity, this step MUST be
skipped silently.

This derivation applies only when the flow is being completed during
the Creating phase. Non-creation flows skip it.

### 6.2 Cancellation policy

The character creation flow MUST be marked non-cancellable. Users
cannot escape creation with `quit`/`cancel`; they can only complete
it or disconnect. Disconnection during creation is handled per §8.

### 6.3 Validation

The flow's completion handler is invoked with the in-progress
entity. It returns either success or failure with an optional
user-facing message.

- **Failure during Creating.** The message (if any) is delivered to
  the connection. The system MUST then **restart** the flow against
  a fresh entity built from the same new-player baseline as the
  initial entity (same name, default stats, etc.). Any partial state
  on the discarded entity MUST NOT leak into the fresh entity. The
  pending-entity tracking is updated to point at the new entity, and
  the session's entity reference is swapped.
- **Failure outside Creating.** (Non-creation flows.) The message is
  delivered and the flow is cleared. No restart, no commit. Out of
  scope for this spec but mentioned for completeness.
- **Success.** Proceed to commit (§6.4).

### 6.4 Commit

The commit step MUST be atomic with respect to concurrent
new-player commits for the same name. Under a mutual-exclusion
guard, the engine:

1. Re-checks that no persisted character exists with the entity's
   canonical name. If one does, this is the **last-chance name
   conflict**: the user is informed, the session is removed, and the
   connection is closed with a name-conflict reason. The entity is
   discarded.
2. Persists the new character, recording the owning account id along
   with the entity state.

Outside the guard:

3. Determines the spawn room: the entity's existing location if set;
   otherwise the configured default spawn room; otherwise any room
   in the world. The chosen room receives the entity.
4. Removes the entity from the pending-entity registry.
5. Registers the entity with the world's live-entity tracking.
6. Publishes a "character created" event on the engine event bus
   carrying the entity id.
7. Cancels the Creating-phase idle timer and clears its reference.
8. Transitions the session phase to Playing and clears the flow
   reference on the session.
9. Sends a welcome message and enqueues a "motd" command and a
   "look" command on the session, so that the player's first frame
   shows the server's MOTD followed by their starting room.

**Acceptance criteria**

- [ ] Alignment seeding runs only during Creating and only when the
      entity has both race and class set.
- [ ] The character-creation flow is non-cancellable.
- [ ] Validation failure during Creating restarts the flow against
      a fresh baseline entity with the same name.
- [ ] No partial state from the discarded entity persists across
      restart.
- [ ] Commit is mutually-exclusive with respect to other new-player
      commits.
- [ ] A persisted name collision at commit time disconnects the
      session with a clear message; no orphan record is written.
- [ ] On successful commit, the character is persisted before any
      world-side placement or events are emitted.
- [ ] Spawn room resolution uses the entity's location, the
      configured default, and any-room-as-last-resort in that order.
- [ ] "character created" is emitted with the entity id after the
      character is placed in the world.
- [ ] The pre-login idle timer is cancelled and the session phase is
      Playing before any post-creation input is processed.
- [ ] The session enqueues MOTD and a look on transition.

---

## 7. Restart semantics

When the completion handler fails during Creating, the engine
restarts the flow. Restart MUST:

1. Build a new entity with the same canonical name using the same
   new-player baseline factory the system used at login handoff
   (§2). This guarantees stats and defaults are identical to a
   first-time entry.
2. Replace the session's entity reference atomically. Anything keyed
   on the old entity id (pending-entity tracking, session-by-entity
   lookups) MUST be updated to reflect the new id.
3. Clear the current flow instance.
4. Start the same flow again, by its id, against the new entity.

Restart MUST NOT re-trigger the new-player login flow, re-prompt for
email/password, or change the bound account id. The user does not
re-authenticate when validation fails.

**Acceptance criteria**

- [ ] The new entity uses the same baseline as the original.
- [ ] Session-side and engine-side references that point at the old
      entity are updated to the new entity.
- [ ] The account id and connection binding are preserved across
      restart.
- [ ] The same flow id is restarted; the trigger is not re-fired.

---

## 8. Mid-creation disconnect

If the connection drops while the session is in the Creating phase:

- The half-created session MUST be removed from the live session set.
- The pending entity MUST be removed from the pending-entity registry.
- Any active-connection counters that were incremented when the
  session was reserved MUST be decremented.
- The reserved canonical name MUST become available for other
  new-player attempts again.
- Nothing MUST be persisted: the character does not exist on disk.
- A diagnostic log entry SHOULD be emitted with the canonical name
  and entity id.

A disconnect handler that fires *after* creation has already
committed (phase moved to Playing) MUST be a no-op for the
creation-cleanup purpose: post-commit cleanup is the world layer's
responsibility, not creation's.

**Acceptance criteria**

- [ ] A drop during Creating leaves no on-disk character.
- [ ] A drop during Creating releases the reserved name.
- [ ] The disconnect handler is idempotent and safe to fire after
      commit (it does not undo a committed character).

---

## 9. Configuration surface

The following are externally configurable and not fixed by this
spec.

| Policy | Where it applies |
|---|---|
| Set of flows registered (content) | §2 |
| Trigger names used to start flows | §2 |
| Step set, prompts, options, validation per flow (content) | §3 |
| Default spawn room id | §6.4 |
| New-player baseline (stats, vitals, regen, prompt template) | §2, §7 |
| Alignment seed derivation rule | §6.1 |
| Help-passthrough keywords | §4 |
| Wizard-panel rendering width and styles | §5 |

---

## 10. Open questions / future work

- **Engine reference to a content-provided room id.** The default
  spawn room id is currently an engine constant that names a
  pack-specific room. Treat the default as configuration provided at
  startup, not as an engine constant.
- **Trigger resolution when multiple flows match.** "Most recently
  registered wins" is the current behavior; whether that is the
  intended policy across packs with mixed priorities should be
  documented.
- **Wizard-step extensibility.** New step types defined by packs are
  conceptually possible but the extension contract is not specified
  here.
- **Mid-creation transient disconnect.** Today, dropping during
  Creating is a hard reset (no character is persisted). Whether to
  offer link-dead resumption *during* creation is a product
  question.
- **Per-flow idle behavior.** Creation runs under the Creating-phase
  idle timeout from login. Whether each step should refresh that
  timer (so a slow but engaged user is not timed out) is unclear.
- **Concurrent restart cap.** Validation failure can in principle
  loop indefinitely if content is broken. A configurable max-restart
  count would bound the failure mode.

---

<!-- Generated: 2026-05-21 · Scope: FlowEngine + FlowInstance + creation commit pipeline · Spec style: narrative + acceptance criteria · Detail level: behavior only -->
