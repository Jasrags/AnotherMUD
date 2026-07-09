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
This is also where `character-identity.md` §5's world gate has its roster
surface: the roster lists only **in-world** characters, and out-of-world
characters are surfaced as a one-line **awareness footnote** (a count) rather
than selectable rows — present enough that a returning player knows they weren't
lost, absent enough that they don't clutter a boot they can't be played on.

### Core concepts

- **Account authentication** — identifying the account by a dedicated
  **account username** + password (§2), replacing the character-name entry as
  the front door; email is demoted to an optional recovery field.
- **Roster** — the list of the account's **in-world** characters presented after
  auth, each with its name and its world (its world in the server's active
  set). Out-of-world characters are hidden from the list and counted in a
  footnote.
- **Selection** — choosing an available roster entry to enter the game, or the
  "create a new character" option.

### Goals

1. Authenticate the **account** (not a character) at the front door.
2. Present a **roster** of the account's characters with per-world availability.
3. Let the player **select** an available character → Playing, or **create** a
   new one (→ the creation wizard, stamped with the active world).
4. Honor the world gate (`character-identity.md` §5) by listing only in-world
   characters and surfacing out-of-world ones as a footnote count — never
   deleted, never silently degraded.

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

## 2. Account identity and authentication

The front door identifies the **account** by a dedicated **account username** —
not a character name and not an email. (Decision 2026-06-16: name-based account
login; email is demoted to an optional recovery field on a path to deprecation.)

### 2.0 Connect splash (per-world)

Before the account-username prompt, the connection is shown a **connect splash**
— the door identity. The splash is **content, not engine**: each `kind: world`
pack supplies its own (`pack.yaml` `splash:` → a text file in the pack, with
engine color markup). It is **required** for world packs and validated at load —
a world pack with no splash, or an unreadable/empty splash file, fails boot;
library packs have no splash (they are never a connect door). The splash shown
is the **primary active world's** (`Registries.Worlds[0]`; co-host is deferred,
character-identity §9). The composition root renders it through the theme once
and hands the final text to the login front door, which emits it ahead of the
prompt; an absent splash falls back to a one-line greeting (tests / non-pack
boots).

**Acceptance criteria**

- [ ] Every `kind: world` pack declares a splash; the loader reads it and
      rejects a world pack that lacks one or points at an empty/missing file.
- [ ] The connect splash (primary world's, theme-rendered) is shown before the
      account-username prompt; an unset splash falls back to a one-line greeting.

### 2.1 The account username

The account model gains a **username**: a unique, case-insensitive account
identifier the player logs in with, distinct from any character name. It is
indexed for lookup alongside (and eventually replacing) the email index.

- **Email becomes optional.** It is retained on the account as a recovery /
  contact field, no longer the login key. New-account creation MAY collect an
  email but does not require it; the deprecation path is to stop collecting it.
- **Uniqueness.** Usernames are unique case-insensitively (like emails today);
  character names remain globally unique and independent of usernames.
- **Migration.** Existing accounts predate the username. They are backfilled
  deterministically — the username is derived from the existing email's local
  part (before the `@`), de-duplicated if it collides — so no operator input is
  needed and every account has a username after migration. (The account store is
  versioned/indexed; the backfill rebuilds the username index at load.)

### 2.2 Authentication

- Prompt for the **username**, then the **password**, authenticating against the
  account service.
- A username that **matches no account** begins the **new-account** path (choose
  the username if not already taken, then choose + confirm a password; email
  optional), creating the account; the roster is then empty and the flow goes
  straight to **create a character** (§4).
- Authentication failures, attempt caps, and password handling reuse `login.md`
  §6.2 unchanged (no new credential mechanics).

**Acceptance criteria**

- [ ] The account model carries a unique (case-insensitive) username, indexed;
      existing accounts are backfilled from the email local part (collision-
      de-duplicated) with no operator input.
- [ ] The front door authenticates by username + password — not character name,
      not email.
- [ ] An unknown username begins new-account creation (username availability +
      password, email optional); on success the account has no characters and the
      flow proceeds to character creation.
- [ ] Email is optional on the account and is not the login key.
- [ ] Password handling and attempt caps match `login.md` §6.2.

## 3. The roster

After authentication, the account's **playable** characters are presented as a
**roster** — the characters whose world is in the active world set
(`character-identity.md` §2). Each entry shows:

- the character **name**, and
- its **world** (the `WorldID` stamp).

Rules:

- The numbered, selectable list holds only **in-world** characters (plus any
  whose save failed to load, kept visible and flagged so a corrupt/newer save is
  never silently hidden). Out-of-world characters are **not listed** as rows —
  nothing can be done with them on this boot.
- **Out-of-world characters are surfaced as a one-line awareness footnote** — a
  count (e.g. "You also have 2 characters in other worlds not running on this
  server.") — never deleted, never silently degraded (`character-identity.md`
  §5). The footnote exists specifically so a returning player on a wrong-world
  boot knows their character wasn't lost; an empty numbered list *with* a
  footnote is shown (not routed to create) for exactly this reason.
- An **empty** roster (a fresh account with **no characters at all** — none in
  any world) sends the player straight to create (§4).
- The roster also offers a **create a new character** action.
- Ordering is stable (policy — e.g. creation order).

**Acceptance criteria**

- [ ] The roster lists the account's in-world characters with their name and world.
- [ ] An out-of-world character is **omitted from the numbered list** but its
      existence is surfaced as a footnote count, not deleted or silently dropped.
- [ ] A save that fails to load stays **visible and flagged** (distinct from the
      out-of-world hide), so it is never silently lost.
- [ ] A brand-new account (no characters at all) routes directly to creation; an
      account with only out-of-world characters shows the roster + footnote, not
      creation.
- [ ] The roster offers a create-new-character action.

## 4. Selection

The player selects a roster entry (by index or name) or the create action:

- **Available character** → the player lands on the **character action menu**
  (§4.1). Choosing *enter the game* loads the character and the session enters
  **Playing**. Because only active-world characters are selectable, the
  `character-identity.md` §5 world gate is satisfied by construction. The
  concurrency check, existing-session resolution, takeover, and link-dead
  reconnect rules (`login.md` §4.3–§4.5) apply at the point of entry, unchanged.
- **Out-of-world character** → not offered (hidden from the list per §3), so
  there is no roster row to select. The `character-identity.md` §5 login gate
  still holds by construction — an out-of-world character is never reachable as
  a selectable entry on this boot. (A save that failed to load *is* listed;
  selecting it reports the load failure and returns to the roster.)
- **Create a new character** → the creation wizard (`character-creation.md`),
  with the new character **stamped with the active world** (`character-identity.md`
  §3). On completion the character is added to the account roster
  (`account.AddCharacter`) and the session enters Playing (or Creating→Playing
  per `login.md` §5.4).

**Acceptance criteria**

- [ ] Selecting an available character loads it and enters Playing, applying the
      existing concurrency/takeover/link-dead rules.
- [ ] An out-of-world character is not a selectable row (hidden per §3); the
      world gate holds by construction.
- [ ] Creating from the roster runs the creation wizard, stamps the active
      world, adds the character to the account, and enters Playing.

### 4.1 Character action menu and roster operations (implemented)

Selecting an available character does not drop straight into the world; it opens
a per-character **action menu** (prior art: the NukeFire intake capture,
`docs/mud-research/nukefire/login-and-character-creation.md`). Plus the roster
itself carries account-scoped actions. These are the roster-operations slice of
§8, now built:

- **Character menu** — after selecting an available character:
  - **Enter the game** → load + Playing (the §4 selection path).
  - **Delete this character** → §4.1 deletion below.
  - **Back** → return to the roster.
- **Delete (hard, confirmed)** — the player must **type the character's name** to
  confirm (anything else, including empty, cancels). On confirm the character is
  hard-deleted: the **player save and all its sibling files** are removed
  (`player.Store.Delete` — the whole `players/<name>/` directory), then the
  character is **unlinked from the account** (`account.RemoveCharacter`).
  Save-first ordering is the recoverable one: a crash between the two leaves an
  account entry whose save is gone, which renders as an **unselectable** roster
  row that can simply be deleted again (both store ops are idempotent). Deletion
  is irreversible — there is no soft-delete/undo.
- **Change account password** — a roster-level action. It re-verifies the
  **current** password (`account.ChangePassword`, which rehashes under the
  account lock) before applying the new one; password length reuses `login.md`
  §6.2. Wrong-current / mismatch / too-short are soft failures (a message, stay
  on the roster); only IO / store errors abort.
- **Quit** — a roster-level action; a clean disconnect (`login.ErrQuit`, which
  the session treats like a normal close).

After a delete the roster is **rebuilt from the account** (so the removed entry
disappears); a roster emptied by deletion routes to create (§3).

**Acceptance criteria**

- [ ] Selecting an available character shows the action menu (enter / delete /
      back), not an immediate world entry.
- [ ] Delete requires typing the character name; a non-matching entry cancels;
      confirming removes the save (with siblings) and the account link.
- [ ] A character whose save is gone but whose account link remains shows as an
      unselectable roster row (and can be deleted again without error).
- [ ] Change-password re-verifies the current password and rejects a wrong one
      without altering the stored credential.
- [ ] Quit ends the connection cleanly without loading a character.

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

- **Account save** gains one field — the **username** (§2.1) — plus its lookup
  index; existing accounts are backfilled at load. Email is retained but
  optional. This is the only new persistence.
- The **roster** itself adds no storage: it is **derived at authentication time**
  from `account.Characters` (the names), each character's persisted `WorldID`
  (its world), and the server's active world set (availability).
- Creating a character adds it to `account.Characters` (already wired) and stamps
  its `WorldID` (already wired, `character-identity.md`).

**Acceptance criteria**

- [ ] The account save carries a username + index; existing accounts are
      backfilled with no operator input.
- [ ] The roster is derived from existing state (account character list + each
      save's WorldID + the active world set); it adds no new player-save field.

## 7. Configuration surface

| Setting | Meaning | Default |
|---|---|---|
| Email / password attempt caps | Reused from `login.md` §6.2. | login.md defaults |
| Roster ordering | The order characters are listed (§3). | creation order |
| Max characters per account | An optional cap on roster size (§8). | unbounded (policy) |

## 8. Open questions / future work

- **Email deprecation path.** Email is demoted to optional this slice (§2.1);
  when/whether to stop collecting it entirely, and whether to keep it for
  password recovery, is a follow-up. (Decision 2026-06-16: account login is by
  username, not character name and not email.)
- **Keep character-name login as a shortcut?** The front door is the account
  username; whether to also accept a typed character name as a convenience entry
  (resolving to its account) is undecided. Lean: drop it for a single clear
  front door; revisit if players miss it.
- **Max characters per account.** A cap bounds roster size and abuse; unbounded
  is simplest. Decide if/when abuse is a concern.
- **Roster operations** — **character delete** and **account password change**
  are now implemented (§4.1). Still open: **rename**, **email change**, and
  **account deletion** (the rest of account management).
- **Character description / background story** — the NukeFire menu also offered
  "edit description" and "read background"; these need a player description field
  and per-world background text, so they are deferred (not yet built).
- **New-visitor routing** — land on an empty roster then create, vs. go straight
  into creation — is a minor UX choice (§3 routes empty straight to create).

---

<!-- Scope: account-first auth + character roster (per-world availability) + select/create + roster operations (character menu/delete/password/quit, §4.1), revising login.md's identity entry; consumes character-identity §5 world gate + account.Characters · Spec style: narrative + acceptance criteria · Detail level: behavior only · Status: implemented (core + roster operations); deferred: rename / email-change / account deletion / description+background -->
