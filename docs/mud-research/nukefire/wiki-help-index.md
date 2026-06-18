# NukeFire — Wiki Help Index (all 817 topics)

Source: https://nukefire.org/wiki/ (the full help-wiki index) + per-topic pages at `https://nukefire.org/wiki/<topic>`
Fetched: 2026-06-18

---

## How this index was built

- The complete topic list (817 entries) was scraped from the live help wiki at `https://nukefire.org/wiki/`.
- Each topic carries one of **four** category badges in the wiki HTML: `reference` (544), `skills` (155), `commands` (116), `building` (2). Those badge groupings are reproduced verbatim below — they are NukeFire's own taxonomy, not ours.
- Within the big `reference` and `skills` groups, NukeFire does not sub-categorize, so this doc adds a **thematic cross-reference** (below) grouping topics by what they actually are (classes, magic schools, gun/ammo, auction, etc.). The thematic groups were assembled by topic-name inspection plus spot-checking representative pages; they overlap and are a navigation aid, not an authoritative second taxonomy.

**URL pattern:** every topic resolves at `https://nukefire.org/wiki/<topic>`. Many topics also have "Also known as" aliases (e.g. `levels` ⇄ `level-range`, `levelrange`, `xp-levels`); the canonical slug is what is listed here. NukeFire **blocks WebFetch (403)** — fetch with `curl -A "<browser UA>"` (see the project task notes / other nukefire files for the recipe).

**Notation in this file:** topics listed below are *index-only* unless they were deep-read for one of the companion docs (`classes-and-progression.md`, `magic-and-skills.md`, `commands-reference.md`). Those deep-read topics are called out per-doc in each companion file's header.

---

## Thematic cross-reference (navigation aid)

### Base classes (10 playable today) + the older roster
`assassin` `barbarian` `curist` `cyborg` `infiltrator` `fanatic` `knight` `mutant` `ranger` `samurai` `slinger` `vagrant` `pirate`
- The `classes` page lists 10 "available to new players" but enumerates 13 base archetypes; `mage`/`healers`/`pirates`/`fanatics` exist as supporting/alias help.

### Prestige classes (9, unlocked via class remorts) — see `prestige`
`headhunter` `ninja` `wolfman` `kaiju` `heretic` `gypsy` `voidstriker` `outlander` `occultist`

### Progression / character system
`levels` `tnl` `expstart` `exp`(→levels) `remort` `remortskills` `remortskills3` `prestige` `classlegacy`/`runes`/`runremaining` `longwalk` `groupskills` `grouplevel` `alignment` `age` `titles` `score` `gstat` `impstat` `tatstat` `aucstat` `whatsmy` `boundstat` `skillapply`/`skills`/`practice`

### Magic / spells (cast & sling)
`cast` `nofocus` `focus` `spellpower` `magic-missile` `fireball` `firestorm` `lightning-bolt` `chill-touch` `shocking-grasp` `burning-hands` `color-spray`/`colorspray` `charm-person` `sleep` `blindness` `curse` `energy-drain` `disintegrate` `harm` `heal` `cure-light` `cure-critic` `cure-blind` `restoration` `rejuvinate` `invigorate` `sanctuary` `armor-spell` `bless` `augury` `identify` `locate-object` `animate-dead` `summon` `teleport` `word-of-recall` `control-weather` `detect-align`/`detect-invis`/`detect-magic`/`detect-poison` `protection-from-evil`/`protection-from-good` `dispel-good` `remove-curse`/`remove-poison`/`remove-invis` `enchant-armor`/`enchant-weapon`

### Class signature/prestige abilities (skills group)
`shadowform` `shadowclone` `utsusemi` `daggerdance` `deathlotus` `dragon-kick` `whirlwindstrike` `tornado` `arcane-torrent` `eldritch`(cataclysm) `suppress-fire`/`supfire` `chant`(occultist litanies) `mark-of-the-outer-dark` `nanoswarm` `neural-overload` `plasma-arc-storm` `psiattack`/`psiblast`/`psionicwave`/`mindcrush` `grow`(+`grow-brain`/`grow-claws`) `voidwarp`/`voidstep`/`voidpunch`/`voidforge`/`voidharvest` `wraithfire` `soulshredder` `phoenix-nova` `sacred-vengeance`/`sacredvengeance` `oathbreaker-s`/`oathscar` `barrier-of-valor` `war-banner`/`warbanner`

### Melee / martial skills
`kick` `bash` `backstab` `circle` `disembowel` `ambush` `bushwhack` `flurry` `parry` `rescue` `grapple` `headbutt` `bodyslam` `clothesline` `elbowstrike` `kneesmash` `legsweep` `spinkick` `roundhouse` `pound` `crush` `impale` `lunge` `garrote` `cutthroat` `pugilism` `barehand` `swordplay`/`advanced-swordplay`/`sword-mastery`/`sword-sweep` `knifeplay`/`advanced-knifeplay`/`knife-mastery` `dual`(wield) `stun` `subdue` `trample` `wallop` `strike`

### Firearms / ammo
`shoot`/`shoot2` `ammo` `ammobin`/`stow`/`ammoretrieve` `ammo-efficiency` `reload`(→ammo) `unload` `topoff` `marksmanship` `quickdraw` `double-tap` `ricochet` `skeetshot` `strafe` `spray-fire`/`supfire`/`suppress-fire` `covering-fire` `pistol-whip`/`pistolwhip` `weaponthrow`/`throw` `gun-sermon`/`gunsermon` `martyrshot` `sanctified-rounds`

### Auction house & economy
`auction` `bid` `aucstat` `aucstatlive` `auctalk` `whatsauc` `showauction` `endauction` `offer` `buy` `sell` `sellall` `value` `list`/`tradelist` `shops` `shop-identify` `banks`/`deposit`/`withdraw`/`pay`/`balance` `autodeposit` `gold` `autogold` `autosplit` `donate` `junk` `rsell` `stocks`/`stockbuy`/`stocksell` `questpoints` `upgrade` `combine` `socket` `inscribe`/`uninscribe`/`candletrack` `binding`/`bond`/`buy-bind`/`buy-bond`/`buy-rebond`/`buy-unbind`/`dbid` `implants`/`inject`/`tattoos`

### Movement / navigation / world
`map` `automapping` `autobigmap` `gps`/`gpsmap`/`gpsdifficulty` `areas` `zinfo`/`seezone`/`zonerules`/`zcompare`/`ztimer` `run` `longwalk` `track`/`autotrack` `scan` `exits`/`autoexits` `enter` `portal`/`activate-portal` `recall`(→word-of-recall) `where` `locate` `hometowns` `inns`/`offer` `fly`/`hover`/`waterwalk`/`port`/`ship`

### Group / social / channels
`group`/`groups`/`gkick`/`passlead` `gsay`/`gprompt`/`groupprompt`/`showgroup` `groupskills`/`grouproll`/`groupassist`/`grouplevel` `follow`/`followers` `assist`/`autoassist`/`rescue`/`autorescue`/`norescue` `auctalk`/`gossip`/`grats` `says`/`reply`/`notell`/`qsay` `emotes`/`gemotes`/`socials` `who`/`whoami`/`whois`/`whopower`/`whosafk` `report` `tells`(→reply) `bulletins`/`bulletins-2` `postoffice`/`emails`

### Combat tuning / prompts / QoL toggles
`combat`/`combatfix`/`compactcombat` `fightspeed`/`hpr`/`hmvpct`/`dps` `consider`/`huntme`/`countmob`/`mobcount` `damprompt`/`damroll`/`hitroll`/`damcon`/`optimized-damcon` `prompt`/`promptbars`/`promptdiag`/`promptexit`/`promptopponent`/`gprompt`/`groupprompt` `wimpy`/`flee`/`murder`/`kill` `toggles`/`brief`/`compact`/`norepeat`/`spam` `autoloot`/`autokill`/`autokey`/`autodoor`/`autojoin`/`autosacrifice`/`autodiag` `aliases`/`highlight`/`trigger-control`/`trigger-mastery`/`bindings` `afk`/`idle`/`color`/`screenwidths`/`pagelengths`/`screenreader` `gmcp`/`mxp`

### Stats / character info / status
`strength` `score` `age` `alignment` `mana`/`mv`/`movement`/`regen`/`seeregen`/`recup` `encumbrance` `limbs`/`organs`/`bleeding`/`major-wound` `armor-class`(→score) `meleepower`/`spellpower`/`damage-reduction`/`true-damage` `equipment`/`eqstat`/`eqdiff`/`gearcheck`/`inventory`/`compare`/`autocompare`

### Building / admin (OLC)
`oedit-values` `redit-sector-type` `builders` `implementors` `flags` `world-formats` `is-npc`

---

## NukeFire's native category groupings (verbatim from the wiki badges)
<!-- generated from the live index; alphabetical within each category -->

### commands (116 topics)

`afk` `afx` `animal-spirit` `armor-spell-2` `assist` `auctalk` `autoassist` `autodoor`
`autoexits` `autogold` `autokey` `autoloot` `autosacrifice` `autosplit` `bless-2` `break-door`
`brief` `bugs` `buy` `clear` `colorspray` `compact` `compactcombat` `contents`
`date` `diagnose` `dispel-good-2` `displays` `donate` `drop` `eating` `emotes`
`enter` `eqstat` `equipment` `examine` `exits` `fill` `flee` `followers`
`gemotes` `get` `getdeep` `give` `gkick` `gold` `grab` `group`
`gsay` `impstat` `inventory` `junk` `keys` `kill` `killstitle` `leave`
`lightning-bolt-2` `list` `look` `motd` `murder` `norepeat` `north` `notell`
`open` `order` `pagelengths` `pager` `passlead` `pick-locks` `poison-2` `postoffice`
`pour` `promptexit` `protection-from-evil-2` `put` `putdeep` `qsay` `quaff` `quests`
`read` `recite` `remove-curse` `remove-poison` `reply` `report` `rewear` `says`
`scan` `score` `screenwidths` `sell-2` `sellall` `shop-identify` `showins` `slinging`
`socials` `splitting` `strength-2` `subdue-2` `summonable` `tatstat` `titles` `use`
`value` `visible` `waterwalk-2` `wear` `weather` `where` `who` `whoami`
`whois` `wield` `wimpy` `write`

### skills (155 topics)

`advanced-knifeplay` `advanced-swordplay` `ancestralism` `annihilate` `aura` `aura-2` `barkskin` `barrier-of-valor`
`blasphemous-brand` `bonespur` `captains-command` `carnal` `cauterize` `clothesline` `combo` `conceal`
`crippling` `cutthroat` `cyborepair` `daggerdance` `dark` `dark-2` `dark-3` `dark-strike`
`death-2` `demonglow` `dirty-trick` `doom` `dps` `dragon-kick` `eclipse` `eldritch`
`eldritch-2` `emit` `eqdiff` `fancy` `faq` `firebrand` `fortify` `garrote`
`ghost-step` `graceful` `grave` `grifter-s` `groin-rip` `grouproll` `hellquake` `heretic-2`
`hmvpct` `honed` `hunter-killer` `hypnotize` `ill-omen` `immolate` `impale` `impmissing`
`infernal` `inscribe` `jig` `katsuragi-strike` `keelhaul` `kinetic-overload` `kneesmash` `knife-mastery`
`leech` `legsweep` `loadtest` `lockblow` `lunar-rend` `lunge` `mindcrush` `mystic`
`mystic-2` `mystic-3` `mystic-4` `nanorepair` `nanoswarm` `nanoswarm-2` `neck` `nightfall`
`oathbreaker-s` `oathscar` `overboost` `path` `pestilence` `phantom` `phantom-shackles` `piston`
`plasma-arc-storm` `plunder` `pound` `powder` `precise` `profane-goring` `promptbars` `psiattack`
`psiblast` `psionicwave` `pugilism` `radiation` `ravaged` `reapers` `rend` `roguerush`
`romani` `rotting` `rune` `sabotage` `selfrepair` `sell` `severing-wind` `shield`
`shield-of-sacrilege` `shinobi` `shurikenthrow` `silent-storm-flurry` `sinful` `smoke` `snapeq` `soul`
`soulshredder` `spew` `spinkick` `spirit` `spirited` `spiritstab` `strike` `sucker`
`suicide` `swashbuckle` `sword-mastery` `sword-sweep` `tank` `tarot` `tatmissing` `thermal`
`tiger-punch` `tormenting` `trick` `turbo` `umbral` `unconceal` `unholyfist` `upgrade`
`utsusemi` `void` `voidstep` `voidwarp` `voodoo` `wallop` `weapon` `whirlwindstrike`
`wraithfire` `wrangle` `wyrmhole`

### reference (544 topics)

`activate-portal` `age` `aliases` `alignment` `ambush` `ammo` `ammo-efficiency` `ammobin`
`animate-dead` `apostrophes` `arcane-dampening` `arcane-torrent` `areas` `armor-spell` `ashen-pivot` `assassin`
`aucstat` `aucstatlive` `auction` `augury` `aura-of-escape` `aura-of-healing` `aura-of-invigoration` `aura-of-protection`
`aura-of-rejuvination` `aura-of-sanct` `autobigmap` `autocompare` `autocompare-2` `autodeposit` `autodiag` `autojoin`
`autokill` `automapping` `autorescue` `autotrack` `backdoor` `backstab` `banks` `barbarian`
`barehand` `barricade` `bash` `battle` `battle-cant` `battlecant` `bellow` `bid`
`binding` `bindings` `black-amen` `black-rebuke` `bleeding` `bless` `blight-grow` `blindness`
`blood-aegis` `bloodfire-rend` `bloodmana` `blur` `bodyslam` `bond` `bonefoundry` `boneyard`
`borant` `botting` `boundstat` `brass-creed` `brass-passover` `brass-storm` `brutal-strike` `builders`
`bulletins` `bulletins-2` `burning-hands` `burning-revelation` `burningrevelation` `burnished-block` `bushwhack` `buy-bind`
`buy-bond` `buy-rebond` `buy-unbind` `calliope` `candletrack` `carapace` `carnalcleave` `cast`
`cata` `catseye` `chainsaw-dervish` `challenge` `changes` `chant` `charm-person` `chill-touch`
`chilling-howl` `circle` `classes` `classlegacy` `clawed-master` `cleanout` `clone` `coldmercy`
`color-spray` `combat` `combatfix` `combine` `commands` `communication` `compare` `consider`
`contagious` `contagiousfrenzy` `container` `control-weather` `corpse` `countmob` `covering-fire` `crafting`
`crashes` `crimson-reliquary` `crush` `cure-blind` `cure-critic` `cure-light` `curist` `curse`
`cyborg` `damage-reduction` `damcon` `damprompt` `damroll` `darkcovenant` `darkness` `dbid`
`death` `deathlotus` `deaths` `deathstory` `delete` `deliver` `detect-align` `detect-invis`
`detect-magic` `detect-poison` `devour` `discord` `disembowel` `disintegrate` `dispel-good` `disruption`
`divine-smite` `dontburnme` `double-tap` `drink` `dual` `dunereport` `dustcloud` `earthquake`
`eat` `elbowstrike` `emails` `enchant-armor` `enchant-weapon` `encumbrance` `endauction` `energy-drain`
`entrench` `expstart` `fallen` `fallentithe` `fanatic` `fanatics-ward` `feast` `feign-death`
`feral-agility` `field-benediction` `field-dress` `fieldbenediction` `fightspeed` `fireball` `firestorm` `firstaid`
`flags` `flurry` `flushit` `fly` `focus` `foundlist` `gallows-grace` `gearcheck`
`gmcp` `gorge` `gprompt` `gps` `gpsdifficulty` `gpsmap` `grapple` `grave-sacrament`
`gravesmoke` `griftersgambit` `gritward` `groupassist` `grouplevel` `groupprompt` `groupskills` `grow`
`grow-brain` `grow-claws` `gruesome-gnaw` `gslam` `gstat` `gun-sermon` `gunsermon` `gutting-strike`
`gypsy` `gypsydance` `hack` `harm` `harvest` `headbutt` `headhunter` `heal`
`healers` `heat` `heatsinkbreach` `heelbreaker` `help` `helpfind` `heretic` `hex-misfortune`
`hide` `highlight` `history` `hitroll` `holy-fist` `hometowns` `honor` `hover`
`hpr` `hunterkillerprotocol` `huntme` `hydraulic-stomp` `identify` `idle` `idlist` `imp`
`impact-damage` `implants` `implementors` `improved-self-repair` `improvised-weapon` `infiltrator` `inject` `inns`
`interdict` `invigorate` `invisibility` `iron-mercy` `irradiated` `is-npc` `items` `judgment-round`
`ka-s-ward` `ka-ward` `kaiju` `katsuragistrike` `kaward` `keycopy` `kick` `killers`
`knifeplay` `knight` `knock` `lead-layer` `leaderboard` `legendary-item` `levels` `lightning-bolt`
`limbs` `link` `linkless` `locate` `locate-object` `longwalk` `looktat` `mage`
`magic-missile` `major-wound` `majorwound` `mana` `manasacrifice` `map` `mark-of-the-outer-dark` `mark-outer-dark`
`markouterdark` `marksmanship` `marksmanship-2` `martyr-s` `martyr-s-intercession` `martyrshot` `martyrsintercession` `material`
`mattrans` `meleepower` `metal-auger` `missing` `mobcount` `mono-net` `mortals` `movement`
`multiplaying` `mutant` `mutant-repel` `mv` `mxp` `mysticdeception` `natures` `neckbreak`
`neural-overload` `newbie` `nightfallstep` `nightshade-veil` `ninja` `nofocus` `norescue` `nukefireblast`
`objects` `occultist` `offer` `optimized-damcon` `optimized-turbo` `organs` `outlander` `parry`
`passwords` `pay` `phantommirage` `phoenix-nova` `pindrop` `pirates` `pistol-whip` `pistolwhip`
`pistonpunch` `pizza` `plates` `playerinfo` `players` `poison` `policies` `port`
`portal` `pounce` `practice` `precisestrike` `prefedit` `prestige` `prompt` `promptdiag`
`promptopponent` `protection-from-evil` `protection-from-good` `qol` `questions` `questpoints` `qui` `quickdraw`
`quit` `radcloak` `radiation-burst` `radstorm` `rage` `rally` `rallyfind` `ranger`
`razor` `razorbloom` `reader` `reapersmark` `reboot` `rebuke` `recon` `recup`
`regen` `rejuvinate` `releases` `remort` `remortskills` `remortskills3` `remove-invis` `remove-invis-2`
`rescue` `resolve` `respawn` `restoration` `ricochet` `rollbones` `rollbones-groupleader` `roundhouse`
`rsell` `run` `runremaining` `sacred` `sacredvengeance` `samurai` `samuraistrike` `sanctified-rounds`
`sanctifiedroundsi` `sanctuary` `save` `scavengers-instincts` `screenreader` `scry` `seeregen` `seezone`
`sense-life` `servo` `servolock` `severingwind` `shadowclone` `shadowform` `shapeshift` `ship`
`shocking-grasp` `shockwire` `shoot` `shoot2` `shops` `showauction` `showgroup` `showwait`
`shrapnel-creed` `skeetshot` `skillapply` `skills` `skulk` `sleep` `slinger` `smokepowder`
`smokescreen` `sneak` `socket` `soulstorm-eternum` `soulswell` `spam` `spellpower` `spike-guard`
`spines` `spirit-reclamation` `spiritattack` `spiritedrush` `spray-fire` `sr` `stalk` `steal`
`stockbuy` `stocks` `stocksell` `strafe` `strength` `stun` `subdue` `suckerstab`
`suicideroll` `summon` `summon-2` `supfire` `suppress-fire` `supreme-rend` `swarm` `sword-of-predation`
`swordplay` `tailslam` `tankmode` `target` `tarotvision` `tattoos` `tdome` `telepath`
`teleport` `tentacles` `terminal-track` `terminalt` `thanksgiving` `thermalpunch` `throw` `tildes`
`time` `tnl` `toggles` `topoff` `tornado` `tpunch` `track` `tradelist`
`trail` `trample` `trickattack` `trigger-control` `trigger-mastery` `tripwire` `true` `true-damage`
`truedamage` `uninscribe` `unload` `uplink` `usage` `utilities` `vagrant` `variant`
`venom-gash` `voidforge` `voidharvest` `voidpunch` `voidstriker` `vomit` `voodoocontract` `voter`
`wake` `warbanner` `warding` `wardinglitany` `wasteland-calm` `waterwalk` `weapons` `weaponthrow`
`welcome` `whatsauc` `whatsmy` `whirlpunch` `whopower` `whosafk` `wire` `wired`
`wolfman` `wolfout` `word-of-recall` `world-formats` `zcompare` `zinfo` `zonerules` `ztimer`

### building (2 topics)

`oedit-values` `redit-sector-type`

---

## Notes on the taxonomy

- **`reference` is a catch-all.** NukeFire dumps most class pages, spell pages, stat pages, and lore into `reference` — so "reference" does *not* mean "passive doc only." Many `reference` slugs (e.g. `backstab`, `fireball`, `ambush`, `disintegrate`) are live abilities; the thematic groups above are the better lens for "what is this."
- **`skills` vs `reference` for abilities is inconsistent.** The same ability family can straddle both (e.g. `eldritch`/`eldritch-2` are `skills`, but `arcane-torrent` and `suppress-fire` are `reference`). Treat the badge as a soft hint.
- **Duplicate/alias slugs** are common: `katsuragi-strike`/`katsuragistrike`, `gun-sermon`/`gunsermon`, `severing-wind`/`severingwind`, `markouterdark`/`mark-outer-dark`/`mark-of-the-outer-dark`, `bulletins`/`bulletins-2`, `mystic`/`mystic-2`/`mystic-3`/`mystic-4`. These are the wiki's own "Also known as" redirects surfaced as standalone slugs.
