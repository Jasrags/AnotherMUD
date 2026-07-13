# NPC Dialogue — Feature Specification

**Status:** Draft (shipped) · **Scope:** The `ask <npc> about
<topic>` verb — free-form, content-authored NPC dialogue keyed
by topic, distinct from the quest-giver `talk` verb; the
topic → line content shape (single line or a rotating list); the
optional catch-all fallback; the delegation back to `talk` when
no topic is supplied · **Audience:** Anyone reimplementing or
porting this feature in any language.

This document describes *what* the dialogue surface must do, not
*how* to implement it. The exact verb name, the fallback-topic
key, and the line-rotation policy live in the configuration-
surface table at §7.

Dialogue is the flavor-conversation sibling of the quest
[quests](quests.md) `talk` verb. Where `talk` surfaces a giver's
quest offers and turn-ins, `ask <npc> about <topic>` speaks a
content-authored line — lore, rumor, a hint, a character's
running patter — with **no quest state involved**. The two verbs
share an NPC and coexist on it: the same bartender can hand out a
quest via `talk` and answer `ask` about a dozen topics.

---

## 1. Overview

An NPC may carry a **dialogue table**: a map of topic → spoken
line. The player addresses it with `ask <npc> about <topic>`; the
NPC speaks the line for that topic back to the asking player.

- **Topic-keyed.** Each entry is named by a free-text topic
  (`laws`, `work`, `the shadows`). The player names the topic;
  matching is case-insensitive.
- **Content-authored.** The dialogue table is pack content on the
  mob, not code. Any NPC in any pack can carry one; adding topics
  is a content edit, never an engine change.
- **A line may be single or many.** A topic maps either to one
  fixed line or to a list of lines; a list gives the NPC variety
  on repeated asks (§4).
- **Optional fallback.** A reserved catch-all topic (name in §7)
  answers any topic the table does not otherwise match, so an NPC
  can respond in character to an unknown subject instead of a
  flat refusal.

### 1.1 What NPC dialogue is not

- **Not the quest verb.** `ask` with a topic is dialogue; the
  quest offer/turn-in surface is `talk` (and `ask <npc>` with no
  topic, which delegates — §3.1). Dialogue reads and mutates no
  quest state.
- **Not a conversation tree.** There is no state, no branching,
  no memory of what was asked before. Each `ask` is independent;
  the same topic is answerable any number of times.
- **Not room-scoped.** The spoken line goes to the asking player
  only (§5). Unlike `say`/emotes, other players in the room do
  not see another player's `ask` exchange in v1 (§8).
- **Not persisted.** The dialogue table is rebuilt from content
  every boot; no per-player or per-NPC dialogue state is saved
  (§6).

---

## 2. The dialogue table

An NPC's dialogue table is a content-declared map on the mob. Each
entry is:

- **topic** — the map key: a free-text subject the player names
  after "about". Matching is case-insensitive and whitespace-
  trimmed on both sides, so a `laws` entry answers `ask doug
  about LAWS` and `ask doug about  laws `.
- **line** — the value: either a single spoken line, or a list of
  lines (§4 governs which is chosen).
- **fallback** — one reserved topic key (name in §7) is the
  catch-all for any unmatched topic. It is an ordinary entry;
  authoring it is optional.

### 2.1 Content shape

```
dialogue:
  laws:                       # a list → varies on repeat (§4)
    - "Bury the dead. They stink up the joint."
    - "Anything else is always something better."
  work: "Talk to the suit in the booth. I just pour."   # a single line
  default: "I've got a law for most things, but not that."  # fallback (§7)
```

Rules:

- A table with no entries is legal (the NPC simply has nothing to
  say about anything, §3 step 5).
- An empty list value for a topic is legal and behaves as "no
  line" (falls through to the fallback if one exists, §3 step 4).
- Topic keys are content; the engine reserves only the single
  fallback key name (§7). No other key is special.

### Acceptance — table

- [ ] An NPC with no dialogue table answers every `ask … about`
      with the nothing-to-say response (§3 step 5).
- [ ] Topic lookup is case-insensitive and whitespace-trimmed on
      both the authored key and the player's topic.
- [ ] A single-line value and a list value are both valid for any
      topic.
- [ ] An empty-list topic value resolves as "no line" and falls
      through to the fallback when one exists.

---

## 3. Verb dispatch

The verb is `ask <npc> about <topic>` (canonical name in §7). Its
flow:

1. Split the argument on the separator word "about" (case-
   insensitive). Everything before it is the NPC term; everything
   after is the topic.
2. If there is no separator, delegate to the quest `talk` flow
   (§3.1) — this is the no-topic case.
3. If the NPC term is empty, return the ask-whom prompt. If the
   topic is empty (a trailing "about" with nothing after), return
   the about-what prompt. Neither broadcasts.
4. Resolve the NPC by keyword, scoped to the actor's current room,
   via the shared keyword resolver
   ([commands-and-dispatch](commands-and-dispatch.md)). An
   unresolved term returns the no-such-NPC response.
5. Look up the topic in the resolved NPC's dialogue table
   (§2). On a hit, select a line (§4) and speak it (§5). On a
   miss, try the fallback topic; if that also misses (or the NPC
   has no table), return the nothing-to-say response naming the
   NPC.

### 3.1 No-topic delegation

`ask <npc>` with no "about" clause is a synonym for `talk <npc>`:
it routes into the quest-giver flow ([quests](quests.md) §3/§4.3)
— surfacing offers and claiming ready turn-ins. This preserves the
older behavior where `ask` was a bare alias of `talk`, so a player
who types `ask bartender` still discovers a quest. Only the
presence of a topic routes into dialogue.

### Acceptance — dispatch

- [ ] `ask <npc> about <topic>` with a matched topic speaks the
      topic's line.
- [ ] `ask <npc>` with no "about" clause behaves identically to
      `talk <npc>` (quest offers / turn-in).
- [ ] `ask <npc> about` with an empty topic returns a prompt and
      does not speak a line.
- [ ] An unresolved NPC term returns the no-such-NPC response and
      does not speak.
- [ ] A matched NPC with no line for the topic and no fallback
      returns the nothing-to-say response naming the NPC.
- [ ] NPC resolution is scoped to the actor's room; cross-room
      asking never resolves.

---

## 4. Line selection

When a topic's value is a **single line**, that line is spoken
verbatim. When it is a **list of lines**, one line is chosen per
ask:

- Repeated asks about the same list topic vary across the list, so
  a character with several one-liners (a bartender's aphorisms)
  does not repeat the same one every time.
- Selection is a function of the engine time source: at a given
  instant the choice is fixed, but it changes as time advances.
  This keeps selection deterministic under a fixed/simulated clock
  (so it is testable) while varying in live play. The exact
  rotation policy — sequential, time-indexed, or random — is
  configuration (§7), not a behavioral guarantee. The only
  guarantees are: a list of length one always yields its single
  line, and selection never yields an out-of-range or empty line
  when the list is non-empty.

### Acceptance — selection

- [ ] A single-line topic always speaks that exact line.
- [ ] A list topic always speaks one of its lines and never an
      empty/out-of-range result for a non-empty list.
- [ ] Under a fixed simulated time source, list selection is
      deterministic (same instant → same line) — the property the
      test suite relies on.

---

## 5. Rendering

The chosen line is delivered to the **asking player only**, phrased
as NPC speech attributed to the NPC (the NPC's display name plus a
speech verb, e.g. `<npc> says, "<line>"`). Exact phrasing and any
markup are presentation (§7 / [ui-rendering-help](ui-rendering-help.md)).

- The line text is authored prose; the engine does not
  interpret, escape, or reflow it. Authors own their grammar and
  punctuation.
- No other observer in the room receives the line in v1 (§8).

### Acceptance — rendering

- [ ] The line is sent to the asking player, attributed to the
      NPC by display name.
- [ ] The authored line text is delivered without mangling
      (multi-byte, punctuation, decoration preserved).

---

## 6. Persistence

**NPC dialogue does not persist.** The dialogue table is rebuilt
from content on every boot. There is no dialogue history, no
per-player "topics heard" record, no per-NPC cooldown. Nothing
under `saves/` is written or read by the dialogue subsystem.

### Acceptance — persistence

- [ ] No file under `saves/` is written or read by the dialogue
      subsystem.

---

## 7. Configuration surface

| Setting | Default | Meaning |
|---|---|---|
| Verb name | `ask` | Canonical dialogue verb; also the no-topic quest synonym (§3.1). |
| Topic separator | `about` | The word that splits `<npc>` from `<topic>`; matched case-insensitively. |
| Dialogue declaration | a `dialogue` map on the mob's content | Topic → line(s). Authored per NPC; no engine change to add topics. |
| Fallback topic key | `default` | Reserved topic answering any unmatched subject; optional per NPC. |
| Topic match | case-insensitive, whitespace-trimmed | Both the authored key and the player's topic are normalized before comparison. |
| List rotation policy | time-indexed | How a list topic picks a line per ask; deterministic under a fixed time source (§4). |
| Delivery scope | asker-only | Who sees the spoken line. Room-scoped delivery is an open question (§8). |
| Speech attribution | `<npc> says, "…"` | Presentation of the delivered line. |

---

## 8. Open questions

- **Room-scoped delivery.** v1 delivers the line to the asking
  player only. Whether bystanders in the room should also see an
  NPC answer a question (as they would overhear a `say`) is
  deferred — it interacts with flood/spam concerns on busy hubs.
  Revisit if eavesdropping-on-NPC-chatter becomes a desired
  texture.
- **Ambient/unprompted barks.** A separate, richer feature: an
  NPC volunteering lines to the room on a timer, without being
  asked. Out of scope here — it needs a room-broadcast path from
  the mob tick (and, if scripted, a widening of the Lua bridge,
  which the sandbox deliberately does not expose today). Pin in
  its own spec if pursued.
- **Topic discovery.** v1 has no in-game way to list an NPC's
  known topics; the player asks blind (or the NPC's description /
  fallback hints at subjects). A `topics`-style affordance or
  keyword highlighting in NPC prose could land later.
- **Quest / state-gated lines.** All lines are static content in
  v1. Conditioning a line on quest progress, faction standing, or
  time of day is a plausible extension but explicitly not in v1
  scope.
- **Scripted dialogue.** A line computed by a pack Lua handler
  (rather than picked from static content) would let dialogue
  react to world state. Deferred with the scripting bridge's
  room-output question above.

---

## Cross-references

- `quests` — the `talk` verb this delegates to when no topic is
  given; the quest-giver interaction dialogue is deliberately
  *not*.
- `commands-and-dispatch` — verb registration and the shared
  keyword resolver used to resolve the NPC in the room.
- `mobs-ai-spawning` — the mob template + property bag the
  dialogue table rides in.
- `ui-rendering-help` — presentation of the spoken line.
- `scripting-and-packs` — the sandbox boundary referenced by the
  ambient-barks / scripted-dialogue open questions.
- `docs/specs/README.md` — spec layer placement and cross-cutting
  indexes.
