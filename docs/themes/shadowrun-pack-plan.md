# Shadowrun as a pack — running SR alongside WoT on one kernel

> **Status:** design draft (no code). Started 2026-06-18.
> **Companion docs:** `docs/themes/channel-vocabulary.md` (the keystone — multi-ruleset on one kernel), `docs/themes/wot-mechanics-epic.md` (the WoT mechanics track), `docs/shadowrun/` (SR rules reference), `docs/specs/character-identity.md` (world-locking), `docs/ENGINE-VOCABULARY.md` (content↔engine contract).
> **Decision posture:** *spirit, not fidelity* — inherited from `channel-vocabulary.md` §1 and WoT EPIC Decision 0. We keep the `d20 + mod vs difficulty` resolution kernel for every setting and translate SR's *flavor + meaningful choices* onto it. We do **not** simulate dice pools, glitches, drain staging, or addiction.

---

## 1. The question, and the short answer

Can the Wheel of Time pack (d20-based) and a Shadowrun pack (dice-pool-based) both run as content on the existing AnotherMUD kernel?

**Yes — and the substrate is ~60% built.** The catch is that this is only true under the *spirit-not-fidelity* posture the project already adopted. Compared at the **fidelity layer**, the two systems share almost nothing: WoT rolls `d20+mod vs DC`, SR rolls `N d6, count hits ≥5 vs threshold`; WoT has one HP track, SR has two condition monitors; WoT levels up, SR spends karma. Faithfully simulating both would mean parallel pipelines for nearly every subsystem.

But the kernel was deliberately built to resolve **every** setting with one `d20+mod vs difficulty` roll (`channel-vocabulary.md` §1, WoT EPIC Decision 0). Under that posture the two systems stop disagreeing about *how a check resolves* and only disagree about **which attribute feeds which game function** — WoT defense is `10 + dex + armor`; SR defense is `reaction + intuition`. That is a **mapping** problem, and the mapping machinery is shipped.

This doc records: what is already built, how each SR subsystem maps onto it, the handful of pieces that genuinely need Go, a build sequence, and the one strategic decision (advancement model) worth making deliberately.

---

## 2. What is already built (the foundation)

These `channel-vocabulary.md` proposals are **shipped and wired into live combat** (verified against code 2026-06-17):

| Capability | Status | Where |
|---|---|---|
| Generalized resource pools (current/max + regen / overflow / `degrades` / depletion-event) | ✅ Shipped | `internal/pool/` — HP, movement, One Power live; `Vitals` is a facade over it |
| Channel layer (content formula → engine-fixed channel) | ✅ Shipped | `internal/channel/`; `connActor.Stats()` (`session/session.go:~5169`) + `MobInstance.Stats()` (`entities/mob_combat.go:~33`) read `attack`/`defense`/`damage_bonus`/`mitigation` **via the mapping**, not hardcoded stats |
| YAML formula evaluator (`defense: reaction + intuition`) | ✅ Shipped | `channel/expr.go` — safe arithmetic AST (`mod/floor/ceil/trunc/abs/min/max`); unknown stat name → 0 |
| Per-pack channel override (later-wins) | ✅ Shipped | `content/core/channel-map/baseline.yaml` + `content/wot/channel-map/defense.yaml` already coexist |
| Mitigation / soak subtract step | ✅ Shipped | `autoattack.go:~528` — `raw = dmg + damage_bonus − soak`; WoT zeroes `mitigation`, a pack can set `mitigation: body + armor` |
| `pool.degrades: <channel>` (Essence-caps-Magic shape) | ✅ Rule exists, unused | `pool.Rules.Degrades` |
| World vs library packs + world-locking | ✅ Shipped | `pack.Manifest.Kind` (`world`/`library`), `ActiveWorlds` login gate, save `WorldID` (v23) |

**Key seam:** the channel formula `lookup` accepts *arbitrary* stat names — the evaluator does not care that `body`/`reaction`/`intuition` aren't WoT stats. So a Shadowrun mapping can already supply `defense: reaction + intuition` and `mitigation: body + armor_ballistic` **as pure content**, today. WoT's pack migration is the trivial case (its mapping *is* the old hardcode); SR is the first pack that will actually exercise the channel layer with a non-trivial override.

---

## 3. Subsystem map — every SR system → the engine

Each Shadowrun subsystem against the *actual* current code state, tagged by effort
(**Content** = no Go · **Small Go** = a few lines / one struct field · **Medium/Real Go** = a new seam):

| SR subsystem | Maps to | Effort |
|---|---|---|
| Attributes (Body, Quickness, Strength, Charisma, Intelligence, Willpower, Reaction) | Stat names referenced in channel formulas | **Content** — *but* chargen seeding is the gating blocker (§4.1) |
| To-hit (skill+Agility vs Reaction+Intuition) | `attack` / `defense` channels → existing d20 roll | **Content** (mapping file) |
| Damage + armor soak (Body + armor) | `damage_bonus` + the `mitigation` subtract step | **Content** — step already exists |
| Ballistic vs Impact armor | Typed mitigation (reuse `TypedResistance` in `autoattack`) | **Small Go** — add the two damage types |
| Nuyen | Currency — content-defined economy primitive | **Content** (declare a currency) |
| Metatypes (human / elf / dwarf / ork / troll) | Races with attribute modifiers + size (`internal/size`) | **Content** |
| Skills + specializations | Proficiency map (`+2` specialization = source-keyed bonus) | **Mostly content**; flat-skill vs class-track is a model question (§4.2) |
| Cyberware / bioware as stat boosts | Equipment-sourced modifiers (`srckey` pipeline) | **Content** + slot/capacity work |
| **Essence → Magic decay** | `pool.degrades: magic` (rule exists, unwired) | **Small Go** — honor `degrades` at channel-resolve |
| **Two condition monitors** (Physical + Stun) | Two pools + per-attack `target_pool` routing | **Small Go** — pools exist, routing doesn't |
| Drain (spend on casting) | One Power model already does this (weave cost + `resist.backlash`) | **Content** — reuse WoT S2 machinery |
| Initiative + passes | `initiative` channel **declared but unread** (combat is global tick cadence) | **Medium Go** *if* real turn order is wanted |
| **Karma / point-buy advancement** | Nothing — advancement is hardwired levels + tracks + XP | **Real Go** (§4.2) — the biggest single gap |

The result that matters: **most of Shadowrun is content or small Go.** The damage model, the casting-resource model, even Essence's "exotic" decay decompose into machinery already shipped for WoT. The One Power pool + `resist.backlash` built for saidin drain is *structurally the same thing* SR Drain needs.

---

## 4. The real blockers (what actually needs Go)

Four, in rough order of cost.

### 4.1 Content-defined attribute set — the gating blocker
`progression/statblock.go:~50` hardcodes six stat constants and `DefaultPlayerBase()` seeds exactly those. `StatBlock` is *already* a `map[StatType]int`, so storage is fine — the friction is **chargen + `score` display + caps** assuming six. An SR character needs ~7 named attributes seeded at creation that the current wizard won't produce.
**Fix:** make the base attribute set a per-world content declaration the wizard reads, instead of calling `DefaultPlayerBase()`. Moderate. This is the true prerequisite for *any* non-six-stat ruleset (SR, or a future generic pack).

### 4.2 Advancement model — levels vs karma
`progression/manager.go` is hardwired to `XP → track → level-up`. Shadowrun has **no levels**: karma is spent on individual skill/attribute rises à la carte at `cost = rating × multiplier`. No content knob expresses this; it is a genuinely different progression shape.
**Fix:** a pluggable advancement strategy — the level-track model becomes one implementation, a karma-ledger model another. Largest piece; the one strategic decision (§6/§7).

### 4.3 Per-attack pool routing (Physical vs Stun)
Pools exist; what's missing is letting an attack declare *which* monitor it fills (stun baton → Stun, bullet → Physical), and Physical-overflow → death vs Stun-overflow → unconscious. Flagged in `channel-vocabulary.md` §12.
**Fix:** add `target_pool` + `type` to the damage struct (WoT S1 typed damage wants this regardless) and a flag on the `VitalDepleted` event for death-vs-KO. Small.

### 4.4 Essence-degrades-Magic wiring
`pool.Rules.Degrades` is built but used by nobody. An `essence` pool whose `current` clamps a `magic` channel max is the textbook use of an existing feature.
**Fix:** content (declare the pool) + a few lines to honor `degrades` at channel-resolve time. Small.

*(Optional, §12-open: real initiative ordering. Combat is global tick cadence today; the `initiative` channel is reserved-but-unread. SR leans on initiative passes harder than WoT — ship SR with cosmetic initiative first, add ordering later if it matters.)*

---

## 5. What we give up (the cost of the posture)

Spirit-not-fidelity has a felt cost, especially for tabletop SR veterans. All of these are explicit non-goals in `channel-vocabulary.md` §1/§5 — restated here so the tradeoff is owned, not discovered:

- **No dice-pool texture** — no Rule of Six, no glitches / critical glitches. "Count your hits" becomes one margin roll. The biggest felt loss.
- **Drain / addiction simplified** — drain becomes "spend the pool + a backlash resist" (the WoT model); the addiction-point economy drops.
- **Two monitors are pools, not staged wound boxes** — overflow is arithmetic, not L/M/S/D box-staging.
- **Cyberware Essence cascade flattened** — Essence lowers Magic as a cap; fractional grade accounting (alpha/beta/delta) is content flavor, not an engine.

---

## 6. Recommended sequencing

Each step pays for itself even if the program stops there.

1. **Content-define the attribute set** (§4.1). Universal unlock; nothing non-WoT works without it. Clean win for a future generic pack too.
2. **Damage struct: `type` + `target_pool`** (§4.3). Wanted by WoT S1 typed damage anyway; yields Ballistic/Impact and Physical/Stun in one stroke.
3. **Stand up a minimal `shadowrun` world pack** — metatypes (races), nuyen (currency), a channel mapping (`defense: reaction + intuition`, `mitigation: body + armor`), a handful of skills/weapons. Boot it, fight one mob. First end-to-end exercise of the channel layer by a non-WoT pack.
4. **Essence pool + `degrades: magic`** (§4.4). Closes the "Essence is exotic" myth.
5. **The advancement fork** (§4.2) — the one real decision (§7).

Steps 1–4 are a few focused slices and yield a *playable, if simplified, Shadowrun* on the existing level-track advancement (treat karma as XP for v1). Step 5 is where faithfulness is decided.

---

## 7. The one decision to make first — advancement

Everything else is mechanical. This is the fork:

- **Option A — reuse levels / tracks (fast).** Treat karma as XP, give SR archetypes "classes/tracks," advance on the existing engine. Ships in the §6 sequence with zero advancement work. Costs SR's à-la-carte feel.
- **Option B — pluggable advancement (faithful).** Build a karma-ledger strategy alongside the level-track one. The single largest engine investment; the thing that makes SR *feel* like SR, and it future-proofs any point-buy system.

**Recommendation:** ship **Option A first** (it falls out of steps 1–4 almost for free and validates the whole pack pathway), play it, then decide whether SR's identity demands Option B based on how the simplified version actually plays. Do not build the karma engine speculatively.

---

## 8. Validation gate (do before writing pack content)

Mirror of `channel-vocabulary.md` §10, narrowed to SR:

- [ ] The §3 map has an engine home (content / small-Go / real-Go) for every SR subsystem an early-game runner touches.
- [ ] A `shadowrun` `kind: world` pack loads alongside `core` + `wot` without registry collision (namespaced ids; later-wins channel override).
- [ ] One SR combat round resolves attack → soak → route to Physical/Stun using only channels + the §4.3 routing.
- [ ] Essence (§4.4) models as a `degrades: magic` pool with no bespoke code.
- [ ] SR Drain reuses the One Power pool + `resist.backlash` seam (no second drain engine).
- [ ] Chargen seeds the SR attribute set from content, not `DefaultPlayerBase()` (§4.1).

If any box fails, fix the map (§3) or the blocker list (§4) — not the kernel.

---

## 9. Configuration surface (additions beyond channel-vocabulary §11)

| Knob | Default | Notes |
|---|---|---|
| world attribute set | six (WoT/generic) | per-world content declaration consumed by chargen (§4.1) |
| damage `target_pool` | `hp` | which pool a damage instance fills (§4.3) |
| damage `type` | untyped | feeds type-specific `mitigation` (Ballistic/Impact) |
| `VitalDepleted` zero-meaning | death | per-pool flag: Physical→death, Stun→unconscious (§4.3) |
| advancement strategy | level-track | `level-track` | `karma-ledger` (§4.2/§7) — Option B only |

---

## 10. Open questions

- **Initiative.** Cosmetic for v1 (global tick cadence) or real per-pass ordering? SR wants it more than WoT; defer the engine work until it bites (`channel-vocabulary.md` §12).
- **Skill model.** SR skill groups / specializations vs the flat proficiency map — likely content, but flag if it pressures the proficiency engine (`channel-vocabulary.md` §12).
- **Metatype attribute caps.** SR metatypes set per-attribute min/max (troll Body, elf Quickness). Are these race-sourced modifiers + a cap channel, or a new chargen constraint? Lean: modifiers + a per-attribute cap the wizard honors.
- **Magic vs mundane parity in a shared world.** Co-hosting is already world-locked (a character belongs to one world); the real question is whether SR's additive career model (mage *and* hacker *and* shooter) vs WoT's gating class model creates a balance mismatch *within* the SR world. Pack-internal concern, not a kernel one.
- **Lua `derive()` hook.** SR's conditional "melee uses Strength, ranged uses weapon" exceeds the YAML evaluator. Today Lua is event-bus-only (no compute-time derive). Resolve with a second mapping entry (`damage_bonus_ranged`) before reaching for a Lua hook (`channel-vocabulary.md` §7.2).
