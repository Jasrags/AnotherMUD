# NukeFire — Magic & Skills

Source: https://nukefire.org/wiki/skills (and representative spell/skill topic pages)
Fetched: 2026-06-18

Deep-read topics for this doc: `skills`, `practice`, `cast`, `spellpower`, `hpr`, `chant`, `eldritch-2` (Eldritch Cataclysm), `animate-dead`, `augury`, `arcane-torrent`, `suppress-fire`, `ambush`, `advanced-knifeplay`, `backstab`, `identify`, `sell` (Sell Your Soul). Other ability slugs below are characterized from the class ladders in `classes-and-progression.md` and the index, and are noted as index-only where not individually fetched.

---

## How skills/spells are learned and improved

NukeFire is **use-based, not practice-based**. From `skills`:

> "You don't just get these skills handed to you... The more you use a skill in the game, the more it levels up."

`practice` confirms the old Diku/Circle Practice command is **deprecated** — there is no trainer/practice-session economy; mastery comes from *using* abilities in real encounters. Abilities unlock at class+level thresholds (the ladders in `classes-and-progression.md`), then **improve through repeated use** in combat/play.

Ability *kinds* observed:
- **Active commands** — typed (`backstab`, `ambush`, `chant`, `supfire`, `arcane torrent`).
- **Cast spells** — `cast '<spell>'` (mage-line) or `sling '<spell>'` (Curist/Slinger-line). Spell names with spaces go in apostrophes.
- **Innate toggles** — e.g. Assassin Shadowform, persistent until toggled.
- **Passives** — always-on while a condition holds (Advanced Knifeplay while wielding a knife; Barehand Proficiency).
- **Procs** — passive follow-ups fired *inside* another skill or normal melee (Barbarian's finishing move / flying elbow / throat punch chain ride inside Tornado Suplex + plain hitting).

---

## Two casting verbs: `cast` vs `sling`

| Verb | Used by | Syntax |
|---|---|---|
| `cast` | mage-line / generic spells | `cast '<spell>'` / `cast '<spell>' <target>` (e.g. `cast 'magic missile' guard`) |
| `sling` | Curist, Slinger and other casters per help | `sling '<spell>' <target>` (e.g. `sling 'identify' sword`) |

`focus` / `nofocus` toggle spell focus state. Apostrophes around multi-word spell names are required (`apostrophes` help topic exists specifically for this).

**Identify** (`identify`) is the entry caster utility: Slinger L1, Curist L11 — reveals object/creature info; `sling 'identify' <target>`. See also `compare`, `gearcheck`.

---

## Spellpower — the caster damage/heal stat

`spellpower` is NukeFire's caster-side analog of +Damage. Verbatim mechanics:

- Each point of **+Spellpower** grants **+1 flat** to: total Damroll, spell damage, skill damage, melee damage, and outgoing spell healing.
- **Applies only** to classes **Curist, Slinger, Heretic, Voidstriker, Gypsy**. (A Barbarian gets nothing from +Spellpower.)
- Per cast/use/hit; stacks additively.
- Example: a ring of "+4 damage and +4 spellpower" gives a Barbarian +4 damage only; an eligible caster gets **+8 damroll and +4 spell output** (damage or healing).
- Healing is **scaled/capped** so cheap spells (e.g. Cure Light) can't heal absurd amounts for tiny mana.
- **Penalty:** for every 4 spellpower a Slinger gets **+1 armor** (worse armor — heavy magic loadout hurts defense).

Can come from remorting and equipment.

---

## Spell schools / families (by class flavor)

NukeFire has no formal "school" enum surfaced in help; spell families track class identity:

| Family | Representative spells | Source class(es) |
|---|---|---|
| **Holy / divine** | cure light, cure critic, cure blind, heal, restoration, rejuvinate, bless, sanctuary, armor, prot-from-evil/good, dispel evil/good, remove curse/poison, augury, the **Aura** line (protection/healing/escape/invigoration/rejuvination/sanct) | Curist, Knight, Heretic |
| **Arcane / slinger** | magic missile, fireball, firestorm, lightning bolt, chill touch, shocking grasp, burning hands, color spray, disintegrate, charm person, sleep, blindness, curse, energy drain, identify, locate object, teleport, **arcane torrent** (capstone) | Slinger, Voidstriker |
| **Void / eldritch** | eldritch cataclysm, voidwarp, voidstep, voidpunch, voidforge, voidharvest, wraithfire, soulshredder, phoenix-nova | Voidstriker, Occultist (prestige) |
| **Psionic / mutant** | emit, spew, psionicwave, psiattack, psiblast, mind crush, rend, radiation blast | Mutant, Kaiju |
| **Occult / dark pact** | chant litanies (Devotion), mark of the outer dark, animate dead, voodoo / sell-your-soul contract, the brass/black/grave litany set | Occultist, Gypsy, Heretic |
| **Nature** | nature's renewal, bark skin, create water, control weather, animal spirit | Ranger, Wolfman |

### Worked spell examples (deep-read)

**Eldritch Cataclysm** (`eldritch-2`) — Voidstriker L45. `sling 'eldritch cataclysm' <target>`. Initial damage **scaled by caster level + spellpower**, then a **damage-over-time (DOT)** while affected. **Cannot re-apply** while the target is already affected. Usable in combat or to initiate.

**Arcane Torrent** (`arcane-torrent`) — Slinger **capstone, L50**. Combat-only; fires only when **mana < 1000 and movement ≥ 100**, spending **movement** to unleash repeated arcane damage with possible lightning/fire/cold surges. Won't fire in peaceful rooms, on self, or on group members. (Note the inverted resource: it's a *low-mana* dump that spends *movement*.)

**Animate Dead** (`animate-dead`) — necromantic raise/call-the-dead spell, `cast 'animate dead'`, "where the spell is allowed."

**Augury** (`augury`) — Curist L26 omen-read; `sling 'augury'`; quiet pre-danger guidance.

**Chant** (`chant`) — Occultist L1. Speaks **litanies** that spend **Devotion** (the Occultist resource) for brief focused effects that **do not count as spells**. Some litanies are stronger if the target bears your **Mark of the Outer Dark**. Silenced throats can't chant; room effects may disrupt.

**Sell Your Soul** (`sell` → signcontract) — Gypsy L40 dark pact: costs **400 mana**, creates a persistent **Voodoo Contract**, grants **+1000 (+50–150) HP**, **+300 (+50–150) move**, **+17 +¼level damroll**, and **hard-sets alignment to -1000**. Re-casting breaks and replaces the existing contract; can fail on a skill-vs-random roll.

---

## Active combat skills (deep-read examples)

**Backstab** (`backstab`) — `backstab <target>`. Requires wielding a **valid piercing weapon**. On success: a real weapon strike **plus a scaled wound packet**, and can **open the target's guard** (easier to hit, worse dodge) for a short time. Strong performance may add follow-up wounds; certain **concealed-weapon** setups create an extra hidden strike. Failure: no damage, action consumed, may leave you open. See `circle`, `disembowel`, `conceal`.

**Ambush** (`ambush`) — `ambush <target>`. Surprise opener; success scales on ambush skill vs target level; deals direct skill damage and can **stun**. Failure does nothing and may leave you open.

**Advanced Knifeplay** (`advanced-knifeplay`) — Gypsy & Ninja L25, **passive**, always-on while wielding a knife. Adds per-hit knife damage + a chance of **extra hits per round** (both scaled by skill probability + level). Complements **Knife Mastery**.

**Suppress Fire** (`suppress-fire` / `supfire`) — Outlander (prestige). Requires ≥1 firearm. Reloads your gun(s) then **empties them in a mag-dump** (both guns if dual-wielding firearms). AOE "wall of lead"; capped rounds per burst; guns end empty and must reload again. Heavy combat-recovery cost.

---

## Resources

Beyond HP/Mana/Move, classes have **bespoke resource pools**:
- **Mana** — standard caster pool (`mana`, `manasacrifice`, `bloodmana`).
- **Movement** — also spent by some skills (Arcane Torrent spends move, not mana).
- **Devotion** — Occultist litany resource (spent by `chant`).
- Cyborg tech uses **Overboost/Turbo/heat** mechanics (`overboost`, `turbo`, `heatsinkbreach`, `optimized-turbo`).
- Mutant **Devour** regains mana by eating corpses.

---

## Hits per round & fight speed (cross-ref)

Ability output is multiplied by **HPR** (hits per round) and gated by **fight speed** — both detailed in `combat-mechanics.md`. Key from `hpr`: base HPR is per-class; **<200 lbs total weight → +1 hit**; **every 500 lbs carried → -1 hit**; heavy negative AC tiers subtract hits; **>50 remorts → +1 hit**; class-remort milestones (100/200/300 total) grant extra hits. So caster/skill DPS scales with remort tier and encumbrance discipline, not just spellpower.

---

## Relevance to AnotherMUD

- **Use-based skill gain** matches AnotherMUD's existing proficiency-by-use model — NukeFire is a good reference for how far that can go (it fully replaced the practice economy).
- **`cast` vs `sling` split** is a per-class verb gate, analogous to AnotherMUD's command-registry class gating.
- **Spellpower as a class-restricted universal scalar** (with a defensive penalty for casters) is a clean single-stat design worth noting vs. AnotherMUD's channel/spellpower layer.
- **Bespoke per-class resources** (Devotion, Overboost/heat) over a shared pool substrate mirror AnotherMUD's `internal/pool` + channel-map approach.
- **Inverted-resource capstones** (Arcane Torrent firing only at *low* mana, spending *movement*) are a nice anti-spam design pattern.
