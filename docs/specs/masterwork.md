# Masterwork (item quality grades)

EPIC sub-epic **S1** — increment **H** of the WoT Combat & Equipment Depth
program (`docs/themes/wot-mechanics-epic.md`,
`docs/proposals/combat-equipment-depth.md`). Governed by EPIC **Decision 0**
(translate onto the existing tick model; no d20 rewrite). *Spec ahead of code —
build pending.* Layers on `weapon-identity` (the to-hit seam), `armor-depth §6`
(the armor check penalty), `skills.md` (tool checks), the `damage_bonus` channel
(`combat §4.5`), and renders alongside `item-decorations` (rarity/essence) while
staying mechanically independent of it.

## 1. Overview

A **masterwork** item is a higher-quality version of an ordinary item that
confers a small, grade-scaled mechanical bonus. The source ladders three grades —
*masterwork*, *masterpiece*, and *power-wrought* — each better than the last, and
each conferring a bonus appropriate to the **kind** of item it is (a finer blade
hits truer; finer armor hinders less; a finer tool aids a craft). Power-wrought
weapons additionally never break or need maintenance.

The design keeps two things **independent**, exactly as `item-decorations §1.1`
prescribes ("the decoration and the power are independent"):

- **The grade is mechanical.** Its bonus rides the **existing modifier and
  channel seams** — a masterwork weapon contributes to the same to-hit
  adjustment weapon proficiency uses, a power-wrought weapon to the same
  `damage_bonus` channel the strength bonus uses, masterwork armor to the
  armor check penalty (`armor-depth §6`), a masterwork tool to a skill check
  (`skills.md`). A grade never invents a new combat or resolution path.
- **The decoration is presentation.** A masterwork item MAY *also* carry a
  rarity tier or essence so it reads as special in displays (`item-decorations`),
  but that marker is a separate property — the engine does not derive the bonus
  from a decoration key, nor force a graded item to be decorated.

**Goals.** Give crafted and authored gear the WoT quality ladder
(masterwork / masterpiece / power-wrought) as grade-scaled bonuses through
already-shipped seams; keep the mechanical grade and the cosmetic decoration
independent; add the power-wrought "unbreakable" flag as a recorded property.

**Non-goals (this slice).** Masterwork **ammunition** (arrows/bolts/bullets that
stack their bonus with a masterwork launcher and are destroyed on use) — that is
the ranged slice (G). A durability / wear / breakage system — the engine has
none, so power-wrought "unbreakable" is a recorded flag with no live consumer
yet (§4). The Reputation bonus a visibly-carried masterwork item grants in the
source — that rides the reputation sub-epic (S8), not here.

## 2. The grade

An item MAY declare a **quality grade** drawn from an ordered, content-defined
grade vocabulary (the WoT pack uses *masterwork < masterpiece < power-wrought*,
with power-wrought itself carrying a numeric step — +1 / +2 / +3). An item with
no grade is an ordinary item and behaves exactly as today.

- An item carries **at most one** grade.
- The grade ladder is **ordered** (for "finer than" comparisons and for scaling
  the bonus); the vocabulary, its order, and each grade's bonus magnitudes are
  pack-defined policy (§8).
- The grade is item metadata — declared on an authored item's template, or set on
  an instance by the system that produces it (e.g. a high-quality craft, §6).

**Acceptance criteria**

- [ ] An item may declare one quality grade from an ordered, pack-defined
      vocabulary; an item with none behaves exactly as before.
- [ ] Grades are ordered, so "finer than" comparisons and bonus scaling are
      well-defined.

## 3. Bonuses by item kind

A grade's bonus depends on the kind of item it is. Each bonus is delivered
through the seam that resolution already reads, while the item is
wielded / worn / used, under a grade-scoped source key so it composes and
reverses cleanly (like equipment modifiers):

- **Weapon → to-hit.** A graded weapon adds a grade-scaled **to-hit bonus** while
  wielded, contributing to the same per-attacker hit-modifier adjustment that
  weapon proficiency and the darkness penalty feed (`weapon-identity §3`,
  `combat §4.4`). It does not change damage (except power-wrought, below).
- **Power-wrought weapon → to-hit and damage.** A power-wrought weapon adds its
  grade step to **both** the to-hit adjustment **and** the `damage_bonus` channel
  (`combat §4.5`) while wielded. As with the strength bonus, the damage
  contribution is added after any critical dice multiply and is not itself
  multiplied.
- **Armor / shield → check penalty.** A graded armor or shield **improves**
  (reduces the magnitude of) its armor check penalty by the grade amount while
  worn (`armor-depth §6`), to a floor of zero. It does not change the armor
  bonus or the max-Dex cap.
- **Tool → skill check.** A graded tool adds a grade-scaled bonus to the skill
  check it assists while used (`skills.md`). **Multiple graded tools toward the
  same check do not stack** — only the best applies.

A grade bonus is one more **additive contributor** to its target value; it stacks
with proficiency, feats, and other distinct sources (it is a separate source
key), subject to the per-kind non-stacking rule above (tools).

**Acceptance criteria**

- [ ] A graded weapon adds a to-hit bonus while wielded, composing with the
      other to-hit adjustments; it changes damage only when power-wrought.
- [ ] A power-wrought weapon adds its step to both to-hit and `damage_bonus`; the
      damage part is not multiplied by a critical.
- [ ] A graded armor/shield reduces its check penalty by the grade amount, floored
      at zero, without changing armor bonus or max-Dex.
- [ ] A graded tool adds a skill-check bonus while used; multiple graded tools
      toward one check do not stack (best applies).
- [ ] Grade bonuses reverse cleanly when the item is unequipped (grade-scoped
      source key).

## 4. Unbreakable / maintenance flag

A power-wrought item additionally declares an **unbreakable / no-maintenance**
flag (the source's "never breaks, never needs sharpening"). The engine has **no
durability, wear, or maintenance system today**, so this flag has **no live
consumer** — it is recorded on the item as a forward hook. If a durability system
is ever added, an unbreakable item is exempt from its wear and breakage; until
then the flag is inert flavor.

**Acceptance criteria**

- [ ] A power-wrought item records an unbreakable flag; with no durability system
      the flag changes no behavior (it is a recorded forward hook).

## 5. Independence from decoration

The quality grade (§2) and a `item-decorations` rarity/essence marker are
**separate properties** on an item. Content MAY pair them — a power-wrought blade
might also carry a `legendary` rarity tag so it reads as special — but:

- the grade's bonus is **never** derived from a rarity/essence key (the bonus
  comes from the grade, the decoration from the decoration key); and
- a graded item need **not** be decorated, and a decorated item need **not** be
  graded.

A grade MAY register a theme color / marker so a graded item *can* be surfaced in
displays (reusing the decoration rendering surfaces, `item-decorations §4`), but
that rendering is cosmetic and optional, consistent with `item-decorations §1.1`.

**Acceptance criteria**

- [ ] The grade and any rarity/essence marker are independent properties; the
      bonus is read from the grade, never from a decoration key.
- [ ] A graded item with no decoration still confers its bonus; a decorated item
      with no grade confers none.

## 6. Persistence

A grade is item metadata. For an **authored** item it is declared on the template
and needs no per-instance persistence. For an item **produced at runtime** (a
craft result, §7), the grade is set on the **instance** and persists with the
item's instance properties (the same surface other instance properties use).

The unbreakable flag (§4) follows the grade — it is implied by the power-wrought
grade, not separately stored.

**Acceptance criteria**

- [ ] An authored item's grade rides its template; a runtime-produced item's grade
      persists as an instance property and round-trips.

## 7. Interaction with existing systems

- **Weapon identity** (`weapon-identity §3`, `combat §4.4`): the weapon to-hit
  bonus is one more contributor to the per-attacker hit-modifier seam, alongside
  proficiency and darkness.
- **Combat damage** (`combat §4.5`): the power-wrought damage step rides the
  `damage_bonus` channel, added post-crit-multiply.
- **Armor depth** (`armor-depth §6`): the armor/shield grade improves the check
  penalty.
- **Skills** (`skills.md`): the tool grade adds a skill-check bonus (non-stacking
  across same-purpose tools).
- **Crafting** (`crafting-and-cooking`): a craft's quality roll already yields a
  rarity tier (decoration); a high-quality craft MAY *also* set a quality grade on
  the result so a finely-crafted weapon is mechanically masterwork. Whether
  crafting sets the grade — and how the quality roll maps to a grade — is content
  policy (§9), and stays independent of the rarity decoration it already sets.
- **Feats** (`feats.md`): a weapon-focus feat's to-hit bonus and a masterwork
  weapon's to-hit bonus are distinct additive sources and stack.
- **Item decorations** (`item-decorations`): rendering only; mechanically
  independent (§5).

## 8. Configuration surface

| Setting | Meaning | Default |
|---|---|---|
| Grade vocabulary | The ordered set of quality grades (§2). | the WoT pack grades (masterwork / masterpiece / power-wrought ±N) |
| Per-grade bonus magnitudes | The to-hit / damage / check-penalty / skill amounts each grade confers, by item kind (§3). | the WoT values (e.g. masterwork weapon +1 to-hit) |
| Tool non-stacking rule | That multiple graded tools toward one check do not stack (§3). | best-applies (per source) |
| Check-penalty improvement floor | The floor a graded armor's improved check penalty stops at (§3). | zero |

All numeric magnitudes live here; the prose names behaviors, not values.

## 9. Decisions and open questions

**Decided for this slice:**

- **The grade is mechanical, the decoration is cosmetic, and they are
  independent** (§5), per `item-decorations §1.1`. Masterwork "rides rarity"
  only in the sense of optionally reusing its *rendering* surfaces — not its
  meaning.
- **Bonuses ride existing seams** (§3) — the grade never adds a new resolution
  path; it is grade-scoped modifiers on the to-hit, `damage_bonus`, check-penalty,
  and skill-check values already in play.
- **Unbreakable is a recorded forward hook** (§4) — inert until a durability
  system exists, rather than gating the slice on building one.

**Still open (non-blocking):**

- **Masterwork ammunition** — arrows/bolts/bullets whose bonus stacks with a
  masterwork launcher and that are consumed on use — waits on ranged (G).
- **Crafting → grade mapping** — whether and how a craft's quality roll sets a
  quality grade (not just the rarity decoration it already sets) is content
  policy; the seam is here, the mapping is a crafting-content call.
- **Reputation from visible masterwork gear** — the source grants Reputation for
  visibly carrying fine gear; that rides the reputation sub-epic (S8).
- **Durability** — if a wear/breakage system ever lands, the unbreakable flag
  becomes its first exemption (§4).

---

<!-- Spec style: narrative + acceptance criteria · Detail level: behavior only · Status: forward spec (build pending) — EPIC S1 increment H · grade (mechanical) kept independent of decoration (cosmetic) per item-decorations §1.1 -->
