# Admin Verbs — Feature Specification

**Status:** Draft · **Scope:** The mechanism that gates a command on an
administrative role; administrative target resolution (reaching entities
ordinary players cannot); the baseline set of administrative verbs
(inspect, set, teleport, announce, restore, purge, reload); the
audit trail every administrative action leaves · **Audience:** Anyone
reimplementing or porting this feature in any language.

This document describes *what* the administrative verb surface must do,
not *how* to implement it. Specific verb names, the settable-field
catalogue, and defaults live in the configuration-surface table at §8.

Admin verbs are the second half of the *Roles & Administration* theme.
They build directly on `roles-and-permissions.md`: every admin verb is a
command gated by `HasRole`. Reference behavior is ported from the prior
incarnation, whose admin surface is a thin engine API (set / grant /
resolve-target / set-vital) plus commands flagged `admin: true` that the
dispatcher refuses unless the actor holds the admin role. This spec
adopts that split: a small gating primitive in the engine, a baseline
verb set on top, and content-extensible mutation kinds.

---

## 1. Overview

An **admin verb** is an ordinary command carrying an *admin* marker. The
command dispatcher refuses to run it unless the actor holds the
configured administrative role (`roles-and-permissions.md` §3, §8). Once
past the gate, admin verbs may do what ordinary verbs cannot: read and
mutate any entity, reach hidden or offline targets, and broadcast
server-wide.

Two layers:

- **The gate** — a per-command flag plus the dispatcher check. This is
  the only engine primitive strictly required; everything else is verbs
  built on it.
- **The baseline verbs** — a conventional set (inspect, set, teleport,
  announce, restore, purge, reload) the core pack ships, each carrying
  the admin marker and using the administrative APIs below.

### 1.1 What admin verbs are *not*

- Not their own permission system. Authorization is entirely
  `roles-and-permissions.md`. An admin verb asks `HasRole(actor,
  admin-role)` and nothing more; it does not define tiers, ACLs, or
  per-verb grants.
- Not a fixed, hardcoded list. The gate is the engine contract; the verb
  set is content the core pack provides and a deployment may extend or
  trim. New admin verbs are added by marking a command admin, not by
  changing the engine.
- Not exempt from the rest of the engine. An admin verb's mutations flow
  through the same persistence, event, and validation paths as any other
  write — they are privileged in *reach*, not in *bypassing invariants*.
- Not silent. Every administrative action is audited (§6). Privilege
  without accountability is a non-goal.

---

## 2. The admin gate

A command may be marked **admin**. The dispatcher enforces the mark.

- When dispatch resolves a verb to a command marked admin, it checks
  `HasRole(actor, admin-role)` *before* running the handler. On failure
  it refuses.
- The refusal is **indistinguishable from an unknown verb** — the same
  "Huh?" an ordinary player gets for gibberish (`commands-and-dispatch.md`
  §3). A non-admin must not be able to enumerate the admin verb surface
  by probing which words produce "you can't do that" versus "Huh?".
  Equivalently: admin verbs are invisible to those who can't use them, in
  help (`ui-rendering-help.md` §9.5) and in refusal copy alike.
- The gate is checked at dispatch, once, before the handler. A handler
  never re-checks the role (single source of truth) and never runs
  partially before the gate.
- The administrative role name is configuration, not a literal
  (`roles-and-permissions.md` §8). A deployment may split admin
  capabilities by gating different verbs on different roles (e.g. a
  `builder` role for world-edit verbs, `admin` for account verbs); the
  gate reads each command's required role.

This gate is what finally closes the engine's standing "ungated until
roles land" verbs: the `reload` script-reload verb (`scripting-and-packs`
hot-reload) and the `xp` self-grant probe become admin-marked and are
refused for ordinary players.

**Acceptance — the gate**

- [ ] A command marked admin runs for an actor holding the admin role.
- [ ] The same command refuses for an actor without it, with the same
      output an unknown verb produces — no disclosure that the verb exists.
- [ ] The role is checked once at dispatch, before the handler; the
      handler does not re-check and does not partially execute on refusal.
- [ ] `reload` and `xp` are admin-gated (no longer usable by ordinary
      players).

---

## 3. Administrative target resolution

Admin verbs reach what ordinary verbs cannot.

- An admin verb that takes an entity argument resolves it with
  **visibility bypass**: hidden, invisible, or sneaking targets (the
  visibility rules, still greenfield — see `BACKLOG.md`) are reachable,
  where an ordinary verb would not see them. This is the
  `bypass_visibility` argument property
  (`commands-and-dispatch.md` §5).
- Resolution scope is verb-dependent and wider than a normal verb's:
  - Room-scoped admin verbs (inspect, set on a present entity) resolve in
    the actor's room, bypass-visible.
  - World-scoped admin verbs (teleport-to-player, summon) resolve a
    **player by name across the whole world**, including link-dead and,
    where the verb supports it, offline characters (the mutation is then
    write-through to the save).
- The admin entity kinds are **player**, **npc**, and **item** — the
  three the mutation APIs (§4, §5) operate on. A verb declares which
  kinds it accepts; resolving a target of the wrong kind is a usage
  error, not a silent miss.
- Ambiguous resolution surfaces the candidates rather than picking one,
  so an admin acting on the wrong entity is hard to do by accident.

**Acceptance — resolution**

- [ ] An admin verb resolves a hidden/invisible target that an ordinary
      verb in the same room would not.
- [ ] A world-scoped admin verb resolves a player by name regardless of
      room; offline resolution is available only to verbs that declare it.
- [ ] Targeting an entity of a kind the verb does not accept is a usage
      error, not a silent no-op.
- [ ] Ambiguous targets list the candidates rather than acting on one.

---

## 4. The set surface

`set` mutates a single field on a resolved target. It is the
general-purpose admin write.

- The settable fields form a **catalogue** of *kinds* — properties, tags,
  and vitals/attributes — each declaring which entity kinds it applies to
  and whether it is admin-settable. A field not in the catalogue, or not
  flagged admin-settable, cannot be `set`. (Collection-typed fields are
  excluded from the simple `set` path; they need dedicated verbs.)
- Setting a **property** writes the entity's property bag
  (`persistence.md` property registry) and persists it. Setting a **tag**
  adds or removes a gameplay tag (and re-indexes — `world` tag index).
  Setting a **vital** (HP and, when they exist, resource/movement) writes
  the live value, clamped to its maximum.
- `set` validates the value against the field's declared type (a numeric
  field rejects non-numeric input) and reports a usage error rather than
  writing garbage.
- A bare or incomplete `set` (missing kind/type/target/value) renders a
  usage panel listing the available kinds and types rather than failing
  silently — the verb is self-documenting.
- Roles are **not** settable through `set`. Granting and revoking roles
  go through the dedicated, separately-audited grant/revoke surface
  (`roles-and-permissions.md` §4), so privilege changes are never an
  incidental side effect of a generic field write.

**Acceptance — set**

- [x] Setting an admin-settable property on a target persists it.
      *(M19.4h: room mobs/items — live write. Persistence applies once
      player property bags land; mobs/items are transient.)*
- [ ] Setting a tag adds/removes it and updates the tag index.
      *(deferred — M19.4i+, no runtime tag mutator yet.)*
- [x] Setting a vital clamps to its maximum and takes effect immediately.
- [x] A non-admin-settable or unknown field is refused; a type-mismatched
      value is refused with a usage error, writing nothing.
- [x] An incomplete `set` renders the kinds/types usage panel.
- [x] Roles cannot be changed via `set`.

---

## 5. The baseline verb set

The core pack ships these admin verbs. Each is admin-marked (§2) and
audited (§6). A deployment may extend or trim the set; the behaviors
below are the conventional baseline.

| Verb | Behavior |
|---|---|
| `inspect` | Read-only dump of a target's stats, vitals, equipment, properties, levels, and roles. Bypass-visible. The diagnostic verb. |
| `set` | Mutate one field on a target (§4). |
| `teleport` (`goto`) | Move the actor to a target room or to a player; or, with a target, move that player. Publishes the normal room-change events for each moved entity. |
| `announce` | Broadcast a message to every connected session, attributed as an administrative announcement, distinct from any channel. |
| `restore` | Set a target's vitals to full and top off its sustenance (hunger/thirst) when it carries one. The mercy verb. |
| `purge` | Remove a non-player entity (item or npc) from the world, untracking it. Never targets a player. |
| `reload` | Re-discover and hot-swap pack scripts (`scripting-and-packs` hot-reload). Now admin-gated (§2). |

Verbs that mutate persistent state persist through the normal save path;
verbs that move entities reuse the normal movement/teleport events so
observers, GMCP, and the prompt all update as usual.

**Acceptance — baseline verbs**

- [ ] `inspect` shows a target's full diagnostic record and reaches
      bypass-visible targets.
- [ ] `teleport` moves the actor (or a named player) and fires the normal
      room-change events exactly once per moved entity.
- [ ] `announce` reaches every connected session and is attributable.
- [ ] `restore` fills vitals and (for a target with a sustenance pool) tops
      off hunger/thirst; `purge` removes a non-player and refuses a player target.
- [ ] Each baseline verb is refused for a non-admin per §2.

---

## 6. Observability and audit

Administrative power is accountable.

- Every successful admin verb emits an `admin.action` event carrying the
  actor, the verb, the target (when any), and the salient arguments.
- The audit event is **non-cancellable**: it records what happened, it
  does not gate it (gating is the role check at §2).
- A refused admin attempt (gate failure) MAY also be recorded, at a lower
  severity, so probing for the admin surface is visible to operators; the
  choice is in §9.
- Audit records are operational, not gameplay: they go to the structured
  log / audit sink, not to a player-visible feed.

**Acceptance — audit**

- [ ] Each successful admin verb emits one `admin.action` with actor,
      verb, target, and arguments.
- [ ] The audit event cannot be cancelled by a subscriber.

---

## 7. Persistence

Admin verbs hold no persistent state of their own.

- The gate reads roles (`roles-and-permissions.md` §6); it stores nothing.
- Mutations an admin verb makes (a `set` property, a granted item, a
  moved location) persist through the *target's* normal save path, not
  through any admin-specific store.
- Offline-target writes (a world-scoped verb acting on a logged-out
  character) write through to that character's save and apply on next
  login, when the verb declares offline support (§3).

**Acceptance — persistence**

- [ ] An admin `set` to a persistent field survives the target's logout.
- [ ] No admin-verb-specific persistence file exists; all writes ride
      the target's save.

---

## 8. Configuration surface

| Setting | Description |
|---|---|
| Admin role(s) | The role each admin verb requires (default `admin`; verbs may gate on distinct roles, e.g. `builder`) (§2). |
| Admin entity kinds | The entity kinds the set/grant APIs operate on (player / npc / item) (§3). |
| Settable-field catalogue | Which properties/tags/vitals are admin-settable, with their types and applicable kinds (§4). |
| Baseline verb set | Which conventional admin verbs the core pack ships (§5). |
| Offline-target verbs | Which world-scoped verbs may act on logged-out characters (§3). |
| Record refused attempts? | Whether gate failures are audited (§6/§9). |
| Announce attribution | How a server announcement is labelled/styled (§5). |

---

## 9. Open questions

- **Split roles vs. single admin role.** Whether the baseline ships one
  `admin` role for everything or splits world-edit (`builder`) from
  account/operations (`admin`) from the start. The gate supports either;
  the baseline default is the open call.
- **Record refused attempts.** Auditing gate failures catches probing but
  adds noise (and a fast typist fat-fingering an admin verb name is not a
  threat). Default leans toward recording at debug severity.
- **Offline mutation breadth.** Which verbs are safe to apply to a
  logged-out character (a property `set` is safe; a teleport is
  meaningless; a vital `set` races a future login). The conservative
  default is online-only except where a verb explicitly opts in.
- **`purge` on a player.** Hard-forbidden here. If a deployment ever needs
  to remove a player entity (corrupt save recovery), that is a separate,
  even-more-guarded verb, not a `purge` target relaxation.
- **Builder world-edit verbs.** Room/exit/spawn editing (dig, redit, link)
  is a large builder surface deliberately out of this spec's baseline —
  it is its own theme once a world-edit need is concrete.

---

## Cross-references

- `roles-and-permissions.md` — the authorization the gate consults;
  role grant/revoke lives there, deliberately not in `set` (§4).
- `commands-and-dispatch.md` §3 (dispatch / "Huh?"), §5
  (`bypass_visibility` argument property).
- visibility rules (greenfield, not yet specced — see `BACKLOG.md`) —
  what admin target resolution bypasses; the bypass *seam* is real today
  (`commands-and-dispatch.md` §5) even though the hide/sneak rules aren't.
- `scripting-and-packs.md` — the `reload` verb the gate now covers.
- `persistence.md` — the property registry and save path admin writes use.
- `ui-rendering-help.md` §9.5 — admin verbs hidden from help for non-admins.
