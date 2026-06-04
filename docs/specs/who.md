# Who — Feature Specification

**Status:** Implemented (v1 — `internal/command/who.go`, `internal/session/roster.go`) · **Scope:** The `who` verb that lists the characters
currently connected to the server, the per-character roster line, the
summary count, and which characters appear · **Audience:** Anyone
reimplementing or porting this feature in any language.

This document describes *what* the `who` verb must do, not *how* to
implement it. The roster columns and defaults live in the
configuration-surface table at §5.

Unlike most specs in this set, `who` has **no reference implementation**
to port — the prior incarnation never shipped a dedicated `who`. It is
specified here from MUD convention. The behavior is deliberately small
and conventional; the only non-obvious decision (who appears, given that
visibility rules are not yet built) is settled conservatively in §4 and
left as the one real open question.

---

## 1. Overview

`who` answers "who else is here, in the whole world?" It reads the live
set of connected, playing sessions (`session-lifecycle.md` §3,
SessionManager) and renders one line per character plus a summary count.

- It is **presence**, not location: `who` says a character is online, not
  *where* they are. Locating a specific player is a separate (likely
  privileged) concern.
- It reads only live session state — it does not touch saves, does not
  count offline characters, and does not persist anything.

### 1.1 What `who` is *not*

- Not a locator. It does not reveal rooms or areas. (A `where`-style
  locator is a separate, likely admin, verb.)
- Not a friends/channel roster. It lists *everyone connected*, not the
  members of a channel or a buddy list (those belong to
  `chat-channels-and-tells.md`).
- Not a leaderboard. Ordering is for readability (§2), not ranking; `who`
  does not sort by level or score by default.
- Not historical. It is a snapshot of *now*; it has no "last seen" or
  login-history surface.

---

## 2. The roster

`who` renders one line per appearing character (§4).

- Each line shows at least the character's **name**. Conventional
  additional columns — a short **title/description**, a **level** or
  primary-track standing, and an **idle** marker for a character who has
  not acted recently — are configuration (§5); a deployment chooses which
  to show.
- A character holding a privileged role (`roles-and-permissions.md`) MAY
  be marked as such (e.g. a `[Admin]` tag) so players can see who the
  staff are — unless that character is hidden (§4).
- Lines render through the normal color pipeline (`ui-rendering-help.md`)
  so names/titles may carry themed color; in plain mode they strip.
- Ordering is stable and readable — alphabetical by name is the
  conventional default. The order is presentational, not meaningful (§1.1).

**Acceptance — the roster**

- [x] Each appearing character produces exactly one line showing at least
      the name.
- [x] The actor always sees their own line.
- [x] Configured optional columns (title / level / idle) render when
      enabled and are absent when not. *(v1: idle marker + `[Admin]` role tag
      ship; the title/level columns are deferred — columns are optional per §5,
      and a "level" column needs a primary-track decision.)*
- [x] Ordering is stable across repeated invocations with the same
      population. *(case-insensitive alphabetical, `sort.SliceStable`.)*

---

## 3. The summary

- `who` ends with a count of how many characters it listed (e.g. "3
  players online"), matching the number of roster lines actually shown
  (§4) — not the raw connection count, so a hidden character is not
  betrayed by an off-by-one total.
- Singular/plural and exact wording are presentational.

**Acceptance — the summary**

- [x] The reported count equals the number of roster lines rendered.
- [ ] A hidden character (§4) is excluded from both the lines and the count.
      *(forward — no visibility/hiding exists yet; v1 hides no one.)*

---

## 4. Who appears

This is the one decision `who` cannot duck, and it depends on a system
that does not exist yet.

- **v1: every connected, playing session appears**, including link-dead
  characters within their reconnect window (they are still "in the
  world"). Characters still in login or character-creation — not yet
  *playing* — do not appear.
- When **visibility rules** land (currently greenfield — see
  `BACKLOG.md`), `who` becomes a consumer: a character hidden/invisible to
  the viewer is excluded from that viewer's roster and count, exactly as
  an admin walking invisibly should not show in `who`. Until then, no
  character is hidden from `who`, and the actor always sees themselves.
- The exclusion, when it exists, is **per-viewer**: an admin may see
  hidden characters that an ordinary player does not. `who`'s output is
  therefore a function of the viewer, not a global list.

**Acceptance — who appears**

- [x] Every playing session (including link-dead-within-window) appears in
      v1; sessions still logging in or creating do not. *(the SessionManager
      only holds post-login actors — `Add` runs after phase=Playing — so this
      holds by construction; the roster snapshots `byPlayerID`.)*
- [x] The actor always appears in their own `who`.
- [ ] (Forward) once visibility exists, a character hidden from the viewer
      is excluded from that viewer's lines and count. *(seam ready: filtering
      attaches at `managerRoster.OnlineRoster`.)*

---

## 5. Configuration surface

| Setting | Description |
|---|---|
| Roster columns | Which optional columns appear (title / level / idle) (§2). |
| Idle threshold | How long without action before a character is marked idle (§2). |
| Role markers | Whether/how privileged roles are tagged in the roster (§2). |
| Ordering | The default sort (conventionally alphabetical) (§2). |
| Summary wording | The count line's phrasing (§3). |

---

## 6. Open questions

- **Visibility integration.** The shape of per-viewer hiding is pinned to
  the future visibility rules; §4 states the contract, but the exact
  predicate (what hides a character, whether sneak/invisible differ for
  `who` vs. room presence) is settled with that spec.
- **Idle source.** Whether the idle marker reads the same
  last-input timestamp the idle-sweep uses (`session-lifecycle.md` §5) or
  a `who`-specific notion. Reusing the sweep's timestamp is the obvious
  default.
- **Filtered `who`.** Whether `who <substring>` / `who <role>` filtering
  is in scope. Deferred — v1 is the full roster.

---

## Cross-references

- `session-lifecycle.md` §3 (SessionManager — the live roster source),
  §5 (idle timestamp), §7 (link-dead window).
- `roles-and-permissions.md` — role markers / staff visibility in the
  roster.
- visibility rules (greenfield, see `BACKLOG.md`) — the future per-viewer
  hiding predicate (§4).
- `ui-rendering-help.md` — roster line rendering / plain-mode stripping.
