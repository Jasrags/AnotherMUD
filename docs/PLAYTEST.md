# AnotherMUD Playtest Guide — moved

The playtest guide is now split **by world/boot** under
[`docs/playtest/`](playtest/README.md), because each world is a separate testing
session (its own server boot and its own character):

- **[docs/playtest/core.md](playtest/core.md)** — engine mechanics on the
  core/starter-world demo (`make run`): §0–§26, §28–§31, §36.
- **[docs/playtest/wot.md](playtest/wot.md)** — Wheel of Time (`make run-wot`):
  channeling (§27), masterwork (§32), ranged combat (§33), faction (§34),
  reputation (§35).
- **[docs/playtest/shadowrun.md](playtest/shadowrun.md)** — the Street Samurai
  MVP (`ANOTHERMUD_PACKS=shadowrun …`): §37–§43.

Section numbers are guide-wide anchors — `§6` is always Combat, wherever it
lives. Start at [the index](playtest/README.md).
