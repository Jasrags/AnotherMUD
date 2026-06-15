# Proposal: Feats — a player-chosen passive-perk selection engine (EPIC S4)

**Status:** SHIPPED 2026-06-11 (8 reviewed phases, save v20) · **Type:** EPIC sub-epic S4
**Engine contract (setting-agnostic):** the reusable selection engine described
below — registry, prerequisite/eligibility evaluation, multi-take rules, banked
feat credits, the grant bridge, and persistence — is now specified in
[`docs/specs/feats.md`](../specs/feats.md). This proposal remains the record of
the **WoT-specific** content: the feat catalog (Great Fortitude, Toughness,
Weapon Focus, …), the content triage (§3 — what ships vs. defers to which
sub-epic), and the scoping decisions. Numeric content (credit cadence, bonus
magnitudes) is policy, not in the engine spec.
**Feeds:** the deferred half of **S9** (background/class feats), perk depth for
**S1** (weapon feats), and the slot model **S2** channeling feats will reuse.
**Governed by:** EPIC **Decision 0** (translate WoT onto the tick/chance model;
no d20 rewrite) and character-model **D2** (`docs/proposals/wot-character-model.md`
— class features are authored `Path` grants; the feat-*selection* engine was
deferred to this slice).
**Source:** `docs/wot/feats.md` (WoT RPG ch. 5).

## 1. What S4 is

A **feat** is a player-*chosen* passive perk: unlike a skill it has no ranks —
you either have it or you don't (`feats.md` §Overview). Today the engine only
does *authored, deterministic* grants (a class `Path` hands you an ability at a
level; a background hands you a starting package). There is **no mechanism for
the player to pick a perk from an eligible pool.** S4 builds exactly that —
the reusable substrate that classes, backgrounds, and (later) channeling all
hang their player-choice perks off.

**Decisions locked at scope (2026-06-11):**
- **Full selection engine** — registry + slot accounting + prereq graph +
  multi-take/stackable + *both* creation choice and the every-3-levels
  level-up pick + a save migration. The complete S4, not a creation-only MVP.
- **Static-bonus feat family only** for v1 content — the feats whose target
  system already exists. Everything needing an unbuilt subsystem is deferred to
  that subsystem's sub-epic (§3).

**What S4 is NOT:** it is not a combat-model change. It does not add attacks of
opportunity, full-attack sequences, initiative, 5-ft steps, or any d20 action
economy — those are exactly what Decision 0 refuses, and ~half the d20 feat
list assumes them (§3). It does not build ranged combat, armor depth, the One
Power, reputation, or Tel'aran'rhiod; feats that need those wait on their owning
sub-epic. v1 ships the *engine* plus a small *coherent* feat set that exercises
every engine path without touching an unbuilt system.

## 2. The engine (the reusable substrate — this is the real work)

### 2.1 Feat definition (content registry)

A content-defined registry mirroring the ability / background registries
(register / get / all / eligibility). Each feat declares:

- **Identity** — id, display name, description (what a selection menu + a
  `feats` listing show).
- **Prerequisites** — zero or more of: a minimum **ability score**
  (`Str 13+`), a **required feat** (by id — the prereq graph), a **skill/
  proficiency floor**, a **character-level** floor. (BAB prereqs in the source
  map to character level under Decision 0 — we have no BAB.)
- **Multi-take rule** — one of: **single** (take once); **per-parameter, no
  stack** (Weapon Focus / Improved Critical / Skill Emphasis — each take binds
  a new weapon/skill, effects don't stack); **stackable** (Toughness — each
  take adds a counted instance, effects stack).
- **Allowed classes** — optional class-id gate (mirrors background eligibility;
  empty = any). Carries the channeling/Ogier/class-special restrictions later.
- **Grant payload** — *what passive the feat confers*, expressed in the existing
  modifier vocabulary (§2.4): a stat/save/vital modifier, or an authored
  passive ability/effect id. A feat never invents a new modifier path.

### 2.2 Feat slots as banked credits (reuse the training-credit pattern)

Feats are earned, then spent — exactly like **training credits already work**
(`progression/level_up.go` `TrainsCrediter.CreditTrains`, fired from the
level-up cascade). A character earns **feat credits**:

- 1 at character creation, plus any from background/class grants
  (`feats.md` §Acquiring).
- 1 more at every 3rd character level (3/6/9/…), credited from the **`OnLevelUp`
  sink** (`progression/event_sink.go`) the same way trains are.

Credits **bank**: a level-up does not pop a blocking prompt (level-ups fire
mid-combat). Instead the slot sits unspent until the player chooses, mirroring
how trains wait for a `train` verb. This sidesteps the only hard UX problem
(choosing mid-combat) by **not choosing mid-combat**.

### 2.3 Selection flow

- **At creation** — extend the creation wizard with a feat-pick step after
  background (the wizard already does race/class/background choices; a feat
  choice is the same `ChoiceStep` shape, filtered to prereq-eligible feats).
- **At level-up / any banked credit** — a `feats` listing verb (shows known
  feats + eligible-to-take + banked credits) and a **`feat <id> [param]`** verb
  that spends one credit on an eligible feat (the analog of `learn`/`practice`).
  Eligibility = prereqs met ∧ a credit available ∧ (not already taken, unless
  the feat's multi-take rule allows another instance).

### 2.4 The grant → modifier bridge (no new modifier path)

A taken feat confers its passive through the **existing source-keyed modifier
surface** (`internal/srckey`): add a `srckey.Feat(id)` source constructor
alongside `Equipment(id)` / `ClassGrowth(id)`, so a feat's stat/save/vital
bonus rides the same stacking + recompute machinery equipment and class growth
already use. Concretely, the v1 grant kinds:

- **Save bonus** (Great Fortitude / Iron Will / Lightning Reflexes) → a +N
  modifier on a Fort/Reflex/Will axis (**S6 saves** is the consumer).
- **Vital bonus** (Toughness, stackable) → +N max-HP, sourced per-instance so N
  stacks count correctly.
- **To-hit bonus, per weapon** (Weapon Focus) → a hit modifier scoped to a
  weapon category/id, read by the same attack hit-mod chain weapon-proficiency
  uses (**S1**).
- **Crit threat widen, per weapon** (Improved Critical) → adjusts the per-weapon
  threat the crit step already reads (**S1 C**).
- **Skill bonus, per skill** (Skill Emphasis) → a +N proficiency/check modifier
  on a skill ability id (**S3**).
- **Activated-ability gate** (Power Attack) → the feat grants/unlocks the
  existing `power-attack` ability rather than a stat modifier.

### 2.5 Persistence

- A **`known_feats`** save field: a list of `{featID, param, count}` (param for
  per-parameter feats, count for stackable), plus the **banked feat-credit
  count**. New scalar/list fields via an **append-only migration v19 → v20**
  (mirrors the background v18→v19 and class-list v17→v18 migrations); a
  pre-migration save loads with no feats and zero banked credits.
- The conferred modifiers are **recomputed from `known_feats` on load** (like
  equipment modifiers), not separately persisted — `known_feats` is the source
  of truth, the bonuses are derived.

## 3. Content triage — what ships, what defers (and to whom)

The d20 list is ~100 feats; most are unbuildable on the tick model. v1 ships
only the **static-bonus family** (target system already wired):

| v1 feat | Effect | Rides |
|---|---|---|
| Great Fortitude / Iron Will / Lightning Reflexes | +2 to one save axis | S6 saves |
| Toughness | +N max HP (stackable, counted) | vitals |
| Weapon Focus | +1 to-hit with a chosen weapon (per-weapon multi-take) | S1 |
| Improved Critical | widen threat range with a chosen weapon (per-weapon) | S1 crit |
| Skill Emphasis | +N to a chosen skill (per-skill multi-take) | S3 skills |
| Power Attack | unlocks the existing trade-accuracy-for-damage ability | combat |

**Deferred to the owning sub-epic** (record so they don't resurface as "missing"):

- **Action-economy feats** — Cleave/Great Cleave, Combat Expertise, Combat
  Reflexes, Whirlwind, Spring Attack, Improved Trip/Disarm/Bull Rush, the
  Two-Weapon-Fighting family, Heroic Surge. These assume AoO / full-attack /
  5-ft steps the tick model deliberately lacks. **Never** under Decision 0 (or
  re-scoped onto tick semantics individually if a specific one earns it).
- **Ranged feats** — Point Blank/Far/Precise/Rapid Shot, Mounted Archery →
  **S1-G ranged-combat**.
- **Armor / Shield Proficiency** lines → **S1-E armor-depth**.
- **Channeling feats** (Combat Casting, Extra Affinity/Talent, Multiweave,
  Tie Off Weave, …) → **S2 The One Power** (and reuse this slot engine's
  channeling pool).
- **Reputation feats** — Fame, Infamy, Low Profile → **S8 reputation**.
- **Lost-Ability lines** — Dreamwalking, Foretelling, Old Blood, Sniffing,
  Treesinging, Viewing → narrative/GM-gated; a `lost_abilities_enabled` config
  flag, authored if/when those realms exist (Tel'aran'rhiod ≈ S10).
- **Skill-circumstance feats** — Alertness, Athletic, Stealthy, Nimble, etc.
  (+2 to two named d20 skills) → defer until those skills exist; most of their
  skills (Listen/Spot/Hide/Move Silently) belong to **visibility**, not S3.
- **Mounted feats** — no mount system; out of scope indefinitely.

## 4. Decisions (resolved at scope)

- **Full selection engine, static-bonus content.** (the two locked forks above.)
- **Slots are banked credits, spent via a verb** — not a blocking level-up
  prompt. Reuses the training-credit shape; the only new earn-hook is "1 credit
  per 3 character levels" on `OnLevelUp`. *(default chosen — fits the engine.)*
- **Prereqs validated at award time only; dynamic suspension deferred.** The
  source locks a feat when a prereq lapses (Str drained below 13). v1 validates
  when the feat is taken and does **not** re-suspend on later ability loss —
  ability drain is rare and the recompute-graph is bookkeeping. Recorded as a
  deferred refinement, not built. *(default chosen.)*
- **Background/class feats are authored grants in v1** — a background or class
  declares a fixed feat id to confer (closing S9's deferred background-feat
  item); "choose-1-from-a-background-pool" is a later refinement. The grant
  reuses the same award path the player verb uses, just without the choice.
- **Character level drives feat progression** for multiclass — total character
  level, not per-track (matches `feats.md` §Acquiring); the multiclass seam
  already sums tracks.

## 5. Build order (phased, per-slice commits)

0. **Feat registry substrate** — `internal/feat` (Feat + Registry, prereq +
   multi-take vocabulary), pack `FeatFile` / `decodeFeat` + `feats:` glob,
   `Registries.Feats`. No grant yet. (mirrors the background Phase 1.)
1. **Prereq evaluation** — a pure `Eligible(feat, character-view)` over ability
   scores / known feats / skills / level; unit-tested in isolation.
2. **Slot accounting + save v20** — `known_feats` + banked-credit field +
   migration; the `OnLevelUp` "1 credit / 3 levels" hook reusing the trains
   crediter shape.
3. **The grant bridge** — `srckey.Feat(id)` + the six v1 grant kinds wiring into
   saves / vitals / hit-mod / crit / skill / ability surfaces. Recompute on load.
4. **Award path + verbs** — the `feat <id> [param]` spend verb + the `feats`
   listing; eligibility + credit consumption + multi-take/stackable handling.
5. **Creation + background/class grants** — the creation-wizard feat step; the
   authored background/class feat grant (closes the S9 deferred item).
6. **Content** — the six v1 feats in `tapestry-core` (engine baseline) + a demo
   on a class/background so the loop is live-verifiable end to end.

## 6. Seams with shipped systems

- **abilities/effects + srckey** — the grant bridge; Power Attack is an existing
  ability the feat gates.
- **S6 saves / S1 weapon-identity / S3 skills / vitals** — the six v1 feats'
  consumers; each already exposes the modifier hook a feat writes to.
- **level_up.go trains crediter / OnLevelUp** — the slot-credit pattern, copied.
- **creation wizard + backgrounds (S9)** — the creation feat step; the authored
  background/class feat grant.
- **player save migrations** — append-only v19 → v20, the established rhythm.

## 7. Open / deferred (non-blocking)

- **Dynamic prereq suspension** (lock-on-drain) — §4; build if ability drain
  becomes common (e.g. an S5/S7 stat-drain condition lands).
- **Background/class feat *choice*** (pick-1-from-pool) — v1 grants are fixed.
- **The deferred feat clusters** — §3 (ranged/armor/channeling/reputation/
  lost-ability/skill-circumstance), each owned by its sub-epic; the slot engine
  here is built to host them (esp. the **channeling slot pool** S2 reuses).
- **Per-pool slot accounting** — the source separates general / channeling /
  free-class slots (`feats.md` §Slot accounting). v1 has one **general** pool;
  the channeling pool is added with S2, the free-class proficiency grants are
  authored (already how weapon/armor proficiency will work).
