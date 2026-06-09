# AnotherMUD Playtest Guide

A manual QA checklist for verifying implemented features (M0–M27 + recent
polish: the look/consider appearance lens, tab-completion surfaces, weapon
damage dice + critical hits, **light & darkness** — §21, **crafting &
cooking** — §22, and **player maps** — §23). Work top-to-bottom or jump to a
section. Each step gives a **command** and the **expected behavior**; tick the
box when it matches, and note anything that doesn't.

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
ANOTHERMUD_WS_ADDR=:4001 \
go run ./cmd/anothermud
```

(WebSocket on `:4001` is only needed for §17.)

### Test characters (already provisioned)

Two characters are pre-built in `saves/`. Log in by typing the **name**, then
your existing password (the accounts already exist — `jrags@jasrags.net` /
`bob@bob.com`).

| Char | Level | Role | Gear / gold | Use for |
|---|---|---|---|---|
| **Jasrags** | 10 fighter (110 HP) | **admin** | full kit + 1000g, slash/parry/kick/heal/bless learned | most testing + all admin verbs |
| **Bob** | 5 fighter (55 HP) | player | starter kit + 200g, slash learned | 2nd player for social/combat/trade |

Both spawn in **Town Square** with their kit in **inventory** — `equip sword`
and `equip cap` first thing. Multi-player sections (§11) need both logged in at
once (two telnet windows).

### The world (core pack)

```
        Hearthwick Forge (Maerys trainer; Brandr      [indoors]
              |  blacksmith — smithing trainer+shop,    Tier-2 smithing
              |  down: oak door (plain)                  station
        Forge Cellar (iron key)                   [underground]
              |  down: iron door (locked)
        Forge Vault (coins)                       [underground]

        Hearthwick Forge
              |
   Market Row — Town Square — Village Gate — Long-Grass Meadow
   (Marta cook —  (safe hub,        (wilderness)   (bandit — combat arena)
    cooking         well, gear)
    trainer+shop,
    Tier-2 kitchen)
```

- **Town Square** is a `safe-room` — combat is blocked here. The **Meadow**
  (south through the Gate) is the one place you can fight: a hostile **road
  bandit** spawns there with loot + coins.
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

## 1. Connection & login

- [x] `telnet localhost 4000` — you get a name prompt.
- [x] Enter `Jasrags`, then the password — you land in Town Square with its
      room description, exits, and a "You see here:" line.
- [x] Enter a wrong password — rejected, re-prompted (not crashed).
- [x] Type a name with a digit/symbol (e.g. `Tester1`) — rejected with "Names
      must use ASCII letters only." (names are letters-only, 2–16 chars).
- [x] Type a brand-new letters-only name (e.g. `Tester`) — it asks for email,
      then a new password with confirmation (new-account flow).
- [x] `quit` — "Goodbye." and the connection closes; reconnecting and logging
      back in puts you in the room you left.

## 2. Character creation (new character)

- [x] With a new name, walk the wizard — you're prompted to choose a **race**
      (human/dwarf) and **class** (fighter), with descriptions.
- [x] Complete it — character commits, spawns at Town Square, and survives a
      `quit` + relog (it's persisted).

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
- [x] Let the bandit kill a low-HP character (use Bob unarmed) — on death you
      respawn (healed) at the recall/start room, not disconnected.
- [x] `cast slash` / `cast kick` in combat — the ability fires (Jasrags has them).

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
- [x] Ownership window: with Bob also present, Bob looting *your* fresh kill is
      refused during the window, then allowed after it elapses
      (`CORPSE_OWNERSHIP_WINDOW`).
- [x] Decay: kill the bandit, leave the corpse; after `CORPSE_LIFETIME` it
      vanishes (its unlooted contents destroyed).

## 8. Progression & abilities

- [x] `score` (`sc`) — your character sheet: race/class/level, HP/MA/MV, the six
      attributes, AC/hit, alignment, gold, sustenance tier, and XP-to-next.
- [x] `consider` with no target (or `consider me`) — points you to `score` now
      (self stats moved there); `consider <target>` still sizes up that target.
- [x] `abilities` (`abi`) — lists learned abilities + proficiencies.
- [ ] `train str` — spends a train credit, raises STR (Jasrags has credits).
- [ ] At the Forge, `practice slash` (Maerys teaches slash/parry) — raises the
      ability's cap.
- [ ] `cast bless` / `cast heal` — self buff/heal; resolves on the next
      pulse whether in combat or idle (out-of-combat drain). bless bumps
      AC/hit (check `score`); heal restores HP (only visible if injured —
      take a hit first).
- [x] (Admin) `xp 500` — grants XP; crossing a threshold levels you up with a
      level-up message (Jasrags is level 10 / track max, so use a fresh char to
      see a level-up, or grant on a lower track).

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

## 11. Social / multi-session (Jasrags + Bob, two windows)

- [x] Both in Town Square — `look` lists the other in "You see here:";
      movement shows "Bob arrives" / "Bob leaves".
- [ ] `look Bob` — Bob's **generated description** prints (the appearance lens):
      "You see Bob, a &lt;Race&gt; &lt;Class&gt;." composed from his race/class
      (no authored prose — players are described from their character).
- [x] `tell Bob hello` — Bob receives it; `reply hi` goes back; `tells` shows
      the history.
- [x] `channels` (`chanlist`) — lists channels; post on one and the other sees
      it; `chathistory` (`chhist`) shows scrollback.
- [x] `emote waves` (`pose`) — the room sees "Jasrags waves".
- [ ] `give ration to Bob` — the ration leaves your inventory and enters Bob's
      (`i` on each to confirm); both args tab-complete (item from your pack,
      target a player). Bob must be in the room.
- [x] `who` — lists every online character (world-wide, not just this room),
      one per line, alphabetical, then "N players online." Jasrags shows an
      `[Admin]` tag; a character idle >60s shows `(idle)`. You always see
      yourself.
- [x] Log in as Jasrags from a 3rd connection — you're prompted to **take over**
      the existing session; confirming moves you to the new connection.
- [x] Drop a connection abruptly (close the terminal) — the character goes
      **link-dead**, then reconnecting resumes the session; left long enough
      (`LINKDEAD_TIMEOUT`) it's swept.
- [ ] Sit idle past `IDLE_SWEEP_INTERVAL`/idle timeout — you get an idle warning
      then disconnect (admins are exempt — Jasrags won't be swept).

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

## 17. Admin verbs (Jasrags — already admin)

- [x] `inspect bandit` (in the Meadow) — full diagnostic record of the target.
- [ ] `roomdata on` (admin/builder) — `look` now appends a room metadata block
      (room id, coordinates, terrain, tags, properties incl. `craft_stations`,
      exit targets); `roomdata off` removes it. Persists across logout; gated
      to admins/builders at render time.
- [x] `restore` / `restore Bob` — refills vitals to full **and** tops off
      sustenance (hunger/thirst); the reply notes "fully fed" for a player target.
- [x] `set vital hp <target> 1` — sets a field on a target (then `restore`).
- [x] `teleport meadow` (`goto meadow`) — jump to a room by id; `goto Bob`
      jumps to a player.
- [x] `purge bandit` — removes the mob from the world.
- [x] `announce Server test in progress` — all connected players see the
      broadcast.
- [x] `grant builder to Bob` then `revoke builder from Bob` — role changes (Bob
      sees nothing player-facing; verify via `inspect Bob`).
- [x] `xp 1000` — grants XP to yourself (admin probe).
- [x] `reload` — reloads pack Lua scripts. The **count comes back to your
      client**: "Reloaded N script(s)." (core ships one — `track_kills.lua`).
      The **server log** also prints a confirmation (`event=scripting.reload
      count=N`).
- [x] Type a few junk verbs (`xyzzy`, `frobnicate`, `xyzzy`) — each replies
      "Huh?". Then `badinput` lists them ranked by count (`xyzzy` ×2 on top);
      `badinput clear` resets the tracker. (Unknown verbs also log
      `event=command.unknown` on the server.)
- [ ] As **Bob** (non-admin), any admin verb (`inspect`, `goto`, …) — refused /
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

Run as **Jasrags** (admin); prints the raw candidate set (kind/token/display).

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
- [ ] As **Bob** (non-admin), `complete loo` — refused with `Huh?`, identical to
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
a **well-fed** buff). Use **Jasrags** (has 1000g for ingredients).

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

> Acquisition today is **baseline only** (learning a discipline grants its
> starter recipes); common/uncommon/rare/regional recipes via shops/quests/loot,
> and **gathering** as the real ingredient source (vs. the current vendor
> stopgap), are post-MVP — see `docs/BACKLOG.md`.

## 23. Player maps

- [ ] `map` — renders an ASCII map of the rooms you've **explored** in the
      current area (fog-of-war). Walk to a new room, `map` again — the newly
      visited room appears. Rooms you haven't entered stay hidden.
- [ ] `minimap on` — a small map appears alongside the room view on every
      `look`/move; `minimap off` removes it; `minimap` shows the current state.
      The toggle persists across logout.
- [ ] (GMCP, §18) `Room.Info` carries coordinates; a Mudlet-style client can
      drive its native mapper from them.

---

## Notes / known gaps (already understood)

- **Combat only happens in the Meadow.** Town Square is a safe-room; the bandit
  in the Meadow is the intended target.
- **Passwords** for Jasrags/Bob are whatever was set when their accounts were
  created. If unknown, delete `saves/accounts/<id>/` + `saves/players/<name>/`
  and re-create, or just make a fresh character.
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
