package command

import (
	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// Armor speed penalty (armor-depth §7 / movement-cost §4.4): heavier armor slows
// the wearer. equipment.md gives light/unarmored a Speed of 30 and medium/heavy a
// Speed of 20; in this engine's per-step movement-cost model that reduction
// becomes a SURCHARGE on every step, stacking with the terrain cost and the
// encumbrance surcharge. The slowest worn piece governs — the same
// most-restrictive rule the armor max-Dex cap uses. Inert until a character wears
// armor whose armor_speed is below the baseline (i.e. medium/heavy body armor);
// light armor and shields set 30/none and cost nothing extra.
const (
	// armorBaselineSpeed is the unarmored/light speed at which no penalty applies
	// (equipment.md Speed 30). Worn armor at or above it costs nothing extra.
	armorBaselineSpeed = 30
	// armorSpeedPenaltyUnit converts speed lost below the baseline into a movement
	// surcharge: each full unit lost adds 1 to a step's cost (30 -> 20 = +1).
	armorSpeedPenaltyUnit = 10
)

// armorSpeedSurcharge returns the extra movement points a step costs for the
// actor's worn armor (the equipment.md Speed column). 0 when nothing slower than
// the baseline is worn (unarmored, light armor, shields, or any content that sets
// no armor_speed). The slowest worn piece governs. Like encumbranceSurcharge it
// depends only on the mover, not the destination, so it cancels in the
// difficulty-hint comparison and leaves the hint purely terrain-driven.
func (c *Context) armorSpeedSurcharge() int {
	if c.Items == nil {
		return 0
	}
	slowest := 0 // 0 = no speed-setting armor worn
	for _, id := range c.Actor.Equipment() {
		e, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		it, ok := e.(*entities.ItemInstance)
		if !ok {
			continue
		}
		sp := it.ArmorSpeed()
		if sp <= 0 {
			continue // a weapon / shield / helm / light armor without a set speed
		}
		if slowest == 0 || sp < slowest {
			slowest = sp
		}
	}
	if slowest == 0 || slowest >= armorBaselineSpeed {
		return 0
	}
	return (armorBaselineSpeed - slowest) / armorSpeedPenaltyUnit
}
