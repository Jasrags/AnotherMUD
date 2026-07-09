# AnotherMUD Playtest Guides

Manual QA checklists for verifying live behavior. The guide is split **by
world/boot**, because "which server invocation + which character" is the real
unit of a playtest session — one file, one boot, no mid-session restarts.

| Guide | Boot | Covers |
|---|---|---|
| **[core.md](core.md)** | `make run` (starter-world) | The engine-mechanics showcase on the fantasy fighter demo — login, movement, items, combat, loot, progression, economy, quests, social, doors, light, crafting, gathering, maps, saves, conditions, skills, movement cost, visibility, feats, mounts, admin, GMCP, tab-completion. |
| **[wot.md](wot.md)** | `make run-wot` (Wheel of Time) | Channeling (the One Power), masterwork item grades, ranged combat (projectile / range bands / cross-room), faction & standing, reputation & renown. |
| **[shadowrun.md](shadowrun.md)** | `ANOTHERMUD_PACKS=shadowrun …` | The Street Samurai MVP — creation on the eight SR primaries, lethal vs. stun combat, firearms & ammo, cyberware, the nuyen shop, karma advancement. |

## Section numbers are guide-wide anchors

Sections are numbered **once across the whole guide** and don't renumber per
file, so `§6` is always Combat and external references (live-test comments,
`docs/BACKLOG.md`) stay stable. Each file therefore carries a **non-contiguous**
slice of the sequence:

- **core.md** — §0–§26, §28–§31, §36
- **wot.md** — §27, §32, §33 (ranged, both boots), §34, §35
- **shadowrun.md** — §37–§43

(§33 "Ranged combat" is one chapter in **wot.md**: its *thrown* half runs on the
default boot, its *projectile/range-band* half on the WoT boot.)

## Shared conventions

- **Format:** `- [ ] command` — what should happen. Mark `[x]` on pass; add a
  `BUG:` note inline on fail. File the real ones into `docs/BACKLOG.md` or a
  `m<N>-deferred-fixes` memory afterward.
- **Live reload while bug-hunting:** `make watch` rebuilds + restarts on any
  `.go`/`.yaml`/`.lua` save (~1s); player saves persist across the restart, so
  reconnect and resume. One-time: `go install github.com/air-verse/air@latest`.
- **Fast-testing env (optional):** several features are timer-driven — launch
  with shorter timers so you don't wait. See each guide's §0/setup for the knobs
  it needs (e.g. `ANOTHERMUD_CORPSE_LIFETIME`, `ANOTHERMUD_FORAGE_COOLDOWN`,
  `ANOTHERMUD_LINKDEAD_TIMEOUT`). All `ANOTHERMUD_*` knobs also read from a local
  `.env` (see `.env.example`).
- **Self-provisioning:** no pre-built saves are assumed — each guide boots with
  an admin seed (or auto-admin first character) and creates the characters it
  needs.

Each guide opens with its own boot block, character provisioning, and world map;
start at whichever world you want to exercise.
