# Proposal: The One Power (channeling) — WoT EPIC S2

**Status:** Phases 0–3 SHIPPED (Phase 3 = affinities, 2026-06-14) · Phase 4+ open · **Type:** the marquee
WoT sub-epic, a **multi-slice arc** (XL)
> **Shipped:** Phase 0 (generalized resource-pool substrate + persistence + regen +
> the spend knobs reserve-to-begin / spend-on-success), Phase 1 (the `channeler` class
> with a starting One Power pool, the classic-four weaves, the `channel` verb),
> Phase 2 (the `overchannel` verb → Fortitude save → fatigued/stunned/**stilled**
> cascade, with stilling blocking channeling), and **Phase 3 — affinities & the Five
> Powers** (an `elements` field on abilities; gender collected at creation, save
> **v22**; gender-derived two-tier affinity — men Earth/Fire/Spirit, women
> Air/Water/Spirit — driving **soft potency scaling** of weave damage/heal, weakest
> element governing). Phase 4+ (Initiate/Wilder split, the combat interrupt game,
> taint/madness, angreal, linking, a restore path for stilling; v1 affinity scales
> damage+heal only — save-DC + buff-modifier scaling are an affinity follow-up)
> remains open. The phase text below is the original design spine.
**Implements:** EPIC sub-epic **S2** — `docs/themes/wot-mechanics-epic.md` §2 row S2
**Builds on:** `internal/progression` (abilities, proficiency, effects, saves),
`internal/combat` (vitals, `ResolveSave`, heartbeat), `internal/session` (`connActor`,
the dormant `Mana()`/`DeductMana()` stubs), `internal/command` (verb + typed args),
`internal/player` (the save)
**Governed by:** EPIC **Decision 0** — translate WoT onto the tick/chance model; no
d20 rewrite. And the character model: `docs/proposals/wot-character-model.md` (D1
multiclass seam, D4 creation-time gender/category gating).
**Sources:** `docs/wot/the-one-power.md` (the d20 RPG extract) ·
`docs/research/wotmud-channeling.md` (how the shipped WoTMUD real-time MUD translated it)

---

## 1. Why this note exists

The One Power is *the* reason to do a Wheel of Time setting — channelers drawing
saidin/saidar, weaving the elements, risking too much. It is also the **largest and
most cross-cutting** WoT system: it touches a new resource pool, abilities, effects,
saves, conditions, combat timing, the player save, content packs, and a new verb. It
deserves a written spine before any code so the phases land in the right order and
nothing gets built on a model that later needs restructuring.

This note resolves S2's open decisions, fixes the **weave-as-ability contract**, and
defines a **phased build** whose first slice (Phase 0) is a non-WoT-specific engine
substrate that also closes a standing BACKLOG item.

## 2. The two reference points, and why we land where we do

Our design is bracketed by two prior translations of WoT channeling:

- **The d20 RPG** (`docs/wot/the-one-power.md`): per-level **daily weave slots**,
  weaves as known spells, **Initiate** (Int) vs **Wilder** (Wis) classes, a secret
  **Madness rating**, Concentration→Fortitude overchannel cascade, full **linking**
  tables. Much of this is tabletop bookkeeping Decision 0 says to **not port**.
- **WoTMUD** (`docs/research/wotmud-channeling.md`, the only long-running shipped WoT
  MUD): **a single mana pool** ("Spell Points") from one stat, slow tic-refill;
  weaves as **use-improved % skills**; **one Channeler class**; the saidin curse
  expressed **socially**, no madness meter; overchannel risks **permanent stilling**;
  **no player linking**; combat is **cast-time-vs-round chess where interruption
  refunds the resource**.

**The shipped MUD discarded almost all d20 machinery** and landed where our engine
*already stubs the pieces*: `StatResourceMax`/`DeductMana` is the SP pool; our
use-based **proficiency** system is the use-improved weave skill; **S5 conditions** +
**S6 saves** are the overchannel cascade. Decision 0 and the only shipped prior art
agree. So we translate toward the WoTMUD shape, keep the d20 *flavor and meaningful
choices*, and drop the d20 *bookkeeping*.

## 3. What the engine already gives us (verified in code)

The load-bearing finding: **a weave is just an ability, and almost everything is
already wired.**

- **Weaves = abilities with `category: spell`.** That is already the engine's
  mana-drawing active-ability category (`progression/ability.go`). Weave content drops
  into `content/wot/abilities/*.yaml`, auto-loads through the loader
  (`pack/loader.go decodeAbility`), and flows through the resolver
  (`progression/resolution.go AbilityResolver.Resolve`), proficiency, effect
  application, and save-gating — **zero new engine code** for the weave-as-spell path.
- **Save-gated effects are wired:** `apply_save` (entry) and `recurring_save`
  (shake-off) on the ability/effect YAML call `combat.ResolveSave` (S6, shipped). The
  `bash.yaml` ability is the reference pattern — a save-gated condition applier.
- **Conditions (S5, shipped)** give weaves a vocabulary to apply: stun, blind, prone,
  frighten, fatigue — flagged effects the combat phases already react to.
- **The action queue + combat heartbeat** (`progression/queue.go`,
  `combat/heartbeat.go Phases.Ability`) is the slot a cast schedules into; out-of-
  combat casting rides `ActionQueueManager.PendingEntities()`.
- **Known weaves persist for free** as proficiency entries in the existing
  `AbilitySnapshot` — no new persistence for "what weaves do I know."
- **Eligibility gating** uses the existing creation-time `AllowedGenders` /
  `AllowedCategories` filters (`progression/class.go`) — exactly what saidin/saidar
  and Ogier-can't-channel need.

**The one real gap:** there is **no current Power pool**. `connActor.DeductMana`
(`session/session.go`) is a documented no-op; only the `StatResourceMax` *ceiling*
stat exists. This is already a standing BACKLOG item ("Mana / Movement current pools +
regen", `BACKLOG.md`). **Phase 0 closes that engine debt and unblocks channeling at
the same time.**

## 4. Decisions

### D1 — Resource model: **a single "Power" pool (mana-like), not daily slots.** ✅

A current Power pool that regenerates on a tick, drained per-weave by a declared cost,
gated by a stat-derived ceiling. We **drop the d20 daily-slot table** (the bookkeeping
Decision 0 names for removal) and follow WoTMUD's shipped choice. The substrate is
~80% stubbed already (`StatResourceMax` ceiling, `ResourceMana` cost handling in
validation/resolution, the `DeductMana` no-op waiting to be wired).

- **Ceiling:** `StatResourceMax` (an existing canonical stat), derived from the
  channeler's governing stat at class-grant time (the d20 keys it to Int for
  initiates / Wis for wilders; WoTMUD keys SP to Willpower — we key it to our
  governing stat, exact formula in the Phase 0 spec, externalized via config).
- **Cost:** each weave ability declares a Power cost (the existing `ResourceMana` cost
  field). Bigger weaves cost more.
- **Spend-on-success + reserve-to-begin (stolen from WoTMUD):** Power is deducted when
  the cast **completes**, not when it starts; an interrupted/failed cast costs
  **tempo, not Power**. To *begin* a weave you must hold a **reserve multiple** of its
  cost (WoTMUD uses 2×) — a cheap "you need headroom" gate. Both the multiple and
  spend-on-success are config knobs.
- **Regen:** a tick handler refills the pool at a configured rate (slow, like
  WoTMUD's tic-gated refill — channeling should not be spammable). Rides the existing
  regen tick seam (`session.Manager` regen) or a sibling handler.

*Rejected:* the d20 daily-slot budget (Vancian per-level slots + a rest-to-recover
loop the engine doesn't have) — tabletop bookkeeping, and not how the shipped MUD or
our stubs work. A **hybrid pool+slot-cap** stays available as a later tuning lever if
pure-pool play feels unbounded, but is **not** v1.

### D2 — Weaves are **use-based proficiencies that resolve as `spell` abilities.** ✅

A weave is an `Ability{Type: active, Category: spell}` plus a proficiency entry that
climbs with use — exactly the convention crafting and S3 skills already proved, and
exactly WoTMUD's use-improved % weaves. Knowing a weave = having its proficiency;
casting it raises the proficiency; higher proficiency improves the cast (success /
effect magnitude, per-weave). **No new content registry** — weaves are abilities.

### D3 — Gender & taint: **gendered Source via creation gating; defer taint→madness.** ✅

- **Affinity = saidin (male) / saidar (female)**, set at character creation and gated
  by the existing `AllowedGenders` filter on the channeler class. Weaves are
  **mechanically mirrored** across the two (the d20 source: "store one weave
  description per spell"; WoTMUD: near-identical save the `seize`/`embrace` verb).
- **Defer taint → madness to its own later slice.** It is an **asymmetric
  player-experience system** (men only) worth designing deliberately, not bolting onto
  the first playable slice. WoTMUD itself expresses the saidin curse *socially*
  (persecution) rather than as a confirmed madness meter; the d20 Madness rating is a
  GM-secret die. v1 ships gendered channeling without the curse; the curse is Phase
  3+.
- **Runtime gender→cast gate** (stopping an admin/quest-granted cross-gender weave
  from being *used*) is the noted character-model D4 gap; **flag, don't build** in v1
  — creation gating covers normal play.

### D4 — Class shape: **one `channeler` class for v1; the seam to split is free.** ✅

WoTMUD ships one Channeler class; d20 splits Initiate (Int, Talents) vs Wilder (Wis,
emotional Block). Our engine's multiclass seam (character-model D1, save v18, shipped)
makes splitting into two classes **cheap content** later. For v1 we ship **one
`channeler` class** (simpler, fully playable) bound to a `one-power` track, and note
the seam. The Initiate/Wilder distinction, Talents, and the emotional Block become a
later depth slice if wanted.

### D5 — Affinities / the Five Powers: **deferred to a later phase; weaves carry the tags now.** 

The Five Powers (Air/Earth/Fire/Water/Spirit) and per-channeler Affinities are core
WoT flavor and WoTMUD's "specialize-vs-diversify" build identity, but they add an
*eligibility/effective-level* layer on top of casting. **v1 weaves carry their
element tags as metadata** (so content is authored correctly), but the
affinity-adjusts-eligibility math is **Phase 3**. This keeps Phase 1 to "can I afford
and cast this weave," not "do my affinities permit it at what effective level."

### D6 — Overchannel: **the signature risk choice, built from shipped primitives.** 

Keep the *choice* (cast beyond your safe capacity at real risk); drop the d20
Concentration-check-then-Fortitude-cascade arithmetic. Translation: attempting a weave
you can't afford (below the reserve gate, or above your ceiling) may be **overchanneled**
— it casts, then forces a **Fortitude save (S6)**; failure applies the **cascade as
conditions/effects (S5)** scaling with the miss margin, up to **stilled** (a permanent
"cut off from the Source" effect that blocks channeling until a restore path exists).
This reuses S5+S6 almost entirely. Overchannel is **Phase 2**.

## 5. The weave-as-ability contract (the load-bearing interface)

A weave is authored as an ability YAML in `content/wot/abilities/`. The contract (v1):

| Field | Meaning | Engine support |
|---|---|---|
| `type: active`, `category: spell` | a castable, mana-drawing weave | exists |
| `cost: {resource: mana, amount: N}` | Power cost (spent on success) | `ResourceMana` exists; spend-on-success is Phase 0 |
| `cast_time` | pulses to resolve (vs the combat round) | rides action-queue pulse-delay; pick values now (D-combat) |
| `handler: damage`/`heal` + dice | the effect payload | exists (combat damage/heal handlers) |
| `effect: {...}` / `apply_save` / `recurring_save` | applied conditions/buffs, save-gated | exists (S5/S6) |
| `elements: [fire, air, ...]` | the Five Powers this weave uses | **new metadata field** (Phase 1 authoring; Phase 3 eligibility) |
| `affinity_gender` (optional) | restrict to saidin/saidar | creation gating covers normal play; flag |
| proficiency entry | "known," climbs with use | exists (proficiency system) |

**Cast-time discipline (from WoTMUD, the future-proofing rule):** even though v1
weaves resolve simply, **pick `cast_time` values against our tick cadence now** so the
later interrupt/tempo game (getting hit aborts a cast — the channeler-side mirror of
S5's "stunned skips a swing") is possible without re-tuning the whole catalog.

## 6. Phasing (the multi-slice arc)

Each phase is its own commit(s) + go-review, in the project's rhythm.

- **Phase 0 — Power pool substrate** *(non-WoT-specific; closes the BACKLOG item).*
  A current Power pool parallel to `combat.Vitals` (HP): spend-on-cast (wire
  `DeductMana` for real), the reserve-to-begin gate, a regen tick, the prompt `MA`
  column showing real current/max, and persistence (save **v21**, default full on
  login). Deliverable: abilities that declare a mana cost actually spend from a live
  pool. **No WoT content yet** — pure engine substrate, testable in the starter world
  with a throwaway costed ability.

- **Phase 1 — channeling is real & playable.** A `channeler` class (one class, bound
  to a `one-power` track, gated by `AllowedGenders` for saidin/saidar) + affinity set
  at creation + a **handful of starter weaves** as `spell` abilities (a damage weave,
  a heal, a utility/light, a save-gated control) costing Power, with `cast_time`s
  chosen deliberately + the **`channel` (alias `cast`) verb** with typed args
  (`ArgKnownWeave` weave name + optional `ArgEntity` target). Live-verify the full
  loop: embrace → channel a weave → Power drains → effect resolves.

- **Phase 2 — overchannel.** Overdraw the pool → Fortitude save (S6) → cascade
  conditions/effects (S5) by miss margin, up to a `stilled` blocking effect. The
  signature risk choice. Possibly a `embrace`/`release` Source state machine if it
  earns its keep (drives the rest/heal blockers and same-gender detection later).

- **Phase 3+ — depth, each its own slice as content demands:**
  - **Affinities & the Five Powers** — element-tag eligibility + effective-level
    adjustment (D5); the specialize-vs-diversify build identity.
  - **Talents / Initiate-vs-Wilder split** — D4's deferred class depth.
  - **Combat interrupt game** — getting hit aborts a cast (tempo cost); `Slice
    Weaves`-style weave interrupts; `Shield` as a cut-from-Source disable.
  - **Madness / the taint** — the deferred asymmetric saidin curse (D3).
  - **Angreal / sa'angreal** — items that add to the pool/effective level.
  - **Linking / circles** — far later; WoTMUD likely never shipped it.
  - **Traveling / Gateways / wards** — overlap S10 (travel) and the world graph.

## 7. Dependencies & what's already paid for

- **S6 saves** (shipped) — overchannel's Fort save and weave save-gating. ✔
- **S5 conditions** (shipped) — the overchannel cascade and weave-applied control. ✔
- **Proficiency / use-based gain** (shipped) — weaves as use-improved skills. ✔
- **Multiclass seam** (character-model D1, save v18, shipped) — channeling-as-a-class
  and the free seam to split Initiate/Wilder later. ✔
- **The Power pool** — the one genuine new substrate (Phase 0). It is also a standing
  BACKLOG item, so it pays down engine debt regardless of WoT.

## 8. Open questions (resolve as each phase starts)

- **Pool ceiling formula** — which governing stat and curve; how it grows with level
  (WoTMUD freezes SP at level 30; we likely don't need a freeze). Phase 0.
- **Regen rate & reserve multiple** — tuning knobs; start conservative (channeling
  should not be spammable). Phase 0.
- **Embrace/release state machine** — does v1 need it, or only when the rest/heal
  blockers and same-gender detection land? Lean: minimal in Phase 1, full in Phase 2+.
- **Stilling restore path** — overchannel can still you (Phase 2); the d20 "Restore
  the Power" weave is the cure. v1 may leave stilling as admin-reversible until a
  restore weave is authored.
- **Out-of-combat vs in-combat casting** — both ride the action queue; confirm the
  pulse-delay model handles a weave cast with no combat target cleanly. Phase 1.

---

*This is a design note, not a spec. With D1–D6 resolved, the first build deliverable is
**Phase 0 (the Power-pool substrate)** — small, non-WoT-specific, and it closes the
BACKLOG "Mana / Movement current pools" item. WoT channeling content (the channeler
class + starter weaves + the `channel` verb) is Phase 1. Each phase gets its own
go-review in the project rhythm; depth (affinities, the interrupt game, the taint,
angreal, linking, Traveling) follows in Phase 3+ as content demands.*
