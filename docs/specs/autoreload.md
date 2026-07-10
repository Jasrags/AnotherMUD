# Autoreload (a per-character reload-on-dry preference)

**Status:** Draft · **Scope:** a per-character **preference** that, when a
wielded holder-fed weapon is dry at the moment of firing, automatically performs
the normal `reload` on the actor's behalf — instead of the shot failing with a
"reload first" notice. Behavior only: it decides *whether and with what* to invoke
the existing reload, and adds no combat math. Extends `ammo-and-reloading` (the
`reload` verb it delegates to) and `ranged-combat §3` (the dry-attempt seam it
hooks). Shadowrun's firearms are the reference consumer.

## 1. Overview

`ammo-and-reloading` made reloading a deliberate act: a dry weapon produces a
dry attempt, and the player types `reload` to swap in a fresh holder. In a
sustained firefight that is a `reload` between every magazine. **Autoreload** is a
convenience toggle that removes the *manual step* while preserving the *cost*:
when the toggle is on and a shot lands on a dry weapon, the engine runs the same
timed `reload` it would have run for a manual command — the same holder selection,
the same busy window, the same ejection — and the replaced shot simply does not
fire that beat.

**Goals.** Spare a player in a firefight from typing `reload` between magazines;
keep the automatic reload legible (same selection and cost as the manual verb);
introduce no new mechanics — autoreload is a thin decision layer over
`ammo-and-reloading §3`. **Non-goals:** partial reloads or mixed-ammo selection
(owned by `ammo-and-reloading`, and out of scope / not planned there);
auto-*switching* to another weapon when out of ammo entirely (§5 — explicitly not
this spec); any proactive "reload the instant the magazine empties" behavior (§3
is reactive by decision).

**Related specs.** `ammo-and-reloading` (the `reload` verb, holder selection, and
ejection this delegates to unchanged), `ranged-combat` (the dry-attempt / out-of-ammo
seam autoreload branches on), `action-economy` (autoreload's reload is the same
timed busy action a manual reload is, inheriting its cost and interrupts),
`two-weapon-fighting` (dual-wielded firearms — §4), `saves` (the persisted toggle
— §6).

## 2. The toggle

- `autoreload` with no argument **flips** the current state and confirms the new
  value — the standard binary-toggle grammar shared by every simple on/off
  preference (`autoloot`, `autoassist`, `roomdata`, `minimap`, …).
- `autoreload on` / `autoreload off` set the state explicitly and confirm it.
- Any other argument is a usage error and leaves the state unchanged.
- The state is a **persisted per-character preference** (a sibling of the prompt
  template and other player prefs), unset-safe: a character with no stored value
  takes the configured default (§7).

**Acceptance criteria**

- [ ] A character has an autoreload preference that persists across sessions.
- [ ] `autoreload` alone flips the state (off→on, on→off) and confirms it.
- [ ] `autoreload on` / `autoreload off` set the state explicitly and confirm it.
- [ ] A non-`on`/`off` argument is a usage error and changes nothing.
- [ ] A freshly created character's state is the configured default (§7).

## 3. Triggering — reactive, at fire time

Autoreload is evaluated **reactively**, at the same point `ranged-combat §3`
produces the dry attempt (a shot resolved against a weapon whose feed is empty).
It is **not** proactive: nothing reloads the instant a magazine hits zero — the
reload happens on the *next* attempt to fire the dry weapon. When a fire attempt
finds the wielded holder-fed weapon dry:

- **Toggle off** → the existing dry-attempt behavior stands (the "reload first"
  notice). Unchanged.
- **Toggle on**, and a compatible loaded holder is available (§4) → the engine
  begins the standard timed `reload` (`ammo-and-reloading §3`, §5) against the
  wielded weapon. The attack does **not** resolve this beat: no damage, no round
  consumed, no dry "click".
- **Toggle on**, no compatible loaded holder → §5 (report only, rate-limited).

Because autoreload delegates to the same `reload` routine, it inherits every
downstream behavior: the busy window (`action-economy`), the interrupt rules
(movement / `stop` cancel it mid-reload), holder ejection into the room, and the
already-shipped "don't swap for a worse holder" no-benefit guard. Autoreload adds
no cancellation or ejection logic of its own.

**Acceptance criteria**

- [ ] With the toggle off, a dry firearm produces the existing dry-attempt notice
      and no reload.
- [ ] With the toggle on and a compatible loaded holder present, a dry firearm
      begins the standard timed reload; the busy window matches a manual reload of
      that weapon.
- [ ] The replaced attack deals no damage and consumes no round that beat.
- [ ] An autoreload obeys the same interrupts as a manual reload (movement / `stop`
      cancel it) with no separate logic; ejection of the prior holder is the
      standard `ammo-and-reloading §7` behavior.
- [ ] Reloading is evaluated only on a fire attempt against a dry weapon, never
      proactively on the beat a magazine empties.

## 4. Holder selection and dual-wielding

- **Selection is `ammo-and-reloading`'s, unchanged.** Autoreload does not choose
  holders differently from the manual verb: it picks the **fullest compatible
  loaded holder** carried, subject to the shipped no-benefit guard (an equal-or-worse
  spare is declined rather than churned). Autoreload and manual `reload` can never
  disagree about compatibility or choice.
- **Two wielded firearms.** When a character dual-wields firearms
  (`two-weapon-fighting`) and both are dry, autoreload services the **main-hand
  first**, then the off-hand, each consuming its own reload action within the
  beat's `action-economy` budget. If the budget covers only one reload this beat,
  the off-hand reload is **deferred to the next fire attempt** rather than dropped.

**Acceptance criteria**

- [ ] Autoreload selects the same holder the manual `reload` verb would for the
      same inventory state; a no-benefit swap is declined identically.
- [ ] With both wielded firearms dry, autoreload attempts the main-hand first,
      then the off-hand, each paying its own reload action per the action budget.
- [ ] If the action budget covers only one reload this beat, the off-hand reload
      is deferred to the next fire attempt, not silently dropped.

## 5. Out of ammo — report only

When the toggle is on but **no compatible loaded holder** is available anywhere in
the actor's inventory, autoreload does nothing beyond reporting it. It does **not**
fall through to switching to a melee weapon or a different loaded firearm — that is
a separate, larger feature with its own selection rules and is explicitly out of
scope here. The message is distinct from the plain dry-attempt notice (it names the
*absence of ammo*, not the empty weapon) and is **rate-limited** (§6): in a
prolonged dry spell the runner is told once per window, not once per round.

**Acceptance criteria**

- [ ] With the toggle on and no compatible loaded holder, no reload is attempted
      and no weapon switch occurs.
- [ ] The "nothing to reload with" outcome is reported distinctly from the plain
      dry-attempt notice.
- [ ] The outcome is reported at most once per suppression window (§7), not once
      per fire attempt.

## 6. Persistence

- The autoreload toggle persists with the player save as an ordinary preference —
  a single additive boolean written `omitempty`, so a save with the toggle off (or
  an older save predating the field) writes no key and round-trips unchanged. No
  schema-version bump is needed (the `Autoloot` / `AutoAssist` precedent: a `false`
  zero-value is indistinguishable from "field absent"). An absent value loads as the
  configured default.
- The **rate-limit state** (the last time a "nothing to reload with" notice was
  shown, keyed per character and weapon) is **ephemeral combat state and is not
  persisted** — the same treatment as rest state and active-effect timers. On
  relogin the suppression window resets.

**Acceptance criteria**

- [ ] The toggle survives a relogin; a save written before the field existed loads
      at the default.
- [ ] The no-ammo suppression window is not persisted and resets on relogin.

## 7. Configuration surface

| Setting | Meaning | Default |
|---|---|---|
| Default autoreload state | The toggle value on a character with no stored preference (§2). | off (opt-in) |
| Autoreload master enable | Server-level switch disabling the feature entirely regardless of per-character state (§3). | enabled |
| No-ammo notice window | Minimum interval between repeated "nothing to reload with" notices for the same character/weapon (§5). | policy duration |

Holder capacity, reload cost, compatibility, and selection order are **not**
configured here — they belong to `ammo-and-reloading`, which autoreload delegates
to. All numeric magnitudes live in one of these tables; the prose names behaviors,
not values.

## 8. Decisions and open questions

**Decided:**

- **Reactive, not proactive** (§3). Autoreload fires on the next shot against a
  dry weapon, off the existing dry-attempt seam — never on the beat the magazine
  empties. (Rejected: proactive reload-on-empty — it reloads even after the fight
  is over and needs a second trigger point.)
- **Convenience, not a free action** (§1, §3). The automatic reload costs the same
  `action-economy` busy window a manual reload does; the replaced shot does not
  fire. Autoreload removes the keystroke, not the cost.
- **Delegates selection to `ammo-and-reloading`** (§4). No independent holder
  choice — the fullest-compatible pick plus the shipped no-benefit guard, so manual
  and automatic reloads never diverge.
- **Two-weapon: both, main-hand first, off-hand deferred if the budget is short**
  (§4) — the design intent. **v1 ships main-hand-only** (see Still open): the combat
  engine resolves off-hand strikes as melee (an off-hand weapon never fires as a
  projectile), so an off-hand firearm never produces a dry-fire for autoreload to
  service, and the manual `reload` verb is itself main-hand-only. The design holds
  for when an off-hand ranged path lands.
- **Out of ammo: report only** (§5). No fall-through to melee or an alternate gun;
  weapon-switching is a separate future feature.
- **No-ammo notice is rate-limited and ephemeral** (§5, §6).
- **Default off (opt-in)** (§7). Autoreload consumes the shot's beat and ejects a
  clip on the character's behalf; a player opts into that rather than having it on
  by default.

**Still open (non-blocking):**

- **Off-hand firearm autoreload — main-hand-only in v1.** The dual-wield servicing
  §4 commits to is inert today because the engine never fires an off-hand weapon as
  a projectile (off-hand strikes are melee) and `reload` targets only the wield
  slot. When an off-hand ranged path lands, `RangedDry` needs a hand discriminator
  and `reload`/the autoreload peek an off-hand branch, so the dry event is serviced
  against the correct weapon. Until then autoreload is main-hand-only by
  construction — it cannot misfire against the wrong weapon (no off-hand dry event
  exists to misattribute). Inherited debt, not introduced here.
- **Off-hand ordering when the budget only ever covers one.** Once the above lands,
  §4 defers the off-hand to the next attempt; a heavily action-starved build could
  keep deferring it indefinitely. Acceptable (the player can `reload` manually), but
  a fairness pass may be wanted if it bites.
- **A grade-aware selection preference** (keep fewer special/APDS rounds over more
  regular) is inherited from `ammo-and-reloading §11` and remains tied to the
  deferred mixed-ammo question there; autoreload adopts whatever selection that
  spec lands.
- **Proactive opt-in as a second mode.** Some players may prefer reload-on-empty
  despite the "reload after the fight" cost; if requested it becomes a third toggle
  value (`off` / `reactive` / `proactive`) rather than a redesign.

<!-- Scope: a per-character autoreload preference — a toggle (autoreload on|off) that, reactively at fire time, runs the standard timed reload on a dry wielded holder-fed weapon instead of a dry attempt; delegates holder selection + ejection + cost to ammo-and-reloading and action-economy unchanged; two-weapon = main-hand first then off-hand (deferred if budget short); out-of-ammo = report only, rate-limited + ephemeral; default off (opt-in), persisted toggle · Spec style: narrative + acceptance criteria · Detail level: behavior only · Status: SHIPPED 2026-07-10 — the toggle verb + per-character preference (additive omitempty bool, no schema bump — Autoloot/AutoAssist precedent), the read-only firearm-reload peek, and the dry-fire trigger that delegates to the `reload` verb (holder-fed + internally-fed magazine) with a rate-limited out-of-ammo notice, wired at the OnRangedDry seam behind a server master-enable. Builds on ammo-and-reloading + ranged-combat §3 + action-economy (all SHIPPED). All four original open questions resolved (see §8 Decided). -->
