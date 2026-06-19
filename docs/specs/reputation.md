# Reputation / Renown — Feature Specification

> **Status: behavior contract, written ahead of code.** This spec
> defines the single-axis **renown** score (how famous or infamous a
> character is), distinct from the per-faction **standing** of
> [faction](faction.md). See §1.1 for the relationship — the two are
> separate systems that happen to share the word "reputation" in the
> source material.

## 1. Overview

Reputation is a **single signed score per character** measuring how
widely known they are — fame at the high end, infamy at the low, with
"unknown" at zero. It is an *absolute* renown axis: it answers "has this
person heard of me?", not "does this person's faction like me?".

Renown rises with deeds (dramatic heroism, or villainy — renown does not
care about morality), with class advancement, and from worn signifiers
(famous gear). It is consulted when the world reacts to a character: an
NPC's deference or fear, the difficulty of passing unrecognized, and the
gates a few feats place on it.

This feature is the subsystem the WoT **Fame**, **Infamy**, and **Low
Profile** feats (`feats.md`) require — they were deferred pending it.

### Goals

- A per-character signed renown score with content-defined **tiers**
  (Unknown → Known locally → Known throughout the land), each mirrored
  as a tag so other systems can gate on a band without reading the raw
  number.
- A single **gain/shift pipeline** (cancellable) shared by every source:
  deeds, level advancement, scripted events, and worn signifiers.
- A **recognition check** primitive (`renown roll vs. a difficulty`) for
  "am I recognized here?" / "can I pass unnoticed?".
- The three reputation feats expressed as ordinary modifiers/flags over
  this score, not bespoke code.

### Non-goals

- **Not per-faction standing.** Liking is [faction](faction.md); renown
  is fame. See §1.1.
- **Not morality.** Renown is amoral (a famous hero and an infamous
  Darkfriend can hold the same magnitude); morality is alignment
  (`progression` §6). See §1.2.
- **Not authorization.** Renown gates *gameplay* reactions, never engine
  capability (that is roles, `roles-and-permissions.md`).
- **No account-level fame.** Renown is a per-character property, like
  alignment and standing.

### 1.1 Relationship to faction (PD-1)

Reputation and [faction](faction.md) are **separate, non-interacting
systems** in v1 that the source material confusingly both calls
"reputation":

| | **Reputation (this spec)** | **Faction (`faction.md`)** |
|---|---|---|
| Axis | **one** absolute renown score | **N** relational standings, one per faction |
| Question | "how famous am I?" | "how does the Tower / the Band feel about me?" |
| Sign | fame (+) ↔ infamy (−), unknown at 0 | allied (+) ↔ hostile (−) per faction |
| Earned by | deeds + level + worn signifiers (§5) | faction-named quest rewards + faction-mob kills (`faction` §5) |

They share an **architecture** (both generalize alignment's signed
score + named bands + tag mirror + cancellable shift pipeline +
bounded history — `progression` §6) but **not data and not consumers**:
a renown gain never moves any faction standing and vice-versa; a renown
tier tag and a faction rank tag are different namespaces. A future
"famous *with* a faction" cross-effect is deferred content/event wiring
(§12), exactly as faction defers its alignment cross-effects.

The inert `item.Reputation` field (a signed renown delta on visible
gear — "inert until the reputation system reads it") feeds **this**
score, not faction standing (§5.4).

### 1.2 Relationship to alignment (PD-2)

Renown is **orthogonal to alignment**. A character's morality (good /
evil, `progression` §6) and their fame are independent: villainy raises
renown the same as heroism. The two do not interact in v1 — disposition
and other consumers read each independently.

### 1.3 What reputation is *not*

- **Not standing.** See §1.1 — separate axis, separate storage.
- **Not alignment.** See §1.2.
- **Not a level gate.** Renown rises *with* level (§5.2) but is not a
  level synonym; a low-level character who slays a Forsaken in public
  may out-renown a cautious veteran.
- **Not membership or title.** A title ("Lord", "Aes Sedai") may
  *imply* a starting renown (§3) but renown is the number, not the rank.

### 1.4 Pre-decisions

| ID | Decision | Status |
|---|---|---|
| PD-1 | Reputation is a **single absolute renown axis**, a separate sibling of faction; the two reuse one architecture but share no data and no consumers in v1. | Decided |
| PD-2 | Renown is **orthogonal to alignment** — amoral, no interaction in v1. | Decided |
| PD-3 | Renown is **signed** (fame positive, infamy negative, unknown 0) with content-defined **tier** bands mirrored as tags, mirroring alignment/faction. | Decided |
| PD-4 | Renown is **per-character** (an entity property), not account-shared, mirroring alignment. | Decided |
| PD-5 | The **kind** of reaction renown produces (admired vs. feared) is read from the *sign/Infamy flag*, while the **strength** is read from the magnitude — so one score drives both fame and infamy reactions (PD-1 of the Infamy feat, §7). | Decided |

---

## 2. The reputation score

Each character carries **one signed renown integer**, defaulting to a
configured **starting value** that a content-defined **class** or
**background** may raise (a Noble or an Initiate begins already known;
most begin Unknown at zero). The starting value is applied once at
character creation and is thereafter moved only by the §4 operation.

**Acceptance — score**

- [ ] A new character's renown is its class/background starting value
      (the configured default when none is declared).
- [ ] Renown is a signed integer; negative values are valid (infamy).
- [ ] Renown is per-character and survives a save/load round-trip (§10).

---

## 3. Tiers

Renown is bucketed into an ordered ladder of **named tiers** (Unknown,
Known locally, Known in the region, Known in the land, …) by configured
thresholds, defaulting to a shared ladder. The character's current tier
is **mirrored as a tag** (parallel to alignment's and faction's rank
tags) so room/mob/shop/quest gating reads the *band*, never the raw
number. Crossing a threshold retags (removing the old tier tag, adding
the new) through the §4 pipeline.

**Acceptance — tiers**

- [ ] Each renown value resolves to exactly one tier by the configured
      thresholds.
- [ ] The character carries exactly one current tier tag; a shift that
      crosses a threshold swaps it atomically with the rank-change event
      (§7).
- [ ] An infamy (negative) magnitude resolves to a tier symmetric with
      the fame side (a notorious villain is as "known" as a famous hero).

---

## 4. Operations

A renown change is a **shift**: a signed delta applied to the score
through a single cancellable pipeline (mirroring alignment §6.4 and
faction §4):

1. Compute the proposed new score (current + delta), clamped to the
   configured bounds.
2. Emit a cancellable **`reputation.shift.check`** event; a subscriber
   (or the **admin-immune** guard, mirroring alignment) may veto it.
3. On commit, store the new score, recompute the tier, swap the tier tag
   if it changed, append to bounded history, and emit **`reputation.shifted`**
   (and **`reputation.tier.changed`** when the band moved).

### 4.1 Acceptance — operations

- [ ] A shift clamps to the configured min/max renown.
- [ ] A vetoed `reputation.shift.check` leaves score, tier, tag, and
      history unchanged.
- [ ] A committed shift updates score, tier tag, and history together and
      emits `reputation.shifted` (+ `reputation.tier.changed` on a band
      cross) exactly once.
- [ ] History is bounded (oldest entries drop past the configured cap).

---

## 5. Gaining and losing reputation

All sources funnel through the §4 shift. The sources:

### 5.1 Dramatic deeds

A scripted or systemic "notable act with witnesses" applies a renown
gain. Greater feats (publicly slaying a famed enemy) grant more than
lesser ones; a lesser act may be **gated on a check** (a Charisma-style
roll) before it grants. Villainous acts grant renown identically (§1.2).

### 5.2 Class advancement

Gaining a class level grants a configured renown increment, so renown
trends upward with power. The increment is content-defined per class/track.

### 5.3 Scripted / event sources

Pack scripts (`scripting-and-packs`) and quest rewards (`quests.md`)
shift renown through the same §4 API — the primary authored earn path.

### 5.4 Worn signifiers

Visible gear may carry a **signed renown delta** (`item.Reputation`):
wearing a hero's famous blade or a Darkfriend's dread mask shifts the
wearer's *effective* renown while worn. Whether this is a transient
modifier (recomputed on equip/unequip, like a stat bonus) or a committed
shift is settled in §12 (Q1); v1 leans transient so removing the gear
removes the fame.

### 5.5 Acceptance — earn/lose

- [ ] A dramatic-deed source applies a renown gain through the §4 shift,
      optionally gated on a check for lesser acts.
- [ ] A class level-up applies the configured renown increment.
- [ ] A quest/script source shifts renown through the same API.
- [ ] Equipping signifier gear changes effective renown; unequipping
      reverses it (per the §12 Q1 resolution).

---

## 6. Recognition checks

The renown **check** answers "is this character recognized / can they
trade on their fame here?": a roll of `renown + a die` against a
content-defined **difficulty** (harder the farther from where the
character is known). A renown of zero **cannot** make the check (an
unknown person is not recognized). The check is the primitive the
Infamy/Low Profile feats and the "mask my identity" gameplay read.

**Acceptance — checks**

- [ ] A recognition check rolls renown + die vs. the location difficulty;
      a renown of zero auto-fails (cannot be recognized).
- [ ] The check is exposed as a reusable primitive (mirroring the saves /
      skill check idioms) so disposition, masking, and feats share it.

---

## 7. Feat interactions

The three WoT reputation feats (`feats.md`) are expressed over this
score, not as bespoke systems:

- **Fame** — a flat renown bonus (a fixed positive delta to effective
  renown), the renown sibling of the `ac_bonus` / `save_bonus` grant
  kinds.
- **Infamy** — a **flag** that makes the character's reactions resolve as
  *infamous* (feared/reviled) regardless of the score's sign (PD-5): the
  magnitude still drives strength, the flag drives kind.
- **Low Profile** — a **gain-rate reducer**: renown gains (§5) are scaled
  down (a famous-but-trying-to-stay-quiet character accrues fame slowly).
  Not retroactive.

**Acceptance — feats**

- [ ] Fame raises effective renown by its configured amount.
- [ ] Infamy makes recognition/reactions resolve as infamous independent
      of the raw sign.
- [ ] Low Profile scales subsequent renown gains by its configured factor;
      already-earned renown is untouched.

---

## 8. Content gating and consumers

Consumers read the renown **tier tag** (or run a §6 check), never the raw
number, and adopt the gating helper incrementally — the system ships with
its score, tags, events, and check, and existing systems wire in as the
content calls for it:

- **Disposition / NPC reaction** — a high-renown (or Infamy-flagged)
  character draws deference or fear; consulted independently of faction
  standing and alignment.
- **Identity masking** — passing unrecognized is a §6 check against
  renown (the famous are hard to hide).
- **Shops / quests / rooms** (optional, later) — may gate on a renown
  tier the same way they may gate on alignment or faction.

**Acceptance — gating**

- [ ] At least one consumer (disposition reaction) reads the renown tier
      / check at ship; others adopt the helper without a data change.

---

## 9. Observable events

| Event | Cancellable | When |
|---|---|---|
| `reputation.shift.check` | yes | before a renown delta is applied (§4 step 2) |
| `reputation.shifted` | no | after a committed renown change (§4 step 3) |
| `reputation.tier.changed` | no | when a committed shift crossed a tier threshold |

### 9.1 Acceptance — observability

- [ ] The three events fire at the §4 points; the check is cancellable
      and the two outcomes are not.

---

## 10. Persistence

Renown is a per-character property serialized on the player save: the
signed **score** (and, if §12 Q2 chooses to persist it, the Infamy flag
and Low-Profile state — both otherwise re-derived from held feats). The
tier tag is **derived** (recomputed on load from the score, not stored).
Adding the field is a **save-version bump** with an append-only migration
that defaults pre-feature saves to the configured starting renown for
their class (or zero).

**Acceptance — persistence**

- [ ] Renown round-trips through save/load; the tier tag is recomputed on
      load, not persisted.
- [ ] A pre-feature save migrates to the default starting renown without
      error (no-op-style backfill).

---

## 11. Configuration surface

| Setting | Meaning |
|---|---|
| Default starting renown | The renown a character begins with absent a class/background override. |
| Per-class/background starting renown | Content overrides (e.g. a Noble or Initiate begins known). |
| Tier ladder | Ordered named tiers + their thresholds (the shared default ladder). |
| Renown bounds | Min/max the score clamps to. |
| Class level-up renown increment | Renown granted per class level (per class/track). |
| Dramatic-deed gains | Renown granted by greater/lesser notable acts; the lesser-act check difficulty. |
| Recognition-check difficulties | Per-location difficulty the §6 check rolls against. |
| Fame bonus | The flat renown the Fame feat confers. |
| Low Profile factor | The scale applied to renown gains while Low Profile is held. |
| History cap | Bounded renown-history length. |

All numeric values live here per spec convention; the prose names
behaviors, not magnitudes.

---

## 12. Open questions / future work

- **Q1 — gear renown: transient vs. committed.** Does worn signifier gear
  (§5.4) apply a transient effective-renown modifier (recomputed on
  equip, removed on unequip) or a one-time committed shift? v1 leans
  transient (fame leaves with the gear); revisit if content wants
  permanent renown from a deed-item.
- **Q2 — persist the Infamy flag / Low-Profile state, or re-derive?** Both
  are feat-driven; re-deriving from held feats avoids a second source of
  truth (mirrors how the Power Attack stance keys on the feat cache).
  Persist only if a non-feat source of either is added.
- **Renown decay.** Fame drifting toward Unknown over inactive time
  (parallel to faction's standing-decay open question). Deferred.
- **Famous-with-a-faction cross-effect.** A renown gain that also nudges a
  faction standing (a deed the Whitecloaks love). Pure content/event
  wiring, no data change — deferred, mirroring faction §10.
- **Reputation as a die-roll modifier elsewhere.** The source uses the
  renown score in social-skill contexts; wiring it into a skill-check
  modifier is deferred to the skills sub-epic.

---

## Cross-references

- [faction](faction.md) — the **standing** sibling; §1.1 here is the
  reconciliation (separate axes, shared architecture, no v1 interaction).
- [progression](progression.md) — §6 alignment is the architectural
  template (signed score + named bands + tag mirror + cancellable shift
  + bounded history) this generalizes; §5 class level-up is the §5.2
  renown-increment hook.
- [feats](feats.md) — Fame / Infamy / Low Profile (§7) are the feats this
  unblocks; the grant-bridge hosts the Fame flat bonus.
- [inventory-equipment-items](inventory-equipment-items.md) — the
  `item.Reputation` signed delta on visible gear feeds renown (§5.4).
- [persistence](persistence.md) — §10 the renown save field + versioning.
- [scripting-and-packs](scripting-and-packs.md) — §5.3 scripts shift
  renown through the same API.
- [quests](quests.md) — §5.3 quest rewards as the primary authored earn
  path.
- `docs/specs/README.md` — reading-order placement (layer 2, beside
  progression/faction), the cancellable-events table
  (`reputation.shift.check`), and the player-save surface (§10).
- `feats.md` implementation notes — the "coordinate with the Reputation
  system (Chapter 6)" note that pointed here.
