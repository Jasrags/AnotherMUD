# AnotherMUD — Web Client (P1)

A pure-browser client over the **existing** WebSocket + GMCP wire
(`internal/conn/ws`). It is the **P1 "superset baseline"** from
[`docs/themes/web-client-plan.md`](../../docs/themes/web-client-plan.md): a
browser *view* that sends `command` envelopes, renders `text` frames (ANSI →
HTML), and consumes the engine's GMCP packages into a HUD — with **zero server
changes**.

No build step, no dependencies — three static files (`index.html`, `app.css`,
`app.js`).

## What it shows

- **Terminal** — full ANSI colour (16-colour + 24-bit truecolour), scrollback,
  command history (↑/↓), local echo, and password masking (WebSocket has no
  telnet echo negotiation, so the client masks locally when the prompt asks for
  a password).
- **HUD panels**, driven live by GMCP:
  - **Vitals** — HP + every resource pool from `Char.Vitals`, including the
    generalized `pools` map (a WoT channeler's One Power via `mana`, a Shadowrun
    runner's `essence`/`stun`). Renders any world's resources without hardcoding.
  - **Combat** — target + target HP from `Char.Combat`.
  - **Location** — name/area/terrain/light/coords from `Room.Info`, clickable
    exit chips, and a **neighbourhood map**. The map is driven by the `Room.Map`
    package (P2): the server-computed local graph including rooms you can *see
    but haven't entered* (drawn hollow — fog of war). **Click any room to walk
    there** — the client paths on the graph and sends move commands step by step,
    so a locked door just stops the walk. Without `Room.Map` (a baseline server)
    it degrades to a visited-only minimap.
  - **Inventory** — carried + worn items from the `Char.Inventory` package (P3),
    mirroring the in-game `inventory`/`equipment` verbs: the full worn-slot
    layout (empty slots included), carried items with **stack counts** (a
    crossbow bolt ×18), a **mechanical detail** line (a clip's `15/15 APDS`,
    armor's `Armor 4`, cyberware's `+1 Intuition`, a wielded gun's `7 rds APDS`),
    and per-item **action buttons** (`equip`/`unequip`/`drop`/`reload`/`load`).
    Each action carries its full command, so a click sends exactly what a player
    would type (the authority invariant; no new server verb). A rich superset of
    `Char.Items.List`; a baseline client ignores it.
  - **Crafting** — the craft form from the `Char.Recipes` package (P3 Slice B),
    mirroring the in-game `craft` verb: each known recipe with its ingredients
    (have/need counts, shortfalls in red), the required station + skill gates,
    and a **Craft** button that greys out when the recipe isn't makeable now
    (with the reason). The button sends the full `craft <recipe>` command — a
    click sends exactly what a player would type (the authority invariant; no
    new server verb). The panel hides itself for a character who knows no
    recipes. A rich superset of the `craft` listing; a baseline client ignores it.
  - **Shop** — the trade form from the `Char.Shop` package (P3 Slice B+), shown
    only when the player stands at a shop (`open`). Two columns: the shop's stock
    to **buy** (greyed when unaffordable) and the player's items to **sell**
    (grouped with a qty), each row a button carrying its full `buy <token>` /
    `sell <token>` command — a click sends exactly what a player would type (the
    authority invariant; no new server verb). Prices are pre-formatted
    server-side via the world currency (¥ vs gold). A rich superset of
    `list`/`value`; a baseline client ignores it.
  - **Effects** — active effects from `Char.Effects`.
  - **Progression** — per-track level/XP bars from `Char.Experience`.
  - **Identity** — name/account/race/class from `Char.Login` + `Char.Status`.

Everything is a view: clicking an exit just sends the `north`/`south`/… command
the server already accepts. The client holds no game logic (the P1 authority
invariant).

## Run it

### The quick way — `make run-web`

```bash
make run-web            # starter-world + WebSocket on :4001, opens the client
```

This boots the engine with the WS listener on `:4001`, relaxes the origin check
for local dev, and opens `index.html`. Just press **Connect** (the default URL
`ws://localhost:4001/mud` already matches). For a themed world's richer HUD,
compose the flag with any world target:

```bash
make run-shadowrun WS_ADDR=:4001   # essence + stun bars
make run-wot        WS_ADDR=:4001   # the One Power (create a channeler)
```

Under live-reload (`air`), use the `watch-web` family — the WebSocket listener
survives every rebuild, so keep the browser open and reconnect:

```bash
make watch-web                 # starter-world, live-reload + WS on :4001
make watch-web-wot             # WoT
make watch-web-shadowrun       # Shadowrun
# or compose the flag with any watch target: make watch-wot WS_ADDR=:4001
```

### The manual way

### 1. Start the server with the WebSocket listener on

The WS listener is **off by default**. Enable it (and, for a browser opened from
`file://` or a different port, relax the origin check for local dev):

```bash
ANOTHERMUD_WS_ADDR=:4001 \
ANOTHERMUD_WS_INSECURE_SKIP_VERIFY=true \
go run ./cmd/anothermud
```

- `ANOTHERMUD_WS_ADDR=:4001` — the WebSocket listen address (empty = disabled).
- `ANOTHERMUD_WS_PATH=/mud` — the route (default `/mud`).
- `ANOTHERMUD_WS_INSECURE_SKIP_VERIFY=true` — **dev only.** Skips the WS origin
  check so a page served from `file://` (origin `null`) or a static file server
  on another port can connect. In production set `ANOTHERMUD_WS_ORIGINS` to your
  real origin(s) instead.

For a Shadowrun or WoT boot (to see `essence`/`stun` or the One Power on the HUD):

```bash
ANOTHERMUD_PACKS=shadowrun ANOTHERMUD_WS_ADDR=:4001 \
ANOTHERMUD_WS_INSECURE_SKIP_VERIFY=true go run ./cmd/anothermud
```

### 2. Open the client

Either just open the file:

```bash
open clients/web/index.html        # macOS
```

…or serve the folder (any static server works):

```bash
cd clients/web && python3 -m http.server 8080
# then browse http://localhost:8080
```

### 3. Connect

The URL field defaults to `ws://localhost:4001/mud`. Press **Connect**, then log
in through the normal prompts (create an account / character just like telnet).

## Notes / limits (P1)

- **`ws://` is unencrypted.** For anything past localhost use `wss://` behind a
  TLS terminator; see the WebSocket TLS/rate-limit deferral (`m16-5`).
- **Enriched packages, so far.** Four rich additive packages are surfaced beyond
  the baseline: `Room.Map` (the neighbourhood map, P2), `Char.Inventory` (the
  structured inventory panel, P3 Slice A), `Char.Recipes` (the craft form, P3
  Slice B), and `Char.Shop` (the trade form, P3 Slice B+). Still on the wire but
  not yet surfaced: `Char.Items.List` (superseded by `Char.Inventory` for the
  panel), `Char.StatusVars`, `Comm.Channel.Text`, and `Char.Wizard` — dispatched
  to a no-op, so they never error. Direct-trade + auction **forms** follow the
  same concrete-package + plain-command-submit shape — see the plan doc.
- **Password masking is a heuristic** (the prompt text mentions "password").
  It's a local convenience, not a security boundary.
