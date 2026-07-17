package karma

// Costs is the karma-ledger spend price table for the `improve` verb (SR-M5b):
// how much karma raising a skill or an attribute costs, expressed as the
// per-rating MULTIPLIER (SR5's "new-rating × multiplier" karma economy). A world
// declares these in its manifest (`karma_costs:`); an absent block uses the SR
// canon defaults. Quality (feat) purchases are NOT here — a feat carries its own
// flat `karma_cost` in content, since qualities are priced individually, not by
// a rating curve.
type Costs struct {
	// SkillMult prices a skill-cap tier raise: cost = new-tier-rank × SkillMult
	// (rank 1..4 for Novice/Apprentice/Journeyman/Master). SR5 canon ≈ 2.
	SkillMult int64
	// AttributeMult prices an attribute +1: cost = new-value × AttributeMult.
	// SR5 canon ≈ 5 (raising an attribute to rating N costs N × 5).
	AttributeMult int64
}

// DefaultCosts is the SR5 canon price table, used when a karma-ledger world
// declares no `karma_costs:` block.
func DefaultCosts() Costs {
	return Costs{SkillMult: 2, AttributeMult: 5}
}

// WithDefaults returns a copy of c with any non-positive multiplier replaced by
// its canon default — so a manifest that sets only one knob (or omits one) still
// yields a fully-usable table rather than a zero-cost exploit.
func (c Costs) WithDefaults() Costs {
	d := DefaultCosts()
	if c.SkillMult <= 0 {
		c.SkillMult = d.SkillMult
	}
	if c.AttributeMult <= 0 {
		c.AttributeMult = d.AttributeMult
	}
	return c
}
