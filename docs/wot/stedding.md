# Stedding

Reference for the Ogier *stedding* — the hidden sanctuaries where the One Power
does not exist. Canonical characteristics (from the books, with Robert Jordan /
Brandon Sanderson clarifications), the MUD gameplay implications, and the current
state of the AnotherMUD implementation.

Related: [`other-worlds.md`](other-worlds.md) §Stedding (the Ways / Waygate +
Tel'aran'rhiod context), [`backgrounds.md`](backgrounds.md) §Ogier (the Longing,
the named stedding). In content, Stedding Chinden (`content/wot/`,
`stedding-chinden` area) is the first stedding built — the nearest to the Two
Rivers, reached west of Emond's Field through the Mountains of Mist.

---

## 1. Canonical characteristics

### The True Source is completely blocked

Channelers cannot access the One Power inside a stedding — and critically, they
cannot even *feel* it, as if they never had the ability to channel at all. This
is **total, not partial**. The True Source simply does not exist inside the
boundary. The blocking effect also extends to the **True Power** of the Dark One.

### Existing weaves behave differently

Robert Jordan clarified the nuance:

- The **Warder Bond** and a **shield tied off** on a channeler **remain intact**
  when entering a stedding — persistent bonds survive.
- A **Mirror of Mists** disguise **unravels** upon entry — active weaves collapse.
- **Circles** are less clear; Brandon Sanderson put their survival at roughly
  **75%**, but that is not conclusive.

The rule of thumb: *persistent bonds survive; active/maintained weaves collapse.*

### Shadowspawn won't enter willingly

Neither **Trollocs** nor **Myrddraal** will ever enter a stedding unless driven,
and it requires considerable driving. This also applies to **ravens** while they
are possessed by the Dark One. It is **instinctive revulsion**, not merely
tactical avoidance.

### Inaccessible from Tel'aran'rhiod

Stedding are inaccessible from **Tel'aran'rhiod**, the World of Dreams. The dream
world simply does not contain them.

### An overwhelming aura of peace

An aura of **peace and well-being** is felt by anyone inside a stedding —
channelers and non-channelers alike. It is not subtle; it is viscerally felt by
everyone who enters.

### Great Trees

Stedding are populated by what the Ogier call **Great Trees** — ancient,
enormous, and sacred to the Ogier, but largely undescribed in the books.

### The Longing

Ogier who remain **outside** a stedding too long die of the **Longing**. The
process takes **several years**. It is the biological tether that binds the Ogier
to their homes.

### Historical refuge for male channelers

During the **Breaking**, because stedding suppress channelling, they were sought
as refuge by **male channelers** afraid of going mad from the Taint. They lived
many more years this way — but it **slowed the madness without curing it**, and
the isolation from the Source eventually became unbearable; inevitably they could
not stay.

### The Ways were created inside stedding

It was those sheltering male Aes Sedai who created the **Ways**, presumably for
greater mobility throughout the world. **Every Waygate is therefore located at or
near a stedding.**

### Waygates at each stedding

Except where otherwise noted, the Ogier agreed to set **guards** on their
Waygate — most, but not all. A handful of stedding (**Saishen, Tanhal, Sholoon,
Shadoon**) refused.

---

## 2. MUD gameplay implications

| Canon | Mechanic | Notes |
|---|---|---|
| True Source blocked | **Room-tagged zone effect** — a `stedding` room tag suppresses all One Power abilities; channeling-class abilities cannot activate and non-persistent active weaves are suppressed. The "can't even sense it" flavour means Power-detection/diagnostic abilities should be suppressed too. | The big one. |
| Shadowspawn won't enter | **Mob-AI gate on the room tag** — Trollocs, Fades, and DO-controlled ravens treat `stedding` rooms as forbidden. Fits the existing AI disposition system. | Second most meaningful. |
| Inaccessible from T'a'R | `stedding` rooms are **excluded** from any future dream-world layer. | Only relevant once T'a'R exists. |
| Aura of peace | **Passive rest/regen bonus** for all players in the room, channeler or not. Strongly implies **no PvP** inside — Ogier culture is explicitly pacifist; no violence belongs in a stedding. | Maps to `healing_rate` + a no-PvP flag. |
| Waygate at each stedding | Every stedding should have a **Waygate room** nearby — strategic travel significance even though you cannot channel there. | Ways travel is its own system. |
| The Longing | A **timer/debuff** for Ogier (NPC, or PC if ever playable) Outside too long — worsens over time, cleared only by returning to a stedding. | Ogier are a non-playable NPC race here, so this is NPC-flavour today. |

---

## 3. Implementation status in AnotherMUD

| Effect | Status |
|---|---|
| **One Power suppressed inside** | ✅ **Shipped** (commit `8231edf`). A `stedding` room tag + a gate in the cast chokepoint (`enqueueAbility`) refuses any One-Power weave (`AbilitySpell`) cast from a `stedding`-tagged room ("Within the stedding the True Source lies beyond your reach"). Mundane abilities pass; weaves work outside the bound. Stedding Chinden's five interior rooms carry the tag. |
| **Peace aura — rest haven** | ✅ **Shipped** (commit `261f917`): every stedding room carries a `healing_rate` regen bonus (Stump 3, the rest 2) — the aura of peace as a passive rest/regen for all in the room, channeler or not. |
| **Peace aura — no violence** | ✅ **Shipped** (commit `261f917`): every stedding room is tagged `safe-room`, so the combat engage-refusal blocks all violence inside (PvP and PvE) — "Violence is forbidden here." |
| **Shadowspawn refusal-to-enter** | ✅ **Shipped** (commit `261f917`): a mob tagged `shadowspawn` will not cross into a `stedding`-tagged room — gated in both wander (idle roaming) and retaliate (pursuit drops the grudge at the bound, so fleeing into a stedding is sanctuary). Pre-emptive — no Shadowspawn mobs exist yet; the gate fires on the `shadowspawn` mob tag, ready for the first Trolloc/Fade. |
| **Suppress active weaves on entry / "can't even sense it"** | ⛔ Deferred — the current gate blocks the *cast*; collapsing already-active maintained weaves on entry (keeping persistent bonds) and suppressing Power-detection wants an effect-system hook on room transition. |
| **Block weave effects originating outside, targeting inside** | ⛔ Deferred — needs target-room awareness in the weave-effect path. |
| **Tel'aran'rhiod exclusion** | ⛔ Deferred — T'a'R is unbuilt. |
| **Waygate** | 🟡 Placed as **lore** (Stedding Chinden's Overgrown Waygate, deliberately shut — the Avendesora leaf removed). The Ways as a traversable realm-graph are unbuilt; this is the gateway node for when they are. |
| **The Longing** | ⛔ Not applicable as a player mechanic — Ogier are a non-playable NPC race here. Would be an Ogier-NPC timer only. |
