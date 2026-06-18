# WoTMUD — Reference & Lore

Source:
- https://wotmud.info/2019/03/07/codex/ (Items, Clothes and Weapons Codex)
- https://wotmud.info/2019/03/09/old-tongue-dictionary/ (Old Tongue Dictionary)
- https://wotmud.info/wheel-of-time-concordance/ (Wheel of Time Concordance)

Fetched: 2026-06-18

All three pages render fully (no JS gate). They are **content/authoring reference**, not
game mechanics — WoTMUD prides itself on book-accuracy, and these are the lists its
builders work from. For AnotherMUD's WoT content pack they are the equivalent source: an
item canon, a constructed-language dictionary, and a setting encyclopedia.

---

## 1. Items, Clothes & Weapons Codex

> "This list of 'stuff from the books' was put together for the Wheel of Time MUD. We pride
> ourselves on our accuracy to the books; ergo, this list! This most recent update includes
> all books up to and including book 11. The page numbers reflect the UK paperback page
> numbers."

**Format:** a giant alphabetical catalog (A–W) of every physical object mentioned in the
Wheel of Time novels, each with a **book/page citation and the in-book owner/context**:

```
<Item, descriptive name> :: <book (roman numeral), page>, <owner / context>
```

Verbatim examples (the canonical shape):

| Codex entry | Citation |
|-------------|----------|
| A'dam, Silver Collar With Attached Silver Bracelet | VII, 474, Sul'dam And Damane |
| Angreal, Carved To Resemble A Flower | VIII, 26, Verin |
| Apron, Long Leather Blacksmith's | I, 20, Harad, Emond's Field |
| Armour, Plate-And-Mail | I, 447, Whitecloak Child Byar |
| Ashandarei, Long, Black-Hafted Spear With A Blade At One End | VII, 640, Mat |
| Axe, Half-Moon Blade With Spike | VII, 62, Perrin's Axe |
| Banner Of The Dragon | I, 773, Chest With Horn Of Valere |
| Banner, Bearing The Red Eagle Of Manetheren | VIII, 184 |

**What it is good for:** it is an **item-authoring canon** — when building a WoT pack, every
weapon/armor/clothing/accessory has a book-sourced description and an in-world owner you can
attach to a mob or room. It is organized **by item type embedded in the name** ("Apron,
…", "Armour, …", "Axe, …", "Banner, …"), so it doubles as a category index. It is **not**
mechanical (no stats, no damage) — purely descriptive/canonical.

**For AnotherMUD:** mirrors our content-authoring need for the `item` templates in
`content/wot/`. The "book + page + owner" citation discipline is a good model for keeping a
setting pack defensibly canon. Notable named items already in our world (ashandarei,
heron-mark blade, angreal/sa'angreal) appear here verbatim.

---

## 2. Old Tongue Dictionary

> "Enhance your roleplay with phrases from the Old Tongue! Source: tor.com"

**Format:** an **English → Old Tongue** dictionary (entries A–Y), plus a final **Special
Terms / Prefixes / Suffixes** section that documents the *grammar* of the constructed
language. Sample entries:

| English | Old Tongue |
|---------|-----------|
| air | raia |
| all | aes |
| always | hei |
| attack | baijan |
| battle | dai; gai |
| battle, brother to/of | gaidin (Aes Sedai word for Warders) |
| Battle Lord | Dai Shan (title for Lan) |
| beloved | lan |
| betrayer of hope | ishamael (a Forsaken's name) |
| blade | mandarb (Lan's stallion); manshima |
| Black Wind | Machin Shin ("journey of destruction") |
| blood / bloodline | shar (pl. shari) |
| boar-horse | s'redit (Seanchan) |
| key battle | gai'don (one that will decide the war) |
| those sworn to peace in battle | gai'shain (Aiel term) |

### 2.1 Grammar — prefixes & suffixes (the generative rules)

This is the most useful part for **procedurally naming** WoT content:

| Affix | Role |
|-------|------|
| `al` / `el` (prefix) | added to first name of Malkieri **kings** / **queens** |
| `ma` (prefix) | denotes importance |
| `sa` (prefix) | the superlative |
| `n` (prefix) | "from" |
| `m` / `n` (prefix) | "of" |
| `de` (prefix) | an agent of action |
| `am` (prefix) | beauty |
| `morat` (prefix) | a handler/controller (Seanchan, re exotic animals) |
| `sai` (prefix, adj.) | referring to power |
| `marath` (prefix) | urgency/compulsion (Seanchan) |
| `ghael` (suffix) | brutes, beasts, monsters |
| `don` / `ing` (suffix) | importance / **utmost** importance |
| `shi` (suffix) | multiples of ten (apostrophe before) |
| `de` (suffix) | negation |
| `ae` (suffix) | passive voice (with verbs) |
| `a` / `an` / `en` / `in` / `on` (suffix) | plural |
| `drelle` (suffix) | river/water(s) of |
| `illar` (suffix) | stone |
| `andi` (suffix) | stone-like quality |
| `nen` (suffix) | one who/that which does ("-er") |
| `ane` (suffix) | past tense |
| `ara` (suffix) | personal possession ("my") |
| `dar` / `din` (suffix) | feminine / masculine |
| `era` (suffix) | "blue" |
| `es` (suffix) | "many" |
| `en` (suffix) | "true" |
| `kar` (suffix) | "punishment through the nervous system" |

**For AnotherMUD:** this is a ready-made **name generator grammar** and an RP glossary. The
prefix/suffix table could drive procedural place/item naming in a WoT pack (e.g. "true X" =
X + `en`, "stone of" patterns, gendered honorifics) and the term list feeds emote/RP flavor.

---

## 3. Wheel of Time Concordance

> "A Guide to Geography, Culture and Other Setting Elements. Source: Rhonda Peters."

**Format & size:** a very large (~470 KB) structured **setting encyclopedia** with a
numbered table of contents. It is the deepest worldbuilding reference on the site. The
top-level structure:

- **Part 0 — Introductory notes** (version/copyright, how to use, spoiler warnings).
- **Part I — Culture & Geography:**
  - 1.0 General Cultural Notes · 1.1 Age of Legends · 1.2 Clothing · 1.3 Crime & Punishment
  - **1.4 Economy & Merchants** · 1.5 Festivals · **1.6 Food** · 1.7 Inns & Taverns
  - 1.8 Phrases/Sayings/Adages · 1.9 Recreation · 1.10 Boats · **1.11 Sicknesses & Diseases**
  - 1.12 Spirituality & Superstition · 1.13 Wisdoms
- **Per-nation sections** (each with Culture + Geography + sub-locations): Aiel & the Waste,
  Altara (Ebou Dar, Remen), Amadicia (Amador…), **Andor** (Caemlyn, Emond's Field, Four
  Kings, Two Rivers, Whitebridge…), Arad Doman, and onward — every nation, town, and notable
  site in the series.

### 3.1 Sample content (the texture of the entries)

**Economy & Merchants (1.4)** — concrete price/economy facts with citations:

- A large silver coin buys a good horse in the Two Rivers (I:30).
- Tar Valon coins show a woman balancing a flame; most people **outside Tar Valon get rid
  of Tar Valon marks** as soon as possible (I:30, 452).
- After a hard winter, **prices are 5× higher** and expected to rise (I:49).
- A fine **Domani carpet is worth the price of a farm** (III:218); the ruby on Mat's dagger
  is worth "a dozen farms" (III:220).
- **Andoran marks weigh more than Illian coins and are worth more** (III:344) — regional
  currency weight/value differences.
- The **Aiel barely use currency** — they trade nuggets of gold/silver or valuable goods,
  assess value skillfully, and bargain hard (IV:605).

**Food (1.6)** — a long flavor list: honeybread, stewed pears, spiced/honeyed wine, berry &
mint tea, mulled wine, oakcakes, sweetcakes, plum/melon punch, pickled quail eggs,
honey-smoked tongue, kippered eel, etc. (each book-cited).

**Sicknesses & Diseases (1.11):** sickhouses tended by the local Wisdom; **yelloweye fever**,
**breakbone fever**, rabies, "fevers and worms."

**Phrases & Adages (1.8):** "The Wheel weaves as the Wheel wills"; "What is already woven
cannot be undone"; "The Light shine on you"; "Burn me"; "blood and ashes" — the canonical
RP exclamations and oaths.

**For AnotherMUD:** the Concordance is the **worldbuilding bible** for a WoT pack — it gives
per-nation culture, real **economy/price anchors** (directly relevant to our
`economy/shops/currency` tuning — note regional coin weight/value and 5× post-winter price
shocks), a **food list** for cooking/sustenance content, a **disease list** for status
effects, and a **sayings list** for RP/emote flavor. It is best treated as a *lookup
reference* (cite section numbers) rather than copied wholesale.

---

## Cross-references

- Economy price anchors (1.4) ↔ AnotherMUD `economy/currency/shops`.
- Food list (1.6) ↔ `crafting-and-cooking` + sustenance content.
- Old Tongue affix grammar ↔ a WoT name generator + emote/RP flavor.
- Item codex ↔ `item` templates in `content/wot/`; named items overlap our WoT EPIC work.
