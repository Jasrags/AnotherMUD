package command

import (
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// carryWeightLimited is the optional actor capability for the personal
// carry-weight ceiling (inventory-equipment-items §4.2 step 2). The
// ceiling is read from the actor's StatCarryMax stat; connActor satisfies
// it via its stat block (the same StatValue surface the score sheet
// reads). An actor that doesn't implement it (weightless mobs, minimal
// test stubs) has no limit.
type carryWeightLimited interface {
	StatValue(progression.StatType) int
}

// carryWeightExceeded reports whether picking up incoming would push the
// actor's carried weight past its StatCarryMax ceiling (§4.2 step 2). It
// returns false — "no limit" — when the actor exposes no stat surface or
// its ceiling is non-positive, so weightless content and test stubs are
// unaffected. Item weight comes from the per-instance `weight` property
// (the same key put.go reads for container limits); items with no weight
// contribute zero. The incoming item's own weight is included so the
// check reflects the load *after* the pickup.
func (c *Context) carryWeightExceeded(incoming *entities.ItemInstance) bool {
	lim, ok := c.Actor.(carryWeightLimited)
	if !ok {
		return false
	}
	max := lim.StatValue(progression.StatCarryMax)
	if max <= 0 {
		return false
	}
	total := intProp(incoming, propWeight)
	if c.Items != nil {
		for _, id := range c.Actor.Inventory() {
			e, ok := c.Items.GetByID(id)
			if !ok {
				continue
			}
			if it, ok := e.(*entities.ItemInstance); ok {
				total += intProp(it, propWeight)
			}
		}
	}
	return total > max
}
