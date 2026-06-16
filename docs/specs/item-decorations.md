# Item Decorations: Rarity & Essence — Feature Specification

**Status:** Draft · **Scope:** Two parallel item-marker systems — *rarity
tiers* (an ordered ladder of named tiers that decorate an item's display)
and *essence* (a colored glyph an item carries) — their content-defined
registries, their attachment to items via reserved properties, their
themed colored rendering (inline and column-aligned), and essence's role
in stack identity · **Audience:** Anyone reimplementing or porting this
feature in any language.

This document describes *what* these decorations must do, not *how* to
implement them. The tier/essence vocabularies are content; the reserved
property keys, defaults, and render slots live in the
configuration-surface table at §8.

Rarity and essence are the item-facing half of the *Gameplay Depth*
theme. Reference behavior is ported from the prior incarnation, which
ships them as **two separate registries** — a glyph-bearing essence and
an ordered, decorator-bearing rarity tier. This spec keeps them separate
(they have genuinely different shapes) while specifying the mechanism
they share: a content-registered marker that resolves a theme color and
renders into item display. The "one combined tag system" alternative is
recorded as rejected in §9.

---

## 1. Overview

Both systems answer "how does this item look, beyond its name?" — but at
different jobs:

- **Rarity** classifies an item into one of an *ordered* set of tiers
  (common → legendary, by an explicit order value). A tier carries a
  display label, bracketing decorators, a color, and a visibility flag.
  Rarity renders as a decorated, colored tag — inline next to the name,
  or padded to a fixed width for column-aligned lists.
- **Essence** tags an item with a single colored *glyph* (a symbol)
  drawn from a content registry. Essence renders as that glyph in parens,
  colored. Essence also participates in **stack identity**: two otherwise
  identical items with different essences do not stack.

What they share: each is a content-registered definition keyed by a short
string; each registers a **theme color entry** so its color flows from
the theme (`ui-rendering-help.md` §3) and degrades in plain mode; each
attaches to an item through a reserved property holding the key; each
renders into the same item-display surfaces (look, inventory, shop).

### 1.1 What these are *not*

- Not gameplay stats. A rarity tier or essence is *presentation* — it
  colors and decorates. Any mechanical effect (a "legendary" item being
  stronger) comes from the item's own modifiers, not from its rarity key.
  The decoration and the power are independent. (The **masterwork** quality
  grade, `masterwork.md`, is the mechanical counterpart — a separate property
  that confers a bonus and MAY reuse these rendering surfaces, but is never
  the source of the bonus itself.)
- Not a single system. Rarity is an *ordered ladder with decorators*;
  essence is a *flat glyph set*. They are not two views of one registry
  (§9).
- Not engine-defined vocabularies. The engine ships the mechanism; a pack
  defines which tiers and essences exist, their order, glyphs, and colors.
  An empty registry renders nothing — items just show their names.
- Not required on an item. Both are optional properties. An item with
  neither renders exactly as it does today.

---

## 2. Rarity tiers

A **rarity registry** holds an ordered set of tier definitions.

- A tier definition carries: a **key** (short string, e.g. `rare`), an
  **order** (integer rank, low → high), an optional **display text**
  (e.g. `RARE`), optional **decorators** (a left/right pair, e.g. `[` /
  `]`), a **color**, and a **visible** flag.
- The **order** establishes the ladder: tiers sort by it (for ranking,
  filtering, "is this rarer than that"). Keys are unique and
  case-insensitive.
- A tier with `visible = false`, or with no display text, or with no
  decorators, renders as **nothing** — this is how a baseline tier
  (`common`) carries an order and color for logic without cluttering
  every common item's display.
- Registration is content-driven and idempotent per key (re-registering a
  key replaces it; later registration wins, matching the pack
  later-wins convention).
- Registering a visible tier also registers a theme entry for its color
  (§4), so the tag is themed, not raw-colored.

**Acceptance — rarity tiers**

- [ ] Tiers register with key/order/display/decorators/color/visible and
      sort by order; keys are unique and case-insensitive.
- [ ] A tier that is invisible, or lacks display text or decorators,
      renders as empty.
- [ ] Re-registering a key replaces the prior definition (later wins).
- [ ] Registering a visible tier makes its color resolvable through the
      theme.

## 3. Essence

An **essence registry** holds a flat set of essence definitions.

- An essence definition carries a **key**, a **glyph** (a short symbol,
  typically one display column), and a **color**. Keys are unique and
  case-insensitive.
- Registration is content-driven and idempotent per key (later wins) and,
  like rarity, registers a theme entry for the color (§4).
- There is no order and no decorators — essence is a flat, glyph-only
  marker. Multiple essences are not combined on one item in v1 (an item
  has at most one essence key — §9 notes multi-essence as deferred).

**Acceptance — essence**

- [ ] Essences register with key/glyph/color; keys are unique and
      case-insensitive.
- [ ] Registering an essence makes its color resolvable through the theme.
- [ ] An item carries at most one essence key.

---

## 4. Rendering and theme integration

Both decorations render through the color pipeline, never as raw ANSI.

- **Rarity inline** renders the (decorator-wrapped) display text inside a
  themed color tag — e.g. the visible portion is `[RARE]` colored by the
  tier's theme entry. An unset or invisible tier renders empty.
- **Rarity padded** renders the same text **centered and padded to the
  registry's maximum visible tag width**, so a column of items in a list
  aligns regardless of which tiers appear. An unset/invisible tier renders
  as blank padding of that same width (the column stays aligned).
- **Essence** renders the glyph wrapped in parens inside its themed color
  tag — e.g. a colored `(✦)`.
- Both resolve color through the theme registry (`ui-rendering-help.md`
  §3): the marker carries a semantic tag (`item.<rarity-key>`,
  `essence.<key>`) whose color the theme owns, so a re-themed pack
  recolors decorations without touching item data. In plain (no-color)
  mode the tags strip to their visible text/glyph.

**Acceptance — rendering**

- [ ] Rarity inline shows the decorated display text in the tier's themed
      color; unset/invisible renders empty.
- [ ] Rarity padded centers and pads to the max visible tag width so a
      list column aligns; unset renders as blank padding of that width.
- [ ] Essence renders its glyph in parens in its themed color.
- [ ] In plain mode both strip to visible text/glyph with no color codes.

---

## 5. Attachment and stacking

- An item carries rarity and/or essence as **reserved item properties**
  holding the key (§8). The property may be set on the **template** (all
  instances share it) or on an **instance** (a single drop differs — e.g.
  a rare roll of a common template). Instance properties persist per the
  instance-property rules (`inventory-equipment-items.md`).
- **Essence is part of stack identity.** The stacking service
  (`inventory-equipment-items.md` §5) groups by template *and* essence:
  two items of the same template with different essence keys form
  separate stacks; same essence (including both unset) stack together.
- **Rarity's stacking interaction is left to the stacking spec.** Tapestry
  keys stacks on essence only, not rarity; whether rarity should also
  split stacks is an open question (§9). The conservative default is that
  rarity does not split stacks (it is presentation; identical templates
  with the same essence stack regardless of a per-instance rarity marker).

**Acceptance — attachment & stacking**

- [ ] Rarity/essence may be set on a template (all instances) or an
      instance (one drop), and an instance value persists.
- [ ] Two same-template items with different essence keys do not stack;
      with the same essence key (or both unset) they do.

---

## 6. Persistence

- Rarity and essence are item properties. A template-level value needs no
  per-instance persistence (it is re-derived from the template). An
  instance-level value persists with the item instance's property bag and
  round-trips across logout.
- The registries themselves are content, loaded at boot from packs — not
  saved state. A removed pack tier simply stops resolving; an item
  referencing an unknown key renders as unset (empty), never an error.

**Acceptance — persistence**

- [ ] An instance-level rarity/essence survives logout.
- [ ] An item referencing an unregistered key renders as unset, not an
      error.

---

## 7. Configuration surface

| Setting | Description |
|---|---|
| Rarity property key | The reserved item property holding the rarity tier key (§5). |
| Essence property key | The reserved item property holding the essence key (§5). |
| Rarity tier vocabulary | Content-defined: the set of tiers, their order, display, decorators, color, visibility (§2). |
| Essence vocabulary | Content-defined: the set of essences, their glyphs, colors (§3). |
| Render slots | Where decorations appear (item name in look, inventory line, shop list) and inline-vs-padded per slot (§4). |
| Theme entry naming | The semantic-tag naming for decoration colors (`item.<key>`, `essence.<key>`) (§4). |

No tier or essence name is hardcoded by behavior — the engine renders
whatever the registries hold and resolves colors through the theme.

---

## 8. Open questions

- **Combined system (rejected).** Folding rarity and essence into one
  "item marker" registry was considered and rejected: rarity is an
  ordered ladder with decorators and padding semantics; essence is a flat
  glyph that affects stacking. One abstraction would carry both shapes
  awkwardly. They stay separate.
- **Rarity in stack identity.** Whether a per-instance rarity marker
  should split stacks (like essence does) or is purely cosmetic and
  stack-irrelevant. Default: cosmetic, does not split.
- **Multi-essence items.** Whether an item may carry more than one essence
  (a list of glyphs). v1 is single-essence; multi is deferred until
  content needs it.
- **Rarity-driven generation.** Whether the engine ever *assigns* rarity
  (a drop-table rolling a tier) or rarity is always author-set. This spec
  is render-and-attach only; generation is a separate concern (loot
  tables) if it ever lands.
- **Background color / HTML.** The decoration color is foreground today.
  Whether a tier may carry a background or richer HTML styling for
  web/GMCP clients (the theme entry supports an HTML variant) is deferred.

---

## Cross-references

- `ui-rendering-help.md` §3 (theme registry — decoration colors),
  §2 (color tags / plain-mode stripping).
- `inventory-equipment-items.md` §2 (item properties — where the keys
  live), §5 (stacking — essence as stack identity).
- `persistence.md` — instance-property persistence for per-drop markers.
