# Legality & Licensing (SIN)

> **Layer:** Action/interaction — an extension of [economy-survival](economy-survival.md) §3 (shops).
> **Status:** Slice 1 — the two-tier market gate (§2–§6). **Slice 2 — verification / burn (§7)** —
> the scan roll that burns a fake on failure. **Slice 3 — movement checkpoints (§7.1)** — the same
> scan on a movement/access threshold. Slice 4+ (the security-response / heat consumer, active-
> broadcast SIN, lifestyle upkeep) stay deferred; see §8.
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
- `credential_rating` (optional, integer): the fake's quality. In Slice 1 it is inert; in
  **Slice 2 (§7)** it is the bonus the scan roll adds — a higher-rated fake resists scrutiny.
  Absent → 0 (a fake that relies entirely on the die).
- `burned` (runtime instance state, persisted): set when a scan fails (§7). A burned credential
  clears **no** gate — the presence and permit checks (§4) treat it as if it were not carried —
  and is flagged as spent by the `licenses` verb (§5). It is a per-instance flag (one fake
  burns; other carried credentials are untouched) and survives a relog.

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
   permit). No matching permit → refused (`LicenseRequired`, naming the permit). When a
   credential *does* clear the permit, buying a restricted good is the scrutiny trigger for
   the **scan (§7)**: the store rolls against the fake, and a failure burns it and refuses
   the sale (`SINBurned`).
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
- [ ] A **burned** credential (§7) is excluded from every check here — it neither satisfies the
      presence gate nor clears a permit; a buyer whose only credential is burned is treated as
      SINless.
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
- [ ] A **burned** credential (§7) is listed but marked spent/useless, so the player knows to
      replace it rather than wondering why the store still refuses them.
- [ ] The `sin` alias resolves to the same handler (registered by the SR pack's vocabulary;
      a pack without the alias still has `licenses`).

## 6. Configuration surface

| Key | Where | Default | Meaning |
|---|---|---|---|
| `legality` | item template property | `legal` | `legal` / `restricted` / `forbidden`. |
| `permit` | item template property | *(none)* | Permit category clearing a `restricted` item. |
| `credential` | item template tag | *(absent)* | Marks the item as a carried identity credential. |
| `permits` | item template property | *(empty)* | Permit categories a credential clears. |
| `credential_rating` | item template property | `0` | The fake's quality — the bonus the §7 scan roll adds. |
| `requires_license` | shop block | `false` | Shop scans customers and gates by legality. |
| `scanner_rating` | shop block | `0` | The §7 scan DC. `0` (or unset) = the store checks papers but never rolls a scan (Slice-1 behavior). |
| `checkpoint_scanner` | room property | `0` | The §7.1 movement-checkpoint scan DC. `> 0` makes the room an access-controlled threshold; `0` / unset = not a checkpoint. |
| `checkpoint_permit` | room property | *(none)* | The §7.1 access license a mover must carry to cross. Absent = an identity-only checkpoint (any valid credential is scanned). |

**Slice 1** adds no `ANOTHERMUD_*` knobs and no save bump — the gate is stateless. **Slice 2**
adds the persisted `burned` flag: `InventoryEntry.Burned`, a **save-version bump** with an
additive migration (absent → not burned), re-hydrated onto the credential instance at login the
same way a magazine's loaded-round count is (`inventory-equipment-items`).

## 7. Verification & burn (Slice 2)

The risk layer. A `requires_license` store carries a `scanner_rating` — its scrutiny. When a
buyer attempts to purchase a **restricted** good and a carried credential clears its permit
(§4.2), the store **scans** that credential before completing the sale. This is the *only*
scan trigger this slice: a legal-good purchase clears on presence with no scan, and a
`scanner_rating` of 0 (or unset) means the store checks papers but never rolls — the Slice-1
behavior. (Higher-DC checkpoint / cop scans on a new access axis are Slice 3, §8.)

### The scan

The scan is one skill-check-shaped roll (`skills.md` §3): `d20 + credential_rating` vs.
`scanner_rating`. The natural-1-fails / natural-20-succeeds edges apply, as everywhere. On
**success** the fake holds and the sale completes normally. On **failure** the credential is
**burned** and the sale is **refused** (`SINBurned`, naming the fake).

- The credential scanned is the one that cleared the permit; when several carried credentials
  clear it, the **highest-rated** is used (the runner flashes their best fake) — that is the
  one at risk.
- A higher `credential_rating` is strictly better odds, which is the whole point: it makes a
  premium fake worth its price.

### Burn

A burned credential is a spent fake. It clears no gate (§4 treats it as absent) and shows as
useless under `licenses` (§5). The player must acquire a new one. Burn is a **persisted
per-instance flag** (`InventoryEntry.Burned`): only the scanned fake burns, other carried
credentials are untouched, and the state survives a relog (re-hydrated onto the instance at
login like a magazine's loaded count).

### Acceptance criteria

- [ ] A scan fires only when buying a **restricted** good at a `requires_license` shop whose
      `scanner_rating > 0` and whose permit a non-burned carried credential clears.
- [ ] Buying a **legal** good never triggers a scan (presence gate only).
- [ ] A `scanner_rating` of 0 / unset never rolls — the store keeps Slice-1 behavior.
- [ ] On scan success the sale completes and the credential is unchanged.
- [ ] On scan failure the credential is marked burned and the sale is refused (`SINBurned`);
      the buyer is not charged.
- [ ] Only the highest-rated matching credential is scanned/burned; other credentials are
      untouched.
- [ ] A burned credential is excluded from all §4 checks and marked spent under §5.
- [ ] The burned flag round-trips through save/load (a relog does not un-burn a fake).
- [ ] The scan is deterministic under a seeded roller (mirrors the `pick` / `search` checks).

### 7.1 Movement checkpoints (Slice 3)

A checkpoint is a **destination room** that scans a mover's credentials on entry — a corp-zone
turnstile, a border. A room opts in with `checkpoint_scanner` (the scan DC, > 0) and an optional
`checkpoint_permit` (the access license the mover must carry). Crossing **into** the room runs
the same credential logic as the store gate, then the §7 scan; a failure burns the presented
fake and **refuses the step** (the mover stays put). It is the same scan + burn primitives as
§7, on the movement axis instead of the buy axis.

- The check runs in the player-volition move command, **before** the movement-cost spend, so a
  refused crossing costs nothing. The move *primitive* stays unconditional (mob / scripted /
  admin moves are never gated), exactly like the hidden-exit and darkness gates.
- **SINless** (no valid credential) → refused. **No matching permit** → refused (no scan — the
  license isn't there to check). Otherwise the highest-rated matching fake is scanned.
- Unlike the store's legal-goods path, an **identity-only** checkpoint (no `checkpoint_permit`)
  still **scans** — a border verifies the SIN is real regardless of licenses.
- A checkpoint DC is typically **stricter** than a store counter, so even a good fake burns
  often enough to make the crossing a genuine risk.

Acceptance criteria:

- [ ] A room with no positive `checkpoint_scanner` is not a checkpoint (no gate).
- [ ] A SINless mover and a mover lacking the required permit are both refused, in place.
- [ ] A cleared scan lets the step commit; a failed scan burns the fake and refuses the step.
- [ ] A no-permit refusal runs **no** scan (nothing burns).
- [ ] An identity-only checkpoint (`checkpoint_permit` absent) scans the best carried credential.
- [ ] A burn at a checkpoint persists (same `InventoryEntry.Burned` path as §7).
- [ ] The gate runs before the movement-cost spend (a refused crossing costs no movement).

## 8. Open questions / deferred (Slice 4 and beyond)

- **Security-response / heat — ✅ SHIPPED (v1, `security-response.md`).** The consequence engine
  the checkpoint feeds: a crime (v1: a kill) in a policed zone raises **heat** and schedules a
  timed patrol that grudge-hunts the offender. "SINless = invisible to law" is mechanical there —
  the identity axis (§7.1) gates *access*, heat gates *pursuit*, and a valid vs. burned/absent
  SIN is what lets the law track the offender to their current room (`security-response.md` §4).
  Still deferred there: a **burned-SIN-at-a-scan** heat source, non-kill crimes, and escalation.
- **Active / broadcast credential.** §7 / §7.1 scan "the highest-rated matching carried
  credential". A full broadcast model (the player *chooses* which SIN to present — flash a
  throwaway to protect the premium — and only that one is ever scanned) is deferred; it needs a
  `broadcast` verb + a persisted active-SIN threaded through both the store and checkpoint gates.
- **Directional / exit-level checkpoints.** §7.1 gates a *room* on entry (from every direction).
  A finer model gates a specific *exit* (one threshold, one direction) and would carry the
  checkpoint data on the exit rather than the destination room.
- **Sell-side legality.** Should a legitimate shop refuse to *buy* obviously-forbidden goods
  from a SINless seller? Left to the existing buy-category gate for now.
- **Lifestyle upkeep** (a periodic drain à la sustenance) and **contraband on movement / fast
  travel** (a "no-contraband" gate) are adjacent identity/legality ideas tracked in
  `docs/themes/shadowrun-pack-plan.md`; neither is in this slice.
- **Hiding unbuyable stock.** Whether a `requires_license` shop's listing should omit items
  the buyer can't clear (like the skill gate does) or show-then-refuse (current choice).
