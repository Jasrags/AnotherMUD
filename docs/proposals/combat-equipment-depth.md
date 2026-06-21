# Proposal: Combat & Equipment Depth (WoT weapon/armor system)

**Status:** Draft / for discussion · **Type:** Feature-cluster proposal (pre-spec) · **Audience:** engine + content
**Feeds into:** a future `weapon-identity.md` spec (the recommended first slice) plus separate `ranged-combat.md` and `armor-depth.md` specs
**Builds on:** [`docs/specs/combat.md`](../specs/combat.md) (melee-only today), [`docs/specs/inventory-equipment-items.md`](../specs/inventory-equipment-items.md) (slots + 2h footprint), [`docs/specs/progression.md`](../specs/progression.md) (proficiency is ability-keyed today)
**Reference:** [`docs/wot/equipment.md`](../wot/equipment.md) — the WoT RPG (d20) weapon/armor tables this arc draws fidelity targets from

## 1. Problem / motivation

The Wheel of Time content track wants weapons that *feel* like the source material — a Two Rivers longbow that out-ranges a sling, an Armsman who is genuinely better with a battleaxe than an Initiate, a rapier whose wide threat range plays differently from a mace. `docs/wot/equipment.md` describes all of that: Simple/Martial/Exotic proficiency tiers, melee vs. ranged with range increments and ammo, size-relative wielding (light/1h/2h, 1.5× Str two-handed), crit threat ranges + multipliers, damage types (B/P/S), masterwork grades, and a full armor table (armor bonus / max Dex / check penalty / don timers).

The engine supports **almost none of it.** A weapon today is a single `weapon_damage` dice string plus a few stat `modifiers` (`internal/item/template.go`). Damage is `weapon dice + STR bonus`, unarmed defaults to `1d3` (`internal/combat/damage.go`). There is **no ranged combat** (melee-only, same-room — `combat.md §4.3`), **no weapon-proficiency gating** (proficiency is ability-keyed, not weapon-category-keyed; anyone wields anything penalty-free), **no damage types** (single AC; per-type AC deferred to "M8+"), and **no crit/threat** (combat.md calls it an unimplemented "policy decision"). The 2-handed footprint (`wield` + `offhand` companion) works; the smithing → weapon content loop works. Everything else in the WoT table collapses to "different dice."

This is **the WoT equipment ambition meeting a thin engine.** The longbow forces the decision: with no ranged model it is either a flavor melee weapon or it stays unbuilt. The whole weapon table inherits that fork. This proposal decomposes the arc so the team can pick a fidelity target deliberately instead of being stampeded by one bow.

## 2. Goals & non-goals

**Goals.** Name the full arc and its dependency order so increments ship as coherent, independently-valuable slices. Identify the smallest bundle that makes WoT weapons feel distinct (the **A+B+C** "weapon identity" slice). Keep ranged combat and armor depth as separate, properly-specced themes rather than hacks bolted onto a content push. Capture the WoT-equipment-to-engine mapping so the design survives the session.

**Non-goals.** Not committing to the full d20 port — most of `docs/wot/equipment.md` (encumbrance math, don timers, the long tail of special-weapon handlers, mounts, grenadelike weapons) is cherry-picked indefinitely, never "finished." Not deciding the fidelity tier here — that is the steering decision this document exists to inform. Not authoring weapon content — that is the WoT pack track (`wot-setting-plan` memory, M4); this is the engine substrate beneath it.

## 3. The decomposition (A–J)

Each row is independently shippable except where a dependency is noted. Sizes are rough (S/M/L).

| # | Increment | WoT features unlocked | Depends on | Spec? | Size |
|---|---|---|---|---|---|
| **A** | **Weapon category + proficiency tier metadata** | Simple/Martial/Exotic tier; weapon "kind" (sword/axe/bow) as real data, not free-form tags | — | thin spec | **S** |
| **B** | **Proficiency gating** (the −4 rule) | Class proficiency sets (Armsman→all martial, Initiate→few); non-proficient hit penalty | A; class system | spec slice | **M** |
| **C** | **Crit threat-range + multiplier** | 19–20/x2, x3, x4 columns; threat range + crit dice | combat.md (flags it) | spec slice | **M** |
| **D** | **Damage types (B/P/S)** | Bludgeon/Pierce/Slash on weapons | — (inert until E) | **consumed by E** ([`armor-depth.md`](../specs/armor-depth.md) §4) | **S**, low payoff alone |
| **E** | **Armor depth** | armor bonus / max-Dex / check penalty / per-type **resistance** (mitigation); armor proficiency | D (type→mitigation); slots | **spec written** ([`armor-depth.md`](../specs/armor-depth.md), build pending) | **L** |
| **F** | **Size-relative wielding** | light/1h/2h by (weapon size − wielder size); 1.5× Str two-handed | A | **spec written** ([`size-and-wielding.md`](../specs/size-and-wielding.md), build pending) | **M** |
| **G** | **Ranged combat** | bows/crossbows/thrown; range increments; ammo as consumables; Str-on-thrown-not-projectile | combat-model change | **spec written** ([`ranged-combat.md`](../specs/ranged-combat.md), build pending — range bands within one room; cross-room deferred) | **L** |
| **H** | **Masterwork / masterpiece / power-wrought** | +N attack/damage item grades; unbreakable power-wrought | A, C | **spec written** ([`masterwork.md`](../specs/masterwork.md), build pending) | **S–M** |
| **I** | **Encumbrance / carry weight** | weight caps, armor speed penalty, the `Wt` column | container caps (specced §1) | spec slice | **M** |
| **J** | **Special-weapon handlers** | reach, set-vs-charge, trip, disarm, net entangle, whip, swordbreaker | A + most of the above | per-weapon | **L, open-ended** |

## 4. Dependency notes (what order, and why)

- **A is the keystone.** B, C, F, H, and J all need weapons to *know what they are* beyond a dice string. A is small and unlocks the most — do it first if you do anything.
- **B is the highest flavor-per-effort.** Proficiency gating is what makes "Armsman vs. Initiate" mean something, and it needs only A plus a hit-penalty hook combat already has. This is the headline WoT mechanic.
- **D is a trap alone.** Damage types are inert until armor (E) differentiates them. Record them in A's metadata for free; do not build D as a standalone feature.
- **C is self-contained.** Crit threat range + multiplier needs nothing but the hit-resolution path and combat.md's own pending "policy decision." Gives a rapier (18–20) a different feel from a battleaxe (x3).
- **G (ranged) is genuinely separate.** It is a combat-*model* change, not a weapon-*data* change. It does not depend on A–F and they do not depend on it. It is the only thing that makes the longbow real. Schedule it as its own milestone when ranged matters — do **not** let M4's one longbow pull it forward into a half-baked hack.
- **E (armor) is the other large block** and pairs with D, F, and I — Max-Dex / check-penalty / encumbrance are one interacting system in the sourcebook. Per-damage-type AC was always the deferred "M8+" work.
- **J is bottomless.** Special weapons are content-driven tags the combat pipeline switches on, added one at a time, never finished.

> **✅ SHIPPED 2026-06-10 — M-Weapon-Identity (A + B + C).** The spec
> (`docs/specs/weapon-identity.md`) and all three increments landed: A (weapon
> category / proficiency tier / damage-type metadata + validation), B
> (class-granted proficiency + the non-proficient to-hit penalty composing into
> the existing `HitModAdjust` seam, + a non-proficient-on-equip cue), C
> (per-weapon crit threat range + multiplier), plus demo content (a martial Two
> Rivers longsword + an exotic ashandarei in Haral's forge) and an end-to-end
> chain test. **Remaining in this proposal:** D (damage-type effect, with E),
> E (armor depth), F (size-wield), G (ranged), H (masterwork), I (encumbrance —
> carry-weight shipped separately), J (special weapons).
>
> **J (special weapons) SPECCED + STARTER SET + FIRST TAIL SLICE SHIPPED** —
> `docs/specs/special-weapons.md` establishes the `special:` weapon-tag seam later
> J behaviors reuse, and has shipped slices 1–5: the metadata substrate (template
> + loader validation + instance accessor), **reach** (the range-band-gate
> extension), weapon-aware **trip** (the DC bonus), **disarm** (the new save-gated
> `disarmed` condition + verb) — the starter set (2026-06-17) — and **set vs a
> charge** (§6, a braced polearm's bonus blow on a charging foe, riding the band
> auto-close; `ANOTHERMUD_SET_DAMAGE_BONUS`) as the first tail slice (2026-06-20).
> The bottomless tail (net/entangle, whip, swordbreaker-breaking, double-weapon
> second die, lance charge) stays deferred on the same seam.
>
> **F (size-wield) SPECCED 2026-06-15** — `docs/specs/size-and-wielding.md`
> (forward spec, build pending): creature size + weapon size, the wield mode
> derived from their ordered distance (light / one-handed / two-handed /
> too-large), and the two consequences (the equip footprint via the existing
> companion-slot seam + the two-handed Strength factor on `combat §4.5`). Size
> derives the footprint when both sides carry size data; otherwise the static
> footprint is the fallback (legacy weapons unchanged). Two-weapon penalties and
> ranged Strength rules stay out of scope (their own increments).
>
> **E (armor depth) + D (damage types) SPECCED 2026-06-15** —
> `docs/specs/armor-depth.md` (forward spec, build pending), resolving the §7
> AC-model fork. Single AC on the `defense` channel (decompose-and-cap: armor
> bonus + max-Dex cap, shields stack, legacy unchanged) + **per-type damage
> resistance on the `mitigation` channel** (already subtracted from damage as a
> scalar; this slice makes it per-type and is its first real source — the
> cross-ruleset soak primitive for physical *and* elemental resistance), plus
> armor proficiency, the check penalty, and don/doff timers. D's damage type is
> consumed as the mitigation key. Speed/encumbrance (I) and shield bash (J) stay
> out of scope.
>
> **H (masterwork) SPECCED 2026-06-15** — `docs/specs/masterwork.md` (forward
> spec, build pending): an ordered quality-grade ladder (masterwork / masterpiece
> / power-wrought) whose bonus rides existing seams — weapon to-hit, power-wrought
> `damage_bonus`, armor check-penalty (`armor-depth §6`), tool skill-check — plus
> the power-wrought unbreakable flag (a forward hook; no durability system yet).
> The mechanical grade is kept **independent** of the cosmetic rarity/essence
> decoration per `item-decorations §1.1`. Masterwork ammo defers to ranged (G);
> visible-gear Reputation defers to S8.
>
> **G (ranged) SPECCED 2026-06-16** — `docs/specs/ranged-combat.md` (forward
> spec, build pending), resolving the §7 ranged-model fork. Range = **abstract
> per-engagement bands within one room** (far → near → melee; archer out-ranges,
> melee closes band by band, advance/withdraw = kiting) — **not** cross-room
> targeting (Model C, deferred as its own theme). Built **A-first** (same-room
> ranged mechanics: ranged flag + ammo-as-consumables + thrown/projectile Str
> rules) then **B** (the band state). Masterwork ammo lands here. All inside
> `internal/combat`; the same-room invariant is preserved.

## 5. Recommended first slice: M-Weapon-Identity (A + B + C)

If the goal is "WoT weapons that feel distinct and matter," the **A + B + C** bundle is the sweet spot — one coherent S–M theme, **no ranged, no armor overhaul**:

- **A** gives every weapon a tier and a kind.
- **B** makes class proficiency real — the headline WoT mechanic, and it gives the *existing* classes teeth immediately.
- **C** gives the crit column teeth so weapon choice is more than max damage.

It makes current weapon content meaningfully different *today*, and it cleanly precedes a later ranged milestone (G) and a later armor milestone (E) without blocking either. Suggested spec name: `weapon-identity.md`, covering A+B+C; D's metadata fields ride along (recorded, not yet consumed).

## 6. Sequencing options

- **Path 1 — content-first (keep geography moving).** Author WoT weapons now at **Tier 0** (dice + modifiers; longbow flagged flavor-melee in content, not a fake range mechanic). Pick up M-Weapon-Identity (A+B+C) as the next engine theme when ready. Lowest disruption; weapons stay shallow until the engine theme lands.
- **Path 2 — identity-first.** Spec + build M-Weapon-Identity (A+B+C) before/with the M4 weapon push, so WoT weapons land already mechanically distinct. More up-front engine work; weapons feel WoT from the start.
- **Path 3 — ranged milestone.** If the bow specifically is the point, spec `ranged-combat.md` (G) as a standalone milestone. Largest scope; do this when ranged is a deliberate goal, not to satisfy one content item.

Armor depth (E), size rules (F), encumbrance (I), masterwork (H), and special weapons (J) are later themes in any path.

## 7. Open questions / pre-decisions

- **Proficiency representation (A/B).** Reuse the ability-keyed proficiency store with weapon-category keys, or a parallel weapon-proficiency set on the character? The WoT model is class-granted tiers + per-weapon feats — closer to a set than to use-based gain.
- **Where the non-proficient penalty applies (B).** Hit-mod only (the −4), or also a damage/speed cost? combat.md's hit path is the natural seam.
- **Crit semantics (C).** Doubled dice vs. fixed bonus vs. max-plus-roll; threat range interacts with any future to-hit-roll model (combat is currently chance-based, not a d20 roll — does C imply a roll model?).
- **AC model (D/E). ✅ RESOLVED 2026-06-15** (`armor-depth.md` §3–§4, §10). Armor **layers** onto a decomposed single AC (`defense` channel) — armor bonus + a max-Dex cap on the Dex term; unarmored/legacy AC unchanged ("replace" rejected — d20 armor is additive-with-a-cap). **Per-damage-type differentiation lives on the `mitigation` channel** (damage resistance / soak), **not** on AC — per-type *AC* was a category error (you resist fire, you don't dodge it). This is the cross-ruleset soak primitive: WoT leans on `defense`, a future Shadowrun pack leans on `mitigation` (physical + elemental resistance). Damage types (D) are consumed here as the mitigation key.
- **Ranged model (G). ✅ RESOLVED 2026-06-16** (`ranged-combat.md` §1, §5, §9). "Range" = **abstract per-engagement range bands within one room** (far → near → melee) — the archer out-ranges and the melee opponent closes band by band — built **A-first** (same-room ranged mechanics) then **B** (bands). **Not** cross-room targeting (Model C — a separate theme, deferred: it would invert the same-room invariant + add LoS/two-room render). Ammo = **standard consumables** (no quiver slot; thrown lands recoverable, projectile consumes matching ammo, masterwork ammo destroyed on use). Str = **full on thrown / none-positive on plain projectile (negative still applies) / capped on a Strength-rated bow**.
- **Fidelity ceiling.** How much of the d20 system is actually wanted vs. a lighter MUD-idiomatic abstraction? The sourcebook is a tabletop ruleset; a real-time MUD may want less.

## 8. Relationship to other work

- **Crafting/smithing (M27, shipped) + biomes/gathering (specced §1)** already feed weapon *content* — the ingredient→ingot→blade loop. This arc is the *system* beneath the weapons that loop produces; the two are orthogonal and complementary.
- **Item rarity** (shipped) is the natural carrier for masterwork/power-wrought grades (H).
- **Container caps** (specced §1) are a prerequisite for encumbrance (I).
- **Visibility / light-and-darkness** (specced / shipped) will interact with ranged (G) line-of-sight if ranged ever spans rooms.

---

*This is a proposal, not a spec. The first deliverable when this arc starts is the `weapon-identity.md` spec slice (A+B+C) — or, if ranged is the goal, `ranged-combat.md` (G). Resolve the relevant §7 pre-decisions before writing the spec.*
