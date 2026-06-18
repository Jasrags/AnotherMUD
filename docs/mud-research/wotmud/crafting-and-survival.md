# WoTMUD — Crafting & Survival

Source:
- https://wotmud.info/crafting/
- https://wotmud.info/herb-glossary/
- https://wotmud.info/potions/
- https://wotmud.info/smobbing/

Fetched: 2026-06-18

**Sourcing caveat:** the *Crafting*, *Potions*, and *Smobbing* pages on wotmud.info
are JavaScript-gated — only the intro blurb is server-rendered; the mechanical body
("Enable JavaScript and cookies to continue") never reaches a non-JS fetch. The
**Herb Glossary** renders fully (it is a 33-entry directory, complete A–L, no later
letters exist). What's captured below is verbatim where available and flagged
**[intro only]** where the body is JS-gated.

---

## 1. Crafting [intro only]

> "WoTmud features multiple crafting guilds, including smelting, woodworking, and
> leatherworking. You can craft weapons and armor, as well as various other everyday
> items to use, sell, or trade. Using turn-point tokens (acquired through trade or PK)
> and other supplies, you can craft some of the rarest weapons found in game."

What this tells us about the WoTMUD crafting model:

- **Multiple crafting guilds**, organized by material discipline: **smelting**,
  **woodworking**, **leatherworking** (a guild-membership gate on which crafts you can do).
- Output spans **weapons, armor, and everyday items** — not a pure consumable system.
- The dual purpose is **use / sell / trade** (crafted goods feed the player economy).
- **Turn-point tokens** are a premium crafting currency, **earned through trade or PK**
  (not gathered) — they gate "the rarest weapons found in game." Crafting's top tier is
  thus tied to the *PvP/economy loop*, not to gathering volume.

**For AnotherMUD:** the turn-point-token gate is the interesting borrow — a top crafting
tier whose input is a PK/trade-earned scarce token rather than a gatherable resource. Our
crafting (`internal/crafting`/`recipe`) currently keys on gathered/loot inputs; a
prestige token tier would mirror this.

---

## 2. Herb Glossary

A complete in-game/roleplay herb directory (33 entries). Markers in the source:

- `*` = **Potion Ingredient**
- `**` = **Wisdom Only** (usable only by the Wisdom/healer class)
- `***` = **Quest Item**

The trailing italic phrase is the **in-game item keyword string** (e.g. "a handful of acem").

| Herb | Markers | Effect | In-game item |
|------|---------|--------|--------------|
| Acem | * | Reduce swelling (external) and bleeding (internal) | a handful of acem |
| Andilay | * | Root for fatigue; clears head, eases tired muscles | a bundle of andilay root |
| Asping Rot | — | Potent poison; a drop can kill; peaceful death within an hour of ingestion; strong narcotic/sedative properties | — |
| Blackwasp Nettles | * | Sting when fresh; blackwasp emphasises the sting | a bunch of blackwasp nettles |
| Blisterleaf | — | Grows in a patch; causes rapid skin reaction/rash; **slows the poison from Thakan'dar blades** | — |
| Blue Goatflowers | *** | Hot poultice from boiled petals greatly improves healing of a **broken bone** | a small pile of blue goatflower petals |
| Bluespine | * | No medicinal value; bitter; used as punishment by the Aiel | a few pieces of bluespine |
| Bluewort | — | Tea settles a queasy stomach | — |
| Boneknit | * | Taken internally for **bone fractures** | a bunch of boneknit |
| Broomweed | ** | Signal for the Yellow Ajah; yields strong yellow dye; bundles into a broom | a few stalks of yellow broomweed |
| Catfern | — | Boiled tastes terrible; used as punishment | — |
| Chainleaf | * | Settles a queasy stomach | a bunch of chainleaf leaves |
| Corenroot | * | Assists **regeneration of blood cells** | a handful of corenroot |
| Crimsonthorn | * | Root as painkiller in small doses; **paralytic** in higher doses, death by asphyxiation | a cluster of crimsonthorn root |
| Dogwood | * | Tastes terrible; punishment & coercion; lifts depression | a bouquet of dogwood leaves |
| Dogwort | *** | Assists healing of **wounds** | some fuzzy green dogwort leaves |
| Feverbane | * | Aid for breaking a **fever**; with Acem cures the white shakes | a handful of feverbane |
| Five-finger | * | Heals **bruises**; ointment with sunburst root + ground ivy | a bundle of five-finger |
| Flatwort | *** | Treats **fatigue**; clears head, eases muscles | a blue pouch of flatwort tea |
| Forkroot | *** | **Paralytic against channelers** — distilled tincture cuts them from the Source; stronger on men; used by Seanchan to ferret out damane; cool minty taste | a packet of dried brown forkroot |
| Foxtail | * | Sleep aid without grogginess | a handful of foxtail |
| Gheandin | *** | Treats heart complaints; powdered induces sneezing to relieve headaches/earaches; some poison-relief effect | a vial of powdered red gheandin blossom |
| Goatstongue | * | Makes a patient sleepy; relieves stomach cramps | a handful of goatstongue |
| Goosemint | — | Eases digestion and upset stomachs | — |
| Greenwort | * | With Goatstongue, a sleep tea; relieves cramps | a few leaves of greenwort |
| Grey Fennel | *** | **Rapid-acting weapon poison**, favored by assassins; beneficial in tiny quantities | a gray fennel plant |
| Ground Ivy | *** | Relieves **pain**, heals **bruises**; some fatigue relief | a sack of green ivy |
| Healall | ** | Ointment for **open wounds** | a white healall root |
| Heartleaf | * | Tea as a **contraceptive** | a handful of heartleaf leaves |
| Honey | ** | Energy; soothes sore throats | a tincture of a thick yellow liquid |
| Itchoak | — | Irritates skin, causes blistering | — |
| Itchweed | * | Poisonous; irritates skin | a cluster of itchweed |
| Lionheart | * | Relieves **pain** | a bundle of lionheart |

**Patterns worth noting:**

- Herbs cluster into **healing** (wounds/bruises/fractures/fever/fatigue), **poison**
  (asping rot, grey fennel, crimsonthorn-overdose), **anti-channeler** (forkroot),
  **utility/flavor** (dye, contraceptive, punishment-bitter), and **counter** (blisterleaf
  slows Thakan'dar-blade poison — a direct combat counter to Myrddraal/Trolloc blades).
- **Wisdom-only** herbs (Broomweed, Healall, Honey) reserve the strongest open-wound
  healing to a class — a class-gated consumable tier.
- The **item-keyword string** doubles as the loot/gather noun, so herb pickup and herb use
  share one resolver.

**For AnotherMUD:** this is a clean **consumable-with-effect** table that maps to our
`effect` + `economy/consumables` systems. The marker scheme (potion-ingredient /
class-only / quest-item) is a tidy way to tag a gatherable's *role* without inventing new
machinery. Forkroot's "cut a channeler from the Source" is a notable anti-One-Power
mechanic for the WoT pack.

---

## 3. Potions [intro only]

The *Potions* page (titled "Potions, Herbs, Plants and Cures") is fully JS-gated — the
crawl returns only nav chrome plus the forum "Art of War" sidebar. No mechanical body was
recoverable. Inferred structure (from the title and the herb markers above): potions are
**brewed from the `*`-marked herb ingredients**; the herb glossary is effectively the
ingredient list for this page's recipes.

---

## 4. Smobbing (super-mob / boss hunting) [intro only]

> "'Smobbing' is the act of killing supermobs, or really strong NPCs. This is where most
> of the best gear comes from! Try out solo tactics or join with a group of friends for a
> social activity! This guide will help you understand how to be a productive member of a
> smob group."

What this establishes:

- **Smob = "super-mob"** — a named, very strong NPC boss.
- Smobs are the **primary best-gear source** (the PvE loot fountain), parallel to
  crafting's turn-point-token tier and PK loot.
- Designed for both **solo tactics** and **group play** (a social activity).
- Smobs are well-known fixtures with fixed locations and routes — the PK glossary (see
  `pvp-and-pk.md`) names many by site ("rollie", "granlin", "jimmy the lizard king",
  "the Master Torturer smob"), confirming smobs double as **PK landmarks and traps**.

**For AnotherMUD:** smobs are the WoT/Diku answer to "where does endgame gear come from" —
named bosses on the world map, soloable-or-grouped, that also anchor PvP geography. This
fuses our `mob`/`spawn`/`loot` systems with named-landmark world design.

---

## Cross-references

- The herb/potion effects feed our `effect` + `economy/consumables` model — see
  `pvp-and-pk.md` for how condition states (Wounded/Critical/Bleeding) appear in combat.
- Smobs as gear source + PK landmark connect to `progression-and-exp.md` (turn points,
  ranks) and the PK location glossary.
