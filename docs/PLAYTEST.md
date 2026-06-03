# AnotherMUD Playtest Guide

A manual QA checklist for verifying implemented features (M0–M22). Work
top-to-bottom or jump to a section. Each step gives a **command** and the
**expected behavior**; tick the box when it matches, and note anything that
doesn't.

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
        Hearthwick Forge (Maerys, trainer)        [indoors]
              |  down: oak door (plain)
        Forge Cellar (iron key)                   [underground]
              |  down: iron door (locked)
        Forge Vault (coins)                       [underground]

        Hearthwick Forge
              |
   Market Row — Town Square — Village Gate — Long-Grass Meadow
   (merchant)   (safe hub,        (wilderness)   (bandit — combat arena)
                 well, gear)
```

- **Town Square** is a `safe-room` — combat is blocked here. The **Meadow**
  (south through the Gate) is the one place you can fight: a hostile **road
  bandit** spawns there with loot + coins.
- Below the Forge is a **door test branch** (§12): `down` through a plain
  oak door to the **Forge Cellar**, then `down` through a *locked* iron
  door (key in the cellar) to the **Forge Vault**.

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
- [x] `south`, `east`, `west` — move between Forge/Square/Market/Gate.
- [x] `s` from the Gate — you reach the Meadow (the bandit is here).
- [x] Try a direction with no exit (e.g. `up`) — "You cannot go that way."
- [x] When another player is present, you see "X arrives" / "X leaves" as they
      move (covered in §11).

## 4. Items & inventory

In Town Square:

- [x] `get sword` — picks up the short sword; `inventory` (`i`) lists it.
- [x] `get cap`, `get ration`, `get sack`, `get waterskin` — all enter inventory.
- [x] `get coins` — credits **gold** (currency auto-convert), does *not* add an
      item; `gold` shows the new balance.
- [x] `drop ration` then `get ration` — drops to the room and picks back up.
- [x] `equip sword`, `equip cap` — `equipment` (`eq`) shows them worn; `consider`
      reflects the stat/AC change.
- [x] `unequip sword` — returns to inventory.
- [x] Duplicate items show stacked: pick up two rations → `i` shows
      `a trail ration (x2)`.
- [x] `get 2.ration` style ordinals resolve the Nth match (try with two of a
      kind on the ground).

## 5. Containers

- [x] `fill waterskin` (in Town Square, by the well) — fills it from the well.
- [x] `put sword in sack` — sword moves into the sack.
- [x] `look in sack` — lists the sack's contents.
- [x] `get sword from sack` — takes it back out.
- [ ] (Corpse-as-container is covered in §7.)

## 6. Combat

Go to the **Meadow** (`s` from the Gate). The bandit is hostile.

- [x] `consider bandit` (`con bandit`) — shows its HP/condition and AC.
- [x] Entering the Meadow, the bandit aggros (attacks you) — combat rounds tick;
      you see hit/miss/damage lines for both sides.
- [x] `kill bandit` also initiates combat if not already engaged.
- [x] In **Town Square**, `kill <anyone>` is refused — "safe room"
      (combat blocked in the hub).
- [x] `flee` — escapes combat to an adjacent room (when there's an exit); you
      see the new room rendered, and others see "X flees to the <dir>!".
- [x] `wimpy 30` then fight — at ≤30% HP you auto-flee.
- [ ] Let the bandit kill a low-HP character (use Bob unarmed) — on death you
      respawn (healed) at the recall/start room, not disconnected.
- [x] `cast slash` / `cast kick` in combat — the ability fires (Jasrags has them).

## 7. Loot & corpses

After killing the bandit (Meadow):

- [x] A **corpse** appears in the room ("the corpse of a road bandit").
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

- [ ] `score` (`sc`) — your character sheet: race/class/level, HP/MA/MV, the six
      attributes, AC/hit, alignment, gold, sustenance tier, and XP-to-next.
- [ ] `consider` with no target (or `consider me`) — points you to `score` now
      (self stats moved there); `consider <target>` still sizes up that target.
- [x] `abilities` (`abi`) — lists learned abilities + proficiencies.
- [ ] `train str` — spends a train credit, raises STR (Jasrags has credits).
- [ ] At the Forge, `practice slash` (Maerys teaches slash/parry) — raises the
      ability's cap.
- [ ] `cast bless` / `cast heal` — self buff/heal; resolves on the next
      pulse whether in combat or idle (out-of-combat drain). bless bumps
      AC/hit (check `score`); heal restores HP (only visible if injured —
      take a hit first).
- [ ] (Admin) `xp 500` — grants XP; crossing a threshold levels you up with a
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

- [ ] At the Forge, `talk master` (`ask master`) — Maerys lists her offers
      (**Forge Errand**, **Gate Patrol**), each with its pitch and an
      `accept <name>` line. This is how you discover a quest without already
      knowing its name.
- [ ] `quests` (`journal`) before accepting — "no active quests."

### Auto-grant quest — Forge Errand (reward on the spot)

- [ ] `accept Forge Errand` — acceptance banner; `quests` lists it active.
- [ ] (If you already hold a trail ration, `drop ration` first.) `get ration`
      in Town Square — you see a progress line, then the quest **completes
      immediately**: a "Quest complete: Forge Errand" banner listing 25
      experience + 5 gold. No return trip. `gold` reflects the payout.
- [ ] Repeatable: `accept Forge Errand` again works.

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

- [ ] Accept either quest, then `abandon <name>` — it drops from the journal.

## 11. Social / multi-session (Jasrags + Bob, two windows)

- [x] Both in Town Square — `look` lists the other in "You see here:";
      movement shows "Bob arrives" / "Bob leaves".
- [x] `tell Bob hello` — Bob receives it; `reply hi` goes back; `tells` shows
      the history.
- [x] `channels` (`chanlist`) — lists channels; post on one and the other sees
      it; `chathistory` (`chhist`) shows scrollback.
- [x] `emote waves` (`pose`) — the room sees "Jasrags waves".
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

- [ ] Change state (move, pick up items, spend gold, take damage), `quit`,
      reconnect — everything is as you left it.
- [ ] Restart the **server** (Ctrl-C, `make run` again), log back in — character
      state survived (corpses/weather/mobs reset by design; player save does not).

## 17. Admin verbs (Jasrags — already admin)

- [ ] `inspect bandit` (in the Meadow) — full diagnostic record of the target.
- [ ] `restore` / `restore Bob` — refills vitals to full **and** tops off
      sustenance (hunger/thirst); the reply notes "fully fed" for a player target.
- [ ] `set vital hp <target> 1` — sets a field on a target (then `restore`).
- [ ] `teleport meadow` (`goto meadow`) — jump to a room by id; `goto Bob`
      jumps to a player.
- [ ] `purge bandit` — removes the mob from the world.
- [ ] `announce Server test in progress` — all connected players see the
      broadcast.
- [ ] `grant builder to Bob` then `revoke builder from Bob` — role changes (Bob
      sees nothing player-facing; verify via `inspect Bob`).
- [ ] `xp 1000` — grants XP to yourself (admin probe).
- [ ] `reload` — reloads pack Lua scripts (watch the log for the reload count).
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

- [ ] Kill the bandit and watch the server log — `track_kills.lua` logs a
      `kill: …` line (and a scheduled follow-up ~3s later).
- [ ] As admin, `reload` — the log shows scripts re-discovered/reloaded.

## 20. Tab-completion

Tab-completion **Phase 0** is the server-side enumeration substrate: given a
partial line, it returns the candidates for the token you're typing. There's no
live TAB key yet (raw telnet can't do TAB without a future char-mode surface) —
so it's exposed two ways: the player **`suggest`** verb (line-mode, anyone) and
the admin **`complete`** debug verb (raw candidate dump).

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

---

## Notes / known gaps (already understood)

- **Combat only happens in the Meadow.** Town Square is a safe-room; the bandit
  in the Meadow is the intended target.
- **Passwords** for Jasrags/Bob are whatever was set when their accounts were
  created. If unknown, delete `saves/accounts/<id>/` + `saves/players/<name>/`
  and re-create, or just make a fresh character.
- Time/weather, corpse decay, idle, and link-dead are **timer-driven** — use the
  fast-testing env above to see them quickly.
- **Tab-completion (§20) is server-side substrate only** — there is no live TAB
  key yet (that's a future client surface). The admin `complete` verb is how you
  inspect it. Argument completion only lights up for verbs that declare their arg
  types; most do, and `get`/`take`/`kill`/`look`/`consider` now do too. A few
  still don't (e.g. `unequip`, `fill`, `buy`/`sell`/`value`) — tracked in
  `docs/BACKLOG.md` §2.
- Record any mismatch as a `BUG:` note next to the step; file the real ones into
  `docs/BACKLOG.md` or a `m<N>-deferred-fixes` memory afterward.
