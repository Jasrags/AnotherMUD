# Skills (use-based proficiencies + the skill-check primitive)

EPIC sub-epic **S3** — the skills layer of the WoT Mechanics program
(`docs/themes/wot-mechanics-epic.md`, row S3). Governed by EPIC **Decision 0**
(translate WoT onto the tick/chance model; no d20 rewrite) and the character
model **D3** (`docs/proposals/wot-character-model.md` — skills are use-based
proficiencies, not a point-buy rank system). Builds on `progression`
(proficiency + abilities), the shipped **saves** primitive (`saves.md`, the
resolution shape this mirrors), and the door **lock** system
(`world-rooms-movement`).

## 1. Overview

The WoT RPG has ~40 d20 skills resolved as `1d20 + ranks + ability modifier`
vs a DC (`docs/wot/skills.md`). The engine already proves the translation:
**a skill is a use-based proficiency keyed by an ability id** — exactly how
crafting disciplines (smithing, cooking) already work (proficiency gains with
use, gates a floor, feeds an outcome). So this slice is **not an engine
rebuild**; it generalizes that proven shape and adds the one missing primitive.

This slice ships:

- **A — skills as proficiencies (the convention).** A skill is an ability with
  the existing `skill` category, a **governing stat** (the d20 key ability), and
  a 1–100 proficiency that **gains with use** (the existing `RollUseGain`) and
  is capped/raised by trainers (the existing `practice`/`train` path). No new
  point-buy, no rank-buying (Decision 0 drops the d20 bookkeeping).
- **B — the skill-check primitive.** `ResolveSkillCheck` — the analog of
  `ResolveSave`: `d20 + skill bonus ≥ DC`, where the bonus is derived from the
  character's proficiency plus the governing-ability modifier. One primitive
  every skill consumer calls, instead of each reinventing a check.
- **C — class-skill grants.** Which classes grant which skills (and cap them
  higher) rides the existing class `Path` + trainer tiers. This is the seam
  **backgrounds (S9) hang off** — a background is a skill/feat grant package.
- **D — the first consumer: lockpicking.** A `pick` verb resolves an Open-Lock
  skill check against a door's existing **pick difficulty** — an alternative to
  the key. Proves the primitive end-to-end with a self-contained, satisfying
  feature.

**Goals.** Establish the skill = proficiency convention + the reusable
skill-check primitive so visibility, locks, and backgrounds compose them rather
than reinvent them; ship one real consumer; keep the vocabulary lean.

**Non-goals (this slice).**
- **Hide / Move Silently / Search / Spot / Listen** — these belong to the
  **visibility** spec (§1, write-ahead-of-code). This slice gives them the
  skill-check primitive; it does **not** author them.
- **Craft** — already shipped (crafting disciplines are this pattern).
- **The GM-adjudicated social/knowledge skills** (Bluff, Diplomacy, Sense
  Motive, Knowledge, Perform, Intimidate, …) — a MUD cannot mechanically resolve
  an open-ended social roll; dropped, not stubbed.
- **d20 rank-buying, cross-class half-ranks, skill points, synergy bonuses** —
  the point-buy bookkeeping (Decision 0 / D3). Proficiency *is* the rank.
- **A broad skill catalog.** v1 authors the lockpicking skill + the class-skill
  grant seam; other skills are authored by the systems that own them (a future
  locks/climb slice, visibility) or by backgrounds (S9).

## 2. Skills as proficiencies (A)

A skill is an ordinary **ability** (`abilities-and-effects`) carrying the
existing **`skill` category** plus the metadata a check needs:

- **Governing stat** — the d20 "key ability" whose modifier feeds the check
  (e.g. Dexterity for Open Lock). Read from the ability's existing gain-stat
  declaration this slice (the key ability and the gain stat are the same for the
  shipped skill); a dedicated check-stat field is added only if a skill ever
  needs them to differ.
- **Proficiency** — the 1–100 value stored in the existing `ProficiencyManager`,
  keyed by `(entity, skill ability id)`. It **gains with use** through the
  existing `RollUseGain` (the use-based training loop crafting/gathering already
  use), scaled by the governing stat, and is bounded by a per-skill cap that
  trainers raise (the existing `practice`/`train` path).

A character is **trained** in a skill once they hold proficiency in it (≥ 1);
an **untrained** character has no proficiency and — for a skill that allows
untrained use — checks at proficiency zero (governing-stat modifier only). A
skill flagged **trained-only** cannot be attempted without proficiency.

**Acceptance criteria**

- [ ] A skill is an ability with the `skill` category and a governing stat;
      proficiency is stored + gained through the existing proficiency manager.
- [ ] Using a skill rolls the existing use-based proficiency gain (improves with
      use, scaled by the governing stat, bounded by the cap).
- [ ] A trainer raises a skill's cap via the existing `practice`/`train` path
      (no skill-specific training code).
- [ ] An untrained character may attempt a non-trained-only skill at proficiency
      zero (stat modifier only); a trained-only skill is refused without
      proficiency.

## 3. The skill-check primitive (B)

A single resolution function every skill consumer calls — the analog of
`ResolveSave` (`saves §3`):

```
ResolveSkillCheck(roller, bonus, dc) → { success, roll, total, natural1, natural20 }
```

- Rolls a fresh `d20`; **success** when `roll + bonus ≥ dc`.
- **Natural 1 always fails; natural 20 always succeeds** — the same edges the
  to-hit roll and saves use (`combat §4.4`, `saves §3`), so all three checks
  share one resolution idiom.
- Returns the full roll detail (not just the boolean) so a consumer can render
  the math and a future degrees-of-success consumer can read the margin.

The **skill bonus** is composed from the character's skill state:

```
bonus = proficiency-bonus(proficiency) + AbilityModifier(governing-stat score)
```

- **proficiency-bonus** maps the 1–100 proficiency onto the d20 bonus scale by a
  configurable factor (§7) — a novice contributes ~nothing, a master a large
  bonus. This is the WoT "ranks" term, sourced from use-based proficiency
  instead of point-buy.
- **AbilityModifier** is the existing `(score − 10) / 2` d20 modifier of the
  governing ability (the same helper saves use), read off the live stat block —
  so a Dexterity buff helps a Dexterity skill for free.

The check is **pure over the injected roller** (deterministic under a seeded
roller, like saves), and emits a **skill-resolved event** so a consumer can
narrate the result. The primitive does not itself decide the *consequence* of a
pass/fail — the consumer owns that.

**Acceptance criteria**

- [ ] `ResolveSkillCheck` rolls `d20 + bonus` and succeeds when the total is at
      least the DC.
- [ ] A natural 1 fails and a natural 20 succeeds regardless of bonus/DC.
- [ ] The skill bonus = proficiency-derived bonus + the governing-ability
      modifier, read off the live stat block.
- [ ] The check is deterministic under a seeded roller (no hidden state).
- [ ] Each resolved skill check emits one informational skill-resolved event.

## 4. First consumer: lockpicking (D)

The WoT **Open Lock** skill, resolved against a door's existing **pick
difficulty** (`world-rooms-movement` — doors already carry a `pickable` flag +
a `pick-difficulty` threshold). A `pick` verb is the keyless alternative to
`unlock`.

- **`pick <door>`** (alias `picklock`) targets a door in the room. The door must
  be **lockable + locked + pickable** (a keyless or non-pickable door is
  refused with a fitting message).
- The actor resolves an **Open-Lock skill check** (`ResolveSkillCheck`) with
  their lockpicking bonus against the door's pick difficulty (the DC).
- **On success** the lock opens — the same state transition `unlock`-with-key
  produces, with the two door sides kept in sync (`world-rooms-movement`). A
  success message reads to the actor + room; the skill rolls its use-gain.
- **On failure** the lock stays locked ("You fail to pick the lock."); a
  configurable **retry friction** (a short per-actor cooldown or a noise cue)
  keeps it from being free-retried to certainty in one tick. A failed attempt
  may still roll a (reduced) use-gain so the skill improves even from failure
  (the existing gain-on-miss multiplier).
- An actor with **no lockpicking proficiency** (untrained, and Open Lock is
  trained-only) is refused — "You don't know how to pick locks."
- **Tools (skills.md tool seam, built 2026-06-16).** A carried item declaring
  the skill it assists (`skill_tool: open-lock`) adds its base `skill_tool_bonus`
  to the check — a lockpick aids the pick. A quality grade on the tool adds the
  grade's tool bonus on top (`masterwork §3`). Tools toward one check do **not**
  stack — only the best carried tool's contribution counts. The seam is generic
  (keyed by the skill id) so a future Str/Dex skill reuses it.

Lockpicking changes no existing door mechanic: the key path, lock/unlock verbs,
and door sync are untouched; `pick` is an additional way to clear a lock gated
on skill instead of an item.

**Acceptance criteria**

- [ ] `pick <door>` on a locked, pickable door rolls an Open-Lock skill check
      vs the door's pick difficulty.
- [ ] A successful pick opens the lock (the same transition as a keyed unlock),
      syncs both sides, and narrates to actor + room.
- [ ] A failed pick leaves the door locked and applies the retry friction; the
      skill still rolls its (reduced) use-gain.
- [ ] An untrained actor is refused (Open Lock is trained-only).
- [ ] A non-lockable / unlocked / non-pickable door is refused with a fitting
      message; the key path and `unlock` are unchanged.
- [ ] Picking improves the lockpicking proficiency over repeated use.

## 5. Display

Skills surface without a new framework:

- A **`skills`** listing shows the actor's known skills (the `skill`-category
  abilities they hold) with proficiency and cap — the same data the `abilities`
  view reads, filtered to skills.
- Skill checks narrate through the **skill-resolved event** (the consumer
  renders "You pick the lock." / "You fail to pick the lock.").

**Acceptance criteria**

- [ ] `skills` lists the actor's known skills with proficiency / cap.
- [ ] A resolved skill check produces a player-visible line at its consumer.

## 6. Interaction with existing systems

- **Proficiency / abilities** (`progression`, `abilities-and-effects`): a skill
  is an ability with the `skill` category; proficiency, use-gain, caps, and
  trainer raising all reuse the existing manager + `practice`/`train` path. No
  parallel skill store.
- **Saves** (`saves.md`): `ResolveSkillCheck` mirrors `ResolveSave` (same d20 +
  bonus vs target shape, same natural-1/20 edges, same `AbilityModifier`), so
  the engine has one consistent check idiom across to-hit, saves, and skills.
- **Doors / locks** (`world-rooms-movement`): lockpicking consumes the existing
  `pickable` + `pick-difficulty` door data and the existing unlock transition; it
  adds a verb, not a new lock model.
- **Classes** (`progression`, character-model D3): class-skill grants ride the
  existing class `Path`; **backgrounds (S9)** will grant skill packages through
  the same seam.
- **Crafting / gathering**: already skills-as-proficiencies; unchanged. Their
  outcome rolls are quality-weighted, distinct from the pass/fail skill check
  this slice adds — both are valid uses of a proficiency.
- **Visibility** (§1, write-ahead-of-code): hide/sneak/search/spot are skills
  that will call `ResolveSkillCheck` (often *opposed* — one character's check vs
  another's); this slice ships the un-opposed DC form, and the opposed form is a
  thin extension the visibility slice adds.

## 7. Configuration surface

| Setting | Meaning | Default (engine) |
|---|---|---|
| Proficiency-bonus factor | Maps a 1–100 proficiency onto the d20 skill-bonus scale (§3). | a factor that makes a master skill a large bonus and a novice ~zero |
| Lockpick retry friction | The cooldown / friction applied after a failed pick (§4) so a lock isn't free-retried to certainty. | the WoT pack value |
| Default skill cap | The proficiency cap a freshly learned skill starts at before trainers raise it (§2). | the existing ability default cap |
| Open-Lock pick difficulties | Per-door pick difficulty (the DC) — already door content (§4). | per-door content values |

All numeric magnitudes live here per spec convention; the prose names
behaviors, not values.

## 8. Decisions (resolved at slice start)

- **Check model — d20 + skill bonus vs DC.** Mirrors the shipped `saves`
  primitive (one check idiom across to-hit / saves / skills) and matches the
  WoT `1d20 + ranks + mod` shape; the proficiency supplies the "ranks" term via
  a configurable scale. (Rejected: a proficiency-percentage chance like the
  ability hit-roll — it would give the engine two different check shapes.)
- **Governing stat — reuse the gain stat for v1.** A skill's check stat is its
  gain stat (the same d20 key ability for the shipped skill); a dedicated
  check-stat field is added only if a skill needs them to differ.
- **First consumer — lockpicking.** Open Lock vs the door's existing pick
  difficulty: self-contained, data-ready, and doesn't collide with visibility.
- **Vocabulary — lean.** Author the lockpicking skill + the class-skill grant
  seam; other skills are authored by their owning systems (visibility, a future
  locks/climb slice) or by backgrounds (S9).
- **Skills are proficiencies, not a parallel system.** Reuse the proficiency
  store, use-gain, caps, and trainer path. Proficiency is the rank; no point-buy
  (Decision 0 / D3).

### Still open (non-blocking)

- **Degrees of success** (critical success/failure beyond natural-20/1) — the
  outcome exposes the margin; a consumer that wants graded results can read it.
  Not modeled this slice.
- **Opposed checks** (Hide vs Spot) — the visibility slice's concern; the
  un-opposed DC form ships here and the opposed form composes two checks.
- **A dedicated check-stat field** (when a skill's check stat ≠ its gain stat)
  — add when a real skill needs it.
- **Whether class-skill vs cross-class caps differ** (the d20 "class skill =
  higher cap") — modeled today only as "a class grants the skill (capped
  higher) via Path"; cross-class learning at a lower cap is a content/trainer
  decision deferred until a second class + a shared skill exist.
