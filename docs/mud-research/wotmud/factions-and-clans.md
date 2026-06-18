# WoTMUD — Factions and Clans

Source:
- https://wotmud.info/factions/
- https://wotmud.info/clans/ (+ /light-side-clans/, /dark-side-clans-wheel-of-time-game/, /seanchan-clans/)
- Backing data: https://wotmud.fandom.com/wiki/Clan (+ Light/Dark/Seanchan Side Clans)

Fetched: 2026-06-18

> Sourcing note: the wotmud.info clan pages are JS-rendered and embed the WoTMUD
> Fandom wiki (`data-resource="https://wotmud.fandom.com/wiki/Clan"`). Verbatim
> clan names, the rank ladder, and structure below come from the wiki via its
> MediaWiki API; the factions narrative is the server-rendered wotmud.info text.

---

## 1. The three-way racewar

WoTMUD is a **racewar MUD**: every player picks one of **three sides ("factions")** —
the **Light**, the **Dark**, and the **Seanchan**. The racewar produces faction
"heroes": **Ta'veren** (Light), **Reavers** (Dark), and **Sei'taer** (Seanchan).

| Faction | Size | Posture | Notes |
|---------|------|---------|-------|
| **Lightside** | Largest (most territory + clans) | Defensive/diverse | Recommended starting side; most clans, most playstyle options, most dedicated roleplayers |
| **Darkside** | Minority, oldest of the two | PK-dominance | Aggressive player-killing "in the name of the Dark One"; clear military structure |
| **Seanchan** | Minority | Invasion | Serves the Empress; homeland continent + dynamically controlled areas; the Ever Victorious Army |

The Light "deals primarily with the series" and is the new-player default. The
Seanchan and Dark are "geared more towards experienced players."

### Darkside progression path
All Dark players **start as a Trolloc** and join the horde as one of five trolloc
clans (**Ghar'ghael, Dha'vol, Dhai'mon, Ko'bal, Ahf'frait**). After proving
themselves to the Great Lord, they can opt to be **reborn (remort)** as a
**Dreadlord** or a **Fade (Myrddraal)**.

### Seanchan roles
Deathwatch assassins, **sul'dam/damane** (channeler pairs), and **beastmasters of
raken and torm**, among others, all inside the **Ever Victorious Army**. Features
the homeland continent plus dynamically controlled areas.

### Darkfriends (cross-faction infiltrators)
"Friends of the Dark" secretly hide **amongst the Lightside and Seanchan factions**,
serving the Father of Lies. They gather rewards while progressing toward Dreadlord
or Fade, performing tasks (whispering in a commander's ear, poisoning water
sources, etc.). "They could be anyone."

### Clanwar system
Most clans are Lightside but "many have strong beliefs that oppose those of other
clans." WoTMUD has a **clanwar system with clearly delineated winners for each
cycle**; at cycle end, **treaties can be negotiated or war re-declared.**

---

## 2. Clan structure (the rank ladder)

Clans are groups of players with shared goals. Most Lightside clans are the
**justice entity of a nation** (Andoran Lion Wardens, Tairen Defenders of the Stone,
Amadoran Children of the Light, Shienaran Lancers); others share an RP purpose
(Dragonsworn, White Tower, Gleemen, Illuminators, Wisdoms) without being a nation's
justice arm.

Joining methods vary by clan: some just need a conversation, many require an
**in-character letter**, others are **invitation-only**. The two Darkside remort
clans use **automatic placement**.

Rank is a quest-point (qp) ladder:

| Rank | QPs required |
|------|--------------|
| 1 | 0 |
| 2 | 10 |
| 3 | 30 |
| 4 | 75 |
| 5 | 150 |
| 6 | 400 |
| 7 | 1,000 |
| 8 | 3,000 |
| 9 | 10,000 |

A player of **Rank 7 or above is a "master"** with associated benefits. Masters and
the **clan council** are typically the clan's leaders.

---

## 3. Full in-game clan list (verbatim)

From the wiki's "Clan List" (the in-game `clans` output):

```
Ahf'frait              Dha'vol                Dhai'mon
Ghar'ghael             Ko'bal                 Red Ajah
Green Ajah             Yellow Ajah            Blue Ajah
Gray Ajah              Brown Ajah             White Ajah
Dragonsworn            Wolfbrother            White Tower
Gaidin                 Child of Light         Hand of Light
Legion of Unity        Black Talon            Kandori Merchants' Guild
Kin                    Forrester              Lion Warden
Iron Fist              Wisdom                 Watchers
Gleeman                Illian Companion       Deathwatch
Morat'raken            Morat'torm             Thiefbane
Shienaran Lancer       Red Eagle              Myrddraal
Defender of the Stone  Saldaean Cavalry       Rising Sun
Civil Watch            Sword and Hand         Valon Guard
Dreadguard             Winged Guard           Illuminator
Wall Guard             Imperial Army          Seanchan Imperial Guard
Mandarb a'Shar         White Leopards
```

---

## 4. Light Side clans

Lightside has the most clans. "Aligned differently through purpose and politics,
they stand against the Dark One and the Shadow." Traditionally human characters.

### General
Children of the Light · Civil Watch · Defenders of the Stone · Dragonsworn ·
Forresters · Gleemen · Hand of the Light · Illian Companions · Illuminators ·
Legion of Unity · Lion Warden · Red Eagles · Rising Sun · Saldaean Cavalry ·
Shienaran Lancers · Sword and Hand · Thiefbane · Wall Guard · Winged Guard ·
Wisdoms · Charging Boars

### Tower
- **Novices and Accepted:** White Tower
- **Aes Sedai (the seven Ajahs):** Blue Ajah · Brown Ajah · Gray Ajah · Green Ajah ·
  Red Ajah · White Ajah · Yellow Ajah
- **Other:** Gaidin (Warders) · Valon Guard

### Other
Black Talon · Iron Fist · Kin · Wolfbrother

---

## 5. Dark Side clans

"The hordes that do the bidding of their Great Lord" — the trolloc tribes plus the
mighty Chosen, carrying death and destruction to the Light.

### Trolloc clans (entry-level, each with a role)
| Clan | Role |
|------|------|
| **Ghar'ghael** | The warriors — the Blight's tanky defense |
| **Ko'bal** | The assassins — the Great Lord's assassins |
| **Dha'vol** | The scouts |
| **Dhai'mon** | The patient |
| **Ahf'frait** | The guardians |

(The dark-side page tagline: "Ghar'ghael, the warriors. Ko'bal, the assassins.
Dha'vol, the scouts. Dhai'mon, the patient. Ahf'frait, the guardians. Which band
are you?")

### Remort clans
- ~~Chosen~~ (struck through on the wiki list — the Chosen sit atop the Darkside
  hierarchy and contain both myrddraal and dreadlords, but are "not specifically a
  clan in the sense of the normal definition")
- **Dreadguard**
- **Myrddraal**

The remort clans contain all the myrddraal and dreadlords; placement into the two
Darkside remort clans is automatic.

---

## 6. Seanchan side clans

"The forces of the Seanchan race … uphold the laws of the Crystal Throne as well as
usher in the Corenne." Made up of Seanchan citizens, each clan is a part of the
**Ever Victorious Army** filling a different role.

| Clan | Role |
|------|------|
| **Deathwatch Guard** | Defenders of the Blood |
| **Imperial Army** | — |
| **Morat'raken** | The scouting arm (raken flyers) |
| **Morat'torm** | Beastmaster trainers (torm) |

(The in-game list also surfaces Seanchan Imperial Guard and Imperial Army; the
"Damane" and "Sul'dam" testing clans are referenced in the Seanchan story log.)

---

## 7. Takeaways for AnotherMUD (WoT EPIC, faction model)

- **Three-way racewar with a clear progression gate.** Dark players *start* as a
  generic Trolloc and *remort* into the elite Fade/Dreadlord forms — a built-in
  prestige path, not a class pick at creation.
- **Clans = quest-point rank ladder + a justice/RP role per nation.** Rank 7
  ("master") is the social authority tier. Worth mirroring if AnotherMUD wants
  faction-internal hierarchy without bespoke admin.
- **Darkfriends are a cross-faction hidden role** — Light/Seanchan characters who
  are secretly Dark. This is a visibility/concealment mechanic at the *faction*
  layer, complementary to our hide/sneak/invis work (M28).
- **Clanwar cycles with negotiated treaties** is a structured, resettable PvP
  meta-layer (winners per cycle, then treaty-or-war) rather than open free-for-all.
