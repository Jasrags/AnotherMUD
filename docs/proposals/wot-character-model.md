# Proposal: WoT Character Model (classes · multiclass · features · feats · skills)

**Status:** Draft / pre-spec design note — for alignment · **Type:** the keystone pre-decision for the WoT mechanics program
**Feeds:** EPIC sub-epics **S3 (skills)**, **S4 (feats/traits)**, **S9 (class/background/multiclass)**, and the eligibility half of **S2 (the One Power)** — `docs/themes/wot-mechanics-epic.md`
**Builds on:** `internal/progression` (tracks, classes, abilities, proficiency, training), `internal/player` (the save), `internal/session` (`connActor`)
**Governed by:** EPIC **Decision 0** — translate WoT onto the tick/chance model; no d20 rewrite.

## 1. Why this note exists

"How does a WoT character's *build* compose on our engine?" is the single highest-leverage decision in the WoT program, because **classes, multiclass, skills, feats, and channeling-eligibility all hang off it.** Settling it now (as a design pre-decision, not a full spec) de-risks four sub-epics; getting it wrong means S1 weapons or S2 channeling get built on a character model that later needs restructuring. This note resolves the structural questions and recommends positions; the per-sub-epic specs inherit them.

## 2. What the engine already gives us (verified in code)

This is the load-bearing finding, and it's good news:

- **Progression is already multi-track.** Per-entity state is `progression.ProgressionState` = `map[track]→{level, xp}` (`state.go:19-23`); the save persists it as `[]TrackEntry` (`player.go:165`). A character can already hold N independent class tracks, each levelling on its own XP — and it round-trips to disk with no schema change. There is **no "active track"** concept; callers grant XP on whatever named track they choose (`manager.go:69`).
- **A class is a feature-bundle bound to a track,** not the track itself. `Class.BoundTrack` (`class.go:47`) names the track whose level-ups fire that class's `StatGrowth` + ability `Path` + train credits. The level-up bus subscriber already gates on `BoundTrack` match (`level_up.go:86-88`), so **two classes watching two different tracks already route correctly.**
- **Class features are granted deterministically** via `Class.Path = []{Level, AbilityID, UnlockedVia}` (`class.go:17-21`) — at creation (level 1) and on each level-up. Grant chain: `ClassPathProcessor.Apply → Granter.Teach → ProficiencyManager` (sets proficiency=1 + cap).
- **The six WoT attributes already exist** — `StatSTR/INT/WIS/DEX/CON` (+ `LUCK`) in `statblock.go`, plus derived `hit_mod`/`ac`/`hp_max`/`carry_max`/etc. The stat model is essentially already WoT-shaped.
- **Race gates eligibility at creation** via `AllowedCategories`/`AllowedGenders` (`class.go:72-78`, filtered by `GetEligible` `class.go:214-228`), plus `StatCaps` and `CastCostModifier` at runtime (`race.go`).

**The one structural blocker:** `player.Save.Class` is a single `string` (`player.go:167`), mirrored by `connActor.classID string` (`session.go:1703`). That — and only that — pins the engine to one class per character.

**What's genuinely missing:** (a) a **feat-*selection*** mechanism (choose-a-perk-at-level-up); the engine only does authored, deterministic grants. (b) a **runtime race/gender→ability gate** (the creation filter doesn't stop an admin-granted or quest-granted ability from being *used* by an ineligible character).

## 3. Decisions

### D1 — Class model: **multi-track-as-multiclass.** ✅ CONFIRMED 2026-06-10

Adopt the engine's natural grain: **the track is the unit of progression; a class is the feature-bundle bound to a track; "multiclass" = a character holding 2+ class-tracks.** This is exactly WoT (a Warder who channels = warrior-track + channeler-track) and the substrate is 80% there.

- **Engine change (small, one-time):** widen `player.Save.Class string → []string` (or a small `[]ClassEntry{ID}`), mirror on `connActor`, update login restore, bump save to **v18** (append-only migration: wrap the old scalar in a 1-element slice), and **loop** the level-up + character-created subscribers over the class list. Everything else — `ClassPathProcessor`, `TrackDef`, `ProgressionState`, `ProficiencyManager`, stat growth — needs **zero change** (the exploration confirmed this).
- **Scope v1 to single-class behavior** (each character picks one class at creation) but ship the `[]string` field from the start, so unlocking a *second* class later is an additive content/leveling slice, **not a second save migration.** Design the seam now; ship the simple case first.
- **Channeling becomes "a character who holds the Initiate or Wilder class-track"** — no special-casing; it's just another class bound to a `one-power` track.

*Rejected:* literal d20 multiclass (summed BAB, iterative attacks, −2-Defense-per-class) — those are action-economy artifacts the tick model doesn't have (Decision 0). We take the *concept* (stackable class-tracks), not the d20 arithmetic.

### D2 — Class features vs. feats: **features now (free), feat-selection deferred.** ✅ CONFIRMED 2026-06-10

- **WoT class features** (Sneak Attack, Weapon Specialization, Uncanny Dodge, channeling Affinity, Inspire Confidence) → author them as abilities/effects in each class's `Path` at the right level. **Zero new engine** — this is exactly what `Path` does. Passive features (Toughness-style +HP, Weapon Focus +hit) ride the existing passive-ability/effect surface.
- **WoT feats** (the player-*chosen* perks at levels 1/3/6/…) → the engine has **no selection mechanism.** This is EPIC **S4** and the one real gap. **Recommendation: defer the feat-selection engine.** For v1, hand out a class's signature feats as authored `Path` grants (no player choice). Add player-chosen feats — a "pick from an eligible pool at level N" mechanism + a `known_feats` save field + prerequisite gating — as the dedicated S4 slice *if/when* the build-your-character flavor is wanted. This keeps the character-model slice small and unblocks S3/S9 without waiting on S4.
- **Consequence to accept:** v1 WoT characters are mechanically distinct *by class*, not *by individual feat choice*. That's a fine MVP; feat choice is a depth layer, not a foundation.

### D3 — Skills: **use-based proficiencies; detail deferred to S3.**

The proficiency system is keyed by **ability id**, use-based (`proficiency.go`). WoT's ~40 d20 skills translate as: a "skill" is an ability/proficiency that gains with use; "class skills" = which proficiencies a class grants and caps higher (via `Path` + trainer tiers). The detailed skill *list* and check model is **S3** — this note only fixes the *shape* (skills are proficiencies, not a separate point-buy system), so S9/classes can reference "class skills" coherently. Don't port d20 skill *points* or rank-buying (Decision 0: drop the bookkeeping).

### D4 — Channeling eligibility + race/gender gates: **creation-time class gating; note the runtime gap.**

- **Ogier-can't-channel** and **gender-locked saidin/saidar** → use the existing creation filters: the `channeler` classes set `AllowedCategories` (exclude the ogier category) and `AllowedGenders` (`class.go:72-78`). Works today, no engine change.
- **The runtime gap:** there is no race/gender gate on ability *use* — an Ogier admin-granted a weave, or a man taught a saidar weave, wouldn't be blocked at cast time. For v1 this is acceptable (creation gating covers normal play). If it becomes a real concern when S2 lands, add a small `AllowedGenders`/`AllowedCategories` (or `AllowedRaces`) field on `Ability` checked in the resolver — a contained add. Flag, don't build now.

### D5 — Stats: **already fits; ability-score-increase rides a level-up hook.**

The six attributes exist. WoT's "+1 to an ability every 4 levels" maps onto a level-up hook (extend `StatGrowth`, or credit a "stat point" like training credits) — defer the exact mechanism to S9. No new stat *types* are needed for the core six; carry-weight (`carry_max`, just shipped) is the Strength-derived ceiling.

## 4. What this means for sequencing

- **S1 weapons is unblocked and untouched by all of this** — proceed anytime.
- The **character-model work is mostly content + one small engine change** (the `classID → []string` widening + save v18). That single change unlocks S9's multiclass and S2's channeling-as-a-class, and it's the only schema migration the whole character half needs.
- **S3 (skills)** and **S4 (feats)** inherit D2/D3: skills are proficiencies; feats are authored grants until a selection engine is justified.
- Recommended order within the character half: ship the `[]string` widening (tiny, with v1 single-class content) → author WoT classes + their `Path` features → S3 skill content → unlock a second class-track (multiclass) → S4 feat-selection if wanted.

## 5. Decisions

1. **D1 — multi-track-as-multiclass** (widen `classID`→`[]string`, save v18, v1 single-class, model supports N). **✅ CONFIRMED 2026-06-10.**
2. **D2 — defer the feat-selection engine**; author class features/feats as `Path` grants for v1. **✅ CONFIRMED 2026-06-10.**

Still open (do not block the character-model build):

3. **Whether content ever ships a second class-track** (true in-play multiclass) vs. v1 single-class staying the norm. The *engine seam* is decided (D1 builds it); this is a later **content** call, not a structural one.
4. **Channeling gender model** — one `channeler` class with gender-gated *weave abilities*, or the two existing class shapes (Initiate/Wilder) with saidin/saidar as the gender axis. **Deferred to the One Power resource-model note (S2).**

---

*This is a design note, not a spec. With D1/D2 confirmed, the first build deliverable is the `classID → []string` + save-v18 slice (small), after which WoT classes are content authoring. The detailed behavior of skills (S3) and feats (S4) get their own spec slices when built. The One Power resource model is a separate pre-decision note (parked until S2).*

**Build status:** the **`classID → []string` + save-v18 seam SHIPPED 2026-06-10** — `player.Save.Class []string` + `migrateV17toV18` (wraps the scalar), `connActor.classIDs []string` (primary-class for single-value readers; `Saves`/`IsWeaponProficient` compose over the list; `SetClass` replaces; new `ClassIDs()`), and the level-up + character-created subscribers walk the list. Behavior stays single-class; a second class-track is now additive content, not another migration. Live-verified (a v17 scalar save migrates and behaves identically, persists as a v18 list). The remaining character-half work (WoT class content, S3 skills, true multiclass content, S4 feats) is unchanged by this and still sequenced per §4.
