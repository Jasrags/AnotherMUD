# NukeFire — Login & Character Creation Flow

Source: live telnet intake session (`game.nukefire.org`), captured by repo owner 2026-06-18
Fetched: 2026-06-18

This is a **primary capture** — the actual new-character onboarding as a player
experiences it at the connection, not a web/wiki derivation. It documents
NukeFire's intake UX: prompt order, the inline class-inspection affordance, the
progressive quality-of-life toggles, and the post-creation character menu.
Directly relevant prior art for AnotherMUD's `internal/login` + `internal/wizard`
flow (see `docs/specs/login.md`, `character-creation.md`, `character-select.md`).

---

## Flow at a glance

```
connect
  → banner + theme flavor
  → name prompt
  → name confirm (Y/N)
  → [new character] password → retype password
  → sex (M/F)
  → class select (single letter; "? <letter>" inspects a class inline)
  → display setup (Standard / Streamlined / Screen Reader)
  → navigation helpers (autoexits/autokey/autodoor) (Y/N)
  → loot helpers (autogold/autokey/autocompare) (Y/N)
  → prompt style (Simple / Helpful / Minimal)
  → beginner tips (Y/N)
  → PRESS RETURN
  → character menu (enter game / description / background / password / delete)
```

Eight intake decisions, each with a recommended default and a one-line
explanation. Four of the eight (display, navigation, loot, prompt, tips) are
**accessibility / quality-of-life** choices surfaced *up front* rather than
buried in a later `config` command — including an explicit **Screen Reader**
display mode at step one of display setup.

---

## Base classes (authoritative intake list)

The intake screen presents **13 base classes**, each with a single-letter
selector and a one-line identity tagline. This is the canonical starting set a
new player picks from (prestige classes are unlocked later via remort — see
[`classes-and-progression.md`](classes-and-progression.md)).

| Key | Class | Tagline |
|-----|-------|---------|
| C | Curist | faith, healing, divine punishment |
| I | Infiltrator | stealth, sabotage, precision kills |
| F | Fanatic | guns, zeal, battle litanies |
| V | Vagrant | street survival, knives, dirty tricks |
| B | Barbarian | raw melee power and toughness |
| L | Slinger | arcane ranged destruction |
| R | Ranger | tracking, wildcraft, survival |
| A | Assassin | stalking, burst damage, execution |
| S | Samurai | discipline, precision, martial control |
| M | Mutant | radiation, adaptation, body horror |
| K | Knight | armor, duty, resolve |
| Y | Cyborg | machine power, repair, systems |
| P | Pirate | dirty fighting, risk, plunder |

**Inline inspection:** typing `? <letter>` (e.g. `? I`) prints a longer class
blurb and **re-displays the class menu** without consuming the choice — a player
can inspect several classes before committing. Example:

```
INFILTRATOR
Stealth, sabotage, and precision kills. Infiltrators
ghost through enemy lines and strike where it hurts.
Good for tricks, precision, and tactical kills.
```

---

## Intake decisions in detail

### Display setup
| Choice | Mode | Effect |
|--------|------|--------|
| 1 | Standard | Normal room text and normal combat. |
| 2 | Streamlined | Brief rooms, compact spacing, compact combat summaries. |
| 3 | Screen Reader | Reader mode, brief rooms, compact spacing, compact combat summaries. |

### Navigation helpers (Y/N)
- **Y** — turn on `autoexits`, `autokey`, `autodoor`: see obvious exits, get help with doors/keys.
- **N** — classic manual movement and door handling.

### Loot helpers (Y/N)
- **Y** — `autogold`, `autokey`, `autocompare`: auto-loot gold, keys, and compare gear.
- **N** — looting fully manual.

### Prompt style
| Choice | Style | Shows |
|--------|-------|-------|
| 1 | Simple | HP, Mana, Move as actual numbers. |
| 2 | Helpful | HP/Mana/Move values, exits, enemy percentage, level progress. |
| 3 | Minimal | Small prompt for compact clients. |

### Beginner tips (Y/N)
- **Y** — occasional help while you learn (recommended for new players).
- **N** — no tutorial hints.

---

## Post-creation character menu

After intake (and on subsequent logins), the player lands on a numbered menu
rather than dropping straight into the world:

```
0) Exit from NukeFire.
1) Enter the game.
2) Enter description.
3) Read the background story.
4) Change password.
5) Delete this character.
```

---

## Verbatim transcript

```
 _   _       _        _____ _
| \ | |_   _| | _____|  ___(_)_ __ ___
|  \| | | | | |/ / _ \ |_  | | '__/ _ \
| |\  | |_| |   <  __/  _| | | | |  __/
|_| \_|\__,_|_|\_\___|_|   |_|_|  \___|


[WASTELAND BROADCAST] Signal acquired.  Port is hot.  Stay sharp.

       Play hard. Remort Harder.


                         Welcome to:

                         NukeFire : Beyond THUNDERDOME

                         What's your name, freejack?



Jasrags
Did I get that right, Jasrags (Y/N)? y
New character.
Give me a password:

Please retype password:

TEK ANGELES INTAKE
Choose your character's sex.

  M - Male
  F - Female

Choice: m

CHOOSE YOUR PATH
New players begin with one of the base classes below.
Type ? K or ? V to inspect one class.

  C  Curist       faith, healing, divine punishment
  I  Infiltrator  stealth, sabotage, precision kills
  F  Fanatic      guns, zeal, battle litanies
  V  Vagrant      street survival, knives, dirty tricks
  B  Barbarian    raw melee power and toughness
  L  Slinger      arcane ranged destruction
  R  Ranger       tracking, wildcraft, survival
  A  Assassin     stalking, burst damage, execution
  S  Samurai      discipline, precision, martial control
  M  Mutant       radiation, adaptation, body horror
  K  Knight       armor, duty, resolve
  Y  Cyborg       machine power, repair, systems
  P  Pirate       dirty fighting, risk, plunder

Class: ? i

INFILTRATOR
Stealth, sabotage, and precision kills. Infiltrators
ghost through enemy lines and strike where it hurts.
Good for tricks, precision, and tactical kills.

CHOOSE YOUR PATH
New players begin with one of the base classes below.
Type ? K or ? V to inspect one class.

  C  Curist       faith, healing, divine punishment
  I  Infiltrator  stealth, sabotage, precision kills
  F  Fanatic      guns, zeal, battle litanies
  V  Vagrant      street survival, knives, dirty tricks
  B  Barbarian    raw melee power and toughness
  L  Slinger      arcane ranged destruction
  R  Ranger       tracking, wildcraft, survival
  A  Assassin     stalking, burst damage, execution
  S  Samurai      discipline, precision, martial control
  M  Mutant       radiation, adaptation, body horror
  K  Knight       armor, duty, resolve
  Y  Cyborg       machine power, repair, systems
  P  Pirate       dirty fighting, risk, plunder

Class: i

NUKEFIRE DISPLAY SETUP
Choose how much text you want while you learn.

  1 - Standard
      Normal room text and normal combat.

  2 - Streamlined
      Brief rooms, compact spacing, compact combat summaries.

  3 - Screen Reader
      Reader mode, brief rooms, compact spacing, and
      compact combat summaries.

Choice: 1

Standard display selected.

WASTELAND NAVIGATION
Basic helpers can make the old roads less confusing.

  Y - Turn on autoexits, autokey, and autodoor.
      You will see obvious exits and get help with doors/keys.

  N - Keep classic manual movement and door handling.

Choice: y

Navigation helpers enabled: autoexits, autokey, autodoor.

LOOT HELPER SETUP
Small conveniences can keep your first kills cleaner.

  Y - Automatically loot gold, keys, and compare gear.

  N - Keep looting fully manual.

Choice: y

Loot helpers enabled: autogold, autokey, autocompare.

PROMPT STYLE
Choose how much status information your prompt shows.

  1 - Simple
      HP, Mana, and Move as actual numbers.

  2 - Helpful
      Actual HP/Mana/Move values, exits, enemy percentage,
      and level progress.

  3 - Minimal
      Small prompt for compact clients.

Choice: 2

Helpful prompt selected.

BEGINNER TIPS
Short tips can explain commands, gear, maps, and combat.

  Y - Show occasional help while you learn.
      Recommended for new players.

  N - No tutorial hints.

Choice: y

Beginner tips enabled.

*** PRESS RETURN:
                     _   _       _        _____ _
                    | \ | |_   _| | _____|  ___(_)_ __ ___
                    |  \| | | | | |/ / _ \ |_  | | '__/ _ \
                    | |\  | |_| |   <  __/  _| | | | |  __/
                    |_| \_|\__,_|_|\_\___|_|   |_|_|  \___|

            0) Exit from NukeFire.
            1) Enter the game.
            2) Enter description.
            3) Read the background story.
            4) Change password.
            5) Delete this character.

               Make your choice:
```

---

## Takeaways for AnotherMUD

- **QoL toggles surfaced at intake, not hidden in `config`.** NukeFire front-loads
  autoexits/autokey/autodoor, autogold/autocompare, prompt verbosity, and tutorial
  hints as first-run questions — each with a recommended default and a plain-English
  justification. Compare AnotherMUD's wizard (`internal/wizard`), which could expose
  a similar "new-player helpers" step rather than relying on post-login discovery.
- **Accessibility is a first-class intake choice.** A dedicated **Screen Reader**
  display mode (plus a "Minimal prompt for compact clients" option) is offered
  during creation, not as an afterthought — aligns with NukeFire's `sr` command
  (noted in [`overview-classes-systems.md`](overview-classes-systems.md)).
- **Inline, non-committal class inspection (`? <letter>`).** The player can read a
  longer class blurb and return to the menu without spending the choice — a nicer
  affordance than "type the class to see it, then back out."
- **Character menu gates world entry.** Like AnotherMUD's account-first roster
  (`character-select.md`), NukeFire lands the player on a numbered character menu
  (enter game / edit description / read background / change password / delete)
  rather than dropping them straight into the start room.
- **Class taglines do real work.** Each of the 13 base classes ships a 3–5 word
  identity tagline on the selection screen — cheap, high-signal onboarding copy.

### Cross-reference note
The 13 base-class names here (Curist, Infiltrator, Fanatic, Vagrant, Barbarian,
Slinger, Ranger, Assassin, Samurai, Mutant, Knight, Cyborg, Pirate) are the
**authoritative intake set**. The wiki-derived [`classes-and-progression.md`](classes-and-progression.md)
was assembled from `/wiki/<topic>` pages and may use different/overlapping names
(some of which are prestige/remort classes); treat this intake list as canonical
for the *starting* classes.
