# Hireable Mobs — Scoping & Pre-Decisions

> **Status:** design draft (no code). Started 2026-06-25. **Pre-spec** — this is the design pass + pre-decision settlement the BACKLOG requires before a `docs/specs/hireable-mobs.md` slice is written; the spec is the timeless contract, this is the sequence and the forks.
> **Companion docs:** `docs/specs/mounts.md` (the owned-creature precedent — read first), `docs/specs/follow.md` (the move-with-leader primitive this consumes + extends), `docs/specs/grouping.md` (the assist / kill-credit / loot seams), `docs/specs/mobs-ai-spawning.md` (the AI behavior registry), `docs/specs/economy-survival.md` (gold sinks), `docs/themes/shadowrun-pack-plan.md` §3.1 (spirits + drones want this same seam).
> **Decision posture:** *reuse, don't reinvent.* A hireling is **mounts-but-it-fights** — an owned, specialized `MobInstance`. The ownership, materialize/dematerialize, persisted-record, and logout-drain machinery already exist for mounts; the genuinely-new piece is **one AI behavior** (follow + guard its owner) plus a thin command surface.

---

## 1. The question, and the short answer

Can AnotherMUD support **hireable mobs** — NPCs a player hires to follow, fight for, and obey them (mercenaries, henchmen, hirelings)?

**Yes, and the substrate is ~70% built** — because mounts already built the hard part. A mount and a hireling are the *same shape*: an **owned, world-resident `MobInstance`** that travels with its owner, persists as a durable owned record, materializes into the world on login and dematerializes on logout, and is never silently deleted. They differ only in **what they do** (a mount carries; a hireling fights) and **how they're controlled** (you ride a mount; you order a hireling). The mounts spec itself anticipated this twice — §5.5 ("the owned-followable-entity relationship … a future player/NPC follow or hireable-companion system would use — built once, here") and §12 ("hireable companions can share it").

The one genuinely-new engine piece is an **AI behavior that makes the hireling trail and defend its owner** — and that behavior *is* the **mob-move signal** `follow.md` deferred ("following a mob waits on a mob-move signal … the onboarding-guide / hireable-mobs consumer brings it"). So this feature is the keystone that also closes three other deferred items (§7).

---

## 2. What is already built (the owned-creature foundation)

Built for mounts, reusable for hirelings with little or no change (cite by symbol; verify line numbers against current code):

| Capability | Where | Reuse for hirelings |
|---|---|---|
| **Owner field on a mob** | `entities` `MobInstance.OwnerID()` / `SetOwner()` / `IsOwnedBy()` (RWMutex-guarded) | **Direct** — the ownership gate is generic, not mount-specific |
| **Materialize a MobInstance under an owner** | `cmd/anothermud` `MountService.Materialize` → `spawnMob` + `SetOwner` | **Pattern** — a `HirelingService.Materialize` mirrors it (generalize to an owned-companion service) |
| **Dematerialize (remove from world)** | `MountService.Dematerialize` → placement remove + `store.Untrack` | **Pattern** — identical |
| **Durable owned-record list on the save** | `player` `Save.Mounts []MountRecord{TemplateID}` (save v26) | **Pattern** — `Save.Hirelings []HirelingRecord` (save bump) |
| **Transient live-instance tracking + logout drain** | `session` `connActor.liveMounts` + `drainLiveMounts` (called from `Manager.Remove`) | **Pattern** — `liveHirelings` + drain on logout (never strand an owned asset) |
| **AI behavior registry + tick** | `ai` `Registry.Register(name, fn)`, `Behavior func(ctx, mob, deps) error`, the `ai-tick` dispatcher reading `PropBehavior` | **The new seam** — register a `hireling` behavior (§4) |
| **Kill credit** | `eventbus` `MobKilled{KillerID,…}` (the attacker's combatant id) | A hireling's kill carries its mob id → the loot hook maps it to the owner |
| **Auto-pull into a fight** | `session.AutoAssistCandidates` + the combat sink `OnEngagement` (`cmd/anothermud`) + `combat …EngageWithReason` | **Pattern** — a hireling auto-engages what its owner fights |
| **Corpse owner-set / loot rights** | `corpse.Config.OwnerSet` hook → `session.LootOwners` | A hireling's kill routes loot rights to the **owner** |
| **Gold debit (hire cost)** | `economy` `Currency.Debit(…, reason)` (as `buymount` does) | **Direct** — debit at hire |
| **Recurring upkeep drain** | the `sustenance-drain` tick handler pattern (`loop.Register` + a manager sweep over playing actors) | **Pattern** — an `hireling-upkeep` tick |
| **Logout teardown hooks** | `connActor.Persist` drops party + follow; `Manager.Remove` drains mounts | **Pattern** — drop/dematerialize hirelings here |

**Key seam:** `MobInstance.ownerID` is **not** named for mounts — it's a generic owner. A hireling sets the same field. The only mount-specific gate is `IsMount()`; the parallel `IsHireling()` distinguishes the two roles over one entity type.

---

## 3. Subsystem map — every hireling concern → the engine

Tagged by effort (**Content** = no Go · **Small Go** = reuse a pattern · **Real Go** = a new seam):

| Hireling concern | Maps to | Effort |
|---|---|---|
| An NPC you own | `MobInstance` + `SetOwner` | **Content** (a mob template) + reuse |
| Hire it / pay for it | `Currency.Debit` at hire (vendor/recruiter NPC, shop path) | **Small Go** |
| It persists / re-appears on login | `Save.Hirelings []HirelingRecord` + materialize-on-login | **Small Go** (save bump, mirror mounts) |
| It follows me room to room | a **new AI `hireling` behavior** that steps toward the owner's room | **Real Go** (the one new piece — and the mob-move signal `follow.md` wants) |
| It fights what I fight | the auto-assist sink → `EngageWithReason` keyed on the owner's engagements | **Small Go** (reuse auto-assist) |
| Its kills credit me for loot | the `corpse.OwnerSet` hook maps a hireling `KillerID` → its owner | **Small Go** |
| Recurring upkeep (a gold sink) | a `hireling-upkeep` tick (sustenance-drain pattern); lapse → it leaves | **Small Go** |
| Dismiss it | `dismiss` verb → dematerialize + drop the record | **Small Go** |
| Order it (follow/stay/guard/attack) | an `order` verb setting a stance the AI behavior reads | **Small–Real Go** (depth slice) |
| It can die (real stakes) | it's a world creature already; death → contract ends | **Small Go** (policy) |
| Cap on simultaneous hirelings | an env knob (mirror `ANOTHERMUD_PARTY_CAP`) | **Content/Small** |

**The result:** every concern is content or small Go **except the follow/guard AI behavior**, which is the single Real-Go seam — and it's wanted by three other features regardless (§7).

---

## 4. The one new seam: the hireling AI behavior

Today AI behaviors are `stationary` and `wander` (`ai` registry, dispatched on the `ai-tick` from a mob's `PropBehavior` string). A hireling needs a third: **stay near my owner, and join my owner's fights.** Concretely the behavior reads `mob.OwnerID()`, finds the owner's room, and:

- **if not co-located with the owner** → take a step toward them (the same move path mounts/AI use). This step **emits the move signal mobs don't emit today** — which is exactly the missing piece `follow.md` flagged for mob-as-leader following. Build it here, generically (a mob move publishes the move event), and the onboarding-guide + player-follow-a-mob fall out.
- **if the owner is in combat and the hireling is idle** → engage the owner's opponent (reuse `EngageWithReason`, the auto-assist path).
- **else** → idle/guard (the `stay`/`guard` stance from the `order` verb, §5 depth).

This is one `Behavior` function + the generic "a mob move emits the move event" change. It is the whole Real-Go cost of the feature.

---

## 5. Pre-decisions (resolve before the spec)

The forks a greenfield system must settle. Each gets a **recommended default** so the spec isn't blocked.

1. **Entity model — specialized MobInstance vs. new type.** → **Specialized `MobInstance`** (an `IsHireling()` sibling of `IsMount()`, sharing `ownerID`). *Rationale:* mounts already chose this; a hireling reuses ~all of it. A distinct type would duplicate ownership/persistence/teardown.

2. **Lifetime model — permanent / timed-contract / dismissable.** → **Dismissable + recurring upkeep, persists like a mount** (re-materializes on login). A timed contract is a content policy on top, not the base. *Rationale:* the upkeep gold sink is the balance lever; permanence-until-dismissed-or-killed is the simplest durable shape and matches the mount precedent.

3. **Death — does it end the contract?** → **Yes: a hireling's death ends the contract** (your gold investment is lost). *Rationale:* gives the gold sink teeth and makes sending a hireling into danger a real decision; mirrors the killable mount (§7.3 of mounts). (A "downed, re-hire to revive" softening is a deferred policy.)

4. **XP — does fighting through a hireling grant the owner XP?** ✅ **DECIDED 2026-06-25 — option (c).** A hireling's **solo** kill (owner not engaged) grants the owner **no** XP; a kill the owner **also fought** grants normal XP. The rejected alternatives: (a) full XP for any hireling kill (exploitable — "pay gold → risk-free XP / AFK farm"); (b) flat-reduced XP for hireling-assisted kills (muddier, still rewards non-participation). *Rationale:* XP rewards *your* participation, not your wallet — a hireling is a force multiplier in fights you join, not an idle XP farm; the upkeep gold sink is the counterweight. **Implementation note (slice 3):** the kill-XP grant must check whether the owner was an active combatant against the slain mob (the owner is `InCombat` with it / on its threat list) before crediting; a hireling-only kill skips the grant. Loot (PD-5) is unaffected — the owner still owns the corpse.

5. **Loot — who owns a hireling's kill's corpse?** → **The owner** (the `corpse.OwnerSet` hook maps the hireling's `KillerID` → its owner's combatant id; a party owner-set extends to the owner's party under the existing loot policy). *Rationale:* loot is the tangible payoff and the owner paid for the hireling.

6. **Cap — how many at once?** → **1 in v1** (a single henchman), env-configurable (`ANOTHERMUD_HIRELING_CAP`, mirror the party cap). *Rationale:* one companion is the meaningful unit; multi-hireling "armies" raise balance + AI-noise concerns better deferred.

7. **Command surface.** → **`hire <target>`** (at a recruiter/vendor NPC, shop path), **`dismiss <hireling>`**, and a minimal **`order <hireling> <follow|stay|guard|attack>`**; trailing is automatic (the AI behavior). *Rationale:* `hire`/`dismiss` are the lifecycle; `order` is the control depth, sliceable.

8. **Persistence shape.** → **A `Save.Hirelings []HirelingRecord{TemplateID, …}` on the player save** (additive, versioned, mirrors `Save.Mounts`); the **live mob + its follow/combat state is not persisted** (re-materialized fresh on login, like the ride relationship). *Rationale:* the durable thing is the *contract*, not the live creature.

---

## 6. Recommended build sequence

Each slice ships something playable.

1. **Owned-companion substrate.** `HirelingRecord` persistence (save bump) + `HirelingService.Materialize/Dematerialize` (generalize the mount service or parallel it) + `hire`/`dismiss` verbs + the cap + logout drain. *Reuses:* `ownerID`, `spawnMob`, `drainLiveMounts` pattern, `Currency.Debit`. **Outcome:** you can hire a henchman that stands in the room and persists.
2. **The follow/guard AI behavior** (§4) — the hireling trails its owner. *This is the slice that emits the mob-move signal.* **Outcome:** your henchman follows you around; also unblocks `follow.md`'s mob-leader case.
3. **Combat assist + loot.** The hireling auto-engages your opponent (reuse the auto-assist sink) and its kills route loot rights to you (the `corpse.OwnerSet` hook). Resolve the **XP fork** (PD-4). **Outcome:** your henchman fights for you and you loot the kills.
4. **The economy.** Hire cost at recruit + a recurring upkeep drain (sustenance-drain pattern); lapsed upkeep → the hireling departs; death ends the contract. **Outcome:** hirelings are a real, losable gold sink.
5. *(Deferred depth)* the `order` stance verbs (guard/stay/attack-target), multiple hirelings, morale/loyalty, hireling equipment.

Slices 1–4 are a playable hireling on small Go + one new behavior.

---

## 7. Cross-consumer leverage (why this is the high-value pick)

The owned-controllable-entity seam this builds is **shared infrastructure**, not a one-off:

- **Closes `follow.md`'s mob-leader gap** — slice 2 emits the mob-move signal `follow.md` deferred, so "follow the guide" and player-follow-a-mob work for free.
- **Unblocks the onboarding-guide NPC** (BACKLOG §2) — a guide that walks a newbie around is a hireling-shaped owned follower with scripted dialogue.
- **Unblocks the Shadowrun archetypes** (`shadowrun-pack-plan.md` §3.1) — a **bound spirit** (mage) and a **slaved drone** (rigger) are *the same owned-controllable-entity* with different content + control surface. The SR pack plan's "design-together note" calls this the single highest-leverage move toward the magic and rigging archetypes.
- **Mirrors mounts** — a led/un-ridden mount (mounts §5.5) is an owned follower too; the behavior could subsume the mount's "lead" case.

One seam, four consumers. That is the argument for building hireable mobs before any of them.

---

## 8. What v1 defers

- **Multiple hirelings / mercenary bands** (cap 1 in v1).
- **`order` depth** beyond follow/stay/guard/attack (formations, patrol routes, conditional orders).
- **Hireling equipment / advancement** — a hireling is content-fixed (no XP/levels/gear management) in v1, like a mount.
- **Morale / loyalty / desertion** as a system (lapsed-upkeep-departs is the only "loyalty" lever in v1).
- **Hireling-vs-hireling and PvP interactions** beyond what combat already does.
- **The spirit/drone control surfaces** — those are SR-pack consumers built on this seam, not part of the generic v1.

---

## 9. Open questions

- ~~**The XP fork (PD-4)**~~ — **RESOLVED 2026-06-25: option (c)** (XP only for kills the owner actively fought; a hireling-only kill grants none). See §5 PD-4.
- **Does a hireling count as a party member** for grouping's XP/loot split, or is it strictly the *owner's* asset (outside the party math)? Lean: owner's asset, not a party seat — simpler, and avoids diluting human party shares. Revisit if hirelings should share a party's loot policy.
- **Recruiter model** — a dedicated hireling-vendor NPC (the `hire` access point) vs. hiring any suitably-tagged NPC in the world. Lean: a tagged recruiter (mirrors the stablemaster), content-defined.
- **Generalize the owned-companion service now, or extract later?** Building `HirelingService` as a parallel to `MountService` is fast; extracting a shared `CompanionService` (mounts + hirelings + future spirits/drones) is cleaner but bigger. Lean: parallel now, extract when the third consumer (SR) lands and the shape is proven.
