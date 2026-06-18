# WoTMUD — Hunter Class & Skills

Source: https://wotmud.info/hunter/ and https://wotmud.info/hunter-skills/ (both embed the Fandom wiki via `data-resource`; authoritative source: https://wotmud.fandom.com/wiki/Hunter and https://wotmud.fandom.com/wiki/Hunter_skill)
Fetched: 2026-06-18

---

## Class identity

> "Hunters are the independent, self-reliant sort of characters which have an
> easy time developing proficiency in navigation, survival and tracking type
> skills. Depending on stats a hunter can choose to go abs, dodge or combo and
> span the range of offensive playstyles from stab to bash to charge as well.
> Hunters with particularly well-balanced stats have been known to train and
> play one way only to later reset their pracs and train and play a completely
> opposite style. This is possible mainly because **hunters have the only
> balanced practice equations in the game**."

The balanced equation is the mechanical root of the "jack-of-all-trades"
reputation: %-gain doesn't punish any single stat, so a well-statted hunter can
re-train into the opposite build.

> Formerly known as **Rangers**; renamed during the **v4.3 update on February 8,
> 2002**.

## Stat modifiers

| Str | Int | Wil | Dex | Con |
|:---:|:---:|:---:|:---:|:---:|
| 0 | 0 | 0 | 0 | 0 |

(All-zero — the only fully neutral base class.)

## Practice costs (pracs per training session)

| Warrior skills | Hunter skills | Rogue skills |
|:---:|:---:|:---:|
| 2 | 1 | 2 |

## Class abilities (passive / unique)

| Ability | Effect |
|---------|--------|
| **Autotrack** | Unique hunter ability — automatically read tracks on foot and on horseback (rather than manually `track`), based on **Track** proficiency plus **Ride** level if mounted. |
| **Damage Bonus vs Flora/Fauna** | Extra **+50% damage multiplier** (rounded down) vs mobs of type Animal, Bird, Fish, Fowl, Horse, Snake, and Tree — applied **before** any honing bonus to the weapon. |

## Practice-percentage formula

`(Str + Int + Wil + Dex) / 4` % (rounded down) — the only balanced equation in
the game.

---

## Hunter skills

| Skill | Effect |
|-------|--------|
| **Search** | Find hidden items or doors. |
| **Track** | Track the direction and timing of player/mob movement. Hunters get the bonus **Autotrack** option. |
| **Ride** | Ride and lead mounts (horses, mules, cows, oxen). |
| **Wisdom Lore** | **Treat** incapacitated players/mobs, and **mix** various potions. |
| **Swim** | Traverse a water room without drowning. Drown chance derives from current worn+carried weight, character stats, swim-room difficulty (unconfirmed), and swim % practiced — and swim-rooms remain drowning hazards. |
| **Notice** | See hidden things in a room **on entry without searching** — hidden items, hidden exits, and **importantly hidden characters**. Since a stab relies on being hidden, notice is a key PK survival skill. **Moving while using notice costs more movement points** than normal — an interesting tradeoff during heavy mobile PK. |
| **Camouflage** | **Deprecated — should no longer be practiced.** Previously identical to hide but only worked outdoors in the wilderness; hide now covers everything, and new rooms no longer accept the camouflage command. |
| **Ranger Sneak** ("rsneak") | The opposite of sneak: prevents others from noticing a character's **exit** from a room, reduces track information left behind, and makes the rsneaker's exit appear **older** on the track list. |
| **Cover Tracks** | Obscures/confuses the track info in a room (but not the tracks you leave when you depart). Works with ranger sneak to make area-tracking difficult. **Manual** usage — not passive. |
| **Survival** | Makes everything the character does to survive more efficient. Trained in **percentages** but **increases in levels** (like ride). Higher levels grant: butcher slain animals for food (and get more from them), go longer without food/water, and **move more efficiently (reducing the movement-point cost of walking)**. |

## Prac costs for hunter skills, by class

| Warrior | Hunter | Rogue | M. Channeler | F. Channeler | Myrddraal |
|:---:|:---:|:---:|:---:|:---:|:---:|
| 2 | 1 | 2 | 2 | 2 | 1 |
