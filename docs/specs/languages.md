# Languages — Feature Specification

## 1. Overview

A **language** is a tongue a character can speak, read, and write. Each
character knows a set of languages; their **home language** is granted by their
background at creation, and further languages may be learned later. Languages
are content — packs author them — so the engine stays setting-agnostic while a
setting pack (e.g. Wheel of Time) ships its own roster of tongues and dialects.

This spec covers the **identity substrate**: the language registry, the
per-character known-language set, the background home-language grant, and the
surfaces that display known languages. The two heavier features that build on
it — **comprehension gating** (speech in an unknown tongue rendered as garble)
and **bonus-language selection at creation** (an Intelligence-budget pick) — are
deliberately out of scope here and tracked under Open Questions.

### Core concepts

- **Language** — a registered tongue: a stable id, a display name, an optional
  **family**, and flavor text. Authored per pack alongside races/classes/feats.
- **Family** — a comprehension group. Languages in the same family are mutually
  intelligible; dialects of a shared tongue (e.g. the regional dialects of a
  setting's common speech) declare the same family and differ only in name and
  flavor. A language with no peers may stand alone as its own family. The family
  is the unit a future comprehension check keys on, **not** the individual
  language id — so knowing one dialect of a family implies understanding the
  others. It is authored now for correctness; nothing consumes it yet.
- **Home language** — the one language a background grants free at creation. A
  returning character keeps whatever languages they have learned.
- **Known languages** — the per-character set, persisted on the save. A
  character can read and write every language they know.

### Goals

- A content-driven language registry, loaded and namespaced like other pack
  content, with later-pack / higher-priority override.
- A background grants its home language at character creation, fail-soft if the
  referenced language is absent.
- Known languages persist across sessions and are visible to the player.

### Non-goals

- **Comprehension gating** of speech (say/tell/channel) — deferred.
- **Bonus-language selection** and any Intelligence-budget cost model — deferred.
- **Social mechanics** keyed on dialect (the source material's dialect-mismatch
  penalties on social skills) — the engine has no social-skill system, so these
  have no hook and are not modelled.
- Spoken-vs-signed distinctions beyond what `family` already expresses.

## 2. The language registry

### 2.1 Shape

A language declares an `id`, a `name`, an optional `family`, and an optional
`description`. Ids are lowercased and namespaced to the pack; unqualified
references resolve within the declaring pack's namespace, qualified references
(`other-pack:tongue`) cross packs — identical to every other content registry.

### 2.2 Loading

Languages load from a pack's declared content glob, registered into an
id-keyed registry. Registration is idempotent on id; a higher-priority (or
later-pack) entry replaces a lower one, equal priority no-ops. A language with
no id is a load error.

**Acceptance criteria**

- [ ] A pack that declares a languages glob registers every well-formed entry.
- [ ] A language file with no `id` is rejected at load with a clear error.
- [ ] An unqualified language reference resolves within the pack namespace; a
      qualified one crosses packs.
- [ ] Re-registering an id at equal priority is a no-op; higher priority wins.

## 3. The home-language grant

A background MAY declare a `home_language` (a language id) and a list of
`bonus_languages` (ids a future creation step will let the character choose
from). At character creation, the background granter adds the home language to
the new character's known set. A missing or unregistered home language is
skipped fail-soft — exactly as a missing granted item or feat is — so partial
content never blocks creation.

`bonus_languages` is authored now for fidelity and forward use; no creation step
consumes it in this slice (see Open Questions).

**Acceptance criteria**

- [ ] Creating a character whose background declares a `home_language` leaves
      that language in the character's known set.
- [ ] A background with no `home_language` grants no language and does not error.
- [ ] A `home_language` that does not resolve to a registered language is
      skipped without aborting the rest of the grant.
- [ ] The grant is idempotent: re-running it does not duplicate a known language.

## 4. Known-language display

The player can see the languages their character knows. Known languages appear:

- on a dedicated listing command, each shown by display name (and family where
  it aids the reader); and
- as a row on the character sheet (`score`), summarising the known tongues.

Both surfaces resolve ids to display names through the registry and degrade
gracefully — an id with no registered language is shown by its id rather than
crashing the sheet.

**Acceptance criteria**

- [ ] The listing command shows every language the character knows, by name.
- [ ] The character sheet shows a languages row when the character knows at
      least one language, and omits it cleanly when they know none.
- [ ] A known id with no registered language renders by id, not as an error.

## 5. Persistence

Known languages persist on the player save. The field is additive and
optional; a save written before the feature carries no languages and migrates
forward as an empty set (no character loses or gains a language across the
version bump). Re-saving is stable.

**Acceptance criteria**

- [ ] A character's known languages survive logout/login.
- [ ] A pre-feature save loads with an empty known-language set and no error.
- [ ] The known-language set round-trips through save/load unchanged.

## 6. Configuration surface

| Knob | Meaning | Default |
|---|---|---|
| _(none yet)_ | The identity substrate has no runtime knobs. Comprehension gating and bonus-language budgets will add their own when they land. | — |

## 7. Open questions / future work

- **Comprehension gating.** Tag say/tell/channel/emote with a language and
  render it unintelligible (garbled) to listeners who know no language in its
  family. The family model in §1 is shaped for this. Requires integration into
  the chat/say path and a choice of "current speaking language" per speaker.
- **Bonus-language selection.** A creation step that spends an
  Intelligence-derived budget across a background's `bonus_languages` (the
  source material charges one unit per dialect of the common tongue, two per
  fully separate language). Needs a point-budget multi-select creation step.
- **Learning languages in play.** Whether (and how) a character acquires a new
  language after creation — a trainer, a skill, a teacher NPC, or study time.
- **Literacy split.** The current model assumes read = write = speak for every
  known language. A setting may want illiteracy or speak-only as a distinct
  state; not modelled.
- **Signed vs spoken.** Hand-speech and similar are modelled today purely by
  family; a richer medium axis (cannot be overheard, requires line of sight)
  could matter once comprehension gating exists.
