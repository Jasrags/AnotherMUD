# AnotherMUD Playtest — Shadowrun MVP

Manual QA for the **Shadowrun** street-samurai slice (SR-M1 attribute set →
SR-M2 typed damage → SR-M3 the playable pack). This is a **separate world** on
the same engine — a different boot, a different character, its own district. The
core/starter-world guide (`core.md`) and the Wheel of Time guide (`wot.md`) are
siblings; the section numbers here (**§37–§51**) continue the guide-wide anchor
sequence.

> Format: `- [ ] command` — what should happen. Mark `[x]` on pass; add a
> `BUG:` note inline on fail.

Most boxes here are backed by a live smoke test under `cmd/telnet-smoke`
(`shadowrun_*_live_test.go`) — this is the human-facing walkthrough of the same
paths those tests drive. (The newest §38/§51 role×origin + commlink-onboarding
paths are unit-tested rather than live-smoked — this walkthrough is their first
hands-on pass.)

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

A fresh runner's starting nuyen + loadout now vary by **origin** (500 / 1,000 /
2,500 — see §38); the street corner also lays out a full sample kit on the ground.

> **For the creation + onboarding sections (§38, §51),** boot with
> `make run-shadowrun` instead — it starts at `the-flop` (Rook + Patch) and sets
> `ANOTHERMUD_COMMLINK_FIXER=shadowrun:fixer-mentor` so the first-entry commlink
> call fires. The `street-corner` boot above is the **combat/gear** boot (§39–§50).

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

## 38. Character creation — role × origin

Login is account-first (identical to core §1). Create a new account/character;
the Shadowrun wizard is the **default** flow (no channeling step). A runner is
now **metatype × role × origin** — **two roles** (Street Samurai, Face) compose
with **three origins** (Street Kid, Corporate Dropout, Ex-Security).

> **Boot for the full experience:** `make run-shadowrun` starts at `the-flop`
> (Rook the fixer + Patch the guide) and wires `ANOTHERMUD_COMMLINK_FIXER`, so a
> fresh character gets the **first-entry commlink call** (§51). The bare §37 boot
> at `street-corner` still creates characters fine, but no call arrives there
> (the fixer env isn't set).

- [x] Walk the wizard — after the name it prompts, in order: **gender**, then
      **metatype** ("Choose your race:" — Dwarf/Elf/Human/Ork/Troll), then
      **role** ("Choose your class:" — **Face** *or* **Street Samurai**), then
      **origin** ("… background:" — **Corporate Dropout**, **Ex-Security**, *or*
      **Street Kid**).
- [x] Each **origin** carries a pick-one **feat** chooser (e.g. Street Kid:
      Alertness *or* Stealthy; Ex-Security: Toughness *or* Great Fortitude);
      Street Kid also keeps its pick-one weapon **kit**.
- [x] Confirm — the character spawns at the start room (`the-flop` under
      `make run-shadowrun`).

### The role floor — always armed for your role
- [x] `i` + `equipment` — every character carries their **role's floor weapon**,
      guaranteed regardless of origin: a **Street Samurai** starts with a **stun
      baton**, a **Face** with a **Streetline Special** holdout.

### The universal commlink
- [x] `i` — every runner carries a **commlink**, its tier scaled to the origin's
      means: a **Meta Link** (Street Kid), **Renraku Sensei** (Ex-Security), or
      **Hermes Ikon** (Corporate Dropout). This is the device the fixer pings (§51).

### Papers & money by origin (the SIN spectrum)
- [x] `licenses` (aka `sin`, `credentials`) — the papers each origin carries:
      **Street Kid = none** (SINless — the defining edge *and* liability),
      **Ex-Security = a national SIN** (a firearms license),
      **Corporate Dropout = a corporate SIN** (firearms + corporate licenses).
- [x] `score` purse — nuyen by origin: **Street Kid 500 / Ex-Security 1,000 /
      Corporate Dropout 2,500** (shown as `Gold`; the nuyen skin is cosmetic).

### The score sheet
- [x] `score` (`sc`) — the identity line reads **Gender Metatype Class**, a
      **Background** line below it, the **eight SR primaries** (BOD AGI REA STR WIL
      LOG INT CHA) + **Edge** (EDG), the track **Level 1 - The Long Run** (§43),
      and **MA 0/0** (no channeling).
- [x] `skills` — a **Face** shows the social spread (Negotiation, Con,
      Intimidation) + Perception; a **Street Samurai** the weapon-skill spread
      (§50). The origin's life-skills (Street Kid: Sneaking/Perception;
      Ex-Security: Survival/First Aid) fold in — an overlap takes the higher rating.

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

### The downed foe — `rob` (non-lethal) or `finish` (coup-de-grace)

Once a foe is knocked out (**unconscious / helpless**), you choose what happens
next — the whole reason to reach for the baton over the katana:

- [ ] **`rob <foe>`** (alias `mug`) — non-lethal. Take their carried gear **and
      roll their coin purse now**, leaving them **breathing**: no "slain" line, no
      corpse. e.g. *"You loot a caseless round, an Ares Light Fire 70, an armored
      jacket and 68¥ from a Halloweener."* A second `rob` finds them **"already
      picked clean"** (one roll per foe — the single-claim guard).
- [ ] **`finish <foe>`** (alias `execute`) — coup-de-grace. A **guaranteed lethal**
      blow through the normal death pipeline: *"You deliver a killing blow…"* →
      **kill credit + karma → a corpse** to `loot` as usual. Distinct from a plain
      swing, which the stun baton would only *re-*knock-out.
- [ ] Both **refuse a conscious foe** (*"isn't helpless — put them down first"*),
      so neither is a free instakill or free steal. Try each on a live ganger for
      the refusal, then on a knocked-out one.

> The trade-off is heat: `finish` is a real kill (corpse, kill credit, and it
> draws **security heat** like any killing in a policed zone), while `rob` leaves
> the mark alive — a lower profile, but a live witness. Stun-and-rob trades
> finality for staying off Knight-Errant's board.

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
**modifications** consume, a weapon exposes **mount points** each accessory clips
onto, and a **cybereye** carries a capacity its enhancements slot into (three host
domains, one rule). A mod's effect (soak, environmental protection, a to-hit
steadier, sharper perception) rides the normal equip pipeline while the host is
worn/wielded/installed. The verbs are
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

### Cyberware clusters — enhancements into a cybereye (third host domain)

The **same capacity rule** applies to cyberware: a **cybereye** carries a capacity
budget (SR5 R1 = **4**) that **enhancements** install into — chrome you assemble
before you jack it in. Cluster enhancements are **essence-free** (the shell's
Essence covers the cluster). The fixer sells a **vision enhancement chip**.

- [ ] `spawn item cybereyes me` (or `get cybereyes` from the corner tray),
      `buy vision` (or `spawn item cybereye-vision-enhancement me`).
- [ ] `modify cybereyes vision` — **"You install a vision enhancement chip into
      cybereyes. (2 capacity free.)"** — the chip (cost **2**) drops into the eye's
      capacity of 4. `modify cybereyes` shows **capacity 4 (2 used, 2 free)**.
- [ ] `score` — note **INT**. `equip cybereyes` — INT rises by **2**: the shell's
      own **+1** *plus* the enhancement's **+1**, both applied while the eyes are
      worn. (`unequip cybereyes` drops both back.)
- [ ] Essence check: the enhancement adds **no** Essence — installing it doesn't
      move your Essence budget; only the cybereye shell's 0.2 does (as in §41).
- [ ] `unmodify cybereyes vision` while the eyes are worn — INT drops by 1 live
      (the chip pops back to inventory), same modify-worn re-apply as armor.

### Smartlink ↔ smartgun pairing (cross-domain)

The one mechanic that ties two host domains together: a **smartlink** (a cybereye
enhancement) + a **smartgun** (a weapon accessory) grant a to-hit bonus — but only
when you have **both**, and are **wielding** the smart gun. `score` shows a
**"Smartlink: active"** line when the pairing is live (default `+2` to-hit,
`ANOTHERMUD_SMARTLINK_BONUS`). The fixer sells both.

- [ ] `score` — no Smartlink line yet.
- [ ] Fit the smartlink: `spawn item cybereyes me`, `buy smartlink` (or spawn),
      `modify cybereyes smartlink` (cost 3), `equip cybereyes`. `score` — **still no
      Smartlink line** (a smartlink alone is inert; you need the gun).
- [ ] Fit the smartgun: `buy smartgun` (or spawn), `equip pistol wield`,
      `modify pistol smartgun` (onto the top/under-barrel mount). `score` — the
      **"Smartlink: active"** line now shows: both halves present + the gun wielded.
- [ ] `unequip pistol` (or `unmodify pistol smartgun`) — the line disappears; the
      bonus is gone. Fighting the ganger (§39) with the pairing active lands shots
      more reliably (the +2 rides the same to-hit seam as the darkness/armor terms).

### Persistence

- [ ] Install a mod, `quit`, log back in — `modify jacket` (or `look jacket`) still
      shows it installed; a modded piece round-trips through your save. A worn
      modded piece keeps its effect on relog too.

---

## 45. The arms bazaar — Core weapons & armor

The fixer's stock grew: a full **Core weapon line** (hold-out → heavy pistols,
revolver, SMGs, assault rifles, a sniper, a shotgun, an LMG, a stun taser, and
melee — knife/sword/axe/baton) and a **Core armor line** by tier (armored clothing,
suits, jumpsuit, chameleon suit, full body armor) plus a **helmet** and **off-hand
shields**. Armor soak sums across *every* worn slot — body, head, and off-hand.

- [ ] At the fixer (§42), `list` — the catalog now spans the gun and armor tables.
- [ ] `buy helmet`, `buy ballistic shield`, `buy full body armor`. `equip helmet`
      (head), `equip shield` (off hand), `equip full body armor` (body) — three
      pieces, three slots, one stacked soak. `score` shows the combined armor.
- [ ] Non-cyber eyewear: `buy low-light goggles`, `equip goggles` — they take the
      new **eyes** slot (no surgery, no Essence). Backed by
      `shadowrun_gear_features_live_test.go`.

## 46. Firing modes — single / burst / full-auto

An automatic weapon (SMG, assault rifle, machine pistol, LMG, the flamethrower)
supports **firing modes**. Burst and full-auto trade **ammunition and accuracy for
damage**; `firemode` picks the mode.

- [ ] Wield an automatic weapon (`buy hk-227` / `wield smg`). `firemode` — reports
      the current mode and what the weapon supports.
- [ ] `firemode burst` (3 rounds, +damage, −accuracy), `firemode auto` (6 rounds,
      more of both). Fight the ganger (§39) on each — auto chews the magazine fast
      but hits harder.
- [ ] Wield a pistol and `firemode auto` — refused; a semi-auto can't chatter.
      `firemode single` is always accepted.

### Recoil compensation

A weapon's **RC** offsets the firing-mode recoil (the accuracy loss), floored at
zero — a well-compensated gun fires burst as pure upside.

- [ ] Compare burst on an **AK-97** (rc 0 — the kick bites) vs an **Ares Alpha**
      (rc 2 — fully tames burst's −2). The Alpha's burst lands like a single shot.

## 47. Fire & the flamethrower

`fire` is a real damage type now. The **Shiawase Arms Blazer** flamethrower deals
it, feeds **fuel canisters**, and (being SA/BF/FA) composes with firing modes.

- [ ] `buy blazer`, `buy fuel canister` (a few), `wield blazer`, `firemode auto`.
      Torch the ganger (§39) — ordinary ballistic armor doesn't help him much.
- [ ] The counter: `buy fire-resistance` liner, `modify jacket fire` (§44), wear
      it. A fire-resistant target soaks the flame specifically — the right defense
      vs a flamethrower, where a plain vest isn't.
- [ ] Other energy types: **cold** and **electrical** work the same way. Buy the
      **insulation** liner (soaks cold) or the **nonconductivity** liner (soaks
      electrical), `modify jacket insulation` / `... nonconductivity`. A stun baton
      / taser now deals *electrical* — nonconductivity blunts the shock.
- [ ] Cold, offensively: `buy cryo projector`, `buy coolant canister`, `wield cryo`
      — the flamethrower's cold twin freezes the unprotected; an insulation liner
      soaks it.
- [ ] Cold, environmentally — **the Deep Freeze**: from the Edge of Glow City go
      `north` into a breached cold-storage vault (the `cryo` biome). Unshielded, the
      killing cold bites each tick (HP drops); `buy coldsuit`, `wear suit`, and the
      `cold-sealed` seal lets you loot the caskets — the cold sibling of the Glow
      (§ radiation). Backed by `cold_hazard_live_test.go`.

## 48. Armor penetration (AP)

A weapon's **AP** reduces the defender's armor soak — bypassing armor, never the
creature's toughness or a typed resistance. It applies to melee and ranged alike.

- [ ] Fight an armored foe with a low-AP weapon, then a high-AP one — a **katana**
      (ap 3) or **Ranger Arms SM-5** (ap 5) cuts through soak an SMG (ap 0) can't.
      The heavier the target's armor, the more AP matters.
- [ ] AP + fire don't stack against a fire liner: the flamethrower's AP eats the
      *ballistic* soak, but the fire-resistance liner (§47) still soaks the flame —
      the specialized defense survives penetration.
- [ ] Ammo-fed AP: `buy apds`, `reload clip apds` (or fire loose), and the **APDS**
      round adds its own penetration (SR AP -4) on top of the weapon's — the
      armor-piercing round that gets through what caseless can't.

## 49. Seeing in the dark — vision modes

Cyberware and eyewear read the dark in four ways (light-and-darkness §4,
visibility §4.3, ranged-combat §5.3):

- **Thermographic** — see by heat in total darkness (an unconditional see-in-dark).
- **Low-light** — amplify faint light to clear sight (does nothing in true black —
  it needs *some* light to boost).
- **Ultrasound** — echolocation that pierces darkness AND detects hidden/sneaking
  foes, regardless of light (a visibility sense, not a light one).
- **Vision magnification** — telescopic optics that cut the range-band to-hit
  falloff on a projectile shot (a ranged-accuracy aid, not a dark-sight one).

They are sourced two ways — a **cybereye** enhancement (`modify cybereyes <mode>`,
then wear the eyes) or worn **eyewear** (the new `eyes` slot, no surgery):

- [ ] Get the gear (Westlake no longer stocks a tray — source it from a ripperdoc
      in play, or `spawn` it for this check): `spawn item cybereyes me`, `spawn item
      cybereye-thermographic me`, then `modify cybereyes thermographic`, `equip
      cybereyes`. Or `spawn item low-light-goggles me`, `equip goggles` — the
      goggles take the eyes slot.
- [ ] The dark room: from Westlake Plaza go `down` into the **maintenance
      sublevel**. As a **human** (no racial dark vision), `look` reads *pitch black,
      you can see nothing* — name, prose, and exits all withheld. `modify cybereyes
      thermographic` + `equip cybereyes`, go back down, and the room resolves (heat
      vision floors it to gloom — shapes and the name return). Backed by
      `vision_modes_live_test.go`.

> **Metatype matters.** A **dwarf** or **troll** carries *thermographic* natively,
> so they read the sublevel without gear — thermographic cyber is redundant for
> them. An **elf** or **ork** has *low-light* (which needs some ambient, so it does
> nothing in the truly-black sublevel). A **human** has neither, so the dark room
> is where their vision gear earns its nuyen. Ultrasound and magnification show
> their effect against hidden foes / at range, not in room prose.

---

## 50. Skills — Sneaking, weapon skills & training (skills §2/§7)

SR5 resolves an action as **skill + attribute**, and this boot runs the
weapon-skill combat model: a wielded weapon binds a skill, to-hit reads that
skill's rating, and each swing trains it. SR also merges D&D's two stealth
skills into one **Sneaking** (Agility).

### The skills sheet

- [ ] `skills` on a fresh Street Samurai — a **grouped** sheet: *Combat —
      Firearms* (Pistols, Automatics, Longarms, Heavy Weapons), *Combat — Close
      Combat* (Blades, Clubs, Unarmed Combat, Throwing Weapons), *Physical —
      Stealth* (**Sneaking**), plus Perception — each with a 3-letter attribute
      tag like `(AGI)`. There is **one** Sneaking skill, not a separate Hide and
      Move Silently.

### Stealth trains Sneaking (Slice C)

- [ ] `hide` (then `unhide` and repeat) — with `ANOTHERMUD_SKILL_GAIN_NOTIFY_STEP=1`
      you see **"You feel your Sneaking improve."** Hiding AND sneaking both train
      the single Sneaking skill — the merged SR stealth skill actually feeds the
      concealment check, and no inert core `hide`/`move-silently` skill lingers on
      the sheet.

### Weapon skills — to-hit + train-on-attack (§7)

- [ ] Wield a bound weapon (the **stun baton → Clubs**, the **Predator →
      Pistols**, a **katana → Blades**) and fight the Market Street ganger — each
      swing trains the bound skill, so mid-fight you see **"You feel your Clubs
      improve."** (step 1) or the milestone line (default). A trained pistoleer
      out-hits a dabbler: to-hit reads the skill's rating, not a flat proficient/
      not flag.
- [ ] An **untrained** weapon defaults at the non-proficient penalty rather than
      being refused (every combat skill is defaultable).

### Mobs use the model too (mob weapon-skill ratings)

- [ ] The **Knight Errant officer** (Corporate Core) is *trained* — she carries
      an Automatics rating (50), so her smartlinked SMG fires on the weapon-skill
      model and she's markedly more accurate than an unrated street punk with the
      same gun. The ganger, "more enthusiasm than training," stays on the plain
      always-proficient model. (The bonus is felt in hit-rate, not printed.)

---

## 51. The commlink call, licenses & the checkpoint (role × origin payoff)

The commlink every runner carries at creation is now the **onboarding device**,
and the SIN each origin carries decides who walks through a checkpoint. **Boot
with `make run-shadowrun`** (starts at `the-flop`; sets `ANOTHERMUD_COMMLINK_FIXER=
shadowrun:fixer-mentor`). The first character of a fresh save is auto-admin, so
`teleport` works for the checkpoint hop below.

### The first-entry commlink call
- [ ] Create a **new** character (any role × origin) — the moment you land in the
      world your **commlink chimes**: a framed transmission
      (`>> Your commlink chimes — incoming transmission.`) carrying **Rook's**
      welcome + call-to-action (gear up east → take the stairs down to Westlake),
      closed by `>> Transmission ends.`.
- [ ] It fires **once** — `quit`, log back in: **no second call** (shown-once,
      persisted).
- [ ] Story-beat, not a tip: `tips off`, then create another character — the call
      **still fires** (it ignores the tips opt-out).
- [ ] Device-gate (edge check): the call only reaches a runner **carrying a
      commlink**. Every origin grants one, so it's normally satisfied — to see it
      withheld, `drop` the commlink and relog on a character who hasn't seen the
      call yet: **no chime**.

### Licenses at a checkpoint (the SIN spectrum bites)
The Corporate Enclave turnstile north of `fifth-avenue` gates on a **`corporate`**
access license; the Ares Arms clerk on `fifth-avenue` gates sales on a valid SIN
+ the right permit. `teleport shadowrun:fifth-avenue` to reach both.

- [ ] **Street Kid (SINless):** `north` toward the enclave gate — **refused** (no
      SIN to present). At the Ares Arms counter, `list` then try to `buy` a
      restricted piece — **refused** (a SINless buyer can't clear a licensed sale);
      the samurai's own stun-baton floor reads **restricted**, i.e. hot at any scan.
- [ ] **Corporate Dropout (corporate SIN):** its `corporate` permit **clears the
      enclave turnstile** (`north` succeeds) — the real papers open the strip — and
      the firearms license clears a restricted gun buy.
- [ ] **Ex-Security (national SIN, firearms only):** the licensed clerk **sells you
      restricted iron** its firearms permit covers, but the enclave turnstile
      **still refuses** it — a firearms license is not a `corporate` one. Only the
      corp origin walks the strip.

> **Known gap (deferred):** a *real* SIN is modeled today as a high-rating
> credential that can still **burn** under a hard scan — the true real-vs-fake
> distinction (a real SIN being non-burnable + traceable, feeding security heat)
> is a tracked follow-up. So a corp dropout's SIN failing an enclave scan and
> burning is possible-but-wrong-flavor for now. See the sr-role-origin-creation
> build log.

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
- **Firing modes / RC / AP / fire (§46–§48) — known limits:** firing modes'
  burst/auto consume rounds best-effort (a near-empty magazine still fires a short
  burst); **recoil compensation** comes from the weapon only — accessory RC
  (gas-vent, foregrip) is deferred; **AP** comes from the weapon AND the
  fired round (APDS grade — §48); mixed-ammo bursts ride only the first round's
  grade; **fire / cold / electrical** are
  damage types soaked by the matching liner (fire-resistance / insulation /
  nonconductivity), each with a weapon dealing it (flamethrower / cryo projector /
  shock weapons); cold also has a biome hazard (the Deep Freeze), alongside the
  radiation (Glow) and toxin (ashfall) hazards. Fire does not yet ignite / deal
  damage-over-time.
- **Deferred combat depth:** the magazine model and cross-room `shoot` for the SR
  pack are recorded in the SR-M3c deferred-fixes memory.
- **Item modification (§44):** the shipped Core armor mods are the ones with live
  consumers — ballistic weave (soak), chemical protection / seal + radiation
  shielding (hazard toxin/rad), and the laser sight accessory. The rest of the
  Core set (**Fire Resistance, Insulation, Nonconductivity, Shock Frills, Thermal
  Damping**) is **not authored yet** — each needs a damage/detection mechanic that
  doesn't exist (fire/cold/electrical damage, thermographic detection). **Cyberware
  clusters** are shipped (cybereye + vision-enhancement chip) with zero engine
  change; the **smartlink↔smartgun** pairing is shipped (score cue + to-hit bonus).
  Still open: **cyberlimbs** as a second cluster host and more enhancements (vision
  modes that need a light/visibility consumer). One known edge: a **hastily-donned** piece modified while
  worn loses its degradation (low exposure; recorded in the item-modification build
  log).
- Record any mismatch as a `BUG:` note next to the step; file the real ones into
  `docs/BACKLOG.md` or a `m<N>-deferred-fixes` memory afterward.
