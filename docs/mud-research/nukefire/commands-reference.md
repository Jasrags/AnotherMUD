# NukeFire — Commands Reference

Source: https://nukefire.org/wiki/ command/utility topic pages
Fetched: 2026-06-18

Deep-read topics for this doc: `auction`, `bid`, `aucstat`, `aucstatlive`, `auctalk`, `whatsauc`, `showauction`, `endauction`, `offer`, `consider`, `autoloot`, `afk`, `aliases`, `damprompt`, `assist`, `areas`, `ammo`, `ammobin`, `ammo-efficiency`, `activate-portal`, `helpfind`, `autosplit`, `autogold`, `banks`, `shops`, `buy`, `sell`, `identify`, `gmcp`, `score`, `group`. Other commands in the tables below are characterized from the index categories (see `wiki-help-index.md`) and are index-only unless they appear in a deep-read section.

---

## Auction house

The Auction House sells items player-to-player for **credits**. An **Auctioneer** mob announces the current item and the next legal bid.

| Command | Purpose |
|---|---|
| `auction <item> <price>` | Start an auction for an item you carry |
| `bid <amount>` | Place a specific bid on the current auction |
| `bid min` | Bid the current minimum legal amount |
| `bid max <amount>` | Set a **proxy bid** — house bids in legal steps up to your max |
| `bid cancel` / `bid withdraw` | Cancel your buyer bid (when allowed) |
| `endauction [<item>]` | Pull your own item if you listed the wrong one / must leave |
| `whatsauc` | List current auctions + the auction queue (item, current bid, seller) |
| `showauction` / `showauc` | Show the single item currently up for auction |
| `aucstat` | Ledger-style view of auctioned items + compare signals |
| `aucstatlive` | Live item detail while an auction is active |
| `auctalk <msg>` | Auction-house chat channel (alias of `gossip`/`grats`; needs ≥L3) |

**Bidding notes:** credits are spent **only if you win**; if outbid past your `bid max`, you must bid again. Some **auction-compare signals** (UP/EVEN/DOWN vs your current gear) surface in `whatsauc`/`aucstat`.

> `offer` is **not** an auction verb here — it's an inn/rent alias ("rent is free, just quit").

---

## Economy: shops, banks, money

### Shops
| Command | Purpose |
|---|---|
| `list` | Items for sale in the current shop |
| `list upgrades` / `list up` | Only items that look like upgrades for you |
| `tradelist` | What item types this shop will buy |
| `buy <item>` / `buy #<n>` / `buy <n>.<item>` / `buy <count> <item>` | Purchase by name/number/quantity |
| `identify <n>` / `shop-identify` | Identify a numbered shop item before buying |
| `value <item>` | What the shopkeeper would pay |
| `sell <item>` | Sell to the shopkeeper |
| `sellall` | Sell everything the shop buys (taxed) |

**Power markers** in shop listings: `UP` upgrade · `EVEN` sidegrade · `DOWN` weaker · `NO` can't use · `--` not comparable. Guides only — `identify` expensive gear first.

### Banks (Bank of Ilniyr, any branch)
| Command | Purpose |
|---|---|
| `deposit` / `deposit <n>` | Deposit all / a specific amount |
| `withdraw <n>` / `withdraw` | Withdraw amount / all |
| `balance` | Show bank balance |
| `pay <target>` | Give all carried credits to a player/mob |
| `autodeposit` | Auto-bank looted credits (toggle) |

### Money loot toggles
| Command | Purpose |
|---|---|
| `autogold` | Auto-take gold/credits from kills (no items) |
| `autosplit` | Auto-split kill money among group members |
| `gold` | Show carried credits |
| `donate` / `junk` / `rsell` | Dispose/sell items (`rsell` = remort-sell, doubled by a Class Legacy rune) |

Currency is **credits** throughout. Other markets: `stocks`/`stockbuy`/`stocksell`, `questpoints`.

---

## Firearms & ammo

NukeFire models real-world calibers: .22, 9mm, .38, .45, .44 Magnum, .357, 20/12/10 gauge, .410, 10mm, 20mm, 5.56mm, 7.62mm, .308, 30.06, .50 BMG.

| Command | Purpose |
|---|---|
| `reload` | Reload equipped + carried guns (incl. in containers); draws bin first, then mags |
| `unload` | Unload a weapon |
| `stow <ammo\|mag\|all\|caliber>` | Break ammo/mags into your numeric **ammo bin** (empty mags ignored; partial mags keep their rounds) |
| `ammoretrieve <caliber> [mag\|round\|<count>]` | Pull rounds/mags back out of the bin |
| `ammo` | Show ammo-bin contents (colored per-caliber panel + total) |
| `topoff` | Refill wielded guns quickly from the bin |
| `shoot` / `shoot2` | Fire at a target |

**Ammo Efficiency** (`ammo-efficiency`) is an item apply: each successful shot rolls a small chance to **not consume a round** (checked per projectile incl. bursts; can't overfill; stacks). Guns may **auto-reload** mid-combat from mags then the bin — keeping the bin stocked avoids "going dry."

---

## Movement, navigation, world

| Command | Purpose |
|---|---|
| `areas` | List suggested places to visit (`help GPS` for routing) |
| `map` / `automapping` / `autobigmap` | ASCII map + auto-display toggles |
| `gps` / `gpsmap` / `gpsdifficulty` | Routing system (destination/next-step/route string; GMCP-exposed) |
| `run` / `longwalk` | Speedwalk / class-change long walk |
| `track` / `autotrack` | Track a target through the world |
| `scan` | Look into adjacent rooms |
| `exits` / `autoexits` | Show room exits / auto-show |
| `enter` / `portal` / `activate-portal` | Portal traversal |
| `where` / `locate` / `locate-object` | Find players/objects |
| `recall` (`word-of-recall`) | Return to recall point |

**Activate Portal** (`activate-portal` / `activateportal`, Voidwarp-tied) activates a room portal and may move/affect nearby characters.

---

## Group & social

| Command | Purpose |
|---|---|
| `group [new\|join <p>\|leave\|option <o>\|kick <p>\|list]` | Group management; XP shared equally among same-room members at kill time |
| `assist <player>` | Join a player's fight |
| `autoassist` / `autorescue` / `norescue` / `rescue` | Combat support toggles |
| `gsay` / `gprompt` / `groupprompt` / `showgroup` | Group comms/prompt |
| `groupskills` / `gskills` | Show unlocked group powers (see classes-and-progression.md) |
| `passlead` | Pass group leadership |
| `auctalk` / `gossip` / `grats` | Public channels (≥L3) |
| `say` / `reply` / `notell` / `qsay` | Local + tell channels |
| `emotes` / `gemotes` / `socials` | Emotes/socials |
| `who` / `whoami` / `whois` / `whopower` / `whosafk` | Player listings |
| `report` | Report your HP/Mana/Move to the group |

---

## Combat tuning, prompts, info

| Command | Purpose |
|---|---|
| `consider <mob> [why\|full]` | Threat read: label + 0–100 danger meter, dmg edge, staying power, tempo (NPCs only) |
| `huntme` / `countmob` / `mobcount` | Combat target helpers |
| `score` | Status: age, HP/Mana/Move, AC, alignment, XP, playtime, level (money via `gold`) |
| `damprompt` | Toggle damage prompt (only in Training Area) |
| `prompt` / `promptbars` / `promptdiag` / `promptexit` / `promptopponent` / `gprompt` | Prompt customization |
| `damcon` / `optimized-damcon` / `diagnose` | Damage-control / mob diagnosis |
| `fightspeed` / `hpr` / `hmvpct` / `dps` | Combat-math readouts (see combat-mechanics.md) |
| `wimpy` / `flee` / `kill` / `murder` | Combat entry/exit |

---

## Items & inventory

| Command | Purpose |
|---|---|
| `inventory` / `equipment` / `eqstat` / `eqdiff` / `gearcheck` | Carried/worn gear |
| `compare` / `autocompare` / `zcompare` | Compare items |
| `get` / `getdeep` / `put` / `putdeep` / `give` / `drop` / `grab` | Item movement (deep = into nested containers) |
| `wear` / `wield` / `rewear` / `remove` | Equip |
| `examine` / `look` / `looktat` / `contents` | Inspect |
| `combine` / `socket` / `upgrade` | Item crafting/modification |
| `inscribe` / `uninscribe` / `candletrack` | Item inscription + tracking |
| `binding` / `bond` / `buy-bind` / `buy-bond` / `buy-rebond` / `buy-unbind` / `dbid` | Item soulbinding economy |

### Autoloot
`autoloot <value>` — auto-loot from corpses during combat cleanup. **TRASH** always ignored; **KEY**, **MATERIAL**, and **inscribed** items always looted; items below your threshold skipped. `autoloot 500` → loot anything ≥500 credits; bare `autoloot` toggles on/off without changing the value.

---

## QoL / client / utility

| Command | Purpose |
|---|---|
| `aliases` / `alias <name> <cmd>` | Command shorthands; **simple** (no vars/`;`) vs **complex** (`$1`–`$9`, `$*`, `;`-chains); cannot self-call/loop; `[S]`/`[C]` legend in listing |
| `helpfind <string>` | Substring/prefix search across the entire help index (case-insensitive) |
| `afk` | Away flag in prompt + under `who`; notifies of waiting mail on return |
| `idle` / `toggles` / `brief` / `compact` / `norepeat` / `spam` | Display/idle toggles |
| `highlight` / `trigger-control` / `trigger-mastery` / `bindings` | Client-side triggers/keybinds |
| `color` / `screenwidths` / `pagelengths` / `pager` / `screenreader` | Terminal config |
| `gmcp` | Generic MUD Communication Protocol: structured channel/vitals/Devotion/affects/GPS data for modern clients |
| `mxp` | MUD eXtension Protocol support |

### Aliases — full syntax (from `aliases`)
- `alias` — list all (sorted, `#`/Type/Alias→Replacement).
- `alias <name> <command>` — create/update.
- `alias <name>` — delete.
- **Variables:** `$1`…`$9` positional, `$*` = full tail. Example: `alias killem sling 'fireball' $1; sling 'harm' $2`.
- **Chaining:** `;` runs multiple commands (`alias prep stand; cast 'armor' self; cast 'bless' self`). Some clients need `;` escaped.

---

## Relevance to AnotherMUD

- **Ammo-bin abstraction** (numeric stowed pool + retrieve + auto-reload-from-bin) is a clean inventory pattern that avoids partial-magazine clutter — relevant if AnotherMUD ever adds ammunition.
- **Auction proxy-bidding** (`bid max`) and **auction-compare signals** (UP/EVEN/DOWN) are richer than AnotherMUD's current M29 auction house; the compare-vs-current-gear signal is a notable UX add.
- **`consider` with a 0–100 danger meter + why/full breakdown** is a strong "threat read" UX worth comparing to AnotherMUD's combat consider.
- **Autoloot value-threshold + flag rules** (always-loot KEY/MATERIAL/inscribed, never-loot TRASH) is a tidy policy model.
- **`helpfind` substring help search** maps onto AnotherMUD's help system + tab-completion work.
