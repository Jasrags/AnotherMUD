package combat

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// AutoAttackConfig bundles the dependencies the auto-attack phase
// needs beyond the Manager (which the Heartbeat already gives it):
// a Locator to resolve CombatantIDs back to live Combatants, a
// RoomLocator to enforce the §4.1 "different room → disengage" rule,
// an EventSink to publish hit/miss/evade/vital-depleted, and a
// Roller for the dice. Bundled into a struct so the NewAutoAttack
// signature stays stable as M9 abilities thread more dependencies
// through.
type AutoAttackConfig struct {
	Locator     Locator
	RoomLocator RoomLocator
	Sink        EventSink
	Roller      Roller
	// Passives evaluates the §4.2 extra-attack and §4.3 defensive-
	// evade passive hooks. combat must not import the abilities
	// feature, so the host injects an implementation (a
	// progression.PassiveResolver satisfies this interface
	// structurally). nil-safe: a config without it falls back to the
	// pre-M9.5 behavior (one swing, no evades).
	Passives PassiveEvaluator
	// CritMultiplier scales the DICE portion of damage on a critical hit
	// (§4.5 "doubled dice" policy); the STR bonus is not multiplied. A
	// value <= 0 is defaulted to DefaultCritMultiplier at NewAutoAttack;
	// 1 disables the bonus (crit = normal damage). IsCritical still flows
	// on the hit event regardless.
	CritMultiplier int
	// HitModAdjust returns a to-hit modifier DELTA for the attacker this
	// swing, keyed on the full attacker CombatantID. The host uses it to
	// apply the light-and-darkness §5.3 darkness penalty (a negative
	// delta when the attacker is in low light); combat itself stays
	// decoupled from the light surface. nil-safe: no adjustment (the
	// pre-light behavior, and tests/headless). The adjustment degrades
	// accuracy only — combat is never blocked.
	HitModAdjust func(attackerID CombatantID) int
	// MassiveDamage configures the saves §4 massive-damage Fortitude save:
	// a single swing whose applied damage is at or above the threshold and
	// did NOT already kill forces the victim to save or suffer the lethal
	// consequence. nil disables the rule entirely (tests/headless and any
	// boot that does not wire it) — combat then behaves exactly as before
	// this slice. When non-nil, FortBonus MUST be set.
	MassiveDamage *MassiveDamageConfig
}

// MassiveDamageConfig parameterizes the saves §4 massive-damage save. All
// magnitudes are host-supplied config (saves §6); combat owns only the
// pipeline placement.
type MassiveDamageConfig struct {
	// Threshold is the single-hit applied damage at or above which the
	// Fortitude save is forced. A non-positive threshold is treated as
	// "every hit" — callers wanting the rule inert at low levels set it
	// high (the engine default keeps it above ordinary swing damage).
	Threshold int
	// DC is the Fortitude difficulty class for the save.
	DC int
	// FortBonus returns the victim's Fortitude save bonus, keyed on the
	// full victim CombatantID. The host resolves it from progression
	// (class base saves + CON modifier). Required when MassiveDamage is
	// set; a nil func panics at NewAutoAttack rather than per-swing.
	FortBonus func(victimID CombatantID) int
}

// PassiveEvaluator is the combat-side seam to the passive-abilities
// feature (spec abilities-and-effects §6). It is keyed on BARE entity
// ids; the auto-attack phase strips the combatant prefix via
// EntityIDOf before calling. Kept to the two hooks combat consumes
// today (small interface, host-implemented).
type PassiveEvaluator interface {
	// ExtraAttacks returns the extra swings entityID earns this round
	// (combat §4.2 swing count).
	ExtraAttacks(entityID string) int
	// DefensiveEvade reports whether one of defenderID's defensive
	// passives pre-empts an incoming swing, and the evading ability's
	// display name (combat §4.3 step 2).
	DefensiveEvade(defenderID string) (string, bool)
}

// NewAutoAttack returns a PhaseFunc implementing combat §4 (pre-
// flight, swing count, per-swing hit/damage). The returned closure
// captures cfg by value and is safe to register on a Heartbeat once;
// the Heartbeat is in turn safe to call from the tick goroutine
// because each invocation operates on a fresh round-start snapshot.
//
// All cfg fields are required. A nil sink would crash on every swing;
// a nil locator or room locator would crash at the pre-flight; a nil
// roller would crash on the first hit/damage roll. Validation here is
// cheap insurance — the wiring path in cmd/anothermud always supplies
// concrete values, but a test that builds an AutoAttackConfig{} by
// accident gets a clear panic at construction rather than a nil
// dereference at the first swing.
func NewAutoAttack(cfg AutoAttackConfig) PhaseFunc {
	if cfg.Locator == nil {
		panic("combat.NewAutoAttack: nil Locator")
	}
	if cfg.RoomLocator == nil {
		panic("combat.NewAutoAttack: nil RoomLocator")
	}
	if cfg.Sink == nil {
		panic("combat.NewAutoAttack: nil Sink")
	}
	if cfg.Roller == nil {
		panic("combat.NewAutoAttack: nil Roller")
	}
	if cfg.CritMultiplier <= 0 {
		cfg.CritMultiplier = DefaultCritMultiplier
	}
	if cfg.MassiveDamage != nil && cfg.MassiveDamage.FortBonus == nil {
		panic("combat.NewAutoAttack: MassiveDamage set without FortBonus")
	}

	return func(ctx context.Context, attackerID CombatantID, mgr *Manager, _ uint64) {
		runAutoAttack(ctx, attackerID, mgr, cfg)
	}
}

// runAutoAttack executes the §4 sequence for one attacker against
// their current primary target. Extracted from the closure so the
// flow reads top-to-bottom against the spec sections.
func runAutoAttack(ctx context.Context, attackerID CombatantID, mgr *Manager, cfg AutoAttackConfig) {
	// §4.1 pre-flight: primary target presence.
	targetID, hasTarget := mgr.PrimaryTargetOf(attackerID)
	if !hasTarget {
		return
	}
	attacker, ok := cfg.Locator.LookupCombatant(attackerID)
	if !ok {
		// Attacker disappeared between the round snapshot and now.
		// DisengageAll for the attacker cleans up the asymmetric
		// state on every opponent's list.
		mgr.DisengageAll(ctx, attackerID, "")
		return
	}
	attackerRoom, attackerRoomOK := cfg.RoomLocator.RoomOf(attackerID)
	if !attackerRoomOK {
		// Attacker is in no tracked room (logged-out mid-round,
		// despawned mob). Treat symmetrically to the target-missing
		// branch below — disengage from everyone so no opponent
		// holds a stale entry pointing at this id, and skip the
		// swing. The empty RoomID on the CombatEnded payload is
		// acceptable here: combat itself does not consult it, and
		// subscribers that need a room (renderer, quest hooks) skip
		// events that lack one.
		mgr.DisengageAll(ctx, attackerID, "")
		return
	}

	target, targetOK := cfg.Locator.LookupCombatant(targetID)
	if !targetOK || target.Vitals().IsDead() {
		mgr.Disengage(ctx, attackerID, targetID, attackerRoom)
		return
	}
	targetRoom, targetRoomOK := cfg.RoomLocator.RoomOf(targetID)
	if !targetRoomOK || targetRoom != attackerRoom {
		mgr.Disengage(ctx, attackerID, targetID, attackerRoom)
		return
	}

	// §4.2 swing count = 1 + extra-attack. Extra-attack is a passive-
	// abilities concern: the evaluator binary-checks the attacker's
	// extra_attack passives once per round (spec abilities §6). nil
	// evaluator (tests / headless) ⇒ exactly one swing.
	swings := 1 + extraAttackCount(cfg.Passives, attacker)

	atkStats := attacker.Stats()
	defStats := target.Stats()
	// §5.3 darkness penalty: the host adjusts the attacker's to-hit by
	// their effective light once per round (the light doesn't shift
	// mid-round under normal play). nil hook ⇒ no adjustment.
	hitMod := atkStats.HitMod
	if cfg.HitModAdjust != nil {
		hitMod += cfg.HitModAdjust(attackerID)
	}
	damageExpr := atkStats.EffectiveDamage()
	weaponName := atkStats.EffectiveWeaponName()
	// §4 weapon critical: the wielded weapon's threat range + multiplier
	// override the engine defaults. An unset threat-low (0) means "only
	// the natural maximum threatens" (20); an unset multiplier (0) falls
	// back to the configured global default. Stable for the round.
	critThreatLow := atkStats.CritThreatLow
	if critThreatLow <= 0 {
		critThreatLow = 20
	}
	critMultiplier := atkStats.CritMultiplier
	if critMultiplier <= 0 {
		critMultiplier = cfg.CritMultiplier
	}
	atkName := attacker.Name()
	tgtName := target.Name()

	for i := 0; i < swings; i++ {
		// §4.3 step 1 — live check is folded into ApplyDamageIfAlive
		// below (single lock acquisition). Early-exit here if the
		// target is already dead so we skip the full hit/damage
		// computation; the canonical "did the swing land" decision
		// still happens atomically at the damage-apply site, so a
		// concurrent killer cannot trick this branch into double-
		// emitting VitalDepleted.
		if target.Vitals().IsDead() {
			return
		}

		// §4.3 step 2 — defensive passive evade. The evaluator
		// binary-checks the DEFENDER's defensive passives; the first
		// that fires pre-empts this swing. nil evaluator ⇒ no evades.
		if ability, evaded := defensiveEvade(cfg.Passives, target); evaded {
			cfg.Sink.OnEvade(ctx, Evade{
				AttackerID:   attackerID,
				TargetID:     targetID,
				AttackerName: atkName,
				TargetName:   tgtName,
				AbilityName:  ability,
				RoomID:       attackerRoom,
			})
			continue
		}

		// §4.4 hit roll (attacker hit-mod already adjusted for darkness;
		// threat range from the wielded weapon, weapon-identity §4).
		outcome := rollHit(cfg.Roller, hitMod, defStats.AC, critThreatLow)
		if !outcome.hit {
			cfg.Sink.OnMiss(ctx, Miss{
				AttackerID:   attackerID,
				TargetID:     targetID,
				AttackerName: atkName,
				TargetName:   tgtName,
				WeaponName:   weaponName,
				IsFumble:     outcome.fumble,
				RoomID:       attackerRoom,
			})
			continue
		}

		// §4.5 damage roll: dice + STR bonus, clamped to >= 1 on hit.
		//
		// CRIT POLICY: on a critical hit the rolled DICE are multiplied
		// by cfg.CritMultiplier (default 2 — the §4.5 "doubled dice"
		// option); the STR bonus is added afterward and is NOT
		// multiplied. A multiplier of 1 restores the original "crit =
		// normal damage" policy. IsCritical still flows on the event so
		// renderers can dramatize either way.
		dmg := damageExpr.Roll(cfg.Roller)
		if outcome.critical && critMultiplier > 1 {
			dmg *= critMultiplier
		}
		raw := dmg + STRBonus(atkStats.STR)
		if raw < 1 {
			raw = 1
		}

		remainingHP, wasAlive := target.Vitals().ApplyDamageIfAlive(raw)
		if !wasAlive {
			// A concurrent damage source (DoT effect, ability, racing
			// swing from a future parallel phase) killed the target
			// between the early-exit live check above and this
			// damage-apply. Treat as if our swing never landed: no
			// Hit event, no VitalDepleted (the other source emits it),
			// and we stop swinging on a corpse.
			return
		}
		cfg.Sink.OnHit(ctx, Hit{
			AttackerID:   attackerID,
			TargetID:     targetID,
			AttackerName: atkName,
			TargetName:   tgtName,
			WeaponName:   weaponName,
			Damage:       raw,
			DamageType:   DamageTypePhysical,
			IsCritical:   outcome.critical,
			RoomID:       attackerRoom,
		})

		if remainingHP <= 0 {
			cfg.Sink.OnVitalDepleted(ctx, VitalDepleted{
				VictimID:   targetID,
				VictimName: tgtName,
				AttackerID: attackerID,
				Vital:      VitalHP,
				RoomID:     attackerRoom,
			})
			// §4.3 "If HP reached zero ... stop further swings."
			return
		}

		// saves §4 — massive-damage Fortitude save. A single swing whose
		// applied damage meets the threshold and did NOT already kill (the
		// branch above handled that) forces the victim to save; a failure
		// drives the lethal consequence through the same VitalDepleted
		// death path. A success leaves the already-applied normal damage
		// untouched. nil MassiveDamage ⇒ rule disabled (pre-slice behavior).
		if cfg.MassiveDamage != nil && raw >= cfg.MassiveDamage.Threshold {
			if massiveDamageKill(ctx, cfg, attackerID, targetID, tgtName, attackerRoom) {
				// §4.3 stop swinging on a corpse.
				return
			}
		}
	}
}

// massiveDamageKill resolves the saves §4 Fortitude save for a victim who
// just took a threshold-meeting hit and survived it, emits the SaveResolved
// event either way, and on failure depletes the victim and emits exactly one
// VitalDepleted (guarded by Deplete's wasAlive, so a concurrent killer cannot
// double-emit). Returns true when the victim died (caller stops swinging).
func massiveDamageKill(ctx context.Context, cfg AutoAttackConfig, attackerID, targetID CombatantID, tgtName string, room world.RoomID) bool {
	bonus := cfg.MassiveDamage.FortBonus(targetID)
	outcome := ResolveSave(cfg.Roller, bonus, cfg.MassiveDamage.DC)
	cfg.Sink.OnSaveResolved(ctx, SaveResolved{
		CreatureID:   targetID,
		CreatureName: tgtName,
		SaveType:     SaveAxisFortitude,
		Cause:        SaveCauseMassiveDamage,
		Outcome:      outcome,
		RoomID:       room,
	})
	if outcome.Success {
		return false
	}
	target, ok := cfg.Locator.LookupCombatant(targetID)
	if !ok {
		// Victim vanished between the swing and the save resolution; treat
		// as already gone — no death event to emit here.
		return true
	}
	if wasAlive := target.Vitals().Deplete(); wasAlive {
		cfg.Sink.OnVitalDepleted(ctx, VitalDepleted{
			VictimID:   targetID,
			VictimName: tgtName,
			AttackerID: attackerID,
			Vital:      VitalHP,
			RoomID:     room,
		})
	}
	return true
}

// hitOutcome is the local result of one §4.4 roll. fumble and
// critical are mutually exclusive; both fold into hit for the
// caller's branch, but the swing event payload needs each flag
// separately.
type hitOutcome struct {
	hit      bool
	critical bool
	fumble   bool
}

// rollHit resolves one §4.4 attack roll. threatLow is the lowest d20 face
// that threatens a critical (weapon-identity §4): a roll at or above it is
// an automatic critical hit, generalizing the natural-maximum rule (a
// threatLow of 20 reproduces the old "only a natural 20 crits" behavior).
// A natural 1 is always a fumble and is never a threat.
func rollHit(r Roller, hitMod, ac, threatLow int) hitOutcome {
	raw := r.IntN(20) + 1
	if raw == 1 {
		return hitOutcome{hit: false, fumble: true}
	}
	if raw >= threatLow {
		return hitOutcome{hit: true, critical: true}
	}
	return hitOutcome{hit: raw+hitMod >= ac}
}

// extraAttackCount is the §4.2 passive-ability hook. Delegates to the
// injected PassiveEvaluator, keyed on the attacker's bare entity id.
// nil evaluator ⇒ zero (one swing).
func extraAttackCount(p PassiveEvaluator, c Combatant) int {
	if p == nil {
		return 0
	}
	return p.ExtraAttacks(EntityIDOf(c.CombatantID()))
}

// defensiveEvade is the §4.3 step 2 passive-ability hook. Delegates to
// the injected PassiveEvaluator with the DEFENDER's bare entity id;
// returns the evading ability's display name + true when a defensive
// passive pre-empts the swing. nil evaluator ⇒ ("", false).
func defensiveEvade(p PassiveEvaluator, defender Combatant) (string, bool) {
	if p == nil {
		return "", false
	}
	return p.DefensiveEvade(EntityIDOf(defender.CombatantID()))
}

// SortPlayersFirst reorders a snapshot in place so player combatants
// resolve before mobs (combat §4.1 "Player combatants SHOULD be
// ordered before mob combatants"). Within each group, the existing
// (map-iteration) order is preserved — combat does not prescribe a
// secondary order, and stable partition is enough for the tie-break
// rule the spec calls out.
//
// Heartbeat applies this to the round-start snapshot before any
// phase runs, so every phase sees the same iteration order. Exported
// for the heartbeat to call; not part of the phase function surface.
func SortPlayersFirst(ids []CombatantID) {
	if len(ids) < 2 {
		return
	}
	// Two-pointer stable partition: i scans for the next player to
	// move forward; w points at the next slot a player goes into. We
	// rotate the run between w and i so the relative order of
	// already-classified slots is preserved.
	w := 0
	for i := 0; i < len(ids); i++ {
		if !isPlayerID(ids[i]) {
			continue
		}
		if i == w {
			w++
			continue
		}
		// Rotate ids[w..i] right by one so ids[i] lands at ids[w].
		v := ids[i]
		copy(ids[w+1:i+1], ids[w:i])
		ids[w] = v
		w++
	}
}

func isPlayerID(id CombatantID) bool {
	s := string(id)
	return len(s) >= len(PlayerPrefix) && s[:len(PlayerPrefix)] == PlayerPrefix
}
