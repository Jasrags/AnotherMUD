# Proposal: Karma-ledger advancement (SR-M5)

**Status:** Partially shipped — SR-M5a landed (substrate + routing + score); SR-M5b (the `improve` spend verb) pending · **Type:** Engine slice (pluggable advancement strategy) · **Audience:** engine + content
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

## 5. Pending — SR-M5b (the `improve` spend verb)

`improve <thing>` spends karma to raise a target at `cost = new-rating × multiplier`:

- **skill** — raise the ability's trainer-**cap** (skills are 0-100 proficiency + a trainer cap; karma-buy raises the ceiling, use-based gain fills toward it — the pinned skills-D4 model). SR mult ≈ ×2.
- **attribute** — +1 to a primary base stat, mult ≈ ×5, enforced against the metatype cap (SR-M1's per-attribute `Cap`).
- **quality** — grant a feat (qualities = the feat system); cost = the quality's karma value. Burning negative qualities is a later refinement.

No-arg `improve` lists improvable targets + costs + the current balance. Cost multipliers are content/config knobs (a per-strategy or per-world table), not hardcoded.

Open questions for M5b: where the skill-cap raise lives (a new `SpendKarma`-gated progression API vs. an ability-cap mutation); whether attribute raises re-derive channels/pools reactively (they must — the SR channel map reads primaries); the quality catalog's karma-cost source (feat metadata vs. a separate qualities table).

## 6. Configuration surface

| Knob | Default | Notes |
|---|---|---|
| manifest `advancement:` | `level-track` | per world; `level-track` \| `karma-ledger`; unknown → load error |
| karma-per-kill / -quest | the mob's `xp_value` / quest `xp` reward | routing reuses the existing reward magnitudes verbatim — earn tuning is content, not strategy |
| skill / attribute / quality karma multipliers | (SR-M5b) | the `improve` cost table — a per-strategy config, authored with the spend verb |
