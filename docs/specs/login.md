# Login — Feature Specification

**Status:** Draft · **Scope:** Connection accepted → handoff to Playing or
Character Creation · **Audience:** Anyone reimplementing or porting this
feature in any language.

This document describes *what* the login feature must do, not *how* to
implement it. Specific values (lengths, attempt limits, timeouts, hash
algorithms, regex patterns, prompt strings) are treated as policy and live
outside this spec, in configuration.

---

## 1. Overview

The login feature governs the lifecycle of a connection from the moment a
user is accepted by the network layer to the moment they are either
playing a character in the world, entering character creation as a new
player, or disconnected. It also covers re-attaching a connection to an
already-running session ("session takeover") and recovering a session
whose previous connection dropped ("link-dead reconnect").

### Core concepts

- **Connection** — a transport-level link to a single client (telnet,
  websocket, etc.). Has an identifier, a remote address (where
  available), and the ability to send/receive lines and emit out-of-band
  signals to the client.
- **Account** — an identity that owns one or more characters, identified
  by an email address and protected by a password. An account has a
  stable internal id distinct from the email.
- **Character** — a named, persisted, in-world entity owned by exactly
  one account. Looked up by canonical name.
- **Session** — the runtime binding of one connection to one character
  (or to a pre-login state, see §2). A session has a phase.
- **Phase** — the current step the connection is in. The login feature
  drives transitions between a fixed set of phases (§2).
- **Gate** — a pluggable policy that may block a name from entering the
  new-player flow.

### Goals

1. Identify the user by a canonical character name.
2. Authenticate the owning account, or create one.
3. Enforce per-account concurrency and per-name uniqueness.
4. Bound every interactive step with an idle timeout.
5. Hand off cleanly to character creation (new player) or to the world
   (returning player or session takeover or link-dead reconnect).
6. Emit observable phase transitions so clients with structured-data
   support can render appropriate UI.

### Non-goals

- The character creation flow itself (begins at the Creating handoff).
- The world spawn and world-state restore logic invoked after login
  completes (the login feature delegates to it).
- Account recovery, email verification, or password reset flows.
- The network/transport layer's connection acceptance and negotiation.

---

## 2. Phases and state machine

Every connection that reaches the login feature progresses through a
subset of these phases:

```
              ┌──────────────┐
              │  Connected   │  (transport accepted; login takes over)
              └──────┬───────┘
                     ▼
              ┌──────────────┐
              │     Name     │◄──── invalid input / failed sub-phase
              └──────┬───────┘
            known? ──┴── unknown?
              │             │
              ▼             ▼
        ┌──────────┐  (gates) ┌──────────┐
        │ Password │          │  Email   │
        │ (auth)   │          └────┬─────┘
        └────┬─────┘     existing? │ new?
             │            ┌────────┴─────────┐
       ok?   │            ▼                  ▼
     ┌───────┼──────┐ ┌──────────┐    ┌──────────┐
     │ takeover?    │ │ Password │    │ Password │
     │ link-dead?   │ │ (verify) │    │ (create) │
     │ direct play  │ └────┬─────┘    └────┬─────┘
     └───────┬──────┘      │               │
             ▼             ▼               ▼
       ┌──────────┐  ┌──────────────────────────┐
       │ Playing  │  │  Creating (handoff out)  │
       └──────────┘  └──────────────────────────┘
```

`LinkDead` is a phase that an *existing* session can be in when its
connection drops; login can transition such a session back to Playing
when the same account re-authenticates to the same character.

`SessionTakeover` is a transient confirmation phase the user may pass
through when a connection already exists for the requested character.

### Phase transition rules

- A phase change MUST atomically replace any active idle timer with the
  new phase's idle timer.
- A phase change SHOULD emit a structured event to the client so that
  rich clients can render appropriate UI.
- A phase change MUST NOT leak in-flight reads from the prior phase
  into the new phase.

---

## 3. Name phase

When a connection enters the Name phase, the system prompts the user to
enter a character name. Each submitted name is validated against the
name policy (length, allowed characters, reserved/blocked names). On
rejection the user is told why and reprompted; the phase does not
advance.

A validated name is canonicalized into a single, stable form. All
subsequent lookups, comparisons, reservations, and storage operations
MUST use the canonical form, never the raw input.

The system then determines whether a saved character with that
canonical name already exists:

- **Exists →** Returning-Player flow (§4).
- **Does not exist →** New-Player flow (§5), after name-gates pass.

Name-gates are an ordered list of policies registered at startup. A
gate may allow the name, reject the name with a reprompt, or reject the
name and require the connection to be closed.

**Acceptance criteria**

- [ ] On entering Name, a prompt is delivered to the connection.
- [ ] Invalid names are rejected with a specific, user-facing reason.
- [ ] Validation rejections do not advance the phase.
- [ ] Names are canonicalized exactly once, before any storage lookup.
- [ ] Existing canonical names route to Returning-Player.
- [ ] Unknown canonical names route to New-Player only after gates
      allow.
- [ ] A gate may reprompt or disconnect.
- [ ] Idle expiry in this phase closes the connection.

---

## 4. Returning-Player flow

> **Revised by `character-select.md` (account-first roster, build pending):**
> the character-name entry below is superseded by account-first authentication
> + a character roster. The mechanics of §4.3–§4.5 (concurrency, takeover,
> link-dead reconnect) and §4.6 (direct play) are **reused unchanged** — they
> are reached by selecting a roster entry instead of by a typed name. Until that
> ships, the name-first flow described here is current.

The Returning-Player flow loads the saved character, authenticates the
owning account, and routes the connection to one of three terminal
outcomes: takeover, link-dead reconnect, or direct play.

### 4.1 Loading the character

The system loads the saved character record by canonical name. The
record carries (at minimum) the owning account id and the persisted
entity state. If loading fails for transient reasons, the user is
informed and the flow returns to the Name phase.

### 4.2 Password verification

The connection enters the Password phase. The system requests the
account password and suppresses client-side echo while it is being
entered. Echo is restored before any further output.

The submitted password is verified against the owning account record
identified by the loaded character's account id. The verification
operation MUST be performed using a one-way password hash with per-
account salting; raw passwords MUST NOT be stored, logged, or echoed.

On a failed attempt:

- The failure counter for this connection's password phase is
  incremented.
- If the configured maximum failed attempts has been reached, the
  connection is closed with a lockout reason.
- Otherwise the user is reprompted within the same Password phase.

On a successful attempt the flow proceeds to §4.3.

### 4.3 Concurrency check

Before binding the session, the system counts how many *other*
characters belonging to the same account are currently active (i.e.
not counting the character about to be bound). If this count is at or
above the configured per-account concurrency cap, the login is
refused with a user-facing reason and the flow returns to the Name
phase.

### 4.4 Existing-session resolution

The system then checks for an existing session bound to this canonical
character name:

- **No existing session →** the system restores any world-side state
  associated with the saved character (objects, location, etc.) and
  completes login (§4.6).

- **Existing session in LinkDead phase →** link-dead reconnect (§4.5).
  The user is *not* asked to confirm; the dropped session is silently
  re-attached to the new connection.

- **Existing session in any other phase →** session takeover (§4.5).
  The user is prompted to confirm with an affirmative response. On
  decline, the flow returns to the Name phase. On confirm, the new
  connection replaces the old connection on the existing session and
  the old connection is closed.

### 4.5 Takeover and link-dead reconnect

Both takeover and link-dead reconnect leave the *session* intact
(world location, inventory, ongoing combat/quest state) and swap only
the underlying connection. After the swap:

- The session phase is set to Playing.
- The session is re-bound to the new connection's id.
- The previous connection, if still present, is closed.
- The user receives whatever standard reconnection feedback the
  Playing phase provides.

Link-dead reconnect MUST NOT prompt for additional confirmation
beyond the password already verified in §4.2.

### 4.6 Direct play completion

When there is no existing session and concurrency is allowed, the
system:

1. Restores any world-side state that depends on the loaded character.
2. Completes login: binds the loaded entity to a new session for this
   connection and account, removes the connection from the pre-login
   pool, and transitions the phase to Playing.

**Acceptance criteria for §4**

- [ ] The character record is loaded by canonical name, and its owning
      account id is used for password verification.
- [ ] Password input is echo-suppressed; echo is restored before any
      further output.
- [ ] Password verification uses one-way hashing with per-account
      salting; raw passwords never leave the verification call.
- [ ] Failed attempts increment a per-connection counter and a lockout
      occurs at the configured threshold.
- [ ] Per-account concurrency is enforced *before* session binding.
- [ ] A LinkDead session is silently re-attached on successful auth.
- [ ] A live session triggers an affirmative takeover prompt; on
      decline the flow returns to Name.
- [ ] Takeover swaps the connection on the existing session without
      losing world state.
- [ ] Direct play restores world-side state, then transitions to
      Playing.

---

## 5. New-Player flow

The New-Player flow is reached only after a name passes validation,
canonicalization, the existence check (none found), and all name-gates.

It establishes (or attaches to) an account, creates a fresh in-world
entity for the new character, reserves the name, and hands off to
character creation.

### 5.1 Email phase

The system enters the Email phase and prompts for the user's email.
Submitted input is validated against the email policy and normalized
to a single canonical form (e.g. lowercased). Validation failures are
reported and reprompted, bounded by a configured per-phase attempt
cap; exceeding the cap closes the connection.

The normalized email is then used to determine whether an account
already exists:

- **Account exists for email →** existing-account password
  verification (§5.2).
- **No account →** new-account creation (§5.3).

### 5.2 Existing account: password verification

The system enters the Password phase, suppresses echo, and prompts
the user. The submitted password is verified against the existing
account record. On a failed attempt the user is informed and the
flow returns to the Name phase; the user is not held in the password
phase.

On success:

- A per-account concurrency check (as in §4.3) is performed.
- The chosen canonical character name is associated with the existing
  account.
- The flow proceeds to §5.4 (entity creation and handoff).

### 5.3 New account: password creation

The system enters the Password phase, suppresses echo, and asks the
user to choose a password. The submitted candidate is validated
against the password policy (minimum length, etc.). On policy
failure, the candidate is rejected with a specific reason and a
per-phase attempt counter is incremented; exceeding the configured
cap closes the connection.

A policy-valid candidate is then re-prompted for confirmation. A
mismatch is treated the same as a policy failure: report, reprompt,
count, and disconnect at the cap.

Once the candidate is confirmed, echo is restored and a new account
is created, storing only the one-way hash of the password. The
chosen canonical character name is associated with the new account.

### 5.4 Entity creation, name reservation, and handoff

The system creates a new in-world entity using the canonical name and
the default new-player baseline (stats, vitals, regeneration, prompt
template, etc., as defined by the world's new-player policy). Where
the remote address is known, it is recorded on the entity for later
audit.

A pre-Playing session is constructed for the new entity, bound to
the connection, owned by the determined account id, and marked as
being in the Creating phase.

The system then performs a **name-reservation race check** under a
mutual-exclusion guard:

- If no other session currently holds this canonical name, the new
  session is added to the session set and the connection is removed
  from the pre-login pool. The reservation succeeds.
- Otherwise (another connection won the race), the user is informed
  that the name is being created elsewhere and the flow returns to
  the Name phase. No partial state is persisted.

On reservation success, the system MUST register a disconnect handler
that removes the half-created session if the connection drops before
character creation completes. This handler MUST be a no-op if the
session has already advanced past Creating.

Finally, the session phase is set to Creating and the character
creation feature is triggered for the new session. The login feature
is now done with this connection; further interaction is owned by
character creation.

**Acceptance criteria for §5**

- [ ] Email validation rejects malformed input with a specific reason
      and counts attempts toward a configured disconnect cap.
- [ ] Email is normalized exactly once before lookup or storage.
- [ ] Existing accounts authenticate via the same one-way hash
      verification as §4.2; failure returns to Name.
- [ ] New-account password creation enforces the password policy and
      requires confirmation; mismatches and policy failures count
      toward a configured disconnect cap.
- [ ] Echo is suppressed during password entry and restored before
      any further output, including before the next prompt on retry.
- [ ] New accounts persist only a one-way hash of the password.
- [ ] Per-account concurrency is enforced for existing accounts in
      this flow as well.
- [ ] The chosen canonical name is associated with the account before
      handoff.
- [ ] The new entity is built from the world's new-player baseline.
- [ ] Name reservation is mutually exclusive: only one connection can
      win the race for a given canonical name.
- [ ] Losing the reservation race produces a clear message and
      returns to Name, with no partial state persisted.
- [ ] A disconnect during Creating cleans up the reserved session.
- [ ] On success the session is in Creating and character creation
      has been triggered.

---

## 6. Cross-cutting requirements

### 6.1 Phase idle timeouts

Every interactive phase (Name, Email, Password, SessionTakeover,
Creating) MUST be bounded by an idle timeout. Timeout durations are
configurable per phase, with a global fallback when a per-phase value
is not configured. On expiry the connection is closed with a timeout
reason and any pre-login bookkeeping is cleaned up.

The timer associated with a phase MUST be reset (cancelled and
replaced) on every phase transition. A late timer firing for a prior
phase MUST NOT affect the current phase.

### 6.2 Password handling

- Passwords MUST be transmitted from the client only during a phase
  in which client-side echo is suppressed.
- Echo MUST be restored before the next visible output.
- Stored credentials MUST be one-way hashes with per-account salting.
- Raw passwords MUST NOT appear in logs, metrics, traces, error
  messages, GMCP packages, or any persisted artifact.

### 6.3 Concurrency and identity

- A character is owned by exactly one account.
- An account MAY own multiple characters.
- At most one *live* session may exist per canonical character name.
- An account MAY have at most N live characters at once, where N is
  the configured per-account concurrency cap. Login MUST refuse
  attempts that would exceed the cap.
- Name reservation across concurrent new-player attempts MUST be
  mutually exclusive.

### 6.4 Failure modes and disconnect reasons

The login feature distinguishes the following terminal failure modes
and SHOULD record a distinguishable reason for each:

- Pre-login idle timeout (any phase).
- Password lockout (exceeded failed-attempt cap on returning-player
  auth).
- Login failure cap (exceeded failed-attempt cap on email format or
  new-account password creation).
- Gate-induced disconnect (a name-gate returned a terminal block).
- Internal login error (uncaught exception during the flow).

Errors SHOULD be logged with connection id and, when known,
canonical name; raw input that may contain credentials MUST be
excluded.

### 6.5 Observable transitions

The login feature SHOULD emit a structured signal to the client on
every phase transition, sufficient for a rich client to render
appropriate UI without parsing text prompts. The signal format
itself is not specified here.

### 6.6 Connection cleanup

The login feature MUST register a disconnect handler at appropriate
points so that:

- A connection dropped during any pre-Playing phase removes itself
  from the pre-login pool.
- A connection dropped during Creating removes the half-created
  session and decrements any active-connection counters that were
  incremented at reservation.
- A connection dropped after handoff to Playing is owned by the
  Playing/world layer, not by login.

---

## 7. Configuration surface

The following policy values MUST be externally configurable. This
spec does not fix their values.

| Policy | Where it applies |
|---|---|
| Name policy (length, allowed chars, reserved list) | §3 |
| Name canonicalization rule | §3 |
| Name-gates (ordered list) | §3, §5 entry |
| Email format policy | §5.1 |
| Email normalization rule | §5.1 |
| Password policy (min length, etc.) | §5.3 |
| Max failed password attempts (returning) | §4.2 |
| Max failed attempts (email format, new password) | §5.1, §5.3 |
| Per-account concurrency cap | §4.3, §5.2 |
| Per-phase idle timeouts and global fallback | §6.1 |
| New-player entity baseline | §5.4 |
| Prompt strings and structured-event names | §6.5 (out of scope) |

---

## 8. Open questions / future work

- **Account recovery / password reset.** No flow exists today for a
  user who has forgotten their password. Behavior should be specified
  before adding such a flow.
- **Email verification.** Account creation accepts any
  syntactically-valid email without proving control of it.
- **Rate limiting at connection level.** This spec addresses
  per-connection attempt caps but not network-level brute-force
  defenses.
- **Audit log of logins.** Beyond ad-hoc logging, no canonical login
  audit trail is specified.
- **Gate composition semantics.** Gates today are run in registration
  order with first-block-wins; whether gates can also rewrite the
  proposed name or supply telemetry is unspecified.

---

<!-- Generated: 2026-05-21 · Scope: LoginFlow + supporting types · Spec style: narrative + acceptance criteria · Detail level: behavior only -->
