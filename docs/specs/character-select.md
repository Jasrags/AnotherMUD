# Character Select (account login + character roster)

**Status:** Draft · **Scope:** account-first authentication → a roster of the
account's characters (across worlds) → select one to play or create a new one ·
**Audience:** anyone reimplementing the login front end. *Spec ahead of code —
build pending.* **Revises** `login.md`'s identity entry (character-name-first →
account-first) and adds a roster phase between authentication and the
Playing/Creating handoff; reuses everything else login.md owns. Consumes the
world gate of `character-identity.md` §5 and the account character list
(`account.Characters`).

---

## 1. Overview

Login today is **character-name-first** (`login.md` §3): a connection types a
*character* name and authenticates as that character. Two things make an
**account-first** model the natural shape now:

- An account already holds **many characters** (`account.Characters`), and the
  creation flow already binds new characters to an existing account.
- **World-locking** (`character-identity.md`) stamps each character with a world
  and gates which are playable on a given server.

So a player with several characters — possibly **split across worlds** — should
**authenticate to their account once**, then see and pick from a **roster** of
their characters (with per-character world availability), or create a new one.
This is also where `character-identity.md` §5's "greyed roster" finally has a
surface: out-of-world characters are listed but unavailable.

### Core concepts

- **Account authentication** — identifying the account by email + password,
  replacing the character-name entry as the front door.
- **Roster** — the list of the account's characters presented after auth, each
  with its name, its world, and whether it is **available** (its world is in the
  server's active world set) or **unavailable** (greyed).
- **Selection** — choosing an available roster entry to enter the game, or the
  "create a new character" option.

### Goals

1. Authenticate the **account** (not a character) at the front door.
2. Present a **roster** of the account's characters with per-world availability.
3. Let the player **select** an available character → Playing, or **create** a
   new one (→ the creation wizard, stamped with the active world).
4. Render the world gate (`character-identity.md` §5) as greyed, unselectable
   roster entries — never deleted, never silently degraded.

### Non-goals

- The creation wizard internals (`character-creation.md`) and the Playing/
  Creating handoff + concurrency/takeover machinery (`login.md` §4.3–§4.5) —
  reused unchanged, just reached via the roster.
- Account management beyond login: password change, email change, account
  deletion, character rename/delete — separate features (§8).
- Co-hosting multiple worlds in one process (`character-identity.md` §9,
  deferred) — the roster supports characters from many worlds, but only the
  active world's are selectable on a given server.

---

## 2. Account authentication

The connection's front door identifies the **account**:

- Prompt for the account's **email**, then its **password**, authenticating
  against the account service (the same credential store `login.md` uses).
- An email that **matches no account** begins the **new-account** path (choose +
  confirm a password), creating the account, after which the roster is empty and
  the flow goes straight to **create a character** (§4).
- Authentication failures, attempt caps, and password handling reuse `login.md`
  §6.2 unchanged (no new credential mechanics).

**Acceptance criteria**

- [ ] The front door authenticates an account by email + password, not by
      character name.
- [ ] An unknown email begins new-account creation; on success the account has
      no characters and the flow proceeds to character creation.
- [ ] Password handling and attempt caps match `login.md` §6.2.

## 3. The roster

After authentication, the account's characters are presented as a **roster**.
Each entry shows:

- the character **name**,
- its **world** (the `WorldID` stamp), and
- its **availability** on this server: **available** when its world is in the
  active world set (`character-identity.md` §2), otherwise **unavailable**
  (greyed).

Rules:

- An **unavailable** (other-world) character is **listed but greyed** — shown so
  the player knows it exists and where it lives, never hidden, never deleted
  (`character-identity.md` §5).
- An **empty** roster (a fresh account, or one with no characters on any world)
  sends the player straight to create (§4).
- The roster also offers a **create a new character** action.
- Ordering is stable (policy — e.g. creation order); the available characters
  are distinguishable from the greyed ones.

**Acceptance criteria**

- [ ] The roster lists every character on the account with its name, world, and
      availability.
- [ ] An other-world character appears greyed/unavailable, not hidden or removed.
- [ ] An empty roster routes directly to character creation.
- [ ] The roster offers a create-new-character action.

## 4. Selection

The player selects a roster entry (by index or name) or the create action:

- **Available character** → the character is loaded and the session enters
  **Playing**. Because only active-world characters are selectable, the
  `character-identity.md` §5 world gate is satisfied by construction. The
  concurrency check, existing-session resolution, takeover, and link-dead
  reconnect rules (`login.md` §4.3–§4.5) apply here unchanged.
- **Unavailable (greyed) character** → selection is **refused** with the
  `character-identity.md` §5 message ("belongs to a world not running here"); the
  player stays on the roster.
- **Create a new character** → the creation wizard (`character-creation.md`),
  with the new character **stamped with the active world** (`character-identity.md`
  §3). On completion the character is added to the account roster
  (`account.AddCharacter`) and the session enters Playing (or Creating→Playing
  per `login.md` §5.4).

**Acceptance criteria**

- [ ] Selecting an available character loads it and enters Playing, applying the
      existing concurrency/takeover/link-dead rules.
- [ ] Selecting a greyed character is refused with the world-gate message and
      returns to the roster.
- [ ] Creating from the roster runs the creation wizard, stamps the active
      world, adds the character to the account, and enters Playing.

## 5. Relationship to `login.md`

This spec **revises** the front of `login.md` and **reuses** the rest:

- **Revised:** the identity entry. `login.md` §3 (Name phase) and the
  name-keyed returning/new branching (§4 entry, §5.1 email-after-name) are
  replaced by §2 (account email + password) and §3–§4 (roster + select) here.
- **Reused unchanged:** the connection-accepted handoff, password handling
  (§6.2), idle timeouts (§6.1), the concurrency / existing-session / takeover /
  link-dead rules (§4.3–§4.5), and the Creating handoff to `character-creation.md`
  (§5.4). The roster reaches these by selecting/creating instead of by a typed
  name.

`login.md`'s state machine gains a **Roster** phase between authentication and
Playing/Creating; the Playing and Creating phases are entered exactly as before.

**Acceptance criteria**

- [ ] The Playing and Creating handoffs, concurrency rules, and idle timeouts are
      identical to `login.md`; only the identity entry and the roster are new.

## 6. Persistence and state

No new persistence is introduced:

- The roster is **derived at authentication time** from `account.Characters`
  (the names), each character's persisted `WorldID` (its world), and the server's
  active world set (availability).
- Creating a character adds it to `account.Characters` (already wired) and stamps
  its `WorldID` (already wired, `character-identity.md`).

**Acceptance criteria**

- [ ] The roster is derived from existing state (account character list + each
      save's WorldID + the active world set); this spec adds no new save field.

## 7. Configuration surface

| Setting | Meaning | Default |
|---|---|---|
| Email / password attempt caps | Reused from `login.md` §6.2. | login.md defaults |
| Roster ordering | The order characters are listed (§3). | creation order |
| Max characters per account | An optional cap on roster size (§8). | unbounded (policy) |

## 8. Open questions / future work

- **Keep character-name login as a shortcut?** Account-first is the model here;
  whether to also accept a typed character name as a convenience entry (resolving
  to its account) — preserving muscle memory from the name-first flow — is
  undecided. Lean: drop it for a single clear front door; revisit if players miss
  it.
- **Max characters per account.** A cap bounds roster size and abuse; unbounded
  is simplest. Decide if/when abuse is a concern.
- **Roster operations** — delete / rename a character from the roster — are
  deferred; they pair with the broader account-management feature, not this
  selection flow.
- **Account management** (password change, email change, account deletion) is a
  separate feature; this spec only authenticates and selects.
- **New-visitor routing** — land on an empty roster then create, vs. go straight
  into creation — is a minor UX choice (§3 routes empty straight to create).

---

<!-- Scope: account-first auth + character roster (per-world availability) + select/create, revising login.md's identity entry; consumes character-identity §5 world gate + account.Characters · Spec style: narrative + acceptance criteria · Detail level: behavior only · Status: forward spec (build pending) -->
