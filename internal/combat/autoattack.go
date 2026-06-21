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
	// Incapacitated reports whether the attacker is too disabled to land
	// any combat swings this round (conditions §3 — a stunned attacker).
	// When it returns true the attacker takes no swings but stays engaged
	// (it is NOT disengaged — combat resumes when the condition lifts).
	// Keyed on the full attacker CombatantID; the host resolves it from the
	// attacker's active condition flags. nil-safe: never incapacitated (the
	// pre-conditions behavior, tests/headless).
	Incapacitated func(attackerID CombatantID) bool
	// CleaveFor reports whether the attacker has Cleave and/or Great Cleave
	// (feats Bucket C), keyed on the full attacker CombatantID. When a melee
	// swing drops its target, a Cleave-capable attacker makes one bonus melee
	// swing against another engaged foe in the room (Great Cleave keeps cleaving
	// as long as each bonus swing also drops a foe). The host resolves it from
	// the attacker's held feats. nil-safe: no cleaving (the pre-feat behavior,
	// tests/headless). Returns (hasCleave, hasGreatCleave); great implies cleave.
	CleaveFor func(attackerID CombatantID) (cleave, greatCleave bool)
	// DefenderHitAdjust returns a to-hit DELTA applied to every swing aimed
	// at this target (conditions §3 — a prone/stunned/blinded victim is
	// easier to hit). It is the mirror of HitModAdjust: HitModAdjust is
	// keyed on the ATTACKER (darkness, proficiency), this on the DEFENDER
	// (vulnerability). The host resolves it from the target's condition
	// flags. nil-safe: no adjustment. Summed into the effective hit modifier
	// alongside the attacker adjustments before the roll (§4.4).
	DefenderHitAdjust func(targetID CombatantID) int
	// AmmoFor resolves a projectile swing's ammunition (ranged-combat §3).
	// It is called once per swing ONLY when the attacker's wielded weapon is
	// a projectile (Stats.RangedClass == projectile). The host consumes one
	// matching ammo unit from the attacker's inventory and returns
	// canFire=true plus any masterwork-ammo to-hit bonus; with no matching
	// ammo it returns canFire=false and the swing is skipped (a RangedDry
	// event, the attacker stays engaged). Keyed on the full attacker
	// CombatantID. nil-safe: a nil hook fires every projectile swing with no
	// ammo bonus (tests/headless and any boot that doesn't wire ammo) — combat
	// then treats a bow like an infinite-ammo melee weapon. Thrown and melee
	// weapons never call this hook.
	AmmoFor func(attackerID CombatantID) (canFire bool, toHitBonus int)
	// RangeFalloff is the to-hit penalty a projectile takes per band of
	// distance from melee (ranged-combat §5.3) — at the near band it is
	// -RangeFalloff, at far -2×RangeFalloff, and so on. PointBlankPenalty is
	// the to-hit penalty a projectile takes when firing AT the melee band
	// (awkward up close). Both are host policy (§8); zero (tests/headless)
	// means no band-distance to-hit effect, so a projectile shoots at face
	// value from any band. Only projectile weapons consult these — melee and
	// thrown auto-close instead.
	RangeFalloff      int
	PointBlankPenalty int
	// SecondaryOffHandPenalty is the cumulative to-hit penalty applied to each
	// off-hand strike AFTER the first when Improved Two-Weapon Fighting grants
	// more than one (two-weapon-fighting §4.3): strike i (0-based) takes
	// i×SecondaryOffHandPenalty on top of its baked off-hand penalty, so the
	// second strike takes it once, a third twice. Host policy (§6, the source's
	// −5); zero (tests/headless) means extra off-hand strikes fire at the same
	// accuracy as the first. Only consulted when an off-hand profile grants more
	// than one strike — single-strike dual-wielding never reads it.
	SecondaryOffHandPenalty int
	// SetDamageBonus is the extra damage a `set` weapon (special-weapons §4)
	// deals on a braced blow against a foe that CHARGED into strike range this
	// round (the foe closed a band toward the wielder). Added to the round's
	// DamageBonus and consumed once per charge — hit or miss, the braced moment
	// passes. Host policy; zero (tests/headless) means a set weapon plays as an
	// ordinary weapon, so a fight with no set weapon is unchanged.
	SetDamageBonus int
	// KitePolicy decides whether a projectile combatant should WITHDRAW this
	// round to keep distance instead of shooting (ranged-combat §5.4 kiting AI).
	// Called only for a projectile attacker that has room to open the band
	// (band < far); returning true opens one band and skips the shot. The host
	// wires it for mobs (players kite manually via the withdraw verb) and SHOULD
	// make it probabilistic — a deterministic kite stalemates against a foe that
	// closes one band per round. nil-safe: no auto-kiting (tests/headless and
	// the pre-MR2 behavior).
	KitePolicy func(attackerID, targetID CombatantID, band int) bool
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

	// conditions §3 — incapacitation. A stunned (or otherwise incapacitated)
	// attacker lands no swings this round but stays engaged: combat is not
	// disengaged, so it resumes swinging the moment the condition lifts.
	// Checked after the target/room validation above so a despawned target
	// is still cleaned up first.
	if cfg.Incapacitated != nil && cfg.Incapacitated(attackerID) {
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
	// conditions §3 — vulnerability. A prone/stunned/blinded defender is
	// easier to hit: the host returns a positive delta keyed on the target,
	// summed into the attacker's effective hit modifier (mirror of the
	// attacker-keyed HitModAdjust above). Stable for the round.
	// Both deltas are per-attacker/per-target (not per-weapon), so the
	// off-hand swing (two-weapon-fighting §4.1) reuses the same hitAdjust.
	hitAdjust := 0
	if cfg.HitModAdjust != nil {
		hitAdjust += cfg.HitModAdjust(attackerID)
	}
	if cfg.DefenderHitAdjust != nil {
		hitAdjust += cfg.DefenderHitAdjust(targetID)
	}
	hitMod := atkStats.HitMod + hitAdjust
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

	// ranged-combat §5.2/§5.3 — range bands. The band is the distance between
	// this attacker and its target (meleeBand when untracked, so a melee fight
	// is unchanged). Only a PROJECTILE can strike from range; a melee/thrown/
	// unarmed combatant out of melee range CLOSES one band this round instead
	// of swinging (the auto-close that produces an archer's opening volley).
	band := mgr.BandOf(attackerID, targetID)
	isProjectile := atkStats.RangedClass == RangedProjectile
	// special-weapons §3 — reach. A reach melee weapon (Reach > 0) strikes at the
	// `near` band as well as melee, so it does NOT auto-close from near (it is
	// already in range) — landing the polearm's opening blows on a foe still
	// closing. It still closes from `far` (reach is one band, not unlimited).
	canStrikeHere := band == meleeBand || (atkStats.Reach > 0 && band == nearBand)
	if !canStrikeHere && !isProjectile {
		newBand := mgr.AdjustBand(attackerID, targetID, -1)
		// Closing toward the foe is a charge — the foe may answer with a braced
		// set-weapon blow next round (special-weapons §4).
		mgr.recordCharge(attackerID, targetID)
		cfg.Sink.OnBandChange(ctx, BandChange{
			SubjectID:    attackerID,
			SubjectName:  atkName,
			OpponentID:   targetID,
			OpponentName: tgtName,
			NewBand:      newBand,
			NewBandName:  BandName(newBand),
			Closing:      true,
			RoomID:       attackerRoom,
		})
		return
	}
	// ranged-combat §5.4 — kiting (mob AI). A projectile combatant with room to
	// open the distance (band < far) may WITHDRAW this round instead of shooting,
	// keeping a closing melee foe at bay. The host decides via KitePolicy (mobs
	// only — players kite with the withdraw verb); it is deliberately
	// probabilistic so the foe still net-closes (a deterministic kite would
	// stalemate at one-band-per-round) and the kiter trades the shot for the
	// step. nil hook ⇒ no auto-kiting (the pre-MR2 behavior).
	if isProjectile && band < farBand() && cfg.KitePolicy != nil && cfg.KitePolicy(attackerID, targetID, band) {
		newBand := mgr.AdjustBand(attackerID, targetID, +1)
		cfg.Sink.OnBandChange(ctx, BandChange{
			SubjectID:    attackerID,
			SubjectName:  atkName,
			OpponentID:   targetID,
			OpponentName: tgtName,
			NewBand:      newBand,
			NewBandName:  BandName(newBand),
			Closing:      false,
			RoomID:       attackerRoom,
		})
		return
	}
	// A projectile's accuracy depends on the band: a per-band falloff at range,
	// or the point-blank penalty when firing at the melee band (§5.3). Folded
	// into the round-stable hit modifier. Zero config ⇒ no band effect.
	if isProjectile {
		if band == meleeBand {
			hitMod -= cfg.PointBlankPenalty
		} else {
			hitMod -= cfg.RangeFalloff * band
		}
	}

	// special-weapons §4 — set vs a charge. A braced `set` weapon answering a foe
	// that charged into strike range this round (consumed once per charge) lands a
	// bonus blow. Folded into the round's DamageBonus so it flows through the
	// normal damage pipeline; melee-only (a charge closes to a melee weapon's
	// reach). No-op when the weapon isn't `set`, the foe didn't charge, or the
	// bonus is unconfigured — every non-set fight is unchanged.
	if atkStats.Set && cfg.SetDamageBonus != 0 && mgr.ConsumeCharge(targetID, attackerID) {
		atkStats.DamageBonus += cfg.SetDamageBonus
	}

	in := swingInputs{
		attackerID:     attackerID,
		targetID:       targetID,
		attacker:       attacker,
		target:         target,
		atkStats:       atkStats,
		defStats:       defStats,
		atkName:        atkName,
		tgtName:        tgtName,
		weaponName:     weaponName,
		hitMod:         hitMod,
		damageExpr:     damageExpr,
		critThreatLow:  critThreatLow,
		critMultiplier: critMultiplier,
		attackerRoom:   attackerRoom,
	}
	for i := 0; i < swings; i++ {
		switch resolveSwing(ctx, in, cfg) {
		case swingKill:
			cleaveFollowUp(ctx, in, mgr, cfg)
			return
		case swingStop:
			return
		}
	}

	// two-weapon-fighting §3 — the off-hand attack(s). After the main swing(s),
	// a dual-wielding attacker makes its off-hand strike(s) with its off-hand
	// weapon profile (its own dice/crit/type, the off-hand-penalized hit, and the
	// ½× Strength damage — all baked in by the producer's Stats()). The profile's
	// Attacks count (§3.1) is one by default; Improved Two-Weapon Fighting raises
	// it. Each strike after the first takes the cumulative secondary off-hand
	// penalty (§4.3). Off-hand strikes are melee (suppressed while the main weapon
	// fires as a projectile, §3) and reuse the same per-swing resolver. Reaching
	// here means the target survived every main swing (a kill returns above), so
	// the first off-hand strike is made against a live target; a strike that kills
	// stops the remaining off-hand strikes (swingStop), mirroring the main loop.
	if off := atkStats.OffHand; off != nil && !isProjectile {
		offThreatLow := off.CritThreatLow
		if offThreatLow <= 0 {
			offThreatLow = 20
		}
		offMult := off.CritMultiplier
		if offMult <= 0 {
			offMult = cfg.CritMultiplier
		}
		// Copy the round's attacker stats and swap in the off-hand weapon's
		// profile so resolveSwing reads the off-hand DamageBonus / damage types
		// (it sources those from in.atkStats), while the swingInputs fields carry
		// the off-hand dice / name / crit / hit. RangedClass clears with the copy
		// override so the ammo gate never fires for the melee off-hand swing.
		offStats := atkStats
		offStats.Damage = off.Damage
		offStats.WeaponName = off.WeaponName
		offStats.WeaponDamageTypes = off.WeaponDamageTypes
		offStats.DamageBonus = off.DamageBonus
		offStats.CritThreatLow = off.CritThreatLow
		offStats.CritMultiplier = off.CritMultiplier
		offStats.RangedClass = ""
		offStats.OffHand = nil
		offIn := in
		offIn.atkStats = offStats
		offIn.weaponName = offStats.EffectiveWeaponName()
		offIn.damageExpr = offStats.EffectiveDamage()
		offIn.critThreatLow = offThreatLow
		offIn.critMultiplier = offMult
		offHandSwings := off.Attacks
		if offHandSwings < 1 {
			offHandSwings = 1
		}
		for i := 0; i < offHandSwings; i++ {
			// Strike i takes i× the cumulative secondary off-hand penalty (§4.3):
			// the first strike is unpenalized beyond its baked off-hand penalty.
			offIn.hitMod = off.HitMod + hitAdjust - i*cfg.SecondaryOffHandPenalty
			switch resolveSwing(ctx, offIn, cfg) {
			case swingKill:
				// An off-hand kill cleaves with the MAIN-hand profile (`in`),
				// not the off-hand one — the bonus swing is a main-hand strike.
				cleaveFollowUp(ctx, in, mgr, cfg)
				return
			case swingStop:
				return
			}
		}
	}
}

// cleaveFollowUp makes Cleave's bonus melee swing(s) after the attacker drops a
// foe (feats Bucket C — wot-feats.md). Cleave grants ONE bonus swing against
// another engaged foe in the room at the attacker's MAIN-hand profile and bonus;
// Great Cleave keeps cleaving as long as each bonus swing also drops its target
// and a fresh foe remains. `in` is always the main-hand swingInputs (an off-hand
// kill still cleaves with the main hand). No-op without the CleaveFor hook, for a
// non-cleaving attacker, or for a projectile killer (Cleave is a melee feat).
//
// Runs on the tick goroutine inside the attacker's round (same single-goroutine
// guarantee as the swing loop), so the Manager / Locator reads need no extra
// synchronization beyond their own.
func cleaveFollowUp(ctx context.Context, in swingInputs, mgr *Manager, cfg AutoAttackConfig) {
	if cfg.CleaveFor == nil || in.atkStats.RangedClass == RangedProjectile {
		return
	}
	cleave, great := cfg.CleaveFor(in.attackerID)
	if !cleave {
		return
	}
	// The target-independent part of the attacker's to-hit (weapon + Weapon Focus
	// + Power Attack stance are already in atkStats.HitMod; darkness is the only
	// attacker-keyed delta). The per-target defender vulnerability is re-added per
	// foe below, reconstructing the same formula runAutoAttack used — minus the
	// projectile band terms, which never apply (cleave is melee-gated above).
	attackerHitBase := in.atkStats.HitMod
	if cfg.HitModAdjust != nil {
		attackerHitBase += cfg.HitModAdjust(in.attackerID)
	}
	// Exclude the just-killed target. The bounded set of distinct opponents (each
	// excluded once chosen) guarantees termination even if every bonus swing kills.
	excluded := map[CombatantID]bool{in.targetID: true}
	for {
		opp, oppID, ok := nextCleaveTarget(in.attackerID, in.attackerRoom, excluded, mgr, cfg)
		if !ok {
			return
		}
		excluded[oppID] = true
		next := in
		next.targetID = oppID
		next.target = opp
		next.tgtName = opp.Name()
		next.defStats = opp.Stats()
		next.hitMod = attackerHitBase
		if cfg.DefenderHitAdjust != nil {
			next.hitMod += cfg.DefenderHitAdjust(oppID)
		}
		result := resolveSwing(ctx, next, cfg)
		// Cleave is one bonus swing per round regardless of outcome; Great Cleave
		// chains another swing only when this one also dropped its target.
		if result == swingKill && great {
			continue
		}
		return
	}
}

// nextCleaveTarget picks the first engaged opponent of attackerID that is alive,
// in room, and not already excluded — the foe a Cleave bonus swing strikes. The
// just-killed target is excluded by the caller (and would also fail the dead
// check). Order follows the Manager's opponent list, so selection is deterministic.
func nextCleaveTarget(attackerID CombatantID, room world.RoomID, excluded map[CombatantID]bool, mgr *Manager, cfg AutoAttackConfig) (Combatant, CombatantID, bool) {
	for _, oppID := range mgr.OpponentsOf(attackerID) {
		if excluded[oppID] {
			continue
		}
		opp, ok := cfg.Locator.LookupCombatant(oppID)
		if !ok || opp.Vitals().IsDead() {
			continue
		}
		if oppRoom, ok := cfg.RoomLocator.RoomOf(oppID); !ok || oppRoom != room {
			continue
		}
		return opp, oppID, true
	}
	return nil, "", false
}

// swingInputs bundles the round-stable inputs one swing reads, so the shared
// per-swing resolver takes a single argument rather than a dozen. Every field
// is computed once at round start (runAutoAttack) or once at the one-shot call
// site (ResolveSingleAttack) — none mutate across swings.
type swingInputs struct {
	attackerID, targetID CombatantID
	attacker, target     Combatant
	atkStats, defStats   Stats
	atkName, tgtName     string
	weaponName           string
	hitMod               int
	damageExpr           DiceExpr
	critThreatLow        int
	critMultiplier       int
	attackerRoom         world.RoomID
}

// swingResult signals whether the caller should keep swinging.
type swingResult int

const (
	// swingContinue: this swing resolved (hit / miss / evade / dry / non-lethal
	// hit); a multi-swing caller may proceed to the next swing.
	swingContinue swingResult = iota
	// swingStop: the target is gone or dead by another hand (already-dead check
	// or a concurrent killer); the caller must stop swinging but did NOT land the
	// killing blow itself.
	swingStop
	// swingKill: THIS swing dealt the killing blow. The caller must stop swinging
	// at the (now dead) target, exactly like swingStop, but may additionally
	// trigger on-kill follow-ups (Cleave — feats Bucket C). Kept distinct from
	// swingStop so a Cleave swing fires only on a kill the attacker actually
	// caused, never on a corpse a concurrent source produced.
	swingKill
)

// resolveSwing resolves exactly one attack swing — the §4.3–§4.5 body plus the
// ranged-ammo gate (ranged-combat §3) and the massive-damage save (saves §4) —
// shared by runAutoAttack's round loop and the one-shot ResolveSingleAttack
// (ranged-combat §3 throw). It emits the swing's outcome through cfg.Sink and
// returns swingStop when the target is gone/dead (caller stops), swingContinue
// otherwise. All cfg hooks are nil-safe, so a minimal config (sink + roller)
// drives a complete swing.
func resolveSwing(ctx context.Context, in swingInputs, cfg AutoAttackConfig) swingResult {
	// §4.3 step 1 — live check is folded into ApplyDamageIfAlive below (single
	// lock acquisition). Early-exit here if the target is already dead so we
	// skip the full hit/damage computation; the canonical "did the swing land"
	// decision still happens atomically at the damage-apply site, so a
	// concurrent killer cannot trick this branch into double-emitting
	// VitalDepleted.
	if in.target.Vitals().IsDead() {
		return swingStop
	}

	// §4.3 step 2 — defensive passive evade. The evaluator binary-checks the
	// DEFENDER's defensive passives; the first that fires pre-empts this swing.
	// nil evaluator ⇒ no evades.
	if ability, evaded := defensiveEvade(cfg.Passives, in.target); evaded {
		cfg.Sink.OnEvade(ctx, Evade{
			AttackerID:   in.attackerID,
			TargetID:     in.targetID,
			AttackerName: in.atkName,
			TargetName:   in.tgtName,
			AbilityName:  ability,
			RoomID:       in.attackerRoom,
		})
		return swingContinue
	}

	// ranged-combat §3 — ammunition. A projectile swing consumes one matching
	// ammo unit; with none available the swing is skipped (a RangedDry event)
	// and the attacker stays engaged — it resumes the moment ammo returns.
	// Masterwork ammo returns a to-hit bonus folded into this swing only.
	// Thrown/melee weapons never enter this branch, and a nil AmmoFor fires
	// every projectile swing (unwired/headless).
	swingHitMod := in.hitMod
	if in.atkStats.RangedClass == RangedProjectile && cfg.AmmoFor != nil {
		canFire, ammoBonus := cfg.AmmoFor(in.attackerID)
		if !canFire {
			cfg.Sink.OnRangedDry(ctx, RangedDry{
				AttackerID:   in.attackerID,
				TargetID:     in.targetID,
				AttackerName: in.atkName,
				TargetName:   in.tgtName,
				WeaponName:   in.weaponName,
				AmmoKind:     in.atkStats.AmmoKind,
				RoomID:       in.attackerRoom,
			})
			return swingContinue
		}
		swingHitMod += ammoBonus
	}

	// §4.4 hit roll (attacker hit-mod already adjusted for darkness + per-swing
	// masterwork-ammo bonus; threat range from the wielded weapon,
	// weapon-identity §4).
	outcome := rollHit(cfg.Roller, swingHitMod, in.defStats.AC, in.critThreatLow)
	if !outcome.hit {
		cfg.Sink.OnMiss(ctx, Miss{
			AttackerID:   in.attackerID,
			TargetID:     in.targetID,
			AttackerName: in.atkName,
			TargetName:   in.tgtName,
			WeaponName:   in.weaponName,
			IsFumble:     outcome.fumble,
			RoomID:       in.attackerRoom,
		})
		return swingContinue
	}

	// §4.5 damage roll: dice + STR bonus, clamped to >= 1 on hit.
	//
	// CRIT POLICY: on a critical hit the rolled DICE are multiplied by the
	// crit multiplier (default 2 — the §4.5 "doubled dice" option); the STR
	// bonus is added afterward and is NOT multiplied. A multiplier of 1
	// restores the original "crit = normal damage" policy. IsCritical still
	// flows on the event so renderers can dramatize either way.
	dmg := in.damageExpr.Roll(cfg.Roller)
	if outcome.critical && in.critMultiplier > 1 {
		dmg *= in.critMultiplier
	}
	// §4.5 damage: rolled (crit-multiplied) dice + the attacker's flat
	// DamageBonus, minus the defender's soak. DamageBonus is added after the
	// crit multiply (the bonus is not multiplied). Soak is the type-agnostic
	// Mitigation (the channel layer's `mitigation` channel, design §6; 0 for
	// fantasy) PLUS the defender's per-damage-type Resistance against the
	// attacker's weapon types (armor-depth §4) — the two compose additively.
	// The per-swing minimum of 1 still holds, so a landed hit always lands ≥1
	// even under full soak.
	soak := in.defStats.Mitigation + TypedResistance(in.defStats.Resistances, in.atkStats.WeaponDamageTypes)
	raw := dmg + in.atkStats.DamageBonus - soak
	if raw < 1 {
		raw = 1
	}

	remainingHP, wasAlive := in.target.Vitals().ApplyDamageIfAlive(raw)
	if !wasAlive {
		// A concurrent damage source (DoT effect, ability, racing swing from a
		// future parallel phase) killed the target between the early-exit live
		// check above and this damage-apply. Treat as if our swing never
		// landed: no Hit event, no VitalDepleted (the other source emits it),
		// and we stop swinging on a corpse.
		return swingStop
	}
	cfg.Sink.OnHit(ctx, Hit{
		AttackerID:   in.attackerID,
		TargetID:     in.targetID,
		AttackerName: in.atkName,
		TargetName:   in.tgtName,
		WeaponName:   in.weaponName,
		Damage:       raw,
		DamageType:   DamageTypePhysical,
		IsCritical:   outcome.critical,
		RoomID:       in.attackerRoom,
	})

	if remainingHP <= 0 {
		cfg.Sink.OnVitalDepleted(ctx, VitalDepleted{
			VictimID:   in.targetID,
			VictimName: in.tgtName,
			AttackerID: in.attackerID,
			Vital:      VitalHP,
			RoomID:     in.attackerRoom,
		})
		// §4.3 "If HP reached zero ... stop further swings." This swing landed the
		// killing blow, so a Cleave-capable attacker may follow up (Bucket C).
		return swingKill
	}

	// saves §4 — massive-damage Fortitude save. A single swing whose applied
	// damage meets the threshold and did NOT already kill (the branch above
	// handled that) forces the victim to save; a failure drives the lethal
	// consequence through the same VitalDepleted death path. A success leaves
	// the already-applied normal damage untouched. nil MassiveDamage ⇒ rule
	// disabled (pre-slice behavior).
	if cfg.MassiveDamage != nil && raw >= cfg.MassiveDamage.Threshold {
		if massiveDamageKill(ctx, cfg, in.attackerID, in.targetID, in.tgtName, in.attackerRoom) {
			// §4.3 stop swinging on a corpse; the massive-damage save killed the
			// victim by this attacker's swing, so Cleave may follow up (Bucket C).
			return swingKill
		}
	}
	return swingContinue
}

// ResolveSingleAttack resolves exactly ONE attack swing from attacker against
// target outside the round loop — the thrown-weapon one-shot (ranged-combat
// §3). It reuses the same §4.4–§4.5 resolution the auto-attack loop uses
// (resolveSwing), reading the attacker's current Stats so a thrown weapon's
// full-Strength damage and crit profile apply, the defender's AC/soak, and the
// Hit/Miss/VitalDepleted emission through the Manager's sink (so a thrown kill
// runs the identical death flow as a weapon swing). The caller (the throw verb)
// owns engagement, weapon removal, and the land-in-room / masterwork-destroy
// rule; this method only resolves the swing.
//
// roller supplies the d20 + damage rolls; critMult is the dice multiplier on a
// crit (<=0 ⇒ DefaultCritMultiplier, overridden by a per-weapon multiplier when
// the weapon declares one). A thrown weapon is its own ammunition, so no ammo
// hook fires; the massive-damage rule is not applied to a one-shot throw.
// room is stamped on the emitted events for renderers. Returns true when the
// target is still alive after the swing (false when it was missing/already
// dead, or the swing killed it).
func (m *Manager) ResolveSingleAttack(ctx context.Context, attackerID, targetID CombatantID, room world.RoomID, roller Roller, critMult int) bool {
	attacker, ok := m.locator.LookupCombatant(attackerID)
	if !ok {
		return false
	}
	target, ok := m.locator.LookupCombatant(targetID)
	if !ok || target.Vitals().IsDead() {
		return false
	}
	if critMult <= 0 {
		critMult = DefaultCritMultiplier
	}

	atkStats := attacker.Stats()
	defStats := target.Stats()
	critThreatLow := atkStats.CritThreatLow
	if critThreatLow <= 0 {
		critThreatLow = 20
	}
	critMultiplier := atkStats.CritMultiplier
	if critMultiplier <= 0 {
		critMultiplier = critMult
	}

	in := swingInputs{
		attackerID:     attackerID,
		targetID:       targetID,
		attacker:       attacker,
		target:         target,
		atkStats:       atkStats,
		defStats:       defStats,
		atkName:        attacker.Name(),
		tgtName:        target.Name(),
		weaponName:     atkStats.EffectiveWeaponName(),
		hitMod:         atkStats.HitMod,
		damageExpr:     atkStats.EffectiveDamage(),
		critThreatLow:  critThreatLow,
		critMultiplier: critMultiplier,
		attackerRoom:   room,
	}
	// A one-shot throw never consults the ammo hook or the massive-damage rule:
	// the sink + roller are the only cfg fields resolveSwing needs here.
	return resolveSwing(ctx, in, AutoAttackConfig{Sink: m.sink, Roller: roller}) == swingContinue
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
