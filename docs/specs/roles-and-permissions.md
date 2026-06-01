# Roles & Permissions — Feature Specification

**Status:** Draft · **Scope:** The per-character set of *role* strings
that grant capabilities; the `HasRole` authorization check that engine
and content features gate on; the admin operations that grant and revoke
roles; the configuration-driven bootstrap that seeds the first
privileged account; the per-character save field that persists roles
across logout · **Audience:** Anyone reimplementing or porting this
feature in any language.

This document describes *what* the roles surface must do, not *how* to
implement it. Specific role names, verb names, and defaults live in the
configuration-surface table at §8.

Roles are the keystone of the *Roles & Administration* theme: admin
verbs, the admin-channel and admin idle-exemption
(`session-lifecycle.md` §5), and any privilege-gated command all consult
roles. Reference behavior is ported from the prior incarnation, whose
authorization model is a flat, case-insensitive set of role strings per
entity with an `entity.HasRole(name)` check — **capability grants, not a
tier ladder.** This spec adopts that model; the tier-ladder alternative
is recorded as a rejected option in §9.

---

## 1. Overview

A **role** is a short string a character may hold ("admin", "builder").
A character holds a *set* of roles. A feature that needs privilege asks
one question: *does this character hold role X?* That is the entire
authorization primitive — `HasRole`.

- Roles are granted and revoked, persist across logout, and are seeded
  for designated accounts at load so a privileged user exists without an
  in-game bootstrap step.
- Roles are **flat**: holding "admin" does not imply "builder". A
  character that needs both holds both. There is no inheritance,
  precedence, or numeric level.
- Roles are a **separate namespace from gameplay tags** (racial flags,
  alignment buckets, room `safe` tags). A gameplay tag must never grant a
  permission, and a role must never participate in gameplay-tag matching.
  The two are stored and queried independently.

### 1.1 What roles are *not*

- Not a hierarchy. There is no "admin > builder > player" ordering; see
  §9 for why the tier model was rejected.
- Not gameplay tags. A mob's `humanoid` tag or a player's `evil`
  alignment bucket is not a role and cannot be checked with `HasRole`.
- Not a per-command ACL system. Roles are coarse capabilities; a feature
  decides *which* role it requires. There is no per-(command, character)
  permission matrix.
- Not self-service. A character cannot grant themselves a role. Privilege
  enters the system only through the configured seed (§5) or a grant from
  an already-privileged character (§4).
- Not the help-tier renderer's stand-in. `ui-rendering-help.md` §9.5
  describes role *tiers* for help visibility but notes they "do not
  actually elevate" anyone. This spec is the real authorization source;
  the help renderer consults `HasRole` once this lands.

---

## 2. The role set

Every character carries a set of role strings.

- Role names are compared **case-insensitively** and stored normalized
  (lower-cased, trimmed). "Admin", "ADMIN", and " admin " denote the same
  role.
- The set is unordered and deduplicated: a character either holds a role
  or does not. Granting a held role is a no-op; revoking an unheld role
  is a no-op.
- The empty set is the default. A brand-new character holds no roles —
  an unprivileged player *is* the absence of any role, not a "player"
  role. (A `player` role may be defined by config if a feature wants to
  gate on "is a real account", but the engine does not require one.)
- Role names are open strings. The engine does not enforce a fixed
  vocabulary; config and content decide which roles exist and what each
  one gates. A small set of **conventional** roles is used by engine
  features (§8): notably `admin` for administrative verbs, the admin
  channel, and the idle-sweep exemption.

**Acceptance — the role set**

- [ ] A new character holds no roles.
- [ ] Role membership is case-insensitive: granting "Admin" then checking
      "admin" reports held.
- [ ] Granting an already-held role and revoking an unheld role are both
      no-ops (no error, no duplicate, no spurious event).
- [ ] Roles and gameplay tags are independent: granting a role does not
      add a gameplay tag, and a gameplay tag is never returned as a role.

---

## 3. The authorization check

`HasRole(character, role)` is the one question every privileged feature
asks. It returns whether the character currently holds the (normalized)
role.

- The check is **read-only and side-effect-free**: it never grants,
  never logs the subject as privileged, never mutates state.
- A feature that requires privilege calls `HasRole` and, on a false
  result, refuses with a message that does not leak the existence or name
  of the gating role beyond what the player should know. (An ordinary
  player typing an admin verb sees "Huh?" or an ordinary refusal, not
  "you need the admin role".)
- Features that gate on roles today:
  - **Admin verbs** (`admin-verbs` — separate spec) require an
    administrative role before dispatching.
  - **Admin channel** broadcast/subscription is restricted to holders of
    its configured role.
  - **Idle-sweep exemption** (`session-lifecycle.md` §5): a session whose
    character holds the configured exemption role is not swept for
    inactivity.
  - **Help visibility** (`ui-rendering-help.md` §9.5): role-tiered topics
    resolve their visibility through `HasRole`.

**Acceptance — the check**

- [ ] `HasRole` returns true exactly when the character holds the role
      (case-insensitively) and false otherwise.
- [ ] `HasRole` mutates nothing — repeated calls are identical and leave
      the role set unchanged.
- [ ] A privilege-gated feature invoked by a character without the
      required role refuses without disclosing the gating role name.

---

## 4. Granting and revoking

Roles enter and leave a character's set through two operations, each
itself privilege-gated.

- **Grant** adds a role to a target character's set. **Revoke** removes
  it. Both normalize the role name and are idempotent (§2).
- Both operations require the *actor* to hold a configured
  **granting role** (conventionally `admin`). A character without it
  cannot grant or revoke any role — including to themselves. This is what
  prevents privilege escalation: the only way to gain a role is from
  someone who already holds the granting role, or from the seed (§5).
- A grant/revoke takes effect immediately for live authorization
  (`HasRole` reflects it at once) and is persisted (§6) so it survives the
  target's logout.
- Granting or revoking a role on an **offline** character is permitted:
  the change is written to the target's saved roles and applies on their
  next login. (Whether an offline grant is supported in v1, or deferred
  to "online targets only", is an open question — §9.)
- Each grant and revoke emits an observable event (§7) carrying actor,
  target, and role, so the change is auditable.

**Acceptance — grant / revoke**

- [ ] An actor holding the granting role can grant a role to another
      character; `HasRole` on the target immediately reports held.
- [ ] An actor *without* the granting role cannot grant or revoke any
      role; the attempt refuses and changes nothing.
- [ ] Grant and revoke are idempotent and normalize the role name.
- [ ] A grant to an online target takes effect for authorization without
      requiring relog; the change persists across the target's logout.
- [ ] Grant and revoke each emit exactly one observable event with actor,
      target, and role.

---

## 5. Seeding and bootstrap

Privilege must be able to enter an otherwise-empty world without an
already-privileged user — the chicken-and-egg problem. Configuration
solves it.

- A **role seed** maps account (or character) identifiers to a set of
  roles. At character load, the seed is consulted and any seeded roles
  are ensured present on that character (idempotently — re-running the
  seed never duplicates or errors).
- The seed is the *only* out-of-band privilege source. It is operator
  configuration, not in-game content: a pack cannot seed roles for
  itself.
- Seeding is **additive at load** and does not by itself revoke. An
  operator removing a name from the seed does not strip the role on next
  login unless the spec opts into seed-reconciliation (an open question,
  §9 — the conservative default is additive-only, leaving revocation to
  the explicit verb).
- The seed makes the bootstrap reproducible: a fresh deployment with one
  configured admin name yields exactly one admin on that character's
  first login, who can then grant further roles in-game.

**Acceptance — seeding**

- [ ] A character named in the seed holds the seeded roles after load,
      on a fresh save with no prior roles.
- [ ] Re-loading a seeded character does not duplicate roles or error.
- [ ] A character not named in the seed gains no roles from it.
- [ ] Seeding is additive: it never removes a role the character already
      holds.

---

## 6. Persistence

Roles are part of the character's saved state.

- A character's roles persist as a list of role strings on the player
  save and round-trip unchanged across logout/login.
- Roles are restored into the live role set on load, then the seed (§5)
  is applied over the restored set (so a seeded role survives even if an
  older save predates roles).
- Adding roles to the save shape is a save-version change: older saves
  without a roles field load cleanly as the empty set.
- Roles are **player state only**. Mobs may carry roles in principle (the
  check is entity-level), but no mob-role persistence is required by this
  spec; a mob's roles, if any, are re-derived at spawn, not saved.

**Acceptance — persistence**

- [ ] A granted role is present after the character logs out and back in.
- [ ] A save with no roles field loads as the empty set (no error).
- [ ] Restored roles plus seeded roles are both present after load, with
      no duplication.

---

## 7. Observability

Role changes are auditable.

- Granting a role emits a `role.granted` event; revoking emits
  `role.revoked`. Each carries the actor (who performed it, or a sentinel
  for the seed), the target character, and the role name.
- Seed application at load MAY emit `role.granted` with the seed sentinel
  as actor, or MAY be silent; the choice is recorded in §9. Whichever is
  chosen, it is consistent (not sometimes-one, sometimes-the-other).
- These events are **non-cancellable**: authorization changes are facts,
  not proposals. A content layer that wants to *prevent* a grant gates
  the grant verb itself (by requiring its own role), not by cancelling
  the event.

**Acceptance — observability**

- [ ] A grant emits `role.granted`; a revoke emits `role.revoked`; each
      carries actor, target, and role.
- [ ] The events are non-cancellable — a subscriber cannot veto a grant
      by handling the event.

---

## 8. Configuration surface

| Setting | Description |
|---|---|
| Role seed | Map of account/character identifier → set of roles, applied at load (§5). |
| Granting role | The role required to grant or revoke any role (conventionally `admin`) (§4). |
| Admin role | The role engine administrative verbs require (`admin-verbs` spec). |
| Idle-exemption role | The role that exempts a session from the idle sweep (`session-lifecycle.md` §5). |
| Admin-channel role | The role required to use the administrative channel. |
| `player` role required? | Whether the engine defines a baseline `player` role or treats unprivileged as the empty set (§2). |
| Seed reconciliation | Whether removing a name from the seed revokes on next login, or the seed is additive-only (§5/§9). |
| Offline-target grants | Whether grant/revoke may target offline characters (§4/§9). |

No role *names* are hardcoded by behavior: every engine feature that
gates reads its required role from this surface, so an operator can
rename `admin` or split capabilities without code changes.

---

## 9. Open questions

- **Tier ladder (rejected).** An ordered "player < builder < admin"
  hierarchy where a higher tier implies lower ones was considered and
  rejected: it couples unrelated capabilities (a builder who should edit
  rooms does not thereby deserve to read private tells), and the prior
  incarnation shipped flat capability grants successfully. Revisit only
  if a concrete feature needs strict ordering.
- **Seed reconciliation.** Additive-only (conservative) vs.
  seed-as-source-of-truth (removing a name revokes on next login). v1
  leans additive-only; an operator who wants revocation uses the verb.
- **Offline-target grants.** Whether `grant`/`revoke` may target a
  character who is not logged in (write-through to their save) or are
  restricted to online targets in v1.
- **Seed-application events.** Whether load-time seeding emits
  `role.granted` (auditable, but noisy on every login) or is silent
  (quieter, but the audit log can't distinguish seeded from granted).
- **Capability granularity.** Whether very fine capabilities ("can mute",
  "can teleport") become their own roles or whether a coarse `admin`
  role fronts a content-defined sub-capability map. Deferred until admin
  verbs reveal the real granularity need.

---

## Cross-references

- `admin-verbs` (spec, this theme) — the privileged verb surface that
  gates on the admin role.
- `session-lifecycle.md` §5 — admin idle-sweep exemption gates on a role.
- `ui-rendering-help.md` §9.5 — help role tiers resolve through `HasRole`
  (superseding the no-op stub noted there).
- `chat-channels-and-tells.md` — the administrative channel restricts on
  a role.
- `persistence.md` — the player save gains the roles field (save-version
  bump).
