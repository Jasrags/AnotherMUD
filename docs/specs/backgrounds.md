# Backgrounds (character-creation origin + starting package)

EPIC sub-epic **S9** (the backgrounds half) — the character-build origin layer
of the WoT Mechanics program (`docs/themes/wot-mechanics-epic.md`, row S9).
Governed by EPIC **Decision 0** (translate WoT onto the tick model) and the
character model **D2/D3** (`docs/proposals/wot-character-model.md` — class/
background grants ride the `Path` mechanism; skills are use-based proficiencies;
player-chosen feats deferred). Builds on the shipped **skills** (`skills.md`),
the **classID-list** seam, and the existing race/class registries + creation
wizard.

## 1. Overview

A character today picks a **race** and a **class** at creation. A **background**
is the third axis — the character's *origin* (a homeland, a former trade) — and
the main lever of creation-time variety: with one class, two characters differ
mostly by where they came from. A background grants a **starting package** that
makes that origin mechanically real.

The WoT RPG background (`docs/wot/backgrounds.md`) grants a feat, background
skills, languages, and an equipment package. Mapping onto the engine, only part
is buildable today:

- **Skills** — ✅ a background teaches starting skill proficiencies (reuse the
  class `Path`/`Teach` grant; skills shipped in `skills.md`).
- **Starting gold** — ✅ added to the new character's balance.
- **Starting items** — ✅ a small **new** mechanism: a background declares item
  template ids that are placed in the new character's inventory at creation
  (no starting-loadout system exists today — not even class starting gear; this
  introduces it).
- **Background feat** — ✅ shipped (S4 feat-selection + the pick-one chooser).
- **Languages** — ✅ shipped (the home-language grant — see languages.md).
- **Weapon restriction** — ✅ shipped as an **equip refusal** (the Aiel sword
  taboo): a background lists forbidden weapon categories + an in-character
  message, and the equip path blocks wielding them. The d20 XP-gate framing was
  dropped (low-value bookkeeping with no engine hook); the equip-block is the
  faithful, legible form of "will not touch a sword."
- **Skill restriction / required skill** — ⛔ deferred (key on skills the engine
  has no equivalent for — Ride, Profession).

So a v1 background grants the **buildable trio — skills + items + gold**. It is
chosen once at creation, persisted for display, and never re-applied.

**Goals.** Add the creation-time origin axis using the existing race/class
machinery + the shipped skills grant; introduce the one new mechanism (a
starting item/gold package); keep the vocabulary lean and the WoT-specific
homelands a later content concern.

**Non-goals (this slice).** Background **feats** (S4), **languages** (no system),
**skill/weapon restrictions** (deferred). **Changing** a character's background
after creation (it is fixed, like race). The **WoT homelands** themselves —
v1 ships generic, class-agnostic demo backgrounds in the engine baseline; the
setting-specific homelands (Aiel, Tairen, …) are authored later in the WoT
content pack. The d20 **class-skill-cap nuance** (a background's skills cap
higher than cross-class) — a v1 background grants the skill at its ordinary
cap; the higher-cap rule is deferred until cross-class caps are modeled.

## 2. The background definition

A background is content-defined (a registry mirroring races/classes), declaring:

- **Identity** — id, display name, a short tagline, a description (the flavor a
  selection UI and `score` show).
- **Skills** — a list of skill grants: an ability id and a starting proficiency
  (default the baseline "trained" value). Each is taught to the character at
  creation via the same grant seam class `Path` entries use.
- **Items** — a list of item template ids placed in the new character's
  inventory at creation. Unknown ids are skipped (fail-soft, like other
  content references resolved at spawn).
- **Gold** — an amount added to the new character's starting balance.
- **Eligibility** — optional race-category / gender gates (mirroring a class's
  `AllowedCategories`/`AllowedGenders`), so a background can be offered only to
  some characters. Empty = available to all.

A background is **independent of class** (the WoT source does not gate class on
background; a background only influences fit). A character has **exactly one**
background.

**Acceptance criteria**

- [ ] A background declares identity, skill grants, item grants, a gold grant,
      and optional eligibility; all grants are optional (an empty background is
      pure flavor).
- [ ] Backgrounds load from content into a registry mirroring the race/class
      registries (register / get / all / eligibility filter).
- [ ] An unknown skill ability id or item template id in a background is skipped
      at grant time, not a load error (fail-soft).

## 3. Choosing a background

When backgrounds are loaded, the **creation wizard** offers a background choice
(after race + class). The character picks **one**; eligibility filters the
options to those the chosen race/gender allow (mirroring class eligibility). A
character created with no backgrounds loaded simply has none (background-less,
the same way a classless character works).

The choice is **final** — there is no background-swap verb (unlike the quest
class-unlock); a background is an origin, fixed at creation.

**Acceptance criteria**

- [ ] The creation wizard presents a background choice when backgrounds exist,
      after the race and class steps.
- [ ] Only backgrounds whose eligibility accepts the character's race category +
      gender are offered.
- [ ] Exactly one background is chosen; it is recorded on the character.
- [ ] With no backgrounds loaded, creation proceeds with no background step and
      the character is background-less.

## 4. The grant (once, at creation)

A background's grants apply **exactly once, at character creation**, and are
**never re-applied** on subsequent logins (a background that re-granted gold or
items each login would be a duplication exploit):

- **Skills** are taught into the proficiency system at the declared starting
  proficiency — the same grant path class features use (`Teach`); a skill the
  character already holds is not reset (the existing single-grant guard).
- **Items** are placed in the new character's inventory as part of its initial
  persisted state.
- **Gold** is added to the new character's starting balance.

The granted skills, items, and gold then persist and evolve **independently** of
the background — the background id is a label, not a live multiplier. Removing a
background from content later does not strip a character's already-granted
package (the grants are theirs); it only means the label no longer resolves
(fail-soft display, like a removed race/class).

**Acceptance criteria**

- [ ] At creation, a background's skills are taught, its items placed in
      inventory, and its gold added to the balance — once.
- [ ] Logging in again does **not** re-apply any grant (no duplicate items/gold,
      no proficiency reset).
- [ ] The granted package persists and changes independently of the background
      label thereafter.
- [ ] A returning character whose background was removed from content loads
      cleanly (keeps its package; the label degrades gracefully).

## 5. Persistence + display

- The chosen background id is stored on the **player save** (a new scalar field,
  alongside race), via an **append-only save migration** (the next version):
  an existing save with no background field decodes to "no background", the
  correct default. The granted package (skills, items, gold) already persists
  through the existing proficiency / inventory / gold save surfaces.
- The **`score`** sheet shows the background alongside race + class (the
  character's origin reads as part of their identity).

**Acceptance criteria**

- [ ] The background id round-trips through the save; a pre-migration save loads
      as background-less.
- [ ] `score` shows the character's background.

## 6. Interaction with existing systems

- **Races / classes** (`progression`, character-model): backgrounds are a third
  registry of the same shape, applied at creation like race/class; the
  background `Skills` grants reuse the class `Path`/`Teach` seam.
- **Skills** (`skills.md`): a background's skill grants are exactly skill
  proficiencies — the convention that slice established. Lockpicking, crafting
  disciplines, and any future skill are grantable by a background.
- **Creation wizard** (`character-creation`): one new choice step; the commit
  records the background id and applies the package, mirroring how race/class
  ids are committed.
- **Economy** (`economy-survival`): the gold grant uses the existing balance;
  the item grant introduces the first starting-inventory mechanism (which a
  later slice could extend to class starting gear).
- **Persistence** (`persistence`): a new save field + an append-only migration,
  mirroring the recent class-list migration.

## 7. Configuration surface

| Setting | Meaning | Default (engine) |
|---|---|---|
| Background skill grants | The skills + starting proficiency a background teaches (§2). | per-background content |
| Background item grants | The item template ids a background places in inventory (§2). | per-background content |
| Background gold grant | The starting gold a background adds (§2). | per-background content |
| Default starting proficiency | The proficiency a granted skill starts at when a background omits one (§2). | the baseline "trained" value |

All numeric magnitudes live here per spec convention; the prose names behaviors,
not values.

## 8. Decisions (resolved at slice start)

- **Vocabulary — generic demo backgrounds.** v1 ships a few class-agnostic
  backgrounds in the engine baseline (e.g. a soldier, a thief, a smith, a
  wanderer) that reuse the shipped skills + items, so the system is exercised in
  the demo world. The WoT homelands are authored later in the WoT content pack.
- **Grant surface — skills + items + gold.** The buildable trio. Feats and
  languages are blocked (S4 / no system); restrictions are deferred low-value
  bookkeeping. A stat tweak / trait-effect grant is **not** included (it overlaps
  the deferred feat system; add it only if a real background needs it).
- **Persisted, scalar.** One background per character, stored as a scalar save
  field (not a list — unlike multiclass) with an append-only migration; shown on
  `score`.
- **Grants once at creation, never re-applied.** The package is a creation event;
  the persisted skills/items/gold are what carry forward.
- **Reuse the grant seam.** Skill grants ride the class `Path`/`Teach` mechanism;
  the background registry mirrors the race registry. Only the starting-item
  mechanism is new.

### Still open (non-blocking)

- **Class starting gear** — the starting-item mechanism this introduces could
  give classes a starting-loadout too (today neither race nor class grants
  items). Generalize when a class wants it.
- **Background feats** — ✅ **authored fixed grant SHIPPED** (EPIC S4 Phase 5,
  2026-06-11): a background's `feats:` list grants those feat ids free at
  creation (no slot, no prereq), recorded + applied like a taken feat (the
  soldier grants Great Fortitude). The remaining refinement is the WoT
  *choose-from-a-background-feat-list* model — a player pick from a
  background-specific pool, deferred until that breadth is wanted.
- **Class-skill-cap nuance** — a background's skills capping higher than
  cross-class (the d20 "they become class skills") waits on cross-class caps.
- **WoT homelands** — the setting-specific backgrounds (with their feat/language
  packages) are wot-pack content for when those systems exist.
- **Weapon restriction** — ✅ shipped as an equip refusal (a background's
  `weapon_restrictions` list of forbidden weapon categories + a
  `weapon_restriction_message`; the equip path blocks wielding them, derived from
  the background registry at login, no save field). The Aiel forbid swords.
- **Skill restriction / required skill** — still deferred: these key on Ride /
  Profession skills the engine has no equivalent for. Revisit if those skills
  land.
