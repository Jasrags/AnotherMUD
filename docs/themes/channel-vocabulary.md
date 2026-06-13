# Channel Vocabulary — hosting multiple rulesets on one kernel

> **Status:** design draft (no code). Started 2026-06-13.
> **Companion docs:** `docs/themes/wot-mechanics-epic.md` (the WoT mechanics track), `docs/shadowrun/` (Shadowrun reference material), `docs/ENGINE-VOCABULARY.md` (the content↔engine contract this extends), `docs/PRIMER.md` ("the setting is a placeholder").
> **Decision posture:** *spirit, not fidelity.* We keep the existing tick/chance resolution kernel for every setting and translate each game's *flavor + meaningful choices* onto it — we do NOT port dice-pool or d20 math faithfully. This is the same posture WoT EPIC Decision 0 took (posture A), extended to Shadowrun.

---

## 1. The problem

The engine is already **multi-setting**: rooms, items, classes, races, tracks, effects, abilities, quests, and biomes are all content-pack data loaded into registries (`docs/PRIMER.md` §2). Booting `core + starter-world` vs `core + wot` is solved.

It is **not yet multi-ruleset**. The intent is to host three very different stat models on one engine:

- **Generic RPG** — six classic attributes (STR/INT/WIS/DEX/CON/LUCK), levels, classes, HP + mana.
- **Wheel of Time** — d20-adjacent attributes, saves (Fort/Reflex/Will), the One Power as a resource.
- **Shadowrun** — Body/Quickness/Strength/Charisma/Intelligence/Willpower + Reaction, **Essence** (a resource that degrades **Magic**), **two condition monitors** (Mental + Physical), Initiative as `attribute + dice`, armor as Ballistic/Impact.

These do not disagree about *how a check resolves* — once we accept spirit-not-fidelity, the tick/chance kernel resolves all of them. They disagree about **which attribute feeds which game function**: WoT defense is DEX + armor; Shadowrun defense is Reaction + Intuition. WoT melee damage scales off STR; Shadowrun melee damage scales off Strength but ranged off the weapon. That is the whole problem, and it is a **mapping** problem, not a resolution problem.

### Non-goals

- No dice-pool simulation (counting hits ≥ 5 against a threshold). The kernel resolves by probability/margin; flavor text may *say* "you roll 9 dice" without the engine modelling it.
- No faithful condition-monitor overflow / staging. Shadowrun's Mental + Physical monitors become two **pools with simple rules**, not a tabletop damage-staging engine.
- No d20 rewrite. `ResolveSave`, the to-hit roll, and the ability proficiency×variance roll all stay.

---

## 2. The core idea: Attributes vs. Channels vs. Mapping

Three layers, two of them content-owned:

| Layer | Owner | What it is |
|---|---|---|
| **Attributes** | content pack | The pack's *arbitrary* primary stat set. WoT's six; Shadowrun's nine. Engine never hardcodes the set. |
| **Channels** | engine (fixed, curated) | A small abstract vocabulary the kernel consumes: "give me the attacker's `attack` and the defender's `defense`." The engine never names an attribute. |
| **Mapping** | content pack | A per-pack formula that fills each channel from that pack's attributes. WoT: `defense = 10 + dex_mod + armor`. Shadowrun: `defense = reaction + intuition`. |

The kernel reads **channels**. Content decides what feeds them. "Different stats for different things" dissolves: the engine names the *thing* (the channel), the pack names the *stat*.

This is already the house pattern — combat reads `"hit_mod"`/`"ac"` as opaque strings (`combat/stats.go:96-112`) and save axes are opaque strings (`combat/saves.go:14-27`). Channels generalize it into one named vocabulary with a content-supplied derivation.

---

## 3. The channel vocabulary (engine-fixed)

Curated, not open-ended: **every channel must have a kernel consumer.** Content can define arbitrary *attributes*, but it can only fill channels the engine already reads. Adding a channel is an engine change (small); adding an attribute or remapping is content-only.

| Channel | Kernel consumer | Today's hardcoded source |
|---|---|---|
| `attack` | to-hit roll (the `+mod` side) | `combat.Stats.HitMod` (`combat/stats.go:16`) |
| `defense` | to-hit roll (the difficulty side) | `combat.Stats.AC` (`combat/stats.go:21`) |
| `damage_bonus` | added to rolled weapon damage | `STRBonus(STR)`, hardwired (`combat/autoattack.go:280`) |
| `mitigation` | reduces incoming damage (soak / armor) | folded into AC today; no separate step |
| `initiative` | action cadence / ordering | none (tick cadence is global) |
| `save.fortitude` / `save.reflex` / `save.will` | `ResolveSave` bonus (`combat/saves.go:67`) | save bonuses (already string-axis) |
| `potency` | scales ability/weave/spell effect magnitude | none (effects are flat) |
| `resist.backlash` | drain / overchannel / madness resist | none (WoT S2 will need it) |
| pool maxes: `hp_max`, `<pool>_max` | pool capacity (see §5) | `StatHPMax` only live (`statblock.go:57`) |
| `carry_max` | encumbrance ceiling | `StatCarryMax` (already content-fed, `statblock.go:65`) |

**Channel naming:** lowercase dotted strings, exactly like save axes today. A pack that references an undefined channel fails validation at load; a channel with no mapping reads as a configured engine default (e.g. `defense → 10`).

---

## 4. Worked examples — does the table hold?

### 4.1 WoT (d20-adjacent) — a melee exchange

```yaml
# content/wot mapping (illustrative)
channels:
  attack:        bab + mod(str)            # proficient martial weapon
  defense:       10 + mod(dex) + armor     # AC, the current model
  damage_bonus:  mod(str)
  initiative:    mod(dex)
  save.fortitude: base_fort + mod(con)
  save.reflex:    base_ref  + mod(dex)
  save.will:      base_will + mod(wis)
  hp_max:        derived from class + con
```

Kernel: rolls `d20 + attack` vs `defense`, adds `damage_bonus` to the weapon dice, `ResolveSave` uses the `save.*` channels. **This is exactly what the engine does today** — WoT is the trivial case because the current hardcode *is* WoT's mapping. Migrating WoT = lifting today's hardcoded formulas into a `core` mapping file, unchanged behavior.

### 4.2 Shadowrun (from the live character sheet, `docs/shadowrun/CHARACTER.md`) — a firefight

```yaml
# content/shadowrun mapping (illustrative)
channels:
  attack:        quickness + skill         # Agility/Quickness + weapon skill
  defense:       reaction + intuition      # Ranged Dodge on the sheet
  damage_bonus:  strength                  # melee; ranged uses weapon power
  mitigation:    body + armor_ballistic    # Damage Soak = Body; armor 10B/8I
  initiative:    reaction                  # sheet: Initiative [8 + 1d6]
  hp_max:        8 + ceil(body/2)          # Physical condition monitor
  stun_max:      8 + ceil(willpower/2)     # Mental condition monitor
  potency:       magic + skill             # spellcasting force
  resist.backlash: willpower               # drain resist
```

Kernel: same `d20 + attack` vs `defense` roll (flavored as a dice pool in text), `damage_bonus` + weapon → damage, **`mitigation` becomes a new subtract-before-pool step** (§6), damage routes to `hp_max` (Physical) or `stun_max` (Mental) per attack type, **Essence** caps `magic` (§5.2). Initiative `8+1d6` is `initiative` channel + the kernel's existing roller — no dice-pool engine required.

**Verdict:** the channel set above is sufficient for a SR firefight and a WoT exchange with no new resolution math. The two genuinely new kernel pieces are the **`mitigation` subtract step** (§6) and **generalized pools** (§5). Everything else is remapping.

---

## 5. Generalized resource pools

Today only HP is a live pool (`combat/vitals.go` — `Vitals` is HP-only). `mana`/`movement` exist as *stat-max keys* with deduction ops (`DeductMana`/`DeductMovement`, `progression/resolution.go:48`) but no general pool type. Three settings need: HP, mana/One Power, Essence, Edge, Stun.

### 5.1 Pool model

A pool is **content-declared**, engine-instanced: a named pool with `current` / `max` (max fed by a `<pool>_max` channel) plus a small set of composable **rules**:

- `regen` — per-tick or per-rest recovery (HP, mana).
- `overflow_to: <pool>` — excess damage spills (SR Physical overflow → death track).
- `degrades: <channel>` — pool value caps a channel (Essence caps `magic`); this is the SR-specific bit and it's just "pool current clamps a channel max."
- `spend` — drained by ability cost (mana, Edge), already modelled by the `Deduct*` ops generalized to `Deduct(pool, amount)`.
- `depletion_event` — fires the existing `VitalDepleted` seam when a flagged pool hits zero (HP, both SR monitors).

`Vitals` becomes the reference implementation of one pool; the others are instances of the same struct with different rules. This is the **highest-leverage change** and it is **already on the WoT critical path** — the One Power pool (WoT EPIC S2, memory `s2-one-power-scoping`) needs exactly this and currently `DeductMana` is a no-op stub. Build it for One Power; Essence/Edge/Stun fall out.

### 5.2 Essence is not special

Essence reads as exotic but decomposes cleanly: a pool that starts full, only ever decreases (cyberware install = `Deduct(essence, cost)`), and whose `current` clamps the `magic` channel max via the `degrades` rule. No new mechanic — a pool with one rule.

---

## 6. The mitigation step (the one new resolution piece)

Today damage is `clamp(weapon + STRBonus, ≥1)` applied straight to HP (`combat/autoattack.go:280-286`). To host SR's soak (and WoT's future typed armor, weapon-identity §4.5 / S1), insert one step:

```
final = max(floor, (weapon_dice + damage_bonus) - mitigation)
```

- WoT keeps `mitigation` folded into `defense` (AC) → set the channel to 0, behavior unchanged.
- SR sets `mitigation = body + armor` → the soak step bites.
- `floor` (min damage on a hit) stays the configured `≥1` rule.

Damage also gains a `target_pool` (which pool it routes to) and a `type` so `mitigation` can be type-specific later. This is a struct-ifying of the current single int — modest, and it's wanted by WoT S1 typed damage regardless.

---

## 7. The mapping mechanism

Two tiers, both already in the codebase — no new runtime:

1. **YAML formula expressions** for the common cases: `defense: reaction + intuition`, `attack: bab + mod(str)`. A tiny arithmetic evaluator over `{attribute names, mod(), +, -, *, /, min, max, constants}`. Covers ~all of WoT and SR.
2. **Lua `derive(stats) → channels`** for anything richer (conditional mappings, SR's "melee uses Strength, ranged uses weapon"). The sandboxed gopher-lua runtime (`internal/scripting`) and bus bridge already exist; this is a new exposed function, not a new subsystem.

Derived channels are recomputed on the same dirty-flag cadence as `StatBlock.Effective` (`progression/statblock.go:252`) — a channel cache invalidated by any attribute/modifier/equipment change. The combat round reads `entity.Channel(attack)`, never an attribute.

---

## 8. What stays hardcoded (deliberately)

- The **resolution primitives** themselves: `d20 + mod vs difficulty` (to-hit, saves) and proficiency×variance (abilities). Spirit-not-fidelity means these never change shape.
- The **tick/event/round** architecture.
- The **channel vocabulary** is curated — content cannot invent a channel with no kernel consumer.
- Ability **handler tokens** (`damage`/`heal`/…) stay Go-side for now; pushing them to Lua is a separate, later effort (out of scope here).

---

## 9. Implementation sequencing

Each step pays for itself even if the program stops there.

1. **Generalized resource pools** (§5). Lowest risk, immediate WoT payoff (unblocks S2 One Power), turns `DeductMana` from a stub into a real op. *Do this first regardless of the multi-ruleset goal.*
2. **Content-defined attribute schema.** Remove the six-stat assumption from chargen / `score` / display / caps (`statblock.go:50-66`, `DefaultPlayerBase` at `:94`). `StatBlock` is already `map[StatType]int`; the friction is UI + chargen, not storage. Unblocks any non-six-stat system.
3. **Damage struct + `mitigation` step** (§6). Wanted by WoT S1 typed damage anyway.
4. **Channel layer + mapping** (§3, §7). The piece that directly answers "different stats for different things." Fully exercised only once a non-WoT pack exists.

---

## 10. Validation gate (do before writing code)

A one-doc paper spike, narrower than the WoT EPIC:

- [ ] The §3 channel table has a kernel consumer for every channel a SR firefight and a WoT channeling exchange touch.
- [ ] §4.1 WoT mapping reproduces *current* engine behavior with no resolution change (proves WoT migration is lossless).
- [ ] §4.2 SR mapping resolves one full combat round (attack → soak → route to Physical/Mental pool) using only the channel set + the §6 step.
- [ ] Essence (§5.2) and Edge model as pools-with-rules, no bespoke code.
- [ ] One Power (WoT S2) uses the same pool model as #1 — confirm the abstraction serves both tracks before committing.

If any box fails, the channel set is incomplete — fix the table, not the kernel.

---

## 11. Configuration surface

| Knob | Default | Notes |
|---|---|---|
| channel default `defense` | 10 | unmapped `defense` reads this (the current "no armor" AC) |
| channel default `attack` / `damage_bonus` / `mitigation` | 0 | unmapped → no contribution |
| damage `floor` | 1 | min damage on a hit (existing rule) |
| pool `regen` cadence/amount | per-pool | content-declared |
| mapping mechanism | YAML expr; Lua opt-in | §7 |

---

## 12. Open questions

- **Initiative.** The engine has no turn order — combat is tick-cadence. Does `initiative` reorder same-tick actions, or is it cosmetic for now? (SR leans on it; WoT/generic can ignore it.) Likely defer until a setting needs it mechanically.
- **Per-attack pool routing.** How does an attack declare it targets Stun vs Physical (SR stun baton vs bullet)? Probably a damage `type` → `target_pool` map in the mapping file.
- **Channel formula scope.** Is the YAML evaluator enough, or does SR's gear-modified attributes (sheet shows `Body 6 (7)` — base vs augmented) force everything through the existing modifier pipeline first? Lean: augmentations are just `equipment:`-sourced modifiers on attributes; channels derive from `Effective`, so this is free.
- **Skill-group model.** SR skill groups / specializations vs the current flat proficiency map — likely a content concern, but flag if it pressures the proficiency engine.
- **Two condition monitors and death.** Does zero-Physical = death and zero-Stun = unconscious map onto the single `VitalDepleted` seam with a flag, or need a second event? Lean: flag on the event.
