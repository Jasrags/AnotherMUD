# AnotherMUD Playtest — Wheel of Time

Manual QA for the **Wheel of Time** content+mechanics pack — the features that
need the WoT world rather than the core/starter-world demo. The core guide
(`core.md`) and the Shadowrun guide (`shadowrun.md`) are siblings; see
[README.md](README.md) for the split and shared conventions.

This guide covers **§27** (channeling — the One Power), **§32** (masterwork item
grades), **§33** (ranged combat), **§34** (faction & standing), and **§35**
(reputation & renown). Section numbers are guide-wide anchors (they don't
renumber per file); the mechanics *not* here — login (§1), combat (§6), saving
throws (§24), feats (§31), and the rest — live in `core.md` and behave the same
in this world.

### Boot & provisioning

Most sections boot the WoT pack with a section-specific start room; each carries
its own boot block. The base boot is:

```sh
make run-wot          # ANOTHERMUD_PACKS=wot, starts at wot:the-green
telnet localhost 4000
```

The **first character of a fresh WoT save is auto-granted admin** (so `teleport`
/ `xp` / `restore` work); if you've already made WoT characters, boot with
`ANOTHERMUD_ROLE_SEED="<name>:admin"`. A WoT character is world-locked — it can't
be selected under the core boot, and vice-versa (core §1).

> **§33 note:** Ranged combat is one chapter here. Its **thrown-weapon** half
> runs on the **default** boot (`make run`, the throwing knife in Town Square) —
> it's grouped with the rest of ranged combat for coherence; the boot switch is
> called out at that subsection.

> Format: `- [ ] command` — what should happen. Mark `[x]` on pass; add a
> `BUG:` note inline on fail.

---

## 27. Channeling — the One Power (WoT pack)

> **Different world.** This section runs the **Wheel of Time** pack, not the
> core demo. Boot it on its own and make a fresh **channeler** (adminone/playerone are
> core-pack fighters and don't exist here).

A channeler draws the **One Power** (a pool, shown as **MA** on `score`) to weave
**spells** (weaves). Strength in the Five Powers is **gendered** — the
saidin/saidar split — so the same weave is stronger or weaker by gender
(affinity). Weaves take an interruptible **cast time** to channel, and a hit, a
move, or being stunned **breaks** an in-flight weave. Overdrawing the Power
(**overchannel**) risks a Fortitude-save cascade up to being **stilled**.

### Boot (WoT pack, in the Westwood, full channeling flavor)

```sh
# Start in the Westwood (a wild boar to fight), with the channeling knobs on
# and a stark affinity contrast for the demo:
ANOTHERMUD_PACKS=wot \
ANOTHERMUD_START_ROOM=wot:deep-westwood \
ANOTHERMUD_SPEND_ON_SUCCESS=true \
ANOTHERMUD_CHANNEL_RESERVE_MULTIPLE=2 \
ANOTHERMUD_AFFINITY_WEAK_FACTOR=0.25 \
go run ./cmd/anothermud
```

(Plain `make run-wot` works too — it starts at `wot:the-green` with the knobs at
their defaults; the env above just puts the boar at hand and sharpens the demo.)

### Create a channeler & the One Power pool

There are **two channeling classes** (WoT S2 Initiate/Wilder split) — both draw
the One Power and know the same starter weaves; they diverge on the **governing
stat** that deepens the pool and on **backlash resilience**:

- **Initiate** — White-Tower-trained. Pool deepens with **INT** (studied
  discipline); **weak Fortitude**, so the overchannel cascade bites harder.
- **Wilder** — self-taught. Pool deepens with **WIS** (raw instinct); **strong
  Fortitude** (and a bigger HP die), so they survive overdrawing the Power more
  often. The translation of d20's "wilders are more practiced at overchanneling."

- [ ] New name → walk the wizard: it asks **gender** (male/female) **before**
      race/class, then offers both the **Initiate** and **Wilder** classes (pick
      either; the weaves below are identical). Commit it.
- [ ] `score` (`sc`) — the identity line reads **Gender Race Class**, and the
      Combat column shows **MA  30/30** (the One Power pool — non-channelers read
      `0/0`). Strong **Will**. (A Wilder also shows strong **Fort**; an Initiate's
      is weak.)
- [ ] `channel firebolt` with no target out of combat — fizzles (it needs an
      enemy); a self weave like `channel warding` works anywhere.

### Weaving (the four starter weaves)

`channel` is an alias of `cast`. The starters: **Firebolt** (Fire, enemy,
damage), **Healing** (Water, self/ally), **Warding** (Air+Spirit, self-buff
ac/hit), **Bonds of Air** (Air, enemy, save-gated stun).

- [ ] `kill boar` to engage, then `channel firebolt boar` — after a short
      warmup you see **"You cast Firebolt on a wild boar."** + a damage line, and
      **MA drops** by the weave's cost (`score` to confirm). Idle a while → MA
      **regenerates**; `quit` + relog → the drained pool **persists**.
- [ ] `channel warding` (out of combat) — a self-buff; `score` shows **AC/hit
      rise** for its duration.

### Affinity & the Five Powers (the gender split)

Men are strong in **Earth/Fire/Spirit**, women in **Air/Water/Spirit**; a weave
woven outside your strength lands at reduced magnitude (soft, never a hard gate).
With `ANOTHERMUD_AFFINITY_WEAK_FACTOR=0.25` the contrast is stark.

- [ ] As a **man** (saidin), `channel firebolt boar` — Fire is strong, so it
      hits for full damage. As a **woman** (saidar), the same Firebolt hits for
      far less (Fire is weak) — pinned near the floor at a low weak-factor.
- [ ] Affinity also scales the **effect** path: a woman's **Warding** (Air+Spirit,
      both strong) gives the full **AC +2**; a man's gives less (Air is weak) —
      compare the `score` AC delta before/after `channel warding` by gender.

### The cast warmup & the interrupt game

A weave with a `cast_time` no longer resolves instantly — it **begins** ("You
begin to weave X…") and resolves a round or two later. While it warms up, a
**hit, a room change, or a stun** aborts it.

- [ ] `channel warding` — you see **"You begin to weave Warding…"**, then a beat
      later **"You cast Warding."** (the warmup is real — Warding takes 2 rounds).
- [ ] **Hit interrupt:** engage the boar (`kill boar`), then `channel bonds-of-air
      boar` (a 2-round weave). When a boar's blow lands during the warmup:
      **"Your weave of Bonds of Air is disrupted!"** — and **MA is not spent** (an
      interrupted weave never drew the Power; cost was tempo, not Power).
- [ ] **Move interrupt:** out of combat, `channel warding`, and once you see
      "begin to weave Warding" walk `east` before it resolves — **"Your weave of
      Warding is disrupted!"**. You can't channel and travel at once.
- [ ] **Stun interrupt:** being incapacitated mid-weave drops it (cause
      "stunned") — e.g. a foe's Bonds of Air landing on you while you weave. (A
      miss or a dodge does **not** interrupt — only a blow that lands.)

### Overchannel — drawing past the safe reserve

- [ ] Drain your pool low (weave a few times), then `overchannel firebolt boar`
      — it casts a weave you couldn't safely afford (a plain `channel` would
      fizzle "insufficient"), then forces a **Fortitude save**. On a pass: "You
      draw far more of the One Power than is safe…". On a fail, a cascade by
      margin — **fatigued** → **stunned** → **stilled** ("The Source rips away —
      and is simply GONE. You are stilled.").
- [ ] While **stilled**, `channel firebolt` — fizzles "cut off from the One
      Power" (the block lasts the effect's duration; it does **not** survive a
      relogin yet — a known gap).

> The channeling knobs default **off** (the core/fantasy packs don't channel);
> the WoT boot turns them on. Remaining One-Power depth (Initiate/Wilder split,
> taint/madness, angreal, linking, a stilling **restore** path) is tracked in the
> WoT mechanics EPIC (`docs/themes/wot-mechanics-epic.md`, S2) — not in this
> slice. The interrupt game's optional polish (a GMCP cast-bar) is also pending.

---

## 32. Masterwork item grades

Item **quality grades** (masterwork / masterpiece / power-wrought) layer a
mechanical bonus over an item through existing seams — weapon to-hit, power-wrought
damage, armor check-penalty, tool skill-check. The grade is **mechanical only**:
it is *not* printed in the item name or inventory (it's independent of the
cosmetic rarity/essence decoration). The full weapon demo lives in the **WoT
pack**.

### Core pack (the tool seam)

- [ ] The starter-world lockpick (§26) is the **tool skill-check** seam: it grants
      a flat Open Lock bonus and is **ungraded** in core. A *graded* tool would
      aid the check more — there's no graded tool in the core demo to compare, so
      this seam is proven by the §26 pick bonus + the unit tests.

### WoT pack (the weapon demo)

Boot the WoT world:

```sh
ANOTHERMUD_PACKS=wot ANOTHERMUD_START_ROOM=wot:the-forge make run
```

- [ ] In **the-forge**, a **heron-mark blade** (grade **masterwork**) is placed
      ready to claim — `get heron-mark-blade`, `equip heron-mark-blade wield`. It
      carries a **+1 to-hit** from its grade (silent — nothing marks it
      "masterwork" in the name). Fight a mob and note it lands a touch more often
      than an ungraded blade of the same type.
- [ ] **Power-wrought** is the top grade: crafting an **iron dagger** at the WoT
      forge (§22 crafting flow) rolls a quality that stamps a grade —
      *uncommon→masterwork* (+1 hit), *rare→masterpiece* (+2 hit),
      *epic→power-wrought* (+3 hit **and** +3 damage). A power-wrought blade is the
      clearest in-combat jump; it also carries the **unbreakable** flag (a forward
      hook — no durability system yet).
- [ ] (Proficiency cross-check, §6) the WoT **ashandarei** is an *exotic* weapon —
      a non-proficient wielder takes the to-hit penalty regardless of grade until a
      feat grants the kind (§31).

> Boot validation rejects unknown grade keys at load (a typo'd grade fails fast).
> No graded **armor** content ships yet — the armor check-penalty grade seam is
> unit-proven.

---

## 33. Ranged combat (thrown & projectile — Slice A)

Ranged weapons add **ammo** and **Strength rules** on the existing same-room
path (no distance/range bands yet — that's Slice B). Two classes: **thrown**
(the weapon itself is hurled, lands recoverable) and **projectile** (a bow that
consumes arrows each swing). Thrown demos in the **default boot**; the projectile
bow is in the **WoT boot**.

### Thrown weapons (`throw`) — default boot

Town Square holds a **throwing knife** (1d4, thrown).

- [ ] `get knife`, `equip knife` — it occupies the wield slot (`equipment`).
- [ ] Go to the **Meadow** (`s`, `s` from Square). `throw bandit` — "You hurl a
      throwing knife at a road bandit!"; the room sees the hurl; a hit/miss line
      follows (the same combat renderer as a melee swing), and the bandit
      **engages** you (it retaliates).
- [ ] **Full Strength on a throw (§4):** a thrown weapon adds your **full** STR
      damage bonus (unlike a bow) — a hit lands for the die + your STR bonus.
- [ ] After the throw the knife is **gone from your hand** (`equipment` shows the
      wield slot empty) and **lies in the room** — `look` shows it, `get knife`
      picks it back up (thrown weapons are recoverable).
- [ ] `throw bandit` with **nothing throwable wielded** (unequip the knife first)
      — "You aren't wielding anything you can throw."
- [ ] In **Town Square** (safe room), `throw <anyone>` — "Violence is forbidden
      here." (throw honors the same gates as `kill`).

> A *graded* thrown weapon would **shatter on impact** (destroyed, not
> recoverable — §3); no graded thrown weapon ships in core, so that path is
> unit-proven.

### Projectile weapons (bow + arrows) — WoT boot

Boot the WoT world; the forge holds the bow + ammo:

```sh
ANOTHERMUD_PACKS=wot ANOTHERMUD_START_ROOM=wot:the-forge make run
```

- [ ] `get longbow`, `equip longbow` — a **Two Rivers longbow** (`equipment`
      shows it spanning both hands — it's two-handed).
- [ ] `get arrow`, `get arrow`, `get arrow` (and `get fine-arrow` ×2) — arrows
      stack in your inventory (`i` shows `an arrow (x3)`).
- [ ] Engage a foe (find a mob, `kill <it>`) — each combat round the bow
      **consumes one arrow** per swing; `i` shows the stack shrinking. A hit/miss
      line renders like any swing.
- [ ] **Out of ammo (§3):** keep firing until the arrows run out — the swing is
      skipped with **"*click* — you are out of arrows!"** and you **stay
      engaged** (re-supply with `get arrow` and firing resumes next round, no
      re-engage).
- [ ] **Masterwork ammo (§3):** the **fine arrows** (`fine-arrow`, masterwork)
      add a to-hit bonus to the shot and are **spent on use** (gone whether they
      hit or miss). A plain arrow is also consumed per shot.
- [ ] **No positive Strength on a bow (§4):** unlike a thrown knife, a plain bow
      adds **no** positive STR damage bonus (the string does the work) — but the
      Two Rivers longbow is **Strength-rated** (`str_rating: 3`), so it adds a
      positive STR bonus **capped at +3** (a heavy warbow built to a draw).
- [ ] **Non-proficient exotic (§6 / weapon-identity):** the longbow is an
      **exotic** weapon — a fighter isn't proficient, so the to-hit takes the
      non-proficient penalty until a feat grants the kind (compare a few swings'
      hit rate against a martial weapon).

### Range bands (Slice B) — WoT boot

A fight now carries a **range band** (far → near → melee) *per pairing*, within
the one room. An archer opens at distance and looses while a melee foe closes —
the opening volley — and can **kite** by withdrawing. Bands default to melee, so
a pure melee fight is unchanged.

- [ ] Wield the **longbow** (a projectile) and `kill <a melee foe>` — the
      engagement **opens at the far band** (a melee `kill` opens at melee, as
      before). You loose from range while the foe **closes one band per round**:
      each round it can't reach you it shows "X closes on you — now at *near*
      range," then *melee*.
- [ ] **Distance falloff (§5.3):** your shots are **less accurate at far** than
      at near (the per-band to-hit penalty, `ANOTHERMUD_RANGE_FALLOFF`); compare
      hit rates as the foe closes.
- [ ] **Point-blank (§5.3):** once the foe reaches the **melee** band, your bow
      takes a **point-blank penalty** (`ANOTHERMUD_POINT_BLANK_PENALTY`) — the
      cue to switch to a melee sidearm.
- [ ] **Kiting (§5.4):** `withdraw` — "You open the distance … now at *near*
      range." Opens one band, staying in the room (distinct from `flee`, which
      leaves it). Withdraw each round a melee foe advances to keep shooting.
- [ ] `advance` — "You close on X … now at *melee* range." Closes one band; at
      melee it refuses ("You're already in melee range."). `advance`/`withdraw`
      when not fighting — "You aren't fighting anyone."
- [ ] A **melee** combatant (or you wielding a melee weapon) out of melee range
      lands **no swing** that round — it spends the round closing, exactly the
      mechanic that gives the archer its volley.

### Ranged mobs — WoT boot

A bow-wielding **mob** is the player-facing side of all this: it opens at range
and looses while *you* close.

- [ ] From the Smithy go **`east`** to the **Quarry Road** — a hostile **brigand
      archer** (wielding a Two Rivers longbow) lurks there. It aggros on sight
      and, because its weapon is a projectile, the fight **opens at the far
      band**: you see the archer loose at you while you spend the next rounds
      **auto-closing** ("you close … now at *near* range", then *melee*).
- [ ] Its shots are **less accurate while you're far** (the falloff), and once
      you reach melee it's a straight fight. Bring a bow yourself (the longbow in
      the Smithy, §33 above) to trade volleys, or `withdraw` to keep your own
      distance.
- [ ] **Kiting (the mob's AI):** as you close, the archer sometimes **opens the
      distance** back out instead of shooting ("a brigand archer opens the
      distance from you, now at *far* range") — trading a shot to stay out of
      reach. It's **probabilistic** (`ANOTHERMUD_KITE_CHANCE`, default 50%), so
      you still net-close over a few rounds rather than chasing forever; set it
      to `100` to see the archer kite every chance, or `0` to disable. Bring your
      own bow to out-shoot it, or corner it where it can't open further.

### Cross-room targeting (Model C, slice 1) — `shoot`

You can now loose a projectile into the **next room**. It is an opportunistic
**action**, not a sustained cross-room fight: you snipe through one open exit; to
keep fighting you close in. The same-room round loop is untouched.

**Walk it (WoT boot):** start in the **Smithy** (`the-forge`) — it stocks a plain
**hunting bow** (a *simple* weapon, so any class hits with it) and **arrows**, and
the hostile, *stationary* **brigand archer** waits one step **east** on the Quarry
Road. `get hunting-bow`, `get arrow` (×a few), `wield hunting-bow`, then
`shoot archer east`.

- [ ] `shoot archer east` (alias `fire`) — you see "You loose a shot to the east
      at a brigand archer"; the hit/damage line follows, and **the archer's room**
      (the Quarry Road) sees the arrow "streak in from the west" and strike. The
      general form is `shoot <mob> <direction>`.
- [ ] **Line of sight = what you could walk through:** a **closed door** on that
      exit refuses ("The way north is closed; you can't shoot through it"); a
      direction with **no exit** (or an undiscovered **hidden** exit) reads as
      "There's no way to shoot to the …"; a **pitch-black** target room refuses
      ("too dark to make out anything …").
- [ ] **Ammo** is spent exactly as same-room: each shot consumes one matching
      unit, and out of ammo gives the *click* with no shot fired.
- [ ] **No cross-room engagement:** after the shot you are **not** locked in a
      fight across the boundary — the round loop only sustains a fight within one
      room.
- [ ] **Retaliation (slice 2):** the **living mob you shoot charges into your
      room** on the next AI tick and engages — even this *stationary* archer (the
      shot made it personal, so it abandons its post). You'll see "a brigand
      archer charges in from the east!" then the fight. A clean kill provokes
      nothing; a **closed door** between you (`close <dir>`) leaves it stuck the
      other side until it gives up.

> Still deferred (the spec records it, §10): **sustained** cross-room engagement
> (the full round-loop inversion), **multi-room** line of sight and pursuit (a
> shot mob only chases a shooter in the *adjacent* room), and thrown weapons
> across a boundary (`throw` stays same-room).

---

## 34. Faction & standing (WoT pack)

Per-character **standing** with content-defined factions — the WoT pack ships
three (the **Children of the Light**, the **Friends of the Dark**, the **Queen's
Guard of Andor**). Standing rises/falls through play and gates content: mob
hostility, shop access/pricing, quest offers, and ability use. The demo spine is
the **Queen's Guard initiation** questline.

> **WoT pack + admin.** This runs the WoT world, not the core demo. The **first
> character of a fresh save is auto-granted admin**, so `teleport` works for the
> hops below; if you've already made WoT characters, boot with
> `ANOTHERMUD_ROLE_SEED="<name>:admin"`.

### Boot

```sh
ANOTHERMUD_PACKS=wot ANOTHERMUD_START_ROOM=wot:the-royal-palace make run
```

Make a fresh character (any class) — you start at the Royal Palace, where the
**Palace Guardsman** (the quest giver) stands.

### The `standing` verb

- [ ] `standing` (aliases `factions`, `reputation`) — lists all three WoT
      factions, each at the starting **Neutral (0)** for a fresh character
      (`The Children of the Light   Neutral (0)`, …).
- [ ] `score` (`sc`) on a fresh character — **no** Standing section yet (it lists
      only factions you've *touched*, so a clean sheet stays uncluttered).

### Earn path — the oath quest (faction reward)

- [ ] `accept oath-to-the-queen` — accepted (acceptance works anywhere; the giver
      need not be present). `quests` lists it active.
- [ ] `teleport wot:the-caemlyn-square` — entering completes the **visit**
      objective and the auto-grant reward fires: a completion banner. (Teleport
      emits the same move event a walk does, so the visit advances.)
- [ ] `standing` — the **Queen's Guard of Andor** now reads **Honored (700)**
      (the +700 reward; 700 is the Honored floor). `score` now has a **Standing**
      section listing it.

### Prerequisite gate (the follow-up quest)

- [ ] **Before** earning standing (a fresh character / new alt): `accept
      the-queens-trust` — refused, **"You don't meet the requirements for that
      quest."** (its prereq needs Queen's Guard ≥ 500).
- [ ] **After** the oath (standing 700): `accept the-queens-trust` — **accepted**.
      The same prereq, now satisfied.

### Ability gate (guards-bulwark)

The oath reward also **teaches** `guards-bulwark`, a Queen's Guard combat drill
gated on Queen's Guard ≥ 500 (faction §6).

- [ ] `abilities` (`abi`) — `Guard's Bulwark` is now listed (taught by the oath).
- [ ] `cast guards-bulwark` out of combat — fizzles **needing an enemy** (not
      *"You lack the standing…"*), proving the faction gate **passed** at 700.
      (Below 500 the same cast fizzles `faction_restricted` — "You lack the
      standing to use Guard's Bulwark." — observable only with negative standing.)

### Shop pricing by standing (Basel Gill, favored customer)

- [ ] `teleport wot:the-queens-blessing` — Basel Gill's inn (a faction-aware
      shop affiliated with the Queen's Guard, ally threshold 500, 15% off).
- [ ] `value fine-wine` **before** the oath (a fresh alt at Neutral) — full price
      (≈48). **After** the oath (standing 700 ≥ 500) — the **favored-customer**
      price (≈41, 15% off). `buy fine-wine` charges the discounted amount.
- [ ] Basel has **no access gate** — he serves everyone; only the price changes.

### Shop access gate (the Darkfriend fence)

- [ ] `teleport wot:the-wagon-yard` (Four Kings) — a **well-dressed merchant**, a
      Darkfriend fence. `list` shows his stock (a disguise kit, a belt knife,
      wine). At Neutral you **can** buy (his floor is 0 = "not a sworn enemy of
      the Dark").
- [ ] *(Mechanism — needs negative Darkfriend standing to observe live, which no
      content earns in v1.)* A character **hostile** to the Darkfriends (standing
      < 0) is **refused** — "The shopkeeper refuses to deal with the likes of
      you." A **friend of the Dark** (≥ 300) gets a 20% discount.

### Mob disposition by standing (the hostile case)

Non-hostile reactions (neutral/friendly/wary) aren't visibly distinct, but the
**hostile** reaction is — a faction's members attack an enemy of the faction on
sight. The observable path is the on-kill standing loss:

- [ ] *(Involved — a real fight.)* Kill a **Queen's Guardsman** (e.g. `teleport
      wot:the-bridge-foot`, then `kill guard`) — landing the kill **lowers** your
      Queen's Guard standing by 100 (to −100). Now `teleport wot:the-royal-palace`
      — the Palace Guardsman, hostile to anyone below Neutral with the Guard,
      **aggros you on sight** (its `max_standing: -1 → hostile` rule). A bigger
      character (admin `xp`/`restore`) makes the guard fight survivable.

## 35. Reputation & renown (WoT pack)

A single **renown** axis — how widely known you are (fame high, infamy low,
**Unknown** at zero) — distinct from per-faction standing (§34). It shows on
`score`, rises through deeds (quest rewards), and is shaped by three feats. Same
WoT boot as §34.

### The Renown line on `score`

- [ ] `score` (`sc`) on a fresh character — the Character column shows
      **`Renown   Unknown (0)`** (every character has a renown, like alignment).

### Earn path (quest reward)

- [ ] Do §34's **oath quest** (`accept oath-to-the-queen`, `teleport
      wot:the-caemlyn-square`). The reward also grants **+120 renown** (a public,
      witnessed oath).
- [ ] `score` — the Renown line now reads **`Known Locally (120)`** (120 crosses
      the Known-Locally threshold at 100).

### The reputation feats

A fresh character has **1 feat credit** (more every 3rd level — admin `xp` to
bank extra). The three feats live in the WoT pack.

- [ ] `feat fame` — spends a credit; `score` Renown **rises by 150** (Fame is a
      flat *effective*-renown bonus — base + Fame). The tier shifts up if the new
      effective total crosses a band.
- [ ] `feat infamy` — `score` Renown reads **`… (infamous)`** — the same
      magnitude reframed as *feared* rather than admired (the kind, not the
      strength).
- [ ] `feat low-profile` — *(mechanism)* scales **renown gains** down by
      `ANOTHERMUD_LOW_PROFILE_FACTOR` (default 0.5); losses are untouched.
      Observable only with a repeatable renown source (the oath isn't repeatable).

### Infamy & disposition

- [ ] *(Mechanism — wary isn't a visible greeting.)* With the **Infamy** feat,
      the Palace Guardsman's rules mark you **wary** (its `infamous: true → wary`
      rule, ordered after its hostile-to-lawbreakers rule). It does not aggro, so
      there's no visible cue in v1 — the reaction is exercised by the unit tests;
      a future deference/fear-flavored greeting would surface it.

> The **recognition check** primitive (renown + die vs a difficulty — "are you
> recognized here?") exists on the engine but has **no consumer verb yet**
> (deferred). The worn-signifier earn path (gear that carries renown) and a
> class-level renown bump are likewise deferred — see `docs/BACKLOG.md` /
> `reputation.md`.

