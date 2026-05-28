package progression

// Passive abilities (spec abilities-and-effects §6). Passives are
// never queued; other subsystems evaluate them at well-defined hooks
// (combat auto-attack swing count §4.2, defensive evade §4.3 step 2).
// This file provides the three §6 building blocks — binary check,
// scaling bonus, hook discovery — plus a PassiveResolver that wires
// them to the two combat hooks the engine uses today.

// Canonical hook keys (spec §6.3). The hook set is content-defined;
// these are the two the engine consumes today. Content tags a passive
// with one of these (Ability.Hook) to attach it to the corresponding
// combat opportunity.
const (
	// HookExtraAttack — passives that may grant an extra swing
	// (combat §4.2 swing count).
	HookExtraAttack = "extra_attack"
	// HookDefensive — passives that may pre-empt an incoming swing
	// with an evade (combat §4.3 step 2).
	HookDefensive = "defensive"
)

// PassiveBinaryCheck implements the spec §6.1 binary check: "does the
// passive fire on this opportunity?". Effective chance is
// proficiency × variance/100 when variance < 100, otherwise
// proficiency × maxChance/100. The roll is uniform 1..100; success
// when roll ≤ chance.
//
// Integer arithmetic mirrors the active resolver's rollHit (see the
// m9-4 float-precision deferral); both narrow identically. prof ≤ 0
// (unlearned) can never fire. A variance ≥ 100 passive with maxChance
// 0 also never fires — content must set a max-chance ceiling for the
// high-variance branch.
func PassiveBinaryCheck(prof, variance, maxChance int, roller Roller) bool {
	if prof <= 0 {
		return false
	}
	var chance int
	if variance < 100 {
		chance = prof * variance / 100
	} else {
		chance = prof * maxChance / 100
	}
	if chance <= 0 {
		return false
	}
	if chance > 100 {
		chance = 100
	}
	return roller.IntN(100)+1 <= chance
}

// PassiveScalingBonus implements the spec §6.2 scaling bonus:
// maxBonus × proficiency / 100, clamped to ≥ 0. Used by passives that
// contribute an additive numeric bonus (extra hit, extra damage, crit
// chance) proportional to skill. Integer truncation toward zero.
func PassiveScalingBonus(maxBonus, prof int) int {
	if maxBonus <= 0 || prof <= 0 {
		return 0
	}
	bonus := maxBonus * prof / 100
	if bonus < 0 {
		bonus = 0
	}
	return bonus
}

// PassiveResolver wires the §6 building blocks to the combat hooks.
// It is the progression-side implementation a host adapter exposes to
// combat (which must not import progression). Holds no per-entity
// state — proficiency lives in the manager it is handed.
//
// CONCURRENCY: the combat auto-attack phase invokes the resolver on
// the tick goroutine (the same single-goroutine guarantee the Roller
// contract relies on). The ProficiencyManager carries its own lock.
type PassiveResolver struct {
	registry   *AbilityRegistry
	proficient ProficiencyReader
	gainer     ProficiencyMutator
	roller     Roller
}

// NewPassiveResolver builds a resolver. registry, proficient, and
// roller are required (a passive evaluation is meaningless without
// them) and panic at construction if nil, mirroring NewAutoAttack's
// fail-fast. gainer is nil-safe: a resolver without one evaluates
// passives but rolls no §6.3 proficiency gain.
func NewPassiveResolver(registry *AbilityRegistry, proficient ProficiencyReader, gainer ProficiencyMutator, roller Roller) *PassiveResolver {
	if registry == nil {
		panic("progression.NewPassiveResolver: nil registry")
	}
	if proficient == nil {
		panic("progression.NewPassiveResolver: nil proficient")
	}
	if roller == nil {
		panic("progression.NewPassiveResolver: nil roller")
	}
	return &PassiveResolver{
		registry:   registry,
		proficient: proficient,
		gainer:     gainer,
		roller:     roller,
	}
}

// ExtraAttacks returns the number of extra swings entityID earns this
// round from its HookExtraAttack passives (combat §4.2). Each such
// passive runs an independent §6.1 binary check against the entity's
// proficiency; every success adds one swing and rolls a §6.3
// proficiency gain. Returns 0 when the entity knows no extra-attack
// passive or none fires.
func (p *PassiveResolver) ExtraAttacks(entityID string) int {
	extra := 0
	for _, ab := range p.registry.ByHook(HookExtraAttack) {
		if p.fires(entityID, ab) {
			extra++
			p.rollGain(entityID, ab)
		}
	}
	return extra
}

// DefensiveEvade reports whether one of defenderID's HookDefensive
// passives pre-empts an incoming swing (combat §4.3 step 2). Passives
// are tried in id-sorted order; the FIRST that wins its §6.1 binary
// check evades, rolls a §6.3 gain, and returns its display name. No
// further passives are tried once one evades (a swing is evaded once).
func (p *PassiveResolver) DefensiveEvade(defenderID string) (string, bool) {
	for _, ab := range p.registry.ByHook(HookDefensive) {
		if p.fires(defenderID, ab) {
			p.rollGain(defenderID, ab)
			return ab.DisplayName, true
		}
	}
	return "", false
}

// fires runs the §6.1 binary check for entityID against ability ab,
// reading the entity's current proficiency. An unlearned passive
// (prof 0) never fires.
func (p *PassiveResolver) fires(entityID string, ab *Ability) bool {
	prof := proficiencyValueOf(p.proficient, entityID, ab.ID)
	return PassiveBinaryCheck(prof, ab.Variance, ab.MaxHitChance, p.roller)
}

// rollGain rolls a §6.3 proficiency gain for a passive that just
// fired (the entity didn't choose to use it, but using-it-in-context
// still trains them). Uses the shared §3.5 gainThreshold with the
// stat factor omitted (statFactor 1.0) — reading an arbitrary
// entity's gain-stat by id needs a host seam that doesn't exist yet
// (M9.5 deferral). hit is always true: a passive that fired succeeded.
func (p *PassiveResolver) rollGain(entityID string, ab *Ability) {
	if p.gainer == nil {
		return
	}
	prof := proficiencyValueOf(p.proficient, entityID, ab.ID)
	threshold := gainThreshold(
		ab.GainBaseChance, prof, effectiveCapValueOf(p.proficient, entityID, ab.ID),
		1.0, ab.GainFailureMultiplier, true,
	)
	if threshold == 0 {
		return
	}
	if p.roller.IntN(100)+1 <= threshold {
		p.gainer.AddProficiency(entityID, ab.ID, 1)
	}
}
