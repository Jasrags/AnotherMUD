# AnotherMUD Playtest Guide

A manual QA checklist for verifying implemented features (M0–M28 + recent
polish: the look/consider appearance lens, tab-completion surfaces, weapon
damage dice + critical hits, **light & darkness** — §21, **crafting &
cooking** — §22, **player maps** — §23, **saving throws** — §24, **conditions** — §25,
**skills / lockpicking** — §26, **channeling** — §27, **movement cost &
encumbrance** — §28, **gathering** — §29, **visibility & hidden exits** — §30,
**feats** — §31, **masterwork item grades** — §32, and **ranged combat (thrown &
projectile)** — §33). Work top-to-bottom or jump to a section. Each step gives a
**command** and the **expected behavior**; tick the box when it matches, and note
anything that doesn't.

> **§27 (Channeling — the One Power)** is a **separate pack**: it runs the
> **Wheel of Time** world (`make run-wot`), not the core/starter-world demo the
> rest of this guide assumes. The `admin1`/`player1` characters you make below are
> core-pack fighters — make a fresh **channeler** in the WoT boot. See §27's own
> boot block.

> Format: `- [ ] command` — what should happen. Mark `[x]` on pass; add a
> `BUG:` note inline on fail.

---

## 0. Setup

### Build & run

```sh
make run          # or: go run ./cmd/anothermud   (telnet on :4000)
telnet localhost 4000
```

### Live reload while bug-hunting (recommended)

`make watch` rebuilds + restarts the server automatically on any
`.go`/`.yaml`/`.lua` save (~1s), so you don't manually stop/start as you fix
bugs. Connections drop on restart but **player saves persist** — just
reconnect and resume. (Pack Lua can also be hot-reloaded in-session via the
admin `reload` verb, no restart.) One-time install:
`go install github.com/air-verse/air@latest`. Exported env (below) is inherited,
so e.g.: `ANOTHERMUD_CORPSE_LIFETIME=30s make watch`.

### Fast-testing env (optional but recommended)

Some features are time-driven. Launch with shorter timers so you don't wait:

```sh
ANOTHERMUD_CORPSE_LIFETIME=30s \
ANOTHERMUD_CORPSE_OWNERSHIP_WINDOW=20s \
ANOTHERMUD_LINKDEAD_TIMEOUT=20s \
ANOTHERMUD_IDLE_SWEEP_INTERVAL=10s \
ANOTHERMUD_FORAGE_COOLDOWN=5s \
ANOTHERMUD_WS_ADDR=:4001 \
go run ./cmd/anothermud
```

(WebSocket on `:4001` is only needed for §17. `FORAGE_COOLDOWN` helps §29.)

### Provision your test characters (self-serve)

The guide is **self-provisioning** — it does not assume any pre-built saves.
Boot the server with an **admin seed** so the first character you make is an
admin (`roles-and-permissions` §5; the seed grants the role by character name
when that character logs in):

```sh
ANOTHERMUD_ROLE_SEED="admin1:admin" make run
```

Then create three fresh **fighters** (see §1 login + §2 creation):

| Char | Boot as | Role | Use for |
|---|---|---|---|
| **admin1** | seeded `admin1:admin` | **admin** | most testing + all admin verbs |
| **player1** | normal | player | 2nd player for social/combat/trade (§11) |
| **player2** | normal | player | 3rd party where a test needs one |

You can put all three under **one account** (the roster holds many characters —
§1) or separate accounts; for the multi-session tests (§11) log two in at once
in two telnet windows. A fresh fighter starts at **Town Square** at level 1 with
its kit in **inventory** — `equip sword` and `equip cap` first thing.

**Bootstrap `admin1` to a useful state** (it starts level 1). As `admin1`, the
admin verbs (§17) let you skip the grind:

- `xp 5000` — level up (a fighter gains HP + STR per level; repeat to taste).
- `set stat str admin1 16` etc. — raise an attribute directly.
- `restore` — refill vitals + sustenance.
- For gold, sell starter kit or use shops (§9); there is no direct "give gold"
  admin verb, so buy/sell to seed a balance if a test needs one.

A fresh fighter already knows `kick`, `heal`, `bless`, `parry`, `trip`, `bash`,
and `open-lock` at level 1 (the class path); `slash`/`parry` deepen at the
trainer (Maerys, §8). So most `cast …` steps work on a brand-new fighter — only
`slash` needs a trainer visit first.

### The world (core / starter-world pack)

```
   Forge Nook  ·· (hidden: `search` in the Forge — §30)
        |
   Hearthwick Forge (Maerys trainer; Brandr blacksmith)   [indoors, lit]
        |   down: oak door (plain)
   Forge Cellar (iron key; pine torch)                     [underground, dim]
        |   down: iron door (locked / pickable — §26)
   Forge Vault (coins)                                      [underground, black]

   Forge
     |
   Market Row — Town Square — Village Gate
   (Marta cook)   (safe hub)        |
                                  Long-Grass Meadow   (grassland · bandit arena)
                                     |  e
                                  The Forest's Edge   (forest · forage)
                                   /  |  \
                              (meadow) e  s
                                      |   Cave Mouth (cave) — down → Old Diggings (cave)
                                  Deep Forest (forest · forage)
                                      |  s
                                  Rocky Foothills (mountain)
                                      |  w
                                  Cave Mouth → down → Old Diggings
```

- **Town Square** is a `safe-room` — combat is blocked here. The **Meadow**
  (south through the Gate) is the one place you can fight: a hostile **road
  bandit** spawns there with loot + coins.
- The **wilderness loop** east/south of the Meadow (Forest's Edge → Deep Forest
  → Foothills → Cave Mouth → Old Diggings) is the showcase for **biome-weighted
  movement cost** (§28) and **gathering** (§29): grassland/forest/mountain/cave
  cost different amounts to cross and carry different forage.
- Below the Forge is a **door test branch** (§12): `down` through a plain
  oak door to the **Forge Cellar**, then `down` through a *locked* iron
  door (key in the cellar) to the **Forge Vault**.
- **Light (§21):** the Forge is pinned **lit** (forge fire); the Cellar is
  pinned **dim** (a wall lamp) and holds a **pine torch**; the Vault has **no
  light override**, so it is **pitch black** — bring the torch (or play a
  dwarf, who has darkvision). Outdoor rooms cycle **lit → dim → gloom** with
  the day/night clock (one in-game hour per real minute by default; night is
  20:00–04:59); indoor rooms cap at dim.

---

## 1. Connection & login (account-first + roster)

Login is **account-first**: you authenticate an **account username** (not a
character name, and **email is no longer asked**), then pick a character from the
account's **roster** or create a new one. One account can hold many characters.

### New account + first character

- [ ] `telnet localhost 4000` — "Welcome to AnotherMUD." then "Account
      username:".
- [ ] Enter a brand-new username (e.g. `admin1`) — "No account named "admin1"
      exists. Creating it." then "Choose a password:" → "Confirm password:".
- [ ] Mismatch the confirmation — "Passwords did not match…" and it bounces back
      to the username prompt (no account created).
- [ ] A too-short password (< 6) — "Passwords must be at least 6 characters."
- [ ] A bad username (spaces/symbols, or < 3 / > 32 chars) — "Usernames are 3-32
      characters: letters, digits, or underscore."
- [ ] On a matching password, the account is created and the roster is **empty**
      — it drops you straight into character creation (§2).

### Existing account + roster

- [ ] Reconnect, enter `admin1`, then the password — "Your characters:" lists
      your characters, one per numbered line with its `[world]`, plus a final
      "create a new character" entry; the prompt is "Select a character (number
      or name), or 'n' to create:".
- [ ] Select a character by **number** or by **name** — you land in its room with
      the room description, exits, and a "You see here:" line.
- [ ] Enter a wrong password — rejected; after several failures the connection
      closes (not crashed).
- [ ] Pick an out-of-list number/name — "No such character. Pick a number from
      the list, or 'n' to create." (re-prompted).
- [ ] `n` at the roster — starts a fresh character (§2) under the same account.
- [ ] `quit` — "Goodbye." and the connection closes; reconnecting, authenticating,
      and selecting the same character puts you back where you left it.

> **World-locking (character-identity):** a character whose world isn't running
> on this server shows in the roster marked **"(unavailable on this server)"** and
> can't be selected — picking it replies "…belongs to the "<world>" world, which
> is not running on this server." (Make a core character on the default boot and a
> WoT channeler on the `wot` boot — §27 — to see one greyed out under the other.)

## 2. Character creation (new character)

Reached by creating a new account or choosing `n` at the roster (§1). The wizard
asks for a **character name** (letters-only, 2–16 chars — a digit/symbol is
rejected with "Names must use ASCII letters only."), then walks the choice steps.

- [ ] Walk the wizard — after "Time to create your character." it prompts, in
      order:
      **gender** ("Choose your gender:" — 1) Male 2) Female),
      then **race** ("Choose your race:" — 1) Dwarf 2) Human, each with a
      one-line description),
      then **class** ("Choose your class:" — 1) Fighter),
      then **background** ("Choose your background:" — 1) Commoner).
      Each choice accepts the **number** or a **name prefix** (`d`/`dw` → Dwarf).
- [ ] Final step: "Create this character? (yes/no)" — answer `no` and it restarts
      ("All right, let's start over."); answer `yes` and the character commits.
- [ ] It spawns at **Town Square**, and `score` shows the chosen
      **Gender Race Class** identity line plus the background.
- [ ] `quit` + relog (authenticate, pick it from the roster) — the character
      persisted.

## 3. Movement & rooms

- [x] `look` — renders room name, description, exits, and entities present.
- [x] `north` / `n` — move to the Forge; `look` shows Maerys the Training
      Master.
- [ ] `look maerys` (`look master`) at the Forge — her **description** prints
      (the appearance lens: broad-shouldered woman, scarred forearms…).
      Looking *at a creature* works now — it used to say "you don't see that."
- [x] `south`, `east`, `west` — move between Forge/Square/Market/Gate.
- [x] `s` from the Gate — you reach the Meadow (the bandit is here).
- [x] Try a direction with no exit (e.g. `up`) — "You cannot go that way."
- [x] **Chaining:** `n;s` runs both in order (you go north then back south).
      A long chain past the cap (default 10, `ANOTHERMUD_CHAIN_CAP`) silently
      drops the trailing commands.
- [x] **Repeat:** `2n` then `2s` walks two rooms each way (`<count><verb>`);
      `3` alone is just an unknown verb (pure digits don't expand). Commands
      run immediately, not paced across ticks.
- [x] When another player is present, you see "X arrives" / "X leaves" as they
      move (covered in §11).
- [ ] **Wilderness loop:** from the Meadow, `e` to the Forest's Edge, then `e`
      Deep Forest, `s` Foothills, `w` Cave Mouth, `down` to the Old Diggings —
      this loop drives **movement cost** (§28) and **gathering** (§29). Each step
      spends **movement points** (see §28); `score` shows your `MV` pool.

## 4. Items & inventory

In Town Square:

- [x] `get sword` — picks up the short sword; `inventory` (`i`) lists it.
- [x] `get cap`, `get ration`, `get sack`, `get waterskin` — all enter inventory.
- [x] `get coins` — credits **gold** (currency auto-convert), does *not* add an
      item; `gold` shows the new balance.
- [x] `drop ration` then `get ration` — drops to the room and picks back up.
- [x] `equip sword`, `equip cap` — `equipment` (`eq`) shows them worn; `score`
      reflects the stat/AC change (`consider` no longer reports your *own* stats —
      it points you to `score`; see §6/§8).
- [x] `look sword` — the sword's **description** prints (the appearance lens),
      not just its name. An item with no authored description reads
      "You see nothing special about …".
- [x] `unequip sword` — returns to inventory.
- [x] Duplicate items show stacked: pick up two rations → `i` shows
      `a trail ration (x2)`.
- [x] `get 2.ration` style ordinals resolve the Nth match (try with two of a
      kind on the ground).

**Equipment slots (M25 — footprint & contention).** Town Square holds an
**iron greatsword** (two-handed) and a **wooden shield** for this demo:

- [x] `get greatsword`, `equip greatsword` — `equipment` / `score` shows it in
      **both** `wield` and `offhand` (a two-hander's footprint spans both hands).
- [x] `get shield`, `equip shield` — it needs the off hand, so it **displaces**
      the greatsword (auto-swap back to inventory); `equipment` now shows the
      shield in `offhand` and an empty `wield`.
- [x] `equip greatsword` again — it displaces the shield (reclaims both hands).
- [x] `equip greatsword head` (an ineligible slot) — refused (eligibility: a
      greatsword only fits `wield`).

## 5. Containers

- [x] `fill waterskin from well` (in Town Square) — fills it from the well. The
      source is required (`fill <target> from <source>`); both args tab-complete
      (inventory for the vessel, room items for the source).
- [x] `put sword in sack` — sword moves into the sack.
- [x] `look in sack` — lists the sack's contents.
- [x] `get sword from sack` — takes it back out.
- [ ] (Corpse-as-container is covered in §7.)

## 6. Combat

Go to the **Meadow** (`s` from the Gate). The bandit is hostile.

- [ ] `consider bandit` (`con bandit`) — a **qualitative** size-up: a condition
      word (uninjured → dead) plus a relative-threat read ("an even fight",
      "you wouldn't stand a chance"). **No raw HP/AC numbers** — those moved to
      `score`. (`look bandit` is the separate appearance lens — its description,
      no mechanics.)
- [x] Entering the Meadow, the bandit aggros (attacks you) — combat rounds tick;
      you see hit/miss/damage lines for both sides.
- [x] **Weapon dice (§4.5):** wielding the short sword (vs. unarmed) raises your
      hit damage — the sword rolls **1d6** instead of the unarmed **1d3**, and
      its `str`/`hit_mod` modifiers further lift damage and accuracy. Compare a
      few swings with the sword equipped vs. `unequip sword`. (The hit line shows
      the damage *number*, not the weapon name.)
- [x] **Critical hits (§4.5):** fight several rounds — an occasional swing prints
      "**A critical hit!**" and lands for noticeably more, because a natural 20
      multiplies the rolled dice (default ×2; tune with
      `ANOTHERMUD_CRIT_MULTIPLIER`, where `1` disables the bonus).
- [x] **Mob weapon (§3.3 / §4.5):** the bandit spawns wielding a **rusty dagger**
      — it rolls 1d4 (not unarmed 1d3) and the dagger's modifiers buff it, so its
      hits land a touch harder than a bare-fisted mob. (Verified on its corpse in
      §7.)
- [x] `kill bandit` also initiates combat if not already engaged.
- [x] In **Town Square**, `kill <anyone>` is refused — "safe room"
      (combat blocked in the hub).
- [x] `flee` — escapes combat to an adjacent room (when there's an exit); you
      see the new room rendered, and others see "X flees to the <dir>!".
- [x] `wimpy 30` then fight — at ≤30% HP you auto-flee.
- [x] Let the bandit kill a low-HP character (use player1 unarmed) — on death you
      respawn (healed) at the recall/start room, not disconnected.
- [x] `cast kick` in combat — the ability fires (a fresh fighter has `kick`,
      `bash`, `trip`; `slash` is learned at Maerys first — `practice slash`, §8).
- [ ] **Weapon proficiency (weapon-identity §3):** a fighter is proficient with
      **simple + martial** weapons, so the short sword carries no penalty. Wield
      an **exotic** weapon a fighter isn't proficient with (none ship in the core
      demo — see the WoT ashandarei in §32's boot) and the to-hit takes the
      non-proficient penalty until a feat grants the kind (§31).

## 7. Loot & corpses

After killing the bandit (Meadow):

- [x] A **corpse** appears in the room ("the corpse of a road bandit").
- [x] **Equipped gear drops (§3.3):** `look corpse` shows a **rusty dagger** among
      the contents — the weapon the bandit was wielding (equipped at spawn) is
      carried with it and drops on death alongside the rolled loot. `get dagger
      from corpse` takes it; `equip dagger` then works (it's a real `wield` item).
- [x] `look corpse` — lists its contents + a coin amount.
- [x] `get ration from corpse` — takes one item out.
- [x] `get coins from corpse` — credits gold (not inventory).
- [x] `put sword in corpse` — refused (a corpse is a loot source, not storage).
- [x] `loot corpse` (or just `loot`) — takes everything remaining; the corpse
      is removed once empty.
- [x] `autoloot on`, kill the bandit again — its loot is taken automatically at
      death ("You quickly loot…"); `autoloot off` restores manual looting.
- [x] Ownership window: with player1 also present, player1 looting *your* fresh kill is
      refused during the window, then allowed after it elapses
      (`CORPSE_OWNERSHIP_WINDOW`).
- [x] Decay: kill the bandit, leave the corpse; after `CORPSE_LIFETIME` it
      vanishes (its unlooted contents destroyed).

## 8. Progression & abilities

- [x] `score` (`sc`) — your character sheet: race/class/level, HP/MA/MV, the six
      attributes, AC/hit, **saving throws** (Fort/Reflex/Will — §24), alignment,
      gold, sustenance tier, and XP-to-next.
- [x] `consider` with no target (or `consider me`) — points you to `score` now
      (self stats moved there); `consider <target>` still sizes up that target.
- [x] `abilities` (`abi`) — lists learned abilities + proficiencies.
- [ ] `train str` — spends a train credit, raises STR (a fresh fighter has
      starting trains; level-ups grant 5 more each).
- [ ] At the Forge, `practice slash` (Maerys teaches slash/parry) — raises the
      ability's cap.
- [ ] `cast bless` / `cast heal` — self buff/heal (a fresh fighter has both);
      resolves on the next pulse whether in combat or idle (out-of-combat drain).
      bless bumps AC/hit (check `score`); heal restores HP (only visible if
      injured — take a hit first).
- [ ] (Admin) `xp 500` — grants XP; crossing a threshold levels you up with a
      level-up message. On a fresh level-1 fighter the first few grants level you
      quickly (HP + STR climb each level — this is how you bootstrap `admin1`).

## 9. Economy & survival

In **Market Row** (merchant):

- [x] `list` — shows the merchant's wares (healing draught, leather cap).
- [x] `buy healing draught` — gold decreases, item enters inventory.
- [x] `value cap` — shows what the shop would pay.
- [x] `sell cap` — gold increases, item leaves inventory.
- [x] `gold` — balance reflects the trades.
- [x] `eat ration` / `drink waterskin` — sustenance restores (watch over time;
      sustenance slowly drains). Food and drink fill **one shared sustenance
      pool** (0–100; hunger/thirst aren't separate yet — see BACKLOG §2). Drain
      defaults to −1 every 30s; to slow it for testing launch with
      `ANOTHERMUD_SUSTENANCE_DRAIN_INTERVAL=5m` (or bump `_DRAIN_AMOUNT`).
- [x] `use healing draught` (or `drink`) — consumable applies its effect.
- [x] `rest` then `sleep` then `wake` (`stand`) — rest states change; HP/vitals
      regen faster while resting (Town Square has a small regen bonus).

## 10. Quests

Quest giver is **Maerys** (the training master) in the Forge. She offers
two quests — **Forge Errand** (auto-grant) and **Gate Patrol** (turn-in)
— so you can exercise both reward styles plus the new progress messaging.

### Discover & accept (the `talk` verb)

- [x] At the Forge, `talk master` (`ask master`) — Maerys lists her offers
      (**Forge Errand**, **Gate Patrol**), each with its pitch and an
      `accept <name>` line. This is how you discover a quest without already
      knowing its name.
- [x] **`accept` completes the offers:** at the Forge, `accept ` + **TAB** (raw
      telnet) — or `suggest accept ` — lists Maerys's quests by their bare id
      (`forge-errand`, `gate-patrol`); `accept ga` + TAB → `accept gate-patrol`.
      No more typing the full multi-word name. Completion only lists quests
      offered by a giver *in the room*, so it's empty away from Maerys.
- [x] `quests` (`journal`) before accepting — "no active quests."

### Auto-grant quest — Forge Errand (reward on the spot)

- [x] `accept Forge Errand` — acceptance banner; `quests` lists it active.
- [x] (If you already hold a trail ration, `drop ration` first.) `get ration`
      in Town Square — you see a progress line, then the quest **completes
      immediately**: a "Quest complete: Forge Errand" banner listing 25
      experience + 5 gold. No return trip. `gold` reflects the payout.
- [x] Repeatable: `accept Forge Errand` again works.

### Turn-in quest — Gate Patrol (claim at the giver)

- [ ] `accept Gate Patrol` (or `accept gate-patrol`).
- [ ] `s` to the **Village Gate** — a progress line for the *visit*
      objective, then a stage-advance line announcing the next stage.
- [ ] `s` to the **Meadow**, `kill bandit` — a kill progress line, then
      "**Gate Patrol complete!** Return to a training master to claim your
      reward." — note the reward is **withheld** (check `gold`: unchanged).
- [ ] Return to the Forge (`n`, `n`, `n`) and `talk master` — *now* the
      reward is handed over: completion banner with 100 experience + 25 gold
      + a healing draught. `quests` no longer lists it.

### Abandon

- [x] Accept either quest, then `abandon <name>` — it drops from the journal.
- [x] **`abandon` completes your active quests:** with a quest active, `abandon `
      + **TAB** (or `suggest abandon `) lists it by bare id (`gate-patrol`); works
      anywhere (not giver-bound, unlike `accept`). Only *abandonable* active
      quests are offered.

## 11. Social / multi-session (admin1 + player1, two windows)

- [x] Both in Town Square — `look` lists the other in "You see here:";
      movement shows "player1 arrives" / "player1 leaves".
- [ ] `look player1` — player1's **generated description** prints (the appearance lens):
      "You see player1, a &lt;Race&gt; &lt;Class&gt;." composed from his race/class
      (no authored prose — players are described from their character).
- [x] `tell player1 hello` — player1 receives it; `reply hi` goes back; `tells` shows
      the history.
- [x] `channels` (`chanlist`) — lists channels; post on one and the other sees
      it; `chathistory` (`chhist`) shows scrollback.
- [x] `emote waves` (`pose`) — the room sees "admin1 waves".
- [ ] `give ration to player1` — the ration leaves your inventory and enters player1's
      (`i` on each to confirm); both args tab-complete (item from your pack,
      target a player). player1 must be in the room.
- [x] `who` — lists every online character (world-wide, not just this room),
      one per line, alphabetical, then "N players online." admin1 shows an
      `[Admin]` tag; a character idle >60s shows `(idle)`. You always see
      yourself.
- [x] Log in as admin1 from a 3rd connection — you're prompted to **take over**
      the existing session; confirming moves you to the new connection.
- [x] Drop a connection abruptly (close the terminal) — the character goes
      **link-dead**, then reconnecting resumes the session; left long enough
      (`LINKDEAD_TIMEOUT`) it's swept.
- [ ] Sit idle past `IDLE_SWEEP_INTERVAL`/idle timeout — you get an idle warning
      then disconnect (admins are exempt — admin1 won't be swept).

## 12. Doors & locks

The core pack ships a door test branch below the Forge: a **plain oak
door** (`down` from the Forge to the **Forge Cellar**) and a **locked iron
door** (`down` from the cellar to the **Forge Vault**). The **iron key** is
in the cellar. Doors render their state on the exit line, block movement
while closed, and the two sides stay in sync.

### Plain door — open / close (in the Forge)

- [x] `look` — the `down` exit shows the oak door as closed (e.g.
      `down (closed)`).
- [x] `down` while closed — blocked: "A sturdy oak door is closed."
- [x] `open down` (or `open oak`) — "You open a sturdy oak door."; `down`
      now moves you into the **Forge Cellar**.
- [x] Back in the Forge, `close down` — re-closes it; `down` is blocked again.
- [x] `lock down` (or `lock oak`) on the plain door — "There's no lock on a
      sturdy oak door." A keyless door is close-only, not lockable; same for
      `unlock`. (In the Cellar, address a specific door with `up`/`oak` or
      `down`/`iron` — `door` alone is ambiguous when two doors are present.)

### Locked door + key (in the Forge Cellar)

- [x] `get key` — picks up the iron key.
- [x] `down` — blocked (the iron door is closed/locked).
- [x] `unlock down` (or `unlock iron`) **before** holding the key (drop it
      first to test) — "You don't have a key for an iron door."
- [x] With the key, `unlock down` — "You unlock an iron door."; then
      `open down`, `down` → the **Forge Vault**. `get coins` is the payoff
      (credits gold).
- [x] `lock down` requires the door closed first and the key in hand;
      `unlock down` reverses it.
- [x] From the vault, `up` works — unlocking/opening one side syncs the
      reverse, so you're never sealed in.

## 13. Recall

- [x] `recall set` (in Town Square) — binds your recall point.
- [x] Move away (e.g. to the Meadow), then `recall` — returns you to the bound
      room.

## 14. Weather & time

- [x] Stay in an outdoor room (Square/Gate/Meadow) and `look` over time — a
      weather/time ambience line appears and changes (temperate zone).
- [x] In the **Forge** (indoors), no weather ambience appears.

## 15. Help & UI

- [x] `help` — lists command categories/topics.
- [x] `help get` — shows the `get` topic with syntax.
- [x] `prompt` — shows your status prompt; `prompt <template>` changes it;
      `prompt default` restores it.
- [x] `color off` / `color on` — toggles ANSI color in output.

## 16. Persistence

- [x] Change state (move, pick up items, spend gold, take damage), `quit`,
      reconnect — everything is as you left it.
- [x] Restart the **server** (Ctrl-C, `make run` again), log back in — character
      state survived (corpses/weather/mobs reset by design; player save does not).

## 17. Admin verbs (as admin1 — seeded admin via `ANOTHERMUD_ROLE_SEED`)

- [x] `inspect bandit` (in the Meadow) — full diagnostic record of the target.
- [ ] `roomdata on` (admin/builder) — `look` now appends a room metadata block
      (room id, coordinates, terrain, tags, properties incl. `craft_stations`,
      exit targets); `roomdata off` removes it. Persists across logout; gated
      to admins/builders at render time.
- [x] `restore` / `restore player1` — refills vitals to full **and** tops off
      sustenance (hunger/thirst); the reply notes "fully fed" for a player target.
- [x] `set vital hp <target> 1` — sets a field on a target (then `restore`).
- [x] `teleport meadow` (`goto meadow`) — jump to a room by id; `goto player1`
      jumps to a player.
- [x] `purge bandit` — removes the mob from the world.
- [x] `announce Server test in progress` — all connected players see the
      broadcast.
- [x] `grant builder to player1` then `revoke builder from player1` — role changes (player1
      sees nothing player-facing; verify via `inspect player1`).
- [x] `xp 1000` — grants XP to yourself (admin probe).
- [x] `reload` — reloads pack Lua scripts. The **count comes back to your
      client**: "Reloaded N script(s)." (core ships one — `track_kills.lua`).
      The **server log** also prints a confirmation (`event=scripting.reload
      count=N`).
- [x] Type a few junk verbs (`xyzzy`, `frobnicate`, `xyzzy`) — each replies
      "Huh?". Then `badinput` lists them ranked by count (`xyzzy` ×2 on top);
      `badinput clear` resets the tracker. (Unknown verbs also log
      `event=command.unknown` on the server.)
- [ ] As **player1** (non-admin), any admin verb (`inspect`, `goto`, …) — refused /
      hidden in `help`.

## 18. Modern client (WebSocket / GMCP / MSSP)

Needs a GMCP-capable client (e.g. Mudlet) and `ANOTHERMUD_WS_ADDR=:4001`.

- [ ] Connect over WebSocket to `:4001` (path `/mud`) — login + play works the
      same as telnet.
- [ ] GMCP: the client receives `Char.Vitals`, `Char.Status`, `Room.Info`,
      `Comm.Channel`, etc. as you play (inspect the client's GMCP debug view).
- [ ] GMCP tab-completion (`Input.Complete`): send
      `Input.Complete {"line":"get s"}` (client→server) — you get an
      `Input.Complete.List` reply with candidates + `common` prefix. See
      `docs/clients/tab-completion-gmcp.md`; bind it to Tab in the client. Works
      over telnet GMCP and WebSocket.
- [ ] Color: a truecolor/256-capable client shows richer color tiers than a
      plain telnet client.
- [ ] MSSP: a MUD listing tool / Mudlet shows server status variables on connect.

## 19. Scripting (pack Lua)

- [x] Kill the bandit and watch the **server's** log (its stderr — the
      `make run`/`make watch` terminal, *not* your game client) — at INFO level
      `track_kills.lua` logs an `event=scripting.log msg="kill: …"` line, then a
      scheduled follow-up ~3s later. (Suppressed if `ANOTHERMUD_LOG_LEVEL` is
      above `info`.)
- [x] As admin, `reload` — the server log prints `event=scripting.reload
      count=N`; scripts only emit their *own* log lines when they next fire
      (kill the bandit again to see `track_kills.lua` after a reload).

## 20. Tab-completion

Tab-completion exposes the same query four ways: a **real TAB key** in raw telnet
(char-mode, §20.0 below), the GMCP `Input.Complete` package (modern clients, §18),
the player **`suggest`** verb (line-mode, anyone), and the admin **`complete`**
debug verb (raw candidate dump).

### Real TAB on raw telnet (char-mode) — default-on for raw clients

On a **raw telnet client** (not Mudlet/GMCP), char-mode turns on automatically
after you log in, so the TAB key completes.

- [x] In Town Square, type `get sw` and press **TAB** — completes inline to
      `get sword` (single match). Backspace works; Enter submits.
- [x] Type `get s` + **TAB** — several matches: it lists the room items and you
      can keep typing.
- [x] `tabcomplete off` — disables it (back to plain line input); `tabcomplete on`
      re-enables; `tabcomplete` shows status.
- [x] On a **GMCP client (Mudlet)**, TAB is *not* server-driven (it stays
      line-mode and uses the §18 GMCP path); `tabcomplete on` can force char-mode.
- [x] Login + password are line-mode (char-mode only engages after login —
      password input is never echoed).

### `suggest` — player line-mode completion (anyone)

No TAB needed — type `suggest` + a partial command and it lists what you could
type. (Trailing space is trimmed, so type a partial letter: `suggest get s`.)

- [x] In Town Square, `suggest get s` — lists the matching room items
      (`sword   a short sword`, …); `suggest get sw` narrows to the sword.
- [x] `suggest dr` — verb completion: `Commands: drop, drink`.
- [x] At the Forge, `suggest kill ma` — single match → `→ kill maerys
      (Maerys the Training Master)`.
- [x] `suggest get sw` somewhere with no items — `No suggestions for "get sw".`
- [x] `suggest` with no args — guidance ("Suggest what? …").

### `complete` — admin debug dump

Run as **admin1** (admin); prints the raw candidate set (kind/token/display).

> Note: the verb can't express a *trailing space* (the input is trimmed), so to
> see a fresh argument slot type a partial letter (`complete get s`, not
> `complete get `).

### Verb completion (anywhere)

- [x] `complete loo` — verb slot; lists `look`.
- [x] `complete n` — `n` is listed **first** (exact match wins), then `north`.
- [x] `complete` (no args) — lists many verbs, ending with `… (truncated)`.

### get / take / kill (the migrated targeting verbs)

In **Town Square** (the short sword is on the ground here):

- [x] `complete get sw` — argument slot of `get`; lists the short sword
      (token `sword`).
- [x] `complete take sw` — same result via the `take` alias.

In the **Meadow** (`s` from the Gate — the road bandit is here):

- [x] `complete kill ban` — lists the bandit (token `bandit`).
- [x] `complete kill rog` — also lists the bandit, matched on its **keyword**
      `rogue` (not in its display name) — the completion token round-trips.
- [x] `complete consider ban` (`complete con ban`) — same bandit (entity scope).
- [x] `complete look ban` / `complete look at ban` — lists the bandit
      (`visible` scope; the `at`/`in` prepositions are handled).

### Containers & doors

- [x] After `put sword in sack`: `complete get sword from sa` — the `from`
      preposition maps the cursor to the container slot; lists the sack.
- [x] In the **Forge**: `complete open oa` — lists the oak door (token `d`, the
      `down` direction); `complete open d` matches the direction itself.

### Degradation & gating

- [x] `complete say hel` — argument slot, but **no candidates** (`say`'s body is
      free text — nothing to enumerate).
- [x] `complete frobnicate x` — "no completable slot" (unknown verb).
- [ ] As **player1** (non-admin), `complete loo` — refused with `Huh?`, identical to
      an unknown verb (the debug verb's existence is not disclosed).

## 21. Light & darkness

Effective light is **per-viewer** and computed live from time-of-day, terrain,
a room `light` override, lit sources you carry, and darkvision. It gates what
you can see, examine, fight, and walk into. The door branch below the Forge is
the showcase: **Forge** (lit) → **Cellar** (dim, has a torch) → **Vault**
(black). Get to the Vault via §12 (get the iron key in the cellar, `unlock
down`, `open down`, `down`).

### Room render by light level

- [ ] In the **Forge Cellar**, `look` — full render, but the description reads
      **muted** (dim: a wall lamp, not full daylight). Name, exits, items all
      present.
- [ ] In the **Forge** (`up`), `look` — full **lit** render (the forge fire
      pins it bright despite being indoors).
- [ ] In the **Forge Vault** (black), `look` — **suppressed**: a single "It is
      pitch black. You can see nothing." line. No room name, description,
      exits, or occupants.
- [ ] An outdoor room at night (Square/Gate/Meadow after dark) `look` —
      **gloom**: a terse "too dark to make out any detail" line, exits as
      **bare directions** (no door/weather detail), occupants shown as
      anonymous shapes (no names). By day the same room is full **lit**.

### A carried light source (the pine torch)

- [ ] In the Cellar, `get torch`, `equip torch light`, then `light torch` — "You
      light a torch." (`equipment` shows it in the **light** slot).
- [ ] Carry the lit torch into the **black Vault** and `look` — it lifts to
      **gloom**: the room name returns with the terse dark form + bare exits.
      (A basic torch is gloom-level — enough to navigate, not to read detail.)
- [ ] `extinguish torch` (`douse`) in the Vault — back to the black "you can see
      nothing" render. `light torch` again restores gloom.
- [ ] **Fuel:** a lit torch burns down on the fuel tick and eventually **gutters
      out** ("A torch gutters out and goes dark.") — it becomes unlit on its
      own. (Default ~one fuel/30s; `fuel: 120` ≈ an hour. To see it fast, drain
      the torch's fuel via admin `set property fuel <torch> 1`, or just trust
      the unit tests.)
- [ ] Auto-light is **off** by default (you `light` it explicitly). With
      `ANOTHERMUD`-side auto-light enabled, equipping into the light slot would
      ignite it.

### Examination & reading gate

- [ ] In the black Vault (no torch), `look coins` — "It is too dark to make it
      out." (examining a room thing needs at least **dim**).
- [ ] `get coins` in the dark — still **works** (taking isn't gated; credits
      gold). You can grab by feel even if you can't read detail.
- [ ] `look <a held item>` in the dark — its description still prints (you feel
      what you carry; held items are never gated).
- [ ] With the gloom torch lit, `look coins` is **still** too dark (gloom shows
      shapes, not detail — you'd need a brighter, dim-level source).

### Combat in the dark

- [ ] Fight the bandit in the **Meadow at night** (outdoors → gloom) vs **by
      day** (lit) — your hit rate is **lower in the dark**; the penalty scales
      with how dark it is for *you* (the attacker). Daylight or a bright enough
      source removes it. Combat is never *blocked* by darkness — a natural 20
      still lands.

### Movement & the escape invariant

- [ ] In the black Vault, even though `look` hides the exits, on arrival you
      were told the way back ("You can feel your way back up.") and `up` **still
      works** — darkness never traps you. Outdoor rooms are never fully black.
- [ ] (Content opt-in) a room flagged `dark_blocked` refuses entry to a mover
      who can't see it at all (effective black) — a lit torch lets you brave it.
      No core room ships this by default.

### Transitions, darkvision, probe, persistence

- [ ] Stand in an outdoor room across a dawn or dusk boundary — you get a
      **transition** line ("…shadows close in…" / "…the world brightens…") when
      your effective level actually crosses. A pinned-lit room (the Forge) and
      the always-black Vault emit nothing.
- [ ] As a **dwarf** (darkvision), enter the black Vault with no light — you see
      it at **gloom** (shapes + directions), where a human sees nothing.
- [ ] `daylight` (`time`) anywhere — reports the time of day and how well you can
      see ("It is night. It is pitch black here; you can see nothing.").
- [ ] **Persistence:** note the in-game time, restart the server, log back in —
      time-of-day **resumes** where it stopped (the world isn't reset to night;
      saved in `saves/clock.yaml`).
- [ ] (GMCP, §18) `Room.Info` carries a per-viewer `light` field
      (black/gloom/dim/lit) — a capable client can theme the viewport from it.

## 22. Crafting & cooking

Crafting turns inputs into an output via a **recipe**, gated by a **discipline**
(a proficiency you learn at a trainer), the **station** you work at, your
**tool**, and **ingredient** quality. Output quality renders as a rarity tier.
Cooking is crafting whose output is food (clears sustenance; at quality, grants
a **well-fed** buff). Use a character with some gold for ingredients (sell
starter kit or seed a balance via shops, §9 — or gather your own inputs, §29).

The craft NPCs/stations in the core pack:

- **Brandr the blacksmith** — Hearthwick Forge (`n` from Square). Teaches
  **smithing** + sells a rusty dagger and a fine iron hammer (a tool). The
  Forge is a **Tier-2 smithing station**.
- **Marta the cook** — Market Row (`e` from Square). Teaches **cooking** +
  sells raw meat, firewood, and a traveling cook's kit. The Market is a
  **Tier-2 cooking station**.

### Learn a discipline (the trainer-shops)

- [x] At the Forge, `learn smithing` — "Brandr the blacksmith teaches you the
      basics of Smithing. You learn 1 starting recipe." (Works even though
      **Maerys** is also a trainer in the room — the trainer resolver picks the
      one who can teach the skill.)
- [x] `craft` (no argument) — lists your known recipes (`reforge a short sword`).
- [x] At Market Row, `learn cooking` — Marta teaches it; you learn `cook a
      hearty meal`. `learn cooking` somewhere with no trainer — "There is no
      one here who can teach you that." `learn dancing` — "There is no such
      craft to learn."

### Smithing at the forge (Tier-2 station)

- [x] At Market Row, `craft reforge` — **refused**: "You need a proper crafting
      station for that — a forge, a kitchen, or the like." (The market is a
      *cooking* station; reforge needs a smithing station — the station gate.)
- [x] At the Forge, `buy dagger`, then `craft reforge` — "You craft a short
      sword." `inventory`: the **rusty dagger is consumed** and a **short sword**
      produced (atomic — nothing lost on a failed craft).
- [x] `buy hammer` (the fine iron hammer, `[UNCOMMON]`), then `craft reforge`
      again — the tool weights quality up: a sword crafted **with** the hammer
      carried tends to a higher rarity tier than **without** it (the hammer is a
      tool — it is **not** consumed). Tool quality is a separate lever from skill.

### Cooking at the market (Tier-2 station) → well-fed

- [x] At Market Row, `buy meat`, then `craft hearty` — "You craft a cooked meal."
      (raw meat consumed). `eat meal` — clears sustenance (`score` shows it).
- [ ] A freshly-learned (skill-1) cook makes **common** meals = cold rations
      (no buff). Raise cooking: `practice cooking` at Marta + craft repeatedly,
      and a higher-quality meal applies a **well-fed** stat buff on `eat`.

### Field crafting: build a campfire (Tier-1)

- [ ] At Market Row, `buy firewood`. Go to an outdoor room (Meadow: `s`, `s`
      from Square) and `craft hearty` — **refused**: "You need at least a fire
      or workbench for that — build a campfire or find a station."
- [ ] `build campfire` — "You build a campfire; it crackles to life." (consumes
      one firewood). Now `craft hearty` **works** there (the campfire is a
      Tier-1 cooking station).
- [ ] `build campfire` again in the same room — "There is already a fire burning
      here." `build campfire` indoors (the Forge) — "There's no safe place for a
      fire here." In the rain — "The weather won't let a fire catch."
- [ ] Leave the campfire; after `ANOTHERMUD_CAMPFIRE_LIFETIME` (default 10m) it
      decays — "The campfire burns down to cold ashes." (lower it for testing).
- [ ] **Portable tool:** `buy kit` (the cook's kit) at Marta, carry it into a
      field room, and `craft hearty` works **without** a campfire (the kit grants
      Tier-1 cooking in the field).

### Skill & persistence

- [ ] `abilities` — smithing/cooking appear with a proficiency that **climbs as
      you craft** (use-based gain). `quit` + relog — your learned disciplines and
      known recipes persist (player save v17). A crafted item keeps its rolled
      quality across logout.

> **Gathering** is the real ingredient source now (forage/harvest over biome
> resource tables — see **§29**); the vendor ingredients above are a convenience,
> not the only path. Recipe-acquisition breadth (common/uncommon/rare/regional
> recipes via shops/quests/loot beyond the starter set) is the remaining post-MVP
> piece — see `docs/BACKLOG.md`.

## 23. Player maps

- [ ] `map` — renders an ASCII map of the rooms you've **explored** in the
      current area (fog-of-war). Walk to a new room, `map` again — the newly
      visited room appears. Rooms you haven't entered stay hidden.
- [ ] `minimap on` — a small map appears alongside the room view on every
      `look`/move; `minimap off` removes it; `minimap` shows the current state.
      The toggle persists across logout.
- [ ] (GMCP, §18) `Room.Info` carries coordinates; a Mudlet-style client can
      drive its native mapper from them.

## 24. Saving throws (Fortitude / Reflex / Will)

Every character has three saves — **Fortitude** (Constitution), **Reflex**
(Dexterity), **Will** (Wisdom) — each a class-granted base bonus (a *strong*
or *weak* level progression) plus the governing ability's modifier. They're
shown on `score` and resolved as a `d20 + bonus vs DC` check. The first
consumer is the **massive-damage Fortitude save** (a single hit at/above a
threshold forces a Fort save or you suffer the lethal consequence).

### Saves on the score sheet

- [ ] `score` (`sc`) — the **Combat** column shows a `Saves  Fort +X  Ref +Y
      Will +Z` row. A fresh **level-1 fighter with all stats 10** reads
      `Fort +2  Ref +0  Will +0` — the fighter's **strong** Fortitude
      (base 2) vs **weak** Reflex/Will (base 0).
- [ ] Raise the governing ability and re-check: `train con` (or admin
      `set stat con <target> 16`), then `score` — **Fortitude rises** with the
      CON modifier (no separate save plumbing; saves are derived live).
- [ ] (Admin) on a higher-level character, saves scale with level — the strong
      curve climbs faster (2,3,3,4,…) than the weak curve (0,0,1,1,…).

### The massive-damage Fortitude save (combat)

The default threshold is **50** (above ordinary low-level swing damage), so it
**won't fire in normal demo combat** — the bandit hits for ~4. To observe it,
**lower the threshold** so any hit qualifies:

```sh
ANOTHERMUD_MASSIVE_DAMAGE_THRESHOLD=1 ANOTHERMUD_MASSIVE_DAMAGE_DC=50 make run
```

- [ ] With the threshold lowered, fight the bandit in the **Meadow**. On a hit
      that doesn't already kill, you (or the bandit) roll a Fortitude save:
      a pass prints `You resist! (Fortitude save)` (room: "X resists."); a
      fail prints `You fail to resist! (Fortitude save)` and applies the
      lethal consequence — for a player that's the normal death recovery
      (`Everything goes black… wake, dazed, in another place`, HP 1, recall
      room), exactly like any other death.
- [ ] A hit that **already** drops the victim to 0 forces **no** save (the
      kill path runs first) — you just see the normal death, no save line.
- [ ] Restore the default (drop the env vars) — combat no longer triggers
      saves; the rule is inert until something hits for 50+ in one blow.

> Reflex and Will are derived and shown but have **no triggering system yet** —
> they wait on conditions (S5) and poison/fear (S7) in the WoT mechanics EPIC.
> The massive-damage save is the one shipped consumer.

## 25. Conditions (status effects)

Status conditions are flagged effects that change combat. The **Core 5**:
**stunned** (no swings + easier to hit), **prone** (−melee + easier to hit),
**blinded** (heavy −to-hit + easier to hit), **frightened** (−attack/−saves +
flees each round), **fatigued** (−Str/−Dex). They're inflicted by the admin
`afflict` verb or the fighter's save-gated **trip**/**bash** abilities, and
appear in `affects`. Use **admin1** (admin).

### Admin inflict + the listing

- [ ] `afflict admin1 stunned` (on yourself) — "You are stunned!"; `affects`
      (alias `effects`) lists `Stunned — N round(s) [condition]`.
- [ ] `afflict admin1 fatigued` — `score` shows STR/DEX dropped (−2 each);
      `cure admin1 fatigued` restores them. `cure admin1` clears all
      conditions at once (leaving non-condition buffs like bless).
- [ ] `afflict ghost stunned` — "You don't see them here." (bad target);
      `afflict admin1 bless` — "no such condition" (`bless` isn't a condition).
- [ ] As **player1** (non-admin), `afflict admin1 stunned` — "Huh?" (admin-gated).

### Conditions in combat (the Meadow)

Go to the Meadow (`s`, `s`) and engage the bandit.

- [ ] `afflict bandit stunned 5` — "a road bandit is stunned." For the next
      rounds the bandit **lands no hits** (you still hit it) — stunned skips its
      swings. After a bit: "a road bandit shakes off the stun." (its Fortitude
      shake-off save lands) and it resumes attacking.
- [ ] `afflict bandit prone 5` — the bandit is easier to hit (your hit rate
      climbs while it's prone).
- [ ] `afflict bandit frightened 5` — the bandit **flees** the room each round
      (frightened forces flight), with a −2 to its attacks and saves while it
      lasts.

### Save-gated abilities (trip / bash)

The fighter learns **trip** (→ prone, Reflex save) and **bash** (→ stunned,
Fortitude save) at level 1.

- [ ] In combat, `cast trip bandit` — on a failed Reflex save the bandit is
      knocked prone; on a made save you see "a road bandit resists!" (the entry
      save). `cast bash bandit` is the same with a Fortitude save → stunned.
- [ ] After a `trip` lands, `stand` gets **you** up if *you're* prone (it also
      doubles as the wake-from-rest verb); a prone mob gets up on its own when
      the condition's duration runs out.

> Default condition magnitudes/durations are engine defaults
> (`internal/condition`); HP-state conditions (dying/disabled), damage-over-time
> (bleeding/poison — deferred to S7), and the grapple family are **not** in this
> slice. Reflex/Will saves now have their first condition consumers.

## 26. Skills & lockpicking

A skill is a use-based proficiency resolved by a `d20 + skill bonus vs DC`
check (the same idiom as saves). The first consumer is **lockpicking**: the
**Open Lock** skill vs a door's pick difficulty — a keyless alternative to
`unlock`. The fighter learns Open Lock at creation. Use **admin1** or a fresh
fighter.

### The skills listing

- [ ] `skills` — lists your skill-category proficiencies with `prof/cap`, e.g.
      `Open Lock  1/30`. (Combat abilities like Basic Strike / Kick also appear
      — they carry the `skill` category too; that's the engine taxonomy, not a
      bug.) A character who knows no skills sees "no skills yet."

### Lockpicking the forge vault (the demo)

The forge cellar's **iron door** (`down`, to the vault) is **pickable**
(difficulty 15) as well as key-openable. Get to the cellar: at the Forge,
`open down` (the oak door), then `down`.

- [ ] In the cellar, `pick iron` **without** the iron key in hand — on a
      success: "You deftly pick an iron door's lock." (the lock opens, the same
      as a keyed unlock); the room sees "X picks the lock." A novice fighter
      (Open Lock ~1, no Dex bonus) needs a high roll, so it may take a few tries.
- [ ] On a failure: "You fail to pick an iron door's lock." and the room hears
      "X fumbles at the lock." (the noise friction) — the door stays locked.
- [ ] `pick iron` again after it's open — "An iron door isn't locked."
- [ ] After picking, `open down`, `down` reaches the **vault** (the coins are
      the payoff — bring the **pine torch** from the cellar to see in the dark,
      §21). The iron key is never needed.
- [ ] `skills` after a few picks — Open Lock's proficiency has **climbed** (it
      gains with use, even on a failed attempt at a reduced rate).
- [ ] `pick oak` (the plain oak door, no lock) — "There's no lock on a sturdy
      oak door to pick." A non-pickable locked door (none in core) would say
      "can't be picked."

> Pick difficulty is per-door content; the proficiency→bonus scale is an
> engine default (`internal/progression` skill config). Other skills
> (hide/search/spot) are owned by the **visibility** spec and call the same
> `ResolveSkillCheck` primitive; crafting disciplines are skills too (§22).

## 27. Channeling — the One Power (WoT pack)

> **Different world.** This section runs the **Wheel of Time** pack, not the
> core demo. Boot it on its own and make a fresh **channeler** (admin1/player1 are
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

## 28. Movement cost & encumbrance

Walking spends **movement points (MV)** from a pool, and rough terrain costs
more per step. A fresh character has **MV 30**; the flat per-step cost is
**`ANOTHERMUD_MOVE_COST`** (default **2**), and biomes override it. Use a fresh
fighter (full MV).

### The MV pool on the score sheet

- [ ] `score` (`sc`) — the Combat column shows an **`MV <current>/<max>`** line
      (e.g. `MV 30/30`). A non-channeler also shows `MA 0/0`.
- [ ] Walk a few rooms in the village (Square ↔ Gate ↔ Market, all flat cost 2)
      and re-check `score` — **MV drops** by the step cost each move.
- [ ] Idle a while (or `rest`, §9) — **MV regenerates** over time, like HP.

### Biome-weighted step cost (the wilderness loop)

Costs: **grassland 2** (the flat default), **forest 3**, **mountain 4**,
**cave 3**. From the Meadow head east/south into the loop:

- [ ] From the **Meadow** (grassland, 2), `e` into **The Forest's Edge**
      (forest, 3) — the step costs 3, and because it's *rougher than the ground
      you left* you see the hint **"The going is hard here."**
- [ ] `e` again into **Deep Forest** (forest, 3) — same terrain, so **no hint**
      (it only fires when the cost goes *up*).
- [ ] `s` into **Rocky Foothills** (mountain, 4) — the hint fires again (4 > 3),
      and MV drops by 4.
- [ ] `w` into **Cave Mouth** (cave, 3), `down` into the **Old Diggings** — watch
      MV; the loop is the cheapest way to drain the pool.

### Encumbrance (load → surcharge)

Carry capacity is derived from **Strength** (≈ STR × 8). Load raises the per-step
cost: **< 50%** of capacity is free, **50–89%** adds **+1** MV/step, **90%+**
adds **+2**.

- [ ] Load up (pick up the heavy starter items, or admin `set stat str <you> 6`
      to shrink your capacity), then walk — the per-step MV cost is **higher**
      than the unburdened baseline for the same room.
- [ ] Try to `get` an item that would exceed your capacity — refused: **"<item>
      is too heavy for you to carry."**
- [ ] A creature with `carry_max` set **negative** is treated as **unlimited**
      (the sentinel) — mobs/test actors aren't gated.

### The winded gate (and the no-strand rule)

- [ ] Drain MV low (cross several mountain/cave rooms), then try to move with
      **insufficient MV** — refused: **"You are too winded to go that way. Catch
      your breath."** Wait/`rest` for MV to regen, then continue.
- [ ] (Design note) actors with **no MV pool at all** (mobs, scripted/admin
      moves) move for free and are never stranded — only metered characters spend.

> Tune with `ANOTHERMUD_MOVE_COST` (flat default) and per-biome `move_cost:` in
> `content/starter-world/biomes/*.yaml`. No per-step "MV −N" feedback line is
> shown today beyond the difficulty hint (a known LOW gap).

## 29. Gathering (forage & harvest)

Two ways to pull ingredients from the land: ambient **`forage`** (rolls the
room's biome forage table, on a cooldown) and discrete **`harvest <node>`**
(a respawning resource node, often tool-gated). Launch with
`ANOTHERMUD_FORAGE_COOLDOWN=5s` (§0) so you don't wait between forages.

### Forage (ambient, biome-driven)

- [ ] In the **Meadow** (grassland), `forage` (alias `gather`) — **"You forage
      <item>."** (a wild herb or, less often, a wildflower). `i` shows it.
- [ ] `forage` again immediately — cooldown: **"You've picked this area over;
      give it time to recover."** Wait out `FORAGE_COOLDOWN`, then it works again.
- [ ] In **The Forest's Edge** / **Deep Forest** (forest), `forage` — yields the
      forest table (wild herb or forest mushroom, both cooking ingredients).
- [ ] In **Rocky Foothills** (mountain) or a cave room, `forage` — **"There's
      nothing to forage here."** (those biomes declare no forage table — absence,
      not a refusal).
- [ ] `abilities` (`abi`) — a **gathering** proficiency appears and **climbs**
      with successful forages (use-based gain).

### Harvest (resource nodes, tool-gated)

Nodes spawn **per biome on the area cadence** (not hand-placed) — one per room:
**timber stands** in forest rooms, **rock outcrops** in grassland/mountain,
**iron veins** in caves. They need the right tool: an **axe** (woodcutting) for
timber, a **pickaxe** (mining) for outcrops/veins.

- [ ] In a forest room, `look` for **"a stand of straight timber"** (give the
      server a moment after boot for nodes to spawn). With a woodcutting axe in
      hand, `harvest timber` — **"You harvest <item>."** (timber-stand has **3
      charges**).
- [ ] `harvest timber` with **no axe** — refused: **"You need … to harvest
      that."** (the one allowed harvest refusal — a tool gate).
- [ ] Work a node to empty — the last charge reads **"You harvest <item>. <node>
      is exhausted."**; a further `harvest` says **"<node> has nothing left to
      give."** until it respawns.
- [ ] In the **Meadow** or **Foothills**, `harvest outcrop` (rock, 2 charges,
      pickaxe); in a **cave** room, `harvest vein` (iron ore, 3 charges, pickaxe).

> Node/forage state is **transient** — depletion and cooldowns reset on restart
> (they're not persisted). Gathering feeds the §22 crafting loop (the forest/cave
> yields are smithing/cooking inputs).

## 30. Visibility & hidden exits

Concealment is a per-observer "can X see Y?" model: **hide**, moving **sneak**,
admin **wizinvis**, and the **search** verb that finds hidden exits. Two players
(`player1` + `player2`) make hide/detect observable; the forge shows hidden exits
solo.

### Hide / unhide / sneak

- [ ] `hide` — **"You slip into the shadows and go still."** `hide` again — **"You
      are already hidden."** A room with no cover may veto: **"You can't hide
      here."**
- [ ] With `player2` in the room, `player1` `hide`, then `player2` `look` —
      whether `player1` shows up is an opposed **perception contest** (a strong
      observer auto-spots; a weak one may miss). A spotted-once observer stays
      able to see you (sticky).
- [ ] While hidden, take an action that gives you away (`cast …`, attack, or
      `search`) — **"Your sudden action gives you away; you are no longer
      concealed."**
- [ ] `unhide` (`reveal`) — **"You step out of hiding."**; `unhide` when not
      hidden — **"You aren't hidden."**
- [ ] `sneak` — toggle on: **"You begin moving quietly, keeping to the shadows."**
      Move between rooms — sneak **persists across rooms** (each move re-runs the
      contest per observer). `sneak` again — **"You stop moving so carefully."**

### Admin invisibility (`wizinvis`)

- [ ] As `admin1`, `wizinvis` (`invis`) — **"You wink out of sight; only your
      peers can see you now."** A non-admin in the room no longer sees you in
      `look`/`who`; another admin still does. It does **not** break on action.
- [ ] `wizinvis` again — **"You fade back into view."**

> Magical (non-admin) invisibility exists in the engine (the `invisible`
> effect-flag, pierced by `see_invisible`) but **no core/WoT content grants it
> yet** — there's no potion/spell to live-test in v1.

### Hidden exits (the forge secret passage)

The Forge has a **hidden west exit** to the **Forge Nook** (`search_difficulty`
1). It's unlisted *and* unwalkable until found.

- [ ] In the **Forge**, `look` — the exits list shows `south`/`down` but **not**
      `west`. Try `west` anyway — **"You cannot go that way."** (indistinguishable
      from a real no-exit — knowledge-gated).
- [ ] `search` — **"You discover a hidden passage leading west."** (a low
      difficulty, so it finds quickly). `look` now lists `west`; `west` walks you
      into the Forge Nook.
- [ ] From the Nook, `east` back to the Forge — the **reverse is not hidden**
      (authored open). Discovery is per-character and ephemeral (re-find after a
      relog).
- [ ] `search` in a room with nothing hidden (e.g. Town Square) — **"You search
      carefully but find nothing hidden."**

## 31. Feats

Feats are player-chosen passive perks bought with **banked feat credits** (1 at
creation, +1 every 3rd level). A fresh fighter has **1 credit** to spend
immediately.

### List & take

- [ ] `feats` — lists feats you hold, your **available credits**, and the feats
      you're eligible to take. (`feat` with no args also shows the listing.)
- [ ] `feat great-fortitude` — spends a credit and grants the feat; `score` (§24)
      shows **Fortitude +2**. `feats` now lists it held with one fewer credit.
- [ ] `feat iron-will` with **no credits left** — refused (no feat slot
      available).
- [ ] `feat nonesuch` — an unknown feat is rejected gracefully.

### The core feat set (`content/core/feats/`)

Eight feats ship in the core pack, no hard prereqs in v1:

- **great-fortitude / iron-will / lightning-reflexes** — +2 Fort / Will / Reflex
  save respectively (verify on `score`, §24).
- **toughness** — +3 max HP (stackable — take it again with another credit and HP
  climbs again).
- **weapon-focus** / **improved-critical** — *per-parameter*: pass a weapon
  category, e.g. `feat weapon-focus martial` (+1 to-hit) or `feat
  improved-critical martial` (+2 crit threat). Omitting the parameter is refused
  (it needs a target).
- **skill-emphasis** — per-parameter: `feat skill-emphasis open-lock` (+3 to that
  skill check, §26).
- **power-attack** — unlocks the Power Attack ability (the ability's combat effect
  is still pending — a known gap).

> The creation wizard has no feat-pick step yet — feats are taken in-session with
> the verb. Prereqs are not enforced in v1 (the d20 prereq chains are deferred).

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

> Slice A keeps everything same-room: the bow doesn't out-range a melee foe yet
> (no opening volley / kiting) — the far→near→melee **range bands** are Slice B
> (`ranged-combat.md` §5), not built. Mob ranged AI is also deferred.

---

## Notes / known gaps (already understood)

- **Combat only happens in the Meadow.** Town Square is a safe-room; the bandit
  in the Meadow is the intended target.
- **Characters are self-provisioned** (§0) — no pre-built saves are assumed. Boot
  with `ANOTHERMUD_ROLE_SEED="admin1:admin"` so `admin1` is an admin, then create
  the characters you need. Passwords are whatever you set; to reset, delete
  `saves/accounts/<id>/` + `saves/players/<name>/` and re-create.
- **Login is account-first (§1):** authenticate by **account username** (email is
  no longer asked), then pick from the character **roster**.
- Time/weather, corpse decay, idle, and link-dead are **timer-driven** — use the
  fast-testing env above to see them quickly.
- **Light & darkness (§21):** the Forge Vault is deliberately black and the
  Cellar dim — the quickest darkness demo without waiting for nightfall. The
  day/night cycle runs ~24 real minutes/day (one in-game hour per minute), so
  outdoor gloom takes a little waiting; the underground branch is instant.
  Effect-driven sight buffs and room-loose source burn-down are not wired yet
  (see the light deferred-fixes memory).
- **Tab-completion (§20) is feature-complete.** A real **TAB key** works on raw
  telnet (char-mode, §20.0) and via GMCP `Input.Complete` on modern clients
  (§18); `suggest` is the line-mode path and the admin `complete` verb is the
  debug inspector. Argument completion only lights up for verbs that declare
  their arg types — and **every targeting verb now does**:
  `get`/`take`/`kill`/`look`/`consider`/`accept`/`abandon`/`talk`/`unequip`/
  `sell`/`value`/`buy`/`fill`. Each draws on the right scope: `accept` = a
  room giver's offers, `abandon` = your active quests, `unequip` = your worn
  items, `buy` = the room shop's stock, `sell`/`value` = your inventory,
  `talk` = room NPCs, `fill` = inventory (target) then room items (source,
  after `from`).
- Record any mismatch as a `BUG:` note next to the step; file the real ones into
  `docs/BACKLOG.md` or a `m<N>-deferred-fixes` memory afterward.
