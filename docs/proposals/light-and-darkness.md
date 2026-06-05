# Proposal: Light & Darkness

**Status:** Draft / for discussion · **Type:** Feature proposal (pre-spec) · **Audience:** engine
**Feeds into:** a future `light-and-darkness.md` spec + plan
**Builds on:** [`time-and-clock.md`](../specs/time-and-clock.md) (the `gameclock` period machinery — already shipped, M15.4b), [`world-rooms-movement.md`](../specs/world-rooms-movement.md) (`Room.Terrain` + the weather/time eligibility cascade), [`ui-rendering-help.md`](../specs/ui-rendering-help.md) (the per-viewer `RenderRoom` chokepoint + theme color), [`economy-survival.md`](../specs/economy-survival.md) (the sustenance-drain tick model that a burning torch reuses), and [`inventory-equipment-items.md`](../specs/inventory-equipment-items.md) (item properties + equipment slots). Coordinates with [`visibility.md`](../specs/visibility.md) and [`hidden-exits.md`](../specs/hidden-exits.md) on what is *seeable*.

## Decisions taken so far (steering this draft)

Settled in review; the spec will inherit them. The headline steer: **darkness is real friction, not atmosphere.** The day/night cycle already runs ([`time-and-clock.md`](../specs/time-and-clock.md) §3 — the `gameclock` ticks, emits `time.period.change`, and drives weather/time ambience) but it is presently *invisible* — nothing reacts to it except the colored ambience lines. This feature is what gives the cycle (and place, and gear) teeth.

- **PD-1 — Light is a graded ordinal scale, not a boolean (DECIDED).** A room has an *effective light level* in `{0 black, 1 gloom, 2 dim, 3 lit}` per viewer, not a `dark`/`lit` flag. The user's framing is the spec: night is *less* light, not *no* light; a windowless cell is no light; a lamp-lit street at midnight is dim-but-clear. A binary flag can't express that spectrum and is rejected (§9).
- **PD-2 — Light is computed from layers, max-combined (DECIDED).** `effective = clamp( max( ambient·terrain-gate, room.base_light, best_local_source ), viewer_floor, 0..3 )`. No single input owns the answer; a torch rescues a black cave, a content override floors a lamp-lit street, darkvision floors a dwarf's view. Night-≠-black and cell-=-black both fall out of the terrain gate (§4).
- **PD-3 — Real friction, not just atmosphere (DECIDED — the headline).** Below "lit", darkness *withholds information and degrades action*: it hides room detail, blocks `look`/reading, penalizes combat accuracy, and (configurably) makes movement into the unseen risky. Atmosphere (a dimmer-colored room) is the *floor* of the effect, not the whole of it. The impact ladder in §5 is the centre of gravity of this proposal.
- **PD-4 — Effective light is per-viewer (DECIDED).** Two characters in the same black room can see differently (a dwarf's darkvision, a held torch, a light spell). The render path is *already per-viewer* (`RenderRoom` renders once per session and now takes per-viewer predicates — `marker`, `ambience`, `hostile`), so a per-viewer `light` input is the established shape, not a new axis.
- **PD-5 — Light sources are items with a `light` property; fuel-burners reuse the drain model (DECIDED).** A torch/lantern carries `light: <level>`; a *lit* fuel-burner decrements over time on a tick handler that mirrors `sustenance-drain` exactly (`economy-survival` §4.4 — already in `main.go`), and gutters out at zero. A held/light **equipment slot** holds the one active source. Permanent sources (a glowing blade) skip the fuel loop.
- **PD-6 — Terrain is the sky-exposure gate; room `light` is the per-room override; resolution mirrors the weather cascade (DECIDED).** `Room.Terrain` (`outdoors`/`indoors`/`underground`) + the existing `isShielded()` (`weather/cascade.go`) decide whether *ambient* (time-of-day) light reaches the room at all. A per-room `light` property is the authored override (the lamp-lit street, the phosphorescent cave, the sealed vault). The room → area → zone cascade that weather already uses is the template for resolving it.
- **PD-7 — The restart-darkness problem is in scope (DECIDED to confront, not yet how).** `gameclock` deliberately does **not** persist and boots at hour 0 = **night** (`time-and-clock.md` §3.6, an open spec question). Once darkness gates gameplay, every server restart plunges the whole world into a dark night — a real regression in playability. v1 must resolve this (persist game-time, or boot at a lit hour); §11 holds the fork.

## 1. Problem / motivation

There is a day/night cycle, and right now it does almost nothing. `gameclock` advances an in-game hour every real minute, computes a period (`night`/`dawn`/`day`/`dusk`), and emits `time.period.change`; the only consumer is the weather service, which broadcasts ambience lines (recently *colored*, but still just text). A player cannot tell midnight from noon except by reading a flavor line, and "it is dark" carries no weight — you see the room, its exits, its occupants, and fight just as well at the bottom of a sealed crypt as in a sunlit square.

That flatness wastes a system that's already built. Darkness is one of the oldest and best MUD levers: it turns *time* into a resource, *terrain* into a hazard, *a torch* into a meaningful purchase, and *a lit city* into a felt refuge. AnotherMUD already has the hard substrate — a ticking world clock with periods, a terrain classifier with an indoor/outdoor/underground split and a shielding rule, a per-room property bag, an item property system, equipment slots, a fuel-drain tick model, and a per-viewer room renderer. What's missing is the thin connective layer that turns those into an *effective light level* and lets the rest of the game react to it.

The explicit ask: **real friction.** Not a mood tint over the room — a system where you cannot read the room in the dark, cannot fight well without light, and have a reason to carry a torch and to head for the lamps.

## 2. Goals & non-goals

**Goals.** A per-viewer **effective light level** for any room, computed from time-of-day, terrain exposure, a per-room override, carried/equipped light sources, and a per-viewer darkvision floor. **Real consequences** below "lit": withheld room information, blocked examination, a combat accuracy penalty, and configurable movement risk (§5). **Light-source items** (torches, lanterns, glowing gear) with a lit/unlit state and, for fuel-burners, a burn-down loop reusing the survival-drain model. A **held/light equipment slot** for the active source. **Content control** — authors floor/ceiling a room's light via a property without touching engine code. **Presentation** that extends the existing color/ambience work (dim/gloom/black render states) plus a GMCP `room.light` for modern clients. A resolution to the **restart-darkness** problem so the world isn't unplayable after every restart.

**Non-goals (rule out now).** Not a continuous/lumen simulation — four ordinal levels, not a float (§9). Not light-as-physics (no ray-casting between rooms; light is room-local, a torch lights *your* room, not the one north of you). Not a full stealth/hiding system — darkness *interacts* with stealth, but stealth is its own future feature; this proposal only exposes the hooks. Not weather-driven light in v1 (overcast dimming daylight is a tempting §11 option, deferred). Not blinding/over-bright as a mechanic (level 3 is the ceiling; "blinded by glare" is out). Not per-*item* visibility nuance (a dark room hides "the whole occupant list" coarsely; which specific things a partial-light room reveals is a §5 sub-decision, not a per-item fog).

## 3. Proposed approach (the shape)

**One resolver, one render gate, a handful of reactors.**

The **light resolver** is the shared seam: a pure function `effectiveLight(room, viewer) → 0..3`. It reads the `gameclock` period, the room's terrain + `light` override, the viewer's carried/equipped lit sources, and the viewer's darkvision floor, and `max`-combines them under the terrain gate (§4). It is read-only and per-viewer; it owns no state of its own (the *clock* has the time, the *room* has the override, the *items* have their light, the *player* has the floor — the resolver just combines them). Everything downstream calls it.

The **render gate** is the single biggest consumer and the natural chokepoint: `RenderRoom` (`ui-rendering-help`) already renders once per viewer and already takes per-viewer inputs. The gate computes `effectiveLight(room, viewer)` and branches *before* composing the body — black suppresses everything, gloom emits a terse/obscured form, dim emits full-but-tinted, lit is today's render (§5, §8). This is where "real friction" first bites, and it's a localized change because the room view is already funnelled through one function.

The **reactors** are the rest of the friction, each gating at its own existing chokepoint by calling the resolver: the combat manager (accuracy penalty in the dark), the `look`/examine handlers (blocked reads), the move handler (risk into the unseen), and — later — spawn/disposition (nocturnal mobs). None of these need new plumbing beyond "ask the resolver."

**Light sources** are items carrying a `light` property, mirroring how `rarity`/`key_for` already ride the item property bag. A *lit fuel-burner* is registered with a burn-down tick handler that is a near-copy of `sustenance-drain` (decrement on cadence, fire an "it gutters out" event at zero). The **held/light slot** (a new entry in the slot registry, the same machinery as the existing `cloak` slot) names the one active source so `equipment` shows it and the resolver has an unambiguous "your light" to read.

Content authors get one knob: a room `light:` property (override the computed floor/ceiling) plus the terrain they already set. The cascade that resolves it — room override beats area default beats zone/biome default — is the *same* room → area → zone shape the weather service already implements, so the resolution code has a proven template to copy rather than invent.

## 4. The light model (the scale and its computation)

### 4.1 The scale (PD-1)

| Level | Name | The world | Feels like |
|---|---|---|---|
| 3 | **Lit** | full information | daylight outdoors, lamp-lit city, magical light |
| 2 | **Dim** | full info, muted presentation | dusk/dawn, torchlight, a moonlit street |
| 1 | **Gloom** | partial info — shapes, not detail | an unlit road at night, a dim dungeon |
| 0 | **Black** | nothing | a sealed cell, a lightless cave |

### 4.2 The computation (PD-2)

```
ambient        = ambientFor(period)            # day=3, dawn/dusk=2, night=1   (NEVER 0 — starlight)
exposed        = ambientThroughTerrain(ambient, room.terrain)
                   # outdoors    → ambient
                   # indoors     → min(ambient, INDOOR_CAP)     (windows: capped, e.g. ≤2)
                   # underground → 0                             (no sky reaches it)
roomFloor      = room.light  (the per-room override, when authored)
sources        = max(light of each LIT source the viewer carries/holds,
                      light of each luminous item/mob in the room)
viewerFloor    = darkvision/effect floor for THIS viewer (default 0)

effective = clamp( max(exposed, roomFloor, sources), viewerFloor, 0, 3 )
```

The two cases from the brief drop straight out:

- **Unlit road at night** = `outdoors` + `night` → ambient 1, exposed 1, no override, no source → **gloom (1)**. Navigable, shapes only.
- **Windowless cell** = `underground` → exposed **0**; no override, no source → **black (0)**. Only a carried light rescues it.
- **Lamp-lit city street at midnight** = the street rooms author `light: 2`; ambient is 1 but `roomFloor` is 2 → **dim (2)**. Walking from the dark road through the gate into the lit streets is a felt transition — exactly the brief's image.

`ambientFor` never returns 0 — the darkest *natural* sky is gloom, not black; only *enclosure* (terrain) or content (`light: 0` sealed vault) produces true black. That single rule is what keeps "night" honest.

### 4.3 Where the inputs already live

| Input | Source | Status |
|---|---|---|
| period (day/dawn/dusk/night) | `gameclock.CurrentPeriod()` | **exists**, ticking |
| terrain gate | `Room.Terrain` + `isShielded()` (`weather/cascade.go`) | **exists** |
| room override | `Room.Properties["light"]` + property registry | bag **exists**; register the key |
| source light | item `light` property + lit-state | **new** (rides existing item property bag) |
| viewer floor | race flag / active effect | hooks **exist** (`progression` races, `effect`) |

## 5. What darkness costs (the friction ladder — PD-3)

This section is the point of the proposal. Each rung gates at an existing chokepoint by calling the resolver.

| Effective | Room view | Examine / read | Combat | Movement |
|---|---|---|---|---|
| **3 Lit** | full (today's render) | normal | normal | normal |
| **2 Dim** | full, **tinted** dim | normal | normal (or tiny penalty — §11) | normal |
| **1 Gloom** | **terse**: short "dark" description; exits shown as **bare directions** (so you can still flee/navigate); occupants shown **coarsely** ("someone is here", "something moves") — names hidden | `look <thing>` → "too dark to make out detail"; reading blocked | **to-hit penalty**; you can fight but you swing at shapes | allowed; destination unseen until you arrive |
| **0 Black** | **suppressed**: name/description/occupants all hidden → one line ("It is pitch black; you can see nothing.") | blocked | **larger to-hit penalty** (fighting blind) | **risk**: configurable — stumble (move but flavorful/slower) or refuse, per §11 |

Design intents baked into the ladder:

- **Exits survive gloom (deliberate).** At level 1 you still see *which directions* lead out, even though you can't read the room — otherwise darkness becomes a rage-quit trap with no escape. At level 0 even exits are hidden unless the spec carves an exception (a §11 sub-decision — "can you feel for the doorway?").
- **Combat degrades, never becomes impossible.** A penalty is more fun than a hard block, and it's the lever that makes a torch *matter* in a fight without making the dark a wall. Magnitude is config (§11), not baked.
- **Movement is the sharpest knob.** "You can walk into the dark but don't see where you've arrived" is great tension; "you cannot move" is punishing. Default to *stumble*, make *block* a per-room/zone option for genuine hazards (a cliff path).
- **Coarse, not per-item, occupant hiding.** Gloom collapses the occupant list to vague shapes (and *count* is a sub-decision — do you know "three somethings" or just "something"?). This avoids a per-entity fog system; that nuance belongs to the future stealth/visibility work, not here.

The atmosphere layer (the dim *tint* at level 2, §8) is the *floor* of all this, not the whole — per PD-3.

## 6. Light sources & the fuel loop (PD-5)

- **The `light` item property** carries the level a source provides (e.g. a torch `light: 2`, a bright lantern `light: 3`, a candle `light: 1`). Rides the item property bag like `rarity`/`key_for` — no new item field.
- **Lit/unlit state.** A source is inert until lit (a `light`/`kindle`/`ignite` verb, or auto-lit on equip — §11). Only a *lit* source contributes. State lives on the instance property bag (a `lit: true` reserved key), so it survives pickup and is admin-settable.
- **Fuel-burners burn down.** A lit fuel-burner carries a fuel/duration value decremented by a **`light-burn` tick handler that is a structural copy of `sustenance-drain`** (`economy-survival` §4.4, already registered in `main.go` at a configurable cadence). At zero it gutters out — fire an event, flip `lit:false`, and if it was the viewer's only light the room goes dark *mid-action*, which is exactly the tension we want. This also wires the **economy**: torches become a real recurring purchase (`economy-survival` shops already exist).
- **Permanent sources** (a glowing blade, an everburning lantern) set `light` with no fuel key → always-on, skip the burn loop.
- **The held/light slot.** A new entry in the slot registry (same machinery as the existing `cloak` slot) holds the *one active* light, so `equipment` shows "in hand: a lit torch", the resolver reads an unambiguous source, and "you can't wield a two-hander AND hold a torch" becomes a real tradeoff (a slot-contention decision — §11). A looser "any lit source in your pack counts" alternative is simpler but loses that tradeoff and the visibility of *what's* lighting you; the slot is preferred.
- **Light as a beacon (hook, not v1 mechanic).** A lit source is, in principle, visible to *others* in the room ("a torch bobs in the dark to the north" / it defeats your own hiding). The resolver makes this expressible later; v1 just notes the hook so the stealth feature can use it.

## 7. Per-viewer sight: darkvision & light effects (PD-4)

Because effective light is per-viewer, the same black room is navigable for some and blank for others:

- **Racial darkvision** — a race flag sets a `viewer_floor` (e.g. dwarves treat any room as ≥ gloom). Races already carry flags in `progression`; this is one more, read by the resolver. Often paired with a *cap* (darkvision is monochrome/short-range → it floors you to gloom, never to lit) so it's an advantage, not a sun.
- **Light/sight effects** — a `light` or `infravision` spell/effect raises the viewer's floor for a duration. The `effect` system already applies timed per-character modifiers; a light effect is one more, contributing to `viewer_floor`.
- **The render path is already per-viewer**, so none of this needs new plumbing — the resolver is simply called with the viewer in hand, the same way the new `hostile` predicate already is.

## 8. Presentation & GMCP (extends the color work)

- **Render states** map to the scale: **lit** = today's output; **dim (2)** = full output wrapped in a muted/night tint (reuse the just-shipped ambience palette — a `dark`/`night` theme tag); **gloom (1)** = the terse obscured form, heavily `<subtle>`; **black (0)** = the single "pitch black" line. This is a clean extension of the weather/time tinting already merged, through the same `ColorRenderer`, so it degrades to clean text on no-color clients automatically.
- **GMCP `room.light`** — surface the effective level on the `Room.Info` package (`networking-protocols` GMCP) so Mudlet/modern clients can dim their viewport or swap a day/night map theme. Server sends the *level*; the client themes. Fits the existing "server knows state, GMCP surfaces it, client renders" pattern, and pairs naturally with the player-maps proposal's `Room.Info` extension.
- **A `light`/`time` probe verb** (optional, cheap) — "It is night; the room is dark." gives players a way to *read* the cycle directly instead of inferring it, which matters once it has consequences.

## 9. Alternatives considered & rejected

- **A boolean `dark` room flag** — rejected (PD-1): cannot express night-≠-black, the lamp-lit-street override, torchlight, or darkvision. The whole value is in the gradient.
- **Continuous lumen/float simulation** — rejected (non-goal): four ordinal levels are enough to drive every consequence in §5, are trivial to author and reason about, and dodge a tuning rabbit hole. We can subdivide later if a real need appears.
- **Light propagating between rooms (a torch lights the room north of you)** — rejected for v1: room-local light is far simpler, matches MUD convention, and avoids a per-edge light-flow graph. "Light as a beacon others *see*" (§6) is a presentation hook, not propagation.
- **Time-of-day as the sole input** (skip terrain/sources) — rejected: it's exactly today's flatness one step less flat, and it can't make a cave dark at noon or a street safe at night. The layered `max` is the minimum that earns the friction.
- **Pure atmosphere (dim the colors, change nothing else)** — rejected explicitly per the brief (PD-3). It's the *floor* of the effect (§8), not the feature.

## 10. Dependencies & risks

Enabling substrate exists or is specced: the `gameclock` periods (shipped), `Room.Terrain` + `isShielded` (shipped), the room/item property bags + property registry (shipped), equipment slots (shipped), the sustenance-drain tick template (shipped), the per-viewer `RenderRoom` chokepoint (shipped, and just extended), the theme renderer (shipped, just extended), and the GMCP `Room.Info` package to hang `room.light` on. **No greenfield system is required for the v1 model + render gate** — it is a resolver plus a branch in a function that already runs per-viewer.

Risks worth naming now:

- **Restart-darkness (the big one — PD-7).** `gameclock` resets to hour 0 = night and isn't persisted. Today that's harmless (nothing reacts); the moment darkness gates gameplay, a restart blacks out the world. Must be resolved in v1 — see §11. This is the single hardest *coupling*, and it's a `time-and-clock` persistence decision this feature forces to a head.
- **Trap potential.** Darkness that hides exits + blocks movement + has no torch nearby = a new player stuck in the black with no recovery. Mitigations are in the design (exits survive gloom; movement defaults to stumble; ambient never drops below gloom *outdoors*), but the spec must treat "can a player always escape the dark?" as a hard invariant, not an afterthought.
- **Content burden / silent regressions.** Every existing `underground` room becomes **black** the day this ships unless authored otherwise — that could break questlines that assume you can see. Needs a migration pass over existing content (audit `underground`/`indoors` rooms, set `light:` where they should stay visible) and a sane default policy so unaudited content fails *safe* (lit), not *dark*.
- **Visibility-spec coordination.** "What can I see in this room" now has *two* gates: darkness (this feature) and concealment ([`visibility.md`](../specs/visibility.md) / [`hidden-exits.md`](../specs/hidden-exits.md)). They must compose without contradiction (a hidden exit in a lit room is still hidden; a visible exit in a black room is still hidden *by darkness*). Define the precedence explicitly.
- **Combat balance.** A to-hit penalty in the dark touches the combat math; magnitude needs tuning against real encounters, and "mobs in their own dark lair" must not become unkillable. All numbers go in the config surface, none baked.
- **Biome interaction.** A [`biomes.md`](../specs/biomes.md) layer may want to contribute light defaults (a "cavern" biome floors `light: 0`, a "magical forest" glows). The cascade should leave room for a biome tier between area and zone.

## 11. Open questions (for sign-off before the spec)

- **Restart game-time (PD-7) — which fix?** (a) **Persist game-time** on disk so the world resumes where it stopped (resolves the `time-and-clock` §3.6 open question; adds a tiny global save artifact). (b) **Boot at a lit hour** (e.g. always start at midday) — trivial, but the clock then lies about continuity and a restart visibly snaps time. (c) **Both** — boot at midday *and* persist once the save artifact exists. Leaning (a); it's the honest fix and the spec already flags it.
- **Movement into the black — stumble or block?** Default *stumble* (move, see nothing until arrival) with an opt-in per-room/zone *block* for genuine hazards? Or global block? Sets how punishing dark navigation feels.
- **Combat penalty — shape and size.** Flat to-hit malus per level below lit? Does it scale with the *attacker's* light only, or also the *target's* (a lit target is easy to hit even by an unlit attacker — realistic, and it makes *carrying* light a tactical liability, not just an asset)? Numbers → config.
- **Occupant hiding granularity at gloom.** Do you see "something is here" (presence only), a *count* ("several shapes"), or coarse *kind* ("a person, an animal")? More info = less friction; pick the rung.
- **Black-room exits — totally hidden, or "feel for the doorway"?** A full hide is cleanest but risks the trap; an "obvious exits you can feel" carve-out is kinder. Interacts with the escape-invariant.
- **Light slot contention.** Does holding a torch cost a hand (mutually exclusive with two-handed weapons / shields), or is "light" a free non-contending slot? Contention is richer but couples to the equipment model.
- **Lighting a torch — explicit verb or auto-on-equip?** Explicit (`light torch`) is more deliberate and gives a "fumbling in the dark to light it" beat; auto-on-equip is frictionless. Possibly both (equip lights it; `extinguish` to save fuel).
- **Darkvision shape.** Race flag only, or also effect/consumable (a potion of darkvision)? And does it floor to *gloom* (advantage) or *lit* (negates the system for that race)? Strongly leaning gloom-cap.
- **Weather dims daylight? (deferred default: no.)** Heavy overcast/storm knocking day 3→2 is atmospheric and cheap (weather state is already on the area), but it's a second-order input; ship the core first.
- **Default for unaudited content.** Untagged room with empty terrain = `outdoors` = follows the sky (fails *safe*: lit by day, gloom by night). Confirm that's the intended fail-safe and that `indoors`/`underground` content gets the audit pass (§10).

All numeric values (ambient-per-period, indoor cap, combat malus per level, torch light levels + burn rate + cadence, darkvision floor/cap) live in the spec's **configuration surface**, never baked — per house spec convention.

## 12. Phased delivery

Each phase is independently shippable and demoable.

- **Phase 0 — model + render gate.** The light resolver (period × terrain × room `light` override) and the `RenderRoom` branch (black/gloom/dim/lit). No sources, no penalties yet. Resolves restart-darkness (PD-7) because the render gate makes it immediately visible. *Pure presentation + one content property over data we already have* — lowest risk, and it lets us **feel** the cell-vs-street and noon-vs-midnight contrast before committing to mechanics. This is the recommended first prototype.
- **Phase 1 — light sources & fuel.** The `light` item property + lit/unlit + the held slot + the `light-burn` drain handler. Darkness becomes a *problem players solve*, and torches get an economy. Adds the kindle/extinguish verbs.
- **Phase 2 — friction with teeth.** Combat dark-penalty, `look`/read gating, per-viewer darkvision (race floor + light effects), GMCP `room.light`, the `light`/`time` probe verb. This is where §5 fully lands.
- **Phase 3 — world reactivity.** Nocturnal mob schedules and light-fearing/seeking creatures (on the `time.period.change` + spawn/disposition seams), stealth hooks (light-as-beacon), optional weather dimming, biome light defaults.

---

*This proposal consumes shipped substrate and decides the model + friction; the eventual `light-and-darkness.md` spec inherits PD-1…PD-7 and resolves §11. The center of gravity is §5 (what darkness costs) — atmosphere is the floor, not the feature.*
