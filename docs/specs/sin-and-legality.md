# Legality & Licensing (SIN)

> **Layer:** Action/interaction — an extension of [economy-survival](economy-survival.md) §3 (shops).
> **Status:** Slice 1 (this doc) — the two-tier market gate. Slice 2 (verification / burn) is
> deferred; see §7.
>
> **Setting-agnostic engine, per-pack vocabulary.** The engine names this mechanism
> **legality** (a gear property), **licensing** (a shop that demands a credential), and
> **credentials** (carried permit-bearing items). A content pack skins those terms: the
> Shadowrun pack calls a credential a **fake SIN** and a permit a **license**, and exposes
> the SR-flavored `sin` alias for the `licenses` verb. This is the same
> generic-mechanism / pack-label seam as the currency label (`economy-survival` §2.1) and
> the per-pack stealth skill (`skills.md`). Nothing about SINs lives in the kernel — a
> world with no legality tags and no `requires_license` shops never sees any of it.

## 1. Overview

In a cyberpunk (or any regulated) setting, gear is stratified by *legality*: freely-sold
goods, **restricted** goods that a licensed citizen may buy, and **forbidden** goods sold
only in the shadows. A character's ability to shop the legitimate market turns on whether
they can present a valid **credential** (a registered — or convincingly faked — identity)
and, for restricted goods, a matching **permit**.

This slice makes that stratification bite at the **point of purchase**. It introduces:

- a **legality** tag on item templates (`legal` / `restricted` / `forbidden`),
- an optional **permit** category on restricted items (which license clears them),
- a **`requires_license`** flag on a shop (the legitimate storefront that scans customers),
- **credential items** a character carries, each bearing the **permits** it grants,
- and the `licenses` verb (pack alias `sin`) to see what papers you hold.

### Goals

- Create a **two-tier market**: legitimate storefronts demand a credential and gate
  restricted goods behind matching permits; shadow vendors ask for nothing.
- Reuse existing substrate: credentials are ordinary **items** (bought, looted, traded),
  so no new persistence and no save-version bump.
- Keep the whole system **opt-in through content** — an untagged world is unaffected.

### Non-goals (this slice)

- **Verification / burn.** No scan roll, no credential-rating consumption, no "burned"
  state. A credential is a static key here; the risk layer is Slice 2 (§7).
- **Selling** legality gates. The gate is on **buying** from a `requires_license` shop.
  Fencing stolen/forbidden goods keeps the existing `economy-survival` §3.6a buy-category
  gate; no new sell-side legality check.
- **Lifestyle / SIN upkeep**, contraband confiscation on movement, and cop/checkpoint
  encounters. Out of scope; some fold into Slice 2 or a separate lifestyle spec.

## 2. Legality on items

An item template MAY declare a legality band and, when restricted, the permit category that
clears it. Both are read off the template property bag (untyped, per
`inventory-equipment-items`), defaulting so untouched content behaves exactly as before.

- `legality`: one of `legal` (default), `restricted`, `forbidden`. Absent / unrecognized →
  `legal`.
- `permit`: a free-form category string (e.g. `firearms`, `cyberware`). Meaningful only on a
  `restricted` item; ignored otherwise. Absent on a restricted item → the item needs *any*
  valid credential but no specific permit.

### Acceptance criteria

- [ ] An item with no `legality` property is treated as `legal`.
- [ ] `legality` is case-insensitive; an unrecognized value falls back to `legal` (fail-open,
      never trap content behind a typo).
- [ ] `permit` is read only for a `restricted` item; a `permit` on a `legal` or `forbidden`
      item has no effect.

## 3. Credentials

A **credential** is an ordinary item carrying the `credential` tag and a `permits` property —
a list of permit-category strings it clears. A character "has" a credential simply by
**carrying it** (in inventory; equipped is not required this slice). A pack skins the
credential item as it likes — a fake SIN, a forged writ, a guild token.

- `permits`: a list of category strings. A credential with `permits: [firearms, cyberware]`
  clears any restricted item whose `permit` is `firearms` or `cyberware`.
- A credential with an empty / absent `permits` list still counts as a **valid identity**
  (it clears the `requires_license` presence gate and any restricted item that names *no*
  permit), but clears no specific permit category.
- `credential_rating` (optional, integer): **inert this slice.** Authored as forward-looking
  flavor (a rating-N fake SIN); consumed only by the Slice 2 verification check (§7). The
  Slice 1 gate never reads it.

### Acceptance criteria

- [ ] A carried item tagged `credential` is discoverable as the character's identity.
- [ ] A character may carry several credentials; the gate is satisfied if **any** carried
      credential clears the requirement (permits are unioned across carried credentials for
      the presence check, but a single item must supply the specific permit — see §4).
- [ ] Removing / dropping / selling the credential item removes the character's access with
      no extra bookkeeping (it is just an item).

## 4. The purchase gate

A shop MAY set `requires_license: true` — it is a legitimate storefront that scans every
customer. The gate runs inside the buy operation (`economy-survival` §3.5), positioned
**after** the faction standing gate ([faction](faction.md) §6) and **before** the skill and
gold gates, so a refusal on papers precedes any pricing.

For a `requires_license` shop, purchasing an item resolves in order:

1. **Identity presence.** The buyer must carry at least one `credential`. None →
   refused (`SINRequired`). A shop with `requires_license: false` (the default) skips the
   entire gate — shadow vendors never ask.
2. **Restricted → permit.** If the resolved item is `restricted`, some single carried
   credential must list the item's `permit` in its `permits` (or the item must name no
   permit). No matching permit → refused (`LicenseRequired`, naming the permit).
3. **Forbidden.** A `forbidden` item at a `requires_license` shop is refused
   (`ForbiddenGoods`) — a legitimate store does not sell contraband regardless of papers.
   In practice content simply does not stock forbidden goods in a legal shop; the rule is a
   backstop, not a content pattern.

A non-`requires_license` shop applies **no** legality check: it sells whatever it stocks
(including `forbidden` goods) to anyone who clears the existing standing / skill / gold
gates. This is what makes today's fixers "the shadows".

### Acceptance criteria

- [ ] A SINless buyer (no carried credential) at a `requires_license` shop is refused before
      pricing, with a message naming the missing credential.
- [ ] A buyer carrying any valid credential clears the presence gate for a `legal` item.
- [ ] A `restricted` item is sold only when a carried credential lists its `permit`; the
      refusal names the required permit.
- [ ] A `forbidden` item is refused at a `requires_license` shop even with a full credential.
- [ ] A non-`requires_license` shop sells `restricted` and `forbidden` goods with no
      credential and no permit (only the pre-existing gates apply).
- [ ] The gate reads only **carried** items — an equipped-but-not-carried credential does not
      count this slice (documented limitation; revisit if it surprises players).
- [ ] Listings / stock completion at a `requires_license` shop are unaffected (the shop still
      *shows* its stock; the gate bites on the buy attempt). A later slice may hide unbuyable
      stock the way the §7 skill gate hides items.

## 5. Viewing your papers — the `licenses` verb

`licenses` (pack alias `sin`) lists the credentials the character is carrying, each with the
permits it grants. With no credential carried, it reports the character is running SINless
(pack-flavored). This is a read-only convenience command; it charges nothing and changes no
state.

### Acceptance criteria

- [ ] `licenses` with no carried credential prints a "no valid credential / SINless" line.
- [ ] `licenses` lists each carried credential item by name with its permit categories.
- [ ] The `sin` alias resolves to the same handler (registered by the SR pack's vocabulary;
      a pack without the alias still has `licenses`).

## 6. Configuration surface

| Key | Where | Default | Meaning |
|---|---|---|---|
| `legality` | item template property | `legal` | `legal` / `restricted` / `forbidden`. |
| `permit` | item template property | *(none)* | Permit category clearing a `restricted` item. |
| `credential` | item template tag | *(absent)* | Marks the item as a carried identity credential. |
| `permits` | item template property | *(empty)* | Permit categories a credential clears. |
| `credential_rating` | item template property | *(none)* | **Inert this slice**; Slice 2 verification input. |
| `requires_license` | shop block | `false` | Shop scans customers and gates by legality. |

No new `ANOTHERMUD_*` knobs and **no save-version bump** — credentials are items and the gate
is stateless.

## 7. Open questions / deferred (Slice 2 and beyond)

- **Verification & burn (Slice 2).** The risk layer: a scan (border check, legitimate-store
  scanner, a cop's SIN check) rolls against a fake credential's `credential_rating`; a failed
  roll **burns** it (a persisted or item-level "burned" flag) so it no longer clears gates.
  This needs a check primitive (`skills.md` / `saves.md`), a burned-state carrier, and
  scanner content — it is the depth that makes buying a *higher-rated* fake matter. Slice 1
  deliberately ships the market divide first so the risk has something to protect.
- **Equipped vs. carried.** Slice 1 checks carried items only. If "broadcast one SIN at a
  time" matters (it does for burn — a scan burns the *broadcast* credential), Slice 2 likely
  introduces an active/broadcast credential rather than "any carried".
- **Sell-side legality.** Should a legitimate shop refuse to *buy* obviously-forbidden goods
  from a SINless seller? Left to the existing buy-category gate for now.
- **Lifestyle upkeep** (a periodic drain à la sustenance) and **contraband on movement / fast
  travel** (a "no-contraband" gate) are adjacent identity/legality ideas tracked in
  `docs/themes/shadowrun-pack-plan.md`; neither is in this slice.
- **Hiding unbuyable stock.** Whether a `requires_license` shop's listing should omit items
  the buyer can't clear (like the skill gate does) or show-then-refuse (current choice).
