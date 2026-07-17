# Client feature parity matrix — Telnet/Mudlet vs. Web

> Tracking doc for **feature parity across client surfaces.** It answers one
> recurring question: for a given "modern MUD client" feature, does the *server*
> already provide it (so every client gets it for near-free), or is it a
> client-side engine that must be built once **per surface**?
>
> Companion docs: [`web-client-plan.md`](../themes/web-client-plan.md) (the
> additive-GMCP server contract) and [`tab-completion-gmcp.md`](./tab-completion-gmcp.md)
> (the `Input.Complete` wire format).

## The two clients, and why GMCP is the pivot

There are two distinct "clients," and the distinction drives the whole matrix:

- **Telnet + a MUD client (Mudlet/TinTin++/MUSHclient).** The server provides a
  thin assist (char-mode line editing, tab-completion, prompts, GMCP), but the
  heavy conveniences — history, aliases, triggers, scrollback, logging — come
  from **the user's own software.** The server never had to implement them.
- **Web client.** *You are the terminal.* There is no third-party client
  underneath, so every convenience a Mudlet user gets from Mudlet must be built
  **natively** in the web client.

**The parity lens:** anything carried on a **GMCP package** is consumable by
*both* Mudlet (via a package/mapper script) and the web client — so parity there
is cheap and shared. Anything that is a **client-side input/output engine**
(aliases, triggers, macros) has no GMCP representation today and must be built
once per client — *unless* it is promoted server-side (see
[the open fork](#open-decision--where-do-aliases-triggers-macros-live)).

## The authoritative GMCP surface (what the server emits today)

Sourced from `internal/gmcp`:

```
Char.Login  Char.Status  Char.StatusVars  Char.Vitals  Char.Combat
Char.Effects  Char.Experience  Char.Inventory  Char.Items.List
Char.Recipes  Char.Shop  Char.Quests  Char.Trade  Char.Auction
Char.Commands  Char.Wizard
Room.Info  Room.Map
Comm.Channel.Text
Input.Complete  Input.Complete.List
```

**Legend for the GMCP/Mudlet column:**
`GMCP <pkg>` = on the wire, both clients consume · `Mudlet-native` = Mudlet ships
a client engine for it, web must build its own · `char-mode` = server
line-editor assist (`internal/conn/telnet/lineedit.go`).

---

## Bucket A — Server owns the logic (GMCP-carried → parity is cheap)

The hard part lives server-side; each client only surfaces it.

| Feature | Engine (server) | GMCP / Mudlet | Web client |
|---|---|---|---|
| **Autocomplete** | `internal/command/complete.go` (kinds: verb/item/entity/door/bulk/quest) | `GMCP Input.Complete` + `char-mode` Tab; Mudlet also has native tab-complete | ❌ not wired — surface `Input.Complete` |
| **Chat / comms pane** | chat system | `GMCP Comm.Channel.Text`; Mudlet routes to a chat window | ❌ currently no-op'd — render a pane |
| **Command palette / enumeration** | command registry | `GMCP Char.Commands` | ❌ feed autocomplete / palette |
| **Room / map / minimap** | `internal/world` + coords | `GMCP Room.Info/Map`; Mudlet mapper consumes Room.Info | ✅ shipped |
| **Vitals / combat / xp / effects** | respective services | `GMCP Char.Vitals/Combat/Experience/Effects`; Mudlet gauges | ✅ shipped |
| **Inventory / shop / quests / trade / auction** | respective services | `GMCP Char.Inventory/Shop/Quests/Trade/Auction` | ✅ shipped (13 panels total) |

## Bucket B — Client-side engines (Mudlet ships them; web builds natively)

No server representation today. Mudlet users get these from Mudlet; the web
client has to implement each one. **This bucket is the subject of the
[open fork](#open-decision--where-do-aliases-triggers-macros-live).**

| Feature | Engine (server) | GMCP / Mudlet | Web client |
|---|---|---|---|
| **Command history** | none (`lineedit.go`: *"no history yet"*) | Mudlet-native | ⚠️ in-memory only — add localStorage |
| **Aliases** (`k = kill $1`) | none (`registry.go`: *"no aliases"*) | Mudlet-native alias engine | ❌ build |
| **Macros / hotkeys / numpad move** | none | Mudlet-native key bindings | ❌ build client keymap |
| **Speedwalk** (`4n2e`) | none | Mudlet-native (+ mapper/Room.Info) | ❌ client-side expansion |
| **Triggers / highlights** (regex → color/notify) | none | Mudlet-native trigger engine *(its defining feature)* | ❌ high effort |
| **Split scrollback** | n/a | Mudlet-native split screen | ❌ build |
| **Session logging / export** | n/a | Mudlet-native logging | ❌ build |
| **Sound / notifications** | n/a | Mudlet-native (`playSoundFile`); *could* be GMCP-triggered | ❌ build |

## Bucket C — Presentation (no telnet analog; Mudlet ≈ Geyser UI)

| Feature | GMCP / Mudlet | Web client |
|---|---|---|
| Panel HUD / clickable commands | Mudlet Geyser + GMCP | ✅ shipped |
| Settings / theme persistence | Mudlet profile | ❌ localStorage layer (Bucket B prerequisite) |
| Portraits | Mudlet via GMCP + image display | ⏳ remaining |
| Responsive / mobile + on-screen movement pad | n/a (desktop) | ⏳ remaining |

---

## Resolved decision — power-user features are server-side + GMCP

**STATUS: RESOLVED (2026-07-17) → server-side + GMCP.** Aliases, macros, and
speedwalk are pushed into the **player save** and exposed over a new GMCP
package (e.g. `Char.Aliases`), so Mudlet and web get identical behavior and the
config **follows the player across clients.** This is the AnotherMUD-flavored
path — consistent with the "server owns behavior" design — accepting the cost of
a new persisted player-save surface (version bump + migration).

**Division of labor (the rule for every Bucket B row):** the **server owns the
definition + storage + expansion** (the alias table, the speedwalk grammar, the
trigger *patterns*); each **client owns execution/rendering** (matching an
incoming line against a trigger, painting the highlight, playing the sound). For
aliases/macros/speedwalk that means the whole feature effectively roams, because
expansion happens server-side before dispatch. For **triggers/highlights**, the
*patterns* roam via GMCP but the match-and-render still runs per client — so the
web client and any Mudlet package still each need a small rendering hook. That is
the one place "server-side" doesn't buy 100% shared code, and it's expected.

**Implications to sequence:**
- New player-save surface + migration (version bump) for the alias/macro/speedwalk
  store — the first server-side slice.
- A new GMCP package (`Char.Aliases` or similar) carrying the store to clients;
  add it to the authoritative surface list above once it ships.
- Alias **expansion** happens in the server command pipeline (before dispatch),
  preserving the authority invariant — clients still only ever send plain commands.
- Web + Mudlet rendering hooks for trigger execution remain per-client (small).

## Sequencing takeaway

1. **Bucket A wins first** — autocomplete (`Input.Complete`) and the chat pane
   (`Comm.Channel.Text`) are the best value-per-effort on the whole list: the
   server already does the hard part and the packages are being left on the table.
2. **Cheap Bucket B** — persisted history + aliases + speedwalk are self-contained
   (localStorage, little/no server work under Option 1).
3. **Settings/localStorage layer** (Bucket C) is the quiet prerequisite the rest
   of Bucket B hangs off — build it as scaffolding early.
4. **Heavy Bucket B** — triggers, split scrollback, notifications — real work,
   for when courting power users.
