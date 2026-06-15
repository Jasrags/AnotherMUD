# Feats (player-chosen passive perks)

EPIC sub-epic **S4** — the player-choice perk layer of the WoT Mechanics
program (`docs/themes/wot-mechanics-epic.md`, row S4). Governed by EPIC
**Decision 0** (translate WoT onto the tick/chance model; no d20 rewrite) and the
character model **D2** (`docs/proposals/wot-character-model.md` — *authored* class
features ride the `Path` grant; the player-*choice* selection engine is this
slice). Builds on the shipped **abilities-and-effects** passives, the
source-keyed stat-modifier surface (`progression` §2.4), the **saves**
(`saves.md`) and **skills** (`skills.md`) consumers, and the level-up training-
credit pattern (`progression` §5).

This document describes *what* the feat substrate must do, not *how*. Specific
feat content (which perks exist, their bonus magnitudes), the credit cadence, and
the prerequisite thresholds are policy and live outside this spec.

---

## 1. Overview

A **feat** is a player-*chosen* passive perk. Unlike a skill it has no ranks —
you either hold it or you don't. Unlike a class `Path` grant (which the engine
hands you automatically at a level) a feat is *selected* by the player from an
eligible pool. The feat feature is the reusable substrate that classes,
backgrounds, and (later) other systems hang their player-choice perks off; the
catalog of actual feats is content.

### Core concepts

- **Feat definition** — content-defined declaration: id, presentation,
  prerequisites, a multi-take rule, an optional allowed-class gate, and a grant
  payload (the passive it confers).
- **Prerequisite** — a gate the holder must satisfy to take the feat: a minimum
  ability score, a required feat (the prereq graph), a skill/proficiency floor,
  or a character-level floor.
- **Multi-take rule** — whether a feat may be taken once, multiple times each
  bound to a distinct parameter, or multiple times with a stacking count.
- **Feat credit** — a banked, spendable slot. Credits are earned (at creation
  and as the character levels) and spent to take a feat.
- **Grant** — the passive a feat confers, expressed entirely in the existing
  source-keyed modifier vocabulary; a feat never invents a new modifier path.
- **Known feats** — the per-character record of feats taken (with parameter and
  stack count), persisted with the entity. It is the **source of truth**; the
  conferred bonuses are derived from it, never separately stored.

### Goals

1. A registry of feat definitions identifiable by stable ids, overridable by
   pack priority.
2. A pure eligibility check over a character snapshot (prerequisites + class
   gate).
3. Banked feat credits earned on a configurable cadence and spent via a verb,
   never via a blocking mid-combat prompt.
4. Award-time enforcement of the multi-take rule, with parameter and count.
5. A grant bridge that confers passives through the existing source-keyed
   modifier surface, recomputed from known feats on load.
6. An authored grant path (background/class) that reuses the same award
   machinery without a player choice.

### Non-goals

- The specific feat catalog and bonus magnitudes (policy/content).
- Any d20 action-economy change (attacks of opportunity, full-attack
  sequences, initiative). Decision 0 refuses these; feats that depend on them
  are deferred to their owning sub-epic.
- Dynamic prerequisite re-validation: prerequisites are checked when a feat is
  taken, not continuously re-evaluated when an ability later changes (§12).

---

## 2. Feat definitions

### 2.1 Registration

Feats are registered into a single **global** feat registry keyed by id (ids are
lowercased on registration). The registry is not namespaced — a pack authors
feats into the shared vocabulary. When two registrations share an id, the
**higher-priority registration wins**; equal priority is a no-op (first
registration stands). This lets a pack override a baseline feat without
renaming.

The registry exposes lookups: "get by id" (case-insensitive), "has id", and "all
feats" as an id-sorted snapshot.

### 2.2 Fields

A feat definition carries:

- **Identity** — a stable id, a display name (falling back to the id), and a
  description, used by the selection menu and the listing verb.
- **Prerequisites** — zero or more gates (§3).
- **Multi-take rule** — one of the three rules in §4; defaults to *single* when
  omitted.
- **Allowed-class gate** — an optional list of class ids. Empty means any class
  may take the feat; non-empty restricts it to holders of at least one listed
  class.
- **Grant payload** — zero or more grants (§7) describing the passive conferred.

The grant payload is **not** persisted on the character; only the fact that the
feat was taken is (§8). The definition is the authority for what a held feat
confers, re-read every load.

**Acceptance criteria**

- [ ] Registration is by stable, lowercased id into a single global registry.
- [ ] Higher-priority registration wins on duplicate id; equal priority is a
      no-op.
- [ ] Lookups (`get`, `has`, `all`) are stable and case-insensitive on id.
- [ ] A feat with no grant payload registers and can be taken; it simply
      confers nothing.

---

## 3. Prerequisites and eligibility

### 3.1 Prerequisite kinds

A prerequisite is one of:

- **Ability score** — a named ability must be at least a minimum value.
- **Required feat** — the holder must already hold a named feat (this edge is
  what forms the prerequisite graph; a feat may require another feat).
- **Skill / proficiency floor** — a named skill/ability proficiency must be at
  least a minimum value.
- **Character-level floor** — the holder's character level must be at least a
  minimum. (Source material's "base attack bonus" prerequisites map onto
  character level under Decision 0, since the engine has no BAB.)

### 3.2 The character view

Eligibility is computed by a **pure function** over a read-only character
snapshot. The snapshot exposes: an ability score by name, a skill proficiency by
ability id, whether a feat is already held, the character level, and the held
class ids. The function reads only the snapshot and has no side effects, so it
never re-enters the entity's lock and can be evaluated freely (menus, listings,
validation).

### 3.3 Eligibility result

Evaluating a feat against a character yields a structured result:

- an **ok** flag — true when every prerequisite is met AND the class gate passes
  (empty gate, or at least one held class is listed);
- the list of **unmet prerequisites**, in declaration order (so the first
  failure can be reported); and
- a **class-excluded** flag — true when the gate is non-empty and no held class
  matches.

**Acceptance criteria**

- [ ] Eligibility is a pure function over a character snapshot; it does not
      mutate state.
- [ ] All four prerequisite kinds are evaluated; the result lists the unmet
      ones in declaration order.
- [ ] An empty allowed-class gate admits any class; a non-empty gate admits only
      holders of a listed class and otherwise reports class-excluded.

---

## 4. Multi-take rules

A feat declares one of three multi-take rules, enforced at award time (§6):

- **Single** — may be taken at most once. A second attempt is refused.
- **Per-parameter (no stack)** — may be taken multiple times, each bound to a
  distinct **parameter** (a weapon category, a skill id, …). The parameter is
  required; effects do not stack — each take is a separate, parameter-scoped
  instance.
- **Stackable (counted)** — may be taken repeatedly; each take increments a
  **count** on the single known-feat entry, and the conferred bonus applies once
  per count.

A parameter is normalized (trimmed, lowercased) and stored on the known-feat
entry for per-parameter feats; it is empty for single and stackable feats. A
count is meaningful only for stackable feats (a missing or non-positive count is
treated as one).

**Acceptance criteria**

- [ ] A single feat already held cannot be taken again.
- [ ] A per-parameter feat requires a non-empty parameter and records a separate
      instance per parameter.
- [ ] A stackable feat increments a count on one entry and applies its bonus per
      count.

---

## 5. Feat credits

Feats are **earned then spent**, mirroring how training credits already work.

- A character receives credits at **creation** (a configurable starting count)
  and **as it levels**, one credit per a configurable level interval (e.g. every
  third character level), credited from the same level-up signal trains use.
  Background and class grants (§6) may also award credits.
- Credits **bank**: a level-up never pops a blocking prompt (level-ups fire
  mid-combat). The credit sits unspent until the player chooses to spend it,
  exactly as trains wait for a training verb. This avoids the only hard UX
  problem — choosing a perk mid-combat — by not choosing mid-combat.
- The credit count is read for display, incremented when credits are earned, and
  decremented when a feat is taken. All mutations are atomic with respect to the
  entity.

**Acceptance criteria**

- [ ] Credits are granted at creation and on the configured per-level cadence,
      from the level-up signal.
- [ ] Earned credits bank rather than forcing an immediate choice.
- [ ] Taking a feat decrements the credit count atomically.

---

## 6. Taking a feat

Taking a feat — whether by the player verb or an authored grant — follows one
award path:

1. **Resolve the feat** by id. Unknown id is refused.
2. **Check eligibility** (§3). A failed prerequisite or class gate is refused
   with a human-readable reason.
3. **Apply the multi-take guard** (§4): a single feat already held is refused; a
   per-parameter feat requires a parameter.
4. **Spend a credit** when the take is a player selection. (An authored grant —
   below — does not consume a credit.)
5. **Record the take** in the known-feats list: append an entry (single /
   per-parameter) or increment the count (stackable).
6. **Recompute grants** (§7) so the conferred bonuses take effect immediately.

**Authored grants.** A background or class MAY confer a fixed feat at creation.
The authored grant reuses steps 1–3, 5–6 of the award path but **skips the
credit cost** (step 4) and the player choice — it is the same record-and-confer
machinery without a selection. This is how a background's free feat lands.

**Acceptance criteria**

- [ ] The same award path serves both the player verb and authored grants.
- [ ] A player take consumes one credit; an authored grant consumes none.
- [ ] An ineligible take is refused with a reason and mutates nothing (no record,
      no credit spent).
- [ ] A successful take records the known feat and the conferred bonus is live
      without a reload.

---

## 7. The grant bridge

A taken feat confers its passive through the **existing source-keyed modifier
surface** (`progression` §2.4) — the same stacking and recompute machinery
equipment and class growth use. A feat never adds a new modifier path; it writes
to a surface a consumer already reads, under a feat-scoped source key so it can
be replaced cleanly on recompute.

The grant kinds wired today, each riding an already-shipped consumer:

| Grant kind | Effect | Consumer |
|---|---|---|
| Save bonus | +N on a named save axis | saves (`saves.md`) |
| Vital bonus | +N max HP (stackable counts) | vitals (`progression` §2) |
| Hit bonus, per weapon | +N to-hit with a weapon category | weapon hit-mod chain (`weapon-identity.md`) |
| Crit-threat widen, per weapon | widen the per-weapon threat range | crit step (`weapon-identity.md`) |
| Skill bonus, per skill | +N to a named skill check | skills (`skills.md`) |
| Ability unlock | teaches/unlocks a named ability | abilities (`abilities-and-effects.md`) |

The grant set is engine-extensible (a new kind wires a new consumer) but is not
open to content beyond the declared kinds.

**Derived, not stored.** The conferred bonuses are **recomputed from the known-
feats list on every load** and after every take/grant — known feats are the only
persisted truth. A removed-content feat (an id no longer in the registry) is
skipped fail-soft during recompute rather than erroring. An ability unlock is
idempotent: a feat that grants an ability the holder already trained does not
reset its proficiency.

**Acceptance criteria**

- [ ] Feat bonuses are applied through the source-keyed modifier surface under a
      feat-scoped source key.
- [ ] Bonuses are recomputed from known feats on load and after each take, never
      separately persisted.
- [ ] A stackable feat's bonus is applied once per count; a per-parameter feat's
      bonus is scoped to its parameter.
- [ ] A known feat whose definition is no longer registered is skipped without
      error.
- [ ] An ability-unlock grant does not reset the proficiency of an ability the
      holder already trained.

---

## 8. Persistence

The character save carries two feat fields:

- the **known-feats list** — each entry a `(feat id, parameter, count)` triple
  (parameter for per-parameter feats, count for stackable); and
- the **banked feat-credit count** (an integer).

Both are elided when empty/zero. The conferred bonuses are **not** persisted —
they are derived from the known-feats list at load (§7).

The fields were added by an **append-only save migration**; a pre-migration save
loads with no known feats and zero banked credits (the correct default for a
character that predates feats). The migration MUST NOT rewrite older entries.

**Acceptance criteria**

- [ ] Known feats and the banked-credit count persist with the character; the
      conferred bonuses do not.
- [ ] A save predating feats loads cleanly with no feats and zero credits.
- [ ] The feat fields round-trip (parameter and count preserved) across
      save/load.

---

## 9. Selection surface

Two verbs expose the feature to players (the command grammar itself is the
commands feature's concern; this spec fixes their behavior):

- A **listing verb** shows the holder's known feats (with parameter / count),
  the number of banked credits, and — when credits are available — the feats the
  holder is currently eligible to take (single feats already held are filtered
  out; per-parameter and stackable feats may reappear).
- A **spend verb** takes a feat id and an optional parameter, runs the award path
  (§6), and reports success or the refusal reason. With no argument it falls back
  to the listing.

**Acceptance criteria**

- [ ] The listing shows known feats, banked-credit count, and eligible-to-take
      feats (filtered by multi-take rule).
- [ ] The spend verb runs the award path and reports a reason on refusal.

---

## 10. Observable events

The feature does not currently emit a dedicated "feat taken" bus event; a feat
take mutates the stat block in place (under the feat source key) and persists.
Feat credits are earned from the existing level-up signal. Whether feat
transitions should emit their own observable event is an open question (§12).

---

## 11. Configuration surface

The following are externally configurable and not fixed by this spec.

| Policy | Where it applies |
|---|---|
| Feat credits granted at creation | §5 |
| Character-level interval per earned credit | §5 |
| The feat catalog (ids, prerequisites, multi-take rules, grants) | §2 |
| Prerequisite thresholds (ability/skill/level minimums) | §3 |
| Grant magnitudes per feat | §7 |
| Allowed-class gates per feat | §2.2 |

---

## 12. Open questions / future work

- **Multiclass credit cadence.** The per-level credit is currently counted per
  class track, so a multiclass character earning level boundaries on more than
  one track can over-earn credits. Feat progression SHOULD key on **total
  character level**, not per-track level. Fix when a second bound track (true
  multiclass content) ships.
- **Creation-time feat pick.** The creation wizard does not yet offer an initial
  feat selection; the credit granted at creation is spent later via the verb.
  A wizard feat step is a UX refinement, not a correctness gap.
- **Dynamic prerequisite suspension.** Prerequisites are validated only when a
  feat is taken; a later ability drain below a threshold does not suspend the
  feat. Build a lock-on-drain refinement only if ability drain becomes common.
- **Authored feat *choice*.** Background/class grants are fixed feat ids today.
  "Choose one feat from a pool" at creation is a later refinement on the same
  award path.
- **Per-pool credits.** There is one general credit pool. Source material
  separates general, channeling, and free-class slots; additional pools are
  added by their owning sub-epic (the channeling pool reuses this engine).
- **Feat-taken event.** No dedicated event is emitted on take. If observers
  (quests, achievements, admin audit) need one, add it as an observable event.

---

<!-- Scope: feat.Registry + Eligibility + MultiTake + credit cadence + grant bridge (srckey.Feat) + known-feats persistence + feat/feats verbs · Spec style: narrative + acceptance criteria · Detail level: behavior only -->
