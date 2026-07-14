# AnotherMUD Playtest — Shadowrun MVP

Manual QA for the **Shadowrun** street-samurai slice (SR-M1 attribute set →
SR-M2 typed damage → SR-M3 the playable pack). This is a **separate world** on
the same engine — a different boot, a different character, its own district. The
core/starter-world guide (`core.md`) and the Wheel of Time guide (`wot.md`) are
siblings; the section numbers here (**§37–§44**) continue the guide-wide anchor
sequence.

> Format: `- [ ] command` — what should happen. Mark `[x]` on pass; add a
> `BUG:` note inline on fail.

Every box here is backed by a live smoke test under `cmd/telnet-smoke`
(`shadowrun_*_live_test.go`) — this is the human-facing walkthrough of the same
paths those tests drive.

---

## 37. Setup (the Sixth World)

### Boot

```sh
ANOTHERMUD_PACKS=shadowrun \
ANOTHERMUD_START_ROOM=shadowrun:street-corner \
make run
telnet localhost 4000
```

`make run` (core/starter-world) does **not** load this world — you must set
`ANOTHERMUD_PACKS=shadowrun`. The dependency closure pulls in `tapestry-core`
automatically (SR reuses the engine's slots/abilities/effects/channels).

### Admin & provisioning

The **first character of a fresh save is auto-granted admin**, so `teleport` /
`xp` / `set` / `restore` work out of the box for the hops below. If you already
have Shadowrun characters on this save, boot with an explicit seed instead
(names are letters-only, so `Runner`, not `runner1`):

```sh
ANOTHERMUD_ROLE_SEED="Runner:admin" ANOTHERMUD_PACKS=shadowrun \
  ANOTHERMUD_START_ROOM=shadowrun:street-corner make run
```

A fresh Street Kid starts with **500 nuyen** and a pick-one starting loadout
(see §38); the street corner also lays out a full sample kit on the ground.

### The district (the Seattle sprawl)

```
   A Cramped Back Alley                         [safe]
        |  n
   A Rain-Slick Street Corner  ──e──  Market Street  ──e──  Corporate Plaza
   (safe hub · starter gear ·          (ganger —              (sec-guard —
    the fixer's shop)                   HOSTILE)               neutral, corp turf)
```

- **Street Corner** is a `safe-room` — no combat. It's the gear-up hub: the
  **katana**, **Ares Predator V**, a **clip**, a **caseless round**, a **stun
  baton**, an **armored jacket**, and a ripperdoc's cyberware tray (**wired reflexes**,
  **muscle replacement**, **cybereyes**) all lie on the ground, and a **street
  fixer** runs a shop here (§42).
- **Market Street** (`e` from the corner) is contested turf — a hostile **street
  ganger** (katana + jacket) jumps you. This is the fight room.
- **Corporate Plaza** (`e` again) holds a **corp-sec guard** (SMG + vest,
  `xp_value` 55) — **neutral**: she won't start it, but attack her or trespass
  and she returns fire.

---

## 38. Character creation — a Street Samurai

Login is account-first (identical to core §1). Create a new account/character;
the Shadowrun wizard is the **default** flow (no channeling step).

- [ ] Walk the wizard — after the name it prompts, in order:
      **gender** (Male/Female), then **metatype** ("Choose your race:" — 1) Dwarf
      2) Elf 3) Human 4) Ork 5) Troll, alphabetical), then **class** ("Choose
      your class:" — 1) Street Samurai — the only class in the MVP), then
      **background** (1) Street Kid).
- [ ] The **Street Kid** background carries two pick-one choosers: a **feat**
      (Alertness *or* Stealthy) and a **starting kit** (Ares Predator V + round +
      jacket / katana + jacket / stun baton + vest). Pick one of each.
- [ ] Confirm — the character spawns at the **Street Corner**.
- [ ] `score` (`sc`) — the identity line reads **Gender Metatype Class** (e.g.
      "Male Dwarf Street-samurai" — class and background render from their ids,
      hyphenated), with a **Background** line below it; the attribute block shows
      the **eight Shadowrun primaries** — **BOD AGI REA STR WIL LOG INT CHA** —
      plus **Edge** (**EDG**), not the classic STR/DEX/CON/INT/WIS/CHA six. The
      advancement track reads **Level 1 - The Long Run** (§43), and **MA 0/0**
      (a street samurai doesn't channel).
- [ ] `score` purse — **500** nuyen (shown as `Gold`; nuyen is flavor over the
      shared currency — a known cosmetic gap).

## 39. Melee combat — lethal vs. stun (the two monitors)

Shadowrun tracks two damage monitors: the **Physical** monitor (the hp/Vitals
track — lethal) and the **Stun** monitor (a Willpower-derived pool — nonlethal
knock-out). The weapon decides which one your hits land on. Gear up on the
corner, then `teleport shadowrun:market-street` (or walk `e`) to reach the
ganger.

### Lethal — the katana (Physical monitor → a kill)

- [ ] `get katana`, `equip katana wield` — `equipment` shows it wielded.
- [ ] `teleport shadowrun:market-street`, then `kill ganger` — combat rounds
      tick. The katana has **no `target_pool`**, so its damage lands on the
      **Physical** monitor (the engine's default lethal path).
- [ ] **Soak applies:** the ganger's **Body (3)** + his worn **armored jacket
      (`armor_bonus` 3)** reduce each hit through the wired `mitigation` channel
      — the kill grinds through real Shadowrun soak, not raw hp. (A novice runner
      may need several rounds; `restore` between rounds or `xp`-level to speed a
      demo fight, exactly as the live test does.)
- [ ] On the finishing blow the ganger is **slain** — a **corpse** appears, and
      his loose **nuyen** credits to you (loot; §42). This is the lethal outcome.

### Nonlethal — the stun baton (Stun monitor → a knock-out)

- [ ] Back on the corner, `get baton`, `equip baton wield`. The stun baton
      declares **`target_pool: stun`**.
- [ ] Return to Market Street and `kill ganger` — its hits route to the ganger's
      **Stun** monitor. When that monitor bottoms out you **"knock a street
      ganger out cold"** — a **knock-out**, *not* a kill: **no "slain" line and
      no corpse** (the opposite of the katana). He's down, not dead.

> The Stun monitor seeds from **Willpower**; the Physical monitor is the flat
> hp/Vitals track (Design 1 — not yet Body-derived, a tracked SR-M3 tail).

## 40. Firearms, clips & reloading (Ares Predator V)

A firearm is **holder-fed**: rounds live in a **clip**, the clip goes *into* the
gun, and firing draws from the inserted clip. The Ares Predator V takes a
**heavy-pistol clip** (holds **15**, SR5 "15 (c)"). The unified `reload` verb
"tops up the target from the tier below" — `reload <clip>` loads loose rounds
into a clip; `reload` loads a clip into the wielded gun (ejecting the spent one).
Single-district combat is the **melee band**, so a gun fires at a point-blank
penalty (SR5) — buff **Agility** (firearm to-hit = skill + Agility) for reliable
demo hits.

### Clipless until loaded

- [ ] On the corner, `get pistol`, `equip pistol wield`, `get clip` (an **empty**
      clip lies on the corner). `reload` — "You have no loaded clip to load into
      an Ares Predator V." (the clip is empty).
- [ ] `teleport shadowrun:market-street`, `kill ganger` — a gun with no loaded
      clip **clicks dry** every swing (no shot, no damage).

### Fill a clip, load it, fire it down

- [ ] Get rounds: `buy round` several times from the fixer (§42), or `get round`
      on the corner — caseless rounds **stack** in inventory (`i` → `a caseless
      round (xN)`).
- [ ] `reload clip` — "You load rounds into an Ares Predator V clip. (N/15)": loose
      rounds pour into the clip, up to 15. `reload clip` again after buying more →
      tops it up; on a full clip → "It's already full. (15/15)".
- [ ] `reload` — "You slap a fresh clip into an Ares Predator V. (15/15)": the
      loaded clip goes into the gun. Carrying rounds isn't enough, and neither is
      an empty clip — the clip must be **loaded** and **inserted**.
- [ ] `kill ganger` — the pistol **fires**, spending one round from the inserted
      clip per swing; a landed shot is **lethal** (no `target_pool` → the Physical
      monitor, through the ganger's soak). When the clip empties it **clicks dry**.

### Swapping clips ejects the spent one

- [ ] With a partly-spent clip in the gun, load a second clip (`reload clip` on a
      fresh clip, then `reload`) — "The spent clip ejects and clatters to the
      ground." `look` shows the ejected clip on the floor; `get clip` recovers it
      (with its remaining rounds) to refill later. Left alone, it **decays** off
      the ground after `ANOTHERMUD_EJECTED_HOLDER_LIFETIME` (default 3m) so
      firefights don't permanently litter a room.

### The loaded gun persists

- [ ] Load a clip into the gun, `quit`, log back in, and `kill ganger` — it
      **fires**: the inserted clip (and its rounds) round-trips through your save,
      so a loaded gun stays loaded across relog.

> `reload` is the firearm verb; `load` still chambers a crossbow. Reload is a
> **timed busy action** (`ANOTHERMUD_RELOAD_DURATION`, default 1s): `reload`
> reports "You begin reloading." and completes a beat later; a second action
> mid-reload is refused as busy. (Per-method Simple/Complex differentiation is a
> refinement; set the knob to 0 for instant.)
>
> **special ammo through a clip:** buy **APDS rounds** from the fixer (`buy
> apds`), fill a clip with them (`reload clip`), and load it — shots fired from
> that clip carry the round's grade (a to-hit bonus), because the clip is
> homogeneous and the grade rides it (grade-through-holder). The grade persists
> with the clip (carried or inserted). Still deferred: **SMG burst** and
> **cross-room `shoot`**.

## 41. Cyberware (augmentation on the score sheet)

Cyberware installs into a dedicated **cyberware slot** and shifts an SR attribute
through the standard equip → source-key → stat-block pipeline (no bespoke
cyberware code). The corner's sample tray has three pieces.

- [ ] `score` — note your **REA** (Reaction).
- [ ] `get reflexes`, `equip reflexes` — **wired reflexes** installs (its single
      eligible slot, `cyberware`, auto-resolves — no slot argument needed).
      `score` — **REA has risen by 2**.
- [ ] `unequip reflexes` — `score` — **REA drops back** to base.
- [ ] Try the other two: **muscle replacement** (`get muscle`, `equip muscle`)
      raises **STR +1 and BOD +1**; **cybereyes** (`get cybereyes`, `equip
      cybereyes`) raise **INT +1**. Each is visible on `score`.

## 42. Nuyen & the fixer (the shop)

A **street fixer** works the safe corner — the spend side of the nuyen economy
(the earn side is looted corpses, §39). She's friendly, so she fits the
safe-room.

- [ ] `score` — read your nuyen balance (shown as `Gold`; a fresh Street Kid has
      **500**).
- [ ] `list` — the fixer's ammo SKUs (ammo-and-reloading §6) plus gear: a
      **loaded Ares Predator V clip** (the primary buy — arrives full), a
      **caseless round** (loose refills), an **APDS round** (graded ammo, §40), an
      **empty clip** (a cheap spare to fill), an **Ares Predator V**, an **armored
      jacket**, and **cybereyes**.
- [ ] `buy round` — "You buy … for N gold. You have M gold left." — a caseless
      round enters inventory; repeat to fill a clip (§40).
- [ ] `buy loaded` — buys a **pre-loaded** clip. `get pistol`, `equip pistol
      wield`, `reload` — it inserts as a full **(15/15)** with no fill step (the
      SR5 "carry loaded spares" model). `buy clip` buys an **empty** clip to fill
      yourself.
- [ ] Earn the other way: kill the ganger (§39) and `loot corpse` — his loose
      **nuyen credits to your balance** (not an inventory item), same as the core
      coin path.

> Nuyen is currently the shared currency under a Shadowrun skin — the `score`
> purse and buy/sell lines still say **"gold"**. A nuyen-labelled purse is a
> tracked cosmetic follow-up, not a mechanics gap.

## 43. Karma advancement (The Long Run)

Advancement is **karma-as-XP** (pinned decision D3, Option A): a kill grants
karma on the Shadowrun world track, and accumulating it levels the track — the
generic progression engine, exercised on the SR track.

- [ ] `score` on a fresh Street Samurai — the track is **The Long Run**, **Level
      1** (the class binds `bound_track: street`; SR progression stays world-locked
      inside the SR world).
- [ ] Kill a **street ganger** (§39, `xp_value` 30) as a solo killer — **"You gain
      30 experience."** (the full award; the grouping kill-XP seam).
- [ ] Cross the **Level-2 threshold** (100 XP on the street track's curve) — you
      advance to **Level 2** with the samurai's level-up flavor. Admin `xp <n>`
      fast-forwards the accumulation for a demo (the *earn-from-a-kill* signal is
      the part proven by the fight; the level-up is the generic mechanic on the SR
      track).

> The verb is still `xp` and the sheet still says "experience" — karma is the
> SR framing of the same track XP. A dedicated **karma ledger** (SR-M5) and the
> **Essence** pool that cyberware would erode (SR-M4) are post-MVP, deferred.

## 44. Item modification — armor mods & weapon accessories

Gear is **modifiable**: an armor piece carries a **capacity** budget that
**modifications** consume, and a weapon exposes **mount points** each accessory
clips onto. A mod's effect (soak, environmental protection, a to-hit steadier)
rides the normal equip pipeline while the host is worn/wielded. The verbs are
`modify` (install / show) and `unmodify` (remove); the fixer (§42) stocks the
whole catalog. Backed by `shadowrun_armor_mod_live_test.go`.

> **Fast-hazard boot (for the Glow tests below).** The environmental hazard ticks
> on `ANOTHERMUD_BIOME_HAZARD_INTERVAL` (default is slow) — add it to the §37 boot
> so the Glow bites within a beat:
> ```sh
> ANOTHERMUD_BIOME_HAZARD_INTERVAL=1s ANOTHERMUD_PACKS=shadowrun make run
> ```

### The catalog (the fixer stocks mods)

- [ ] At the fixer (§42), `list` — alongside the guns/ammo it now sells an
      **armored vest**, and the modifications: a **ballistic weave insert**, a
      **chemical protection layer**, a **radiation shielding liner**, a **chemical
      seal kit**, and a **laser sight**.
- [ ] `buy weave`, `buy seal`, `buy laser` (or `get` the armored jacket on the
      corner) — each enters inventory like any item; a mod is inert until installed.

### Armor capacity — install, inspect, remove

An armored **jacket** has capacity **12** (light — dons instantly); the heavier
**vest** has **9**. Mods consume that budget; you can't fit everything.

- [ ] `modify jacket` (host only) — shows the budget: **"An armored jacket has 12
      capacity, all free — no modifications installed."**
- [ ] `modify jacket weave` — **"You install a ballistic weave insert into an
      armored jacket. (9 capacity free.)"** — the weave (cost **3**) is consumed
      from inventory and recorded on the jacket.
- [ ] `modify jacket` again — the info form now lists it: **"An armored jacket —
      capacity 12 (3 used, 9 free): - a ballistic weave insert [3]"**.
- [ ] `look jacket` — the description gains a **"Capacity 12 (9 free). Installed:
      a ballistic weave insert."** line.
- [ ] Over-fill: try to add a mod whose cost exceeds the free budget — refused,
      **naming the shortfall** ("… needs N capacity, but an armored jacket has only
      M free."). A wrong-domain mod (a weapon accessory into armor) is refused too.
- [ ] `unmodify jacket weave` — **"You remove a ballistic weave insert from an
      armored jacket and pocket it. (12 capacity free.)"** — the mod returns to
      inventory (re-installable) and the capacity frees up.

### Weapon accessories — mount slots

A weapon exposes named **mounts** (barrel / under-barrel / side / top / stock /
internal); each holds **one** accessory. The Ares Predator V exposes
**barrel · top · under-barrel**.

- [ ] `get pistol` (or buy one), `modify pistol` — lists the mounts, each
      **(empty)**.
- [ ] `modify pistol laser` — **"You attach a laser sight to an Ares Predator V's
      top mount."** — it seats on the first free compatible mount (the laser fits
      top *or* under-barrel).
- [ ] `modify pistol` — the mount now shows its occupant: **"top: a laser sight"**.
- [ ] Occupancy: install a second accessory that only fits an already-taken mount
      — refused (**"… has no free mount that fits …"**), *not* silently double-seated.
- [ ] `unmodify pistol laser` — detaches it back to inventory, freeing the mount.

### The mod matters — ballistic soak in a fight

- [ ] `modify jacket weave` (piercing/ballistic resistance **2**), `equip jacket`,
      `teleport shadowrun:market-street`, `kill ganger` while the ganger shoots or
      the pistol trades fire — the weave **soaks 2 off each piercing hit** through
      the same `mitigation` channel armor uses (§39). Compare kill speed with the
      weave installed vs. removed.

### Environmental protection — surviving the Glow (immunity)

**Glow City** (`shadowrun:glow-city`, radiation) and the **Puyallup ash flats**
(`shadowrun:the-ash-flat`, toxins) deal intrinsic hazard damage each tick unless
you're protected. A **chemical seal kit** grants total immunity (SR sealed
environment) while the modded armor is worn.

- [ ] Baseline: `restore`, `teleport shadowrun:glow-city`, wait a couple ticks —
      **"The Glow sears through you …"** and your **HP drops** (environmental — no
      attacker). `teleport shadowrun:street-corner` back to safety.
- [ ] `equip jacket`, `modify jacket seal` (cost **6**), return to the Glow and
      dwell — **no searing line, no HP loss**. The mod-granted **`rad-shielded`**
      key confers immunity exactly as a dedicated enviro-suit's tag would.
- [ ] `unmodify jacket seal`, back to the Glow — it **bites again**. Removing the
      mod reverses the protection.

### Environmental resistance — taking the edge off (soak)

A **radiation shielding liner** doesn't seal you, it *reduces* the radiation.

- [ ] `modify jacket shielding` (radiation resistance **2**, cost **2**), into the
      Glow — you still take the searing line, but the **HP loss per tick is
      smaller** (payload minus your soak; enough shielding fully absorbs it). This
      is the partial-soak counterpart to the seal's full immunity.

### Modify worn armor (the bench isn't required)

- [ ] With the jacket **equipped**, `modify jacket weave` — installs **while worn**
      and the effect lands immediately (no take-it-off step). `unmodify` likewise
      reverses live.
- [ ] Combat gate: on **Market Street** with the ganger engaged (`kill ganger`),
      `modify jacket seal` — refused: **"You can't re-work your gear in the middle
      of a firefight."** Modding worn gear is a bench action; carried gear is always
      free to work on.

### Persistence

- [ ] Install a mod, `quit`, log back in — `modify jacket` (or `look jacket`) still
      shows it installed; a modded piece round-trips through your save. A worn
      modded piece keeps its effect on relog too.

---

## Notes / known gaps (Shadowrun)

- **This is a separate boot.** `ANOTHERMUD_PACKS=shadowrun` — the core and WoT
  guides run different worlds; a Shadowrun character can't be selected under a
  core/WoT boot (world-locking, core §1).
- **Combat happens on Market Street.** The Street Corner and Back Alley are
  safe-rooms; the ganger on Market Street is the intended target. The corp-sec
  guard on Corporate Plaza is neutral — she fights only if provoked.
- **Two monitors, one weapon choice** (§39): a weapon with no `target_pool` is
  lethal (Physical monitor → corpse); `target_pool: stun` knocks out (Stun
  monitor → no corpse). This is the SR-M2 typed-damage payoff.
- **Cosmetic/tracked gaps:** nuyen renders as "gold" (§42); the Physical monitor
  is flat, not Body-derived (§39); no Essence pool or karma ledger yet (§43).
- **Deferred combat depth:** magazine model, SMG burst, and cross-room `shoot`
  for the SR pack are recorded in the SR-M3c deferred-fixes memory.
- **Item modification (§44):** the shipped Core armor mods are the ones with live
  consumers — ballistic weave (soak), chemical protection / seal + radiation
  shielding (hazard toxin/rad), and the laser sight accessory. The rest of the
  Core set (**Fire Resistance, Insulation, Nonconductivity, Shock Frills, Thermal
  Damping**) is **not authored yet** — each needs a damage/detection mechanic that
  doesn't exist (fire/cold/electrical damage, thermographic detection). **Cyberware
  modification** (enhancements into a cybereye's capacity) reuses the same rule but
  is a later host domain. One known edge: a **hastily-donned** piece modified while
  worn loses its degradation (low exposure; recorded in the item-modification build
  log).
- Record any mismatch as a `BUG:` note next to the step; file the real ones into
  `docs/BACKLOG.md` or a `m<N>-deferred-fixes` memory afterward.
