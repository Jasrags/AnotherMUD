# Proposal: Karma-ledger advancement (SR-M5)

**Status:** Shipped — SR-M5a (substrate + routing + score) and SR-M5b (the `improve` spend verb) both landed · **Type:** Engine slice (pluggable advancement strategy) · **Audience:** engine + content
**Feeds:** [`docs/themes/shadowrun-mvp.md`](../themes/shadowrun-mvp.md) §SR-M5 (Decision D3, Option B)
**Builds on:** [`docs/specs/progression.md`](../specs/progression.md) (the level-track engine), the SR-M1 `attribute_set` world-scoping pattern (the mirror this reuses), [`docs/specs/skills.md`](../specs/skills.md) (0-100 proficiency + trainer-cap — the karma-buy target)

## 1. Problem / motivation

The engine's only advancement model is **level-track**: earned rewards (kills, quests) bank XP onto a character's class `bound_track`; crossing a threshold levels the track up and the class grants stat growth + abilities. WoT and the generic worlds want exactly this.

Shadowrun does not. SR5 is **level-less** — a runner banks **karma** as a spendable currency and raises skills, attributes, and qualities *à la carte* at `cost = new-rating × multiplier`. The MVP shipped on Option A ("karma-as-XP" — karma reskinned onto the level-track engine) precisely because Option B is the largest single engine investment and the plan said not to build it speculatively. With the MVP played and the identity gap felt, Option B is now in scope.

## 2. Goals & non-goals

**Goals.** A **pluggable advancement strategy** selected per world: `level-track` (default; unchanged) vs `karma-ledger` (SR). A karma-ledger world routes rewards into a spendable balance and never levels. The player spends karma via `improve` on skills / attributes / qualities. Level-track worlds are byte-identical.

**Non-goals.** Not touching the level-track engine's behavior. Not modelling SR5's full karma economy (initiation grades, karma-for-nuyen, group karma) — v1 is earn-from-play + spend-to-improve. Not a per-character strategy — strategy is a property of the **world**, resolved at login like the attribute set.

## 3. Design — the two strategies

A world's manifest declares `advancement:` (default `level-track`):

| Strategy | Reward routing | Growth | Save shape |
|---|---|---|---|
| `level-track` (default) | XP → class `bound_track` → level thresholds | class grants stat growth + abilities per level | progression tracks (unchanged) |
| `karma-ledger` (SR) | **karma** → spendable ledger (no track XP, no levels) | `improve <skill\|attr\|quality>` spends at `new-rating × mult` | a `karma:` block (Current + Total) |

The strategy is resolved exactly like the SR-M1 attribute set: `Manifest.Advancement` → `Registries.WorldAdvancement[namespace]` → `session.Config.WorldAdvancement` → `newAdvancementLedger(sel, worldID)` on the connActor at login. A **nil ledger is the level-track signal** — everywhere downstream (reward routing, score, persistence) keys off `karma != nil`, so a level-track character is a true no-op.

## 4. What shipped — SR-M5a (substrate + routing + score)

- **`internal/karma`** — a dependency-free leaf `Ledger` (concurrency-safe): `Current` (spendable) + `Total` (lifetime earned, mirroring SR5's two karma figures). `Grant` raises both; `Spend` deducts Current only (Total is a monotone record); `Restore` clamps negatives.
- **Save v39** — `Save.Karma *karma.Snapshot` (omitempty pointer; a level-track save carries **no** `karma:` key). `migrateV38toV39` is a no-op (absent → nil ledger). Snapshotted in Persist via `syncKarmaToSaveLocked` (mirrors `syncHeatToSaveLocked`); restored at login.
- **Manifest `advancement:`** — `pack.AdvancementLevelTrack` / `AdvancementKarmaLedger` constants + `WorldAdvancement` map; an **unknown value is a hard load error** (a typo can't silently fall through to level-track).
- **Reward routing** — `GrantKillXP` (party.go) and `questXP.GrantExperience` (quest_rewards.go) branch on `UsesKarmaLedger()`: a karma-ledger recipient banks karma ("You gain N karma."), a level-track recipient hits the unchanged XP path.
- **`score`** — a karma-ledger character shows a `Karma: <current> (<total> earned)` line; the level/track block is suppressed (gated on `!d.HasKarma`) — no phantom "Level 1" for a level-less runner.
- **Content** — `content/shadowrun/pack.yaml` declares `advancement: karma-ledger`. SR is now level-less; the Street Samurai class kit is a creation package only (its `bound_track` never accrues; `power-attack@2` becomes a karma purchase under M5b).

Gates: `internal/karma` unit tests, player save round-trip (`TestSave_Karma*`), pack load + unknown-rejection (`TestLoad_ShadowrunAdvancement` / `TestLoad_UnknownAdvancementRejected`), the resolver unit (`TestNewAdvancementLedger`), and the live e2e (`TestLive_ShadowrunKarmaAdvance` — a level-less runner banks 30 karma from a ganger kill).

## 5. Shipped — SR-M5b (the `improve` spend verb)

`improve <target> [param]` spends karma to raise one of three things; a no-arg
`improve` lists what's raisable, each cost, and the balance. Resolution order is
attribute → skill → quality (first kind that owns the name handles it). Every
spend is atomic: `SpendKarma` (the ledger's insufficient-balance gate) succeeds
first, then the raise applies.

- **skill** — the ability's trainer-**cap** rises one tier on the existing
  Novice/Apprentice/Journeyman/Master ladder (`progression.NextTier` +
  `ProficiencyManager.SetCap`); an unlearned skill is `Learn`ed on first buy.
  Cost = `new-tier-rank × SkillMult` (rank 1–4). Persists via the existing
  `syncAbilitiesToSaveLocked`.
- **attribute** — +1 to the primary's base (`StatBlock.AdjustBase`), gated on the
  metatype/race `StatCaps` ceiling; channels re-derive on the next read (pull),
  pool maxes via the `OnMaxChange` push — no manual recompute. Cost =
  `new-value × AttributeMult`. Persisted with `syncStatsToSaveLocked`.
- **quality** — grants a feat carrying `karma_cost > 0` (`GrantFeat`), mirroring
  `TakeFeat`'s validation (already-held / per-param / prerequisites) but spending
  karma instead of a feat slot. A world opts feats into the karma economy by
  tagging them; SR ships its own `content/shadowrun/feats/` qualities
  (Ambidextrous, High Pain Tolerance) with distinct ids so core stays neutral.

Cost multipliers are a **per-world manifest block** (`karma_costs: { skill_mult,
attribute_mult }`), resolved on the actor at login (`karma.Costs.WithDefaults`
fills any omitted knob with the SR canon ×2/×5), so a zero/negative multiplier
can't create a free buy.

Deferred (post-v1): burning negative qualities; karma-for-nuyen / initiation
grades; a listing that also shows unlearned skills; attribute raises gating on
Effective vs. Base (v1 uses Base — the permanent value karma buys).

## 6. Configuration surface

| Knob | Default | Notes |
|---|---|---|
| manifest `advancement:` | `level-track` | per world; `level-track` \| `karma-ledger`; unknown → load error |
| karma-per-kill / -quest | the mob's `xp_value` / quest `xp` reward | routing reuses the existing reward magnitudes verbatim — earn tuning is content, not strategy |
| skill / attribute / quality karma multipliers | (SR-M5b) | the `improve` cost table — a per-strategy config, authored with the spend verb |
