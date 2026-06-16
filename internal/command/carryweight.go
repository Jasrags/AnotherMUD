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
// unaffected. The incoming item's own weight is included so the check
// reflects the load *after* the pickup.
func (c *Context) carryWeightExceeded(incoming *entities.ItemInstance) bool {
	max := c.carryCapacity()
	if max <= 0 {
		return false
	}
	return c.carriedWeight()+intProp(incoming, propWeight) > max
}

// carryPerStrength derives carry capacity from Strength when content sets
// no explicit carry_max: capacity = STR × this. The default (STR 10 →
// capacity 80) keeps a light traveler unburdened and a loot-laden one
// burdened — a balance figure (movement-cost §4.4), not a fixed rule.
const carryPerStrength = 8

// carryCapacity returns the actor's carry-weight ceiling — the shared
// notion of capacity for the pickup gate and the movement-encumbrance
// surcharge. Resolution: a NEGATIVE StatCarryMax is the explicit
// "unlimited" sentinel (a pack-mule / admin opt-out) and reports 0 ("no
// limit"); a POSITIVE StatCarryMax is a content-set cap; otherwise (the
// absent/zero default) capacity is derived from Strength (carryPerStrength
// × STR). It is also 0 for an actor with no stat surface or non-positive
// Strength (weightless content / minimal test stubs). Callers treat a
// non-positive return as "no limit".
func (c *Context) carryCapacity() int {
	lim, ok := c.Actor.(carryWeightLimited)
	if !ok {
		return 0
	}
	switch max := lim.StatValue(progression.StatCarryMax); {
	case max < 0:
		return 0 // explicit unlimited sentinel — overrides STR derivation
	case max > 0:
		return max // explicit content cap
	}
	if str := lim.StatValue(progression.StatSTR); str > 0 {
		return str * carryPerStrength
	}
	return 0
}

// carriedWeight sums the `weight` property over the actor's inventory (the
// same key put.go reads for container limits); items with no weight
// contribute zero. Equipment is excluded, matching the carry-weight gate's
// inventory-only notion of load.
func (c *Context) carriedWeight() int {
	if c.Items == nil {
		return 0
	}
	total := 0
	for _, id := range c.Actor.Inventory() {
		e, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		if it, ok := e.(*entities.ItemInstance); ok {
			total += intProp(it, propWeight)
		}
	}
	return total
}

// Encumbrance tiers (movement-cost §4.4) as a percentage of carry
// capacity. Below the burdened threshold the load is free; each tier
// above adds a flat surcharge to every step's movement cost. Dormant
// until content gives a character a positive carry_max.
const (
	encumbranceBurdenedPct       = 50 // ≥ this fraction of capacity → burdened
	encumbranceHeavyPct          = 90 // ≥ this fraction of capacity → heavily burdened
	encumbranceBurdenedSurcharge = 1
	encumbranceHeavySurcharge    = 2
)

// encumbranceSurcharge returns the extra movement points a step costs for
// the actor's current load (movement-cost §4.4): 0 when unburdened or when
// the actor has no carry capacity (weightless content / test stubs),
// rising by tier as the load nears capacity.
func (c *Context) encumbranceSurcharge() int {
	capacity := c.carryCapacity()
	if capacity <= 0 {
		return 0
	}
	loadPct := c.carriedWeight() * 100 / capacity
	switch {
	case loadPct >= encumbranceHeavyPct:
		return encumbranceHeavySurcharge
	case loadPct >= encumbranceBurdenedPct:
		return encumbranceBurdenedSurcharge
	default:
		return 0
	}
}
