package corpse

import "github.com/Jasrags/AnotherMUD/internal/entities"

// Accessors for the reserved corpse properties (set at creation in
// CreateOnDeath). Corpses are transient and never persisted, so these
// read back the exact Go types they were written with — no YAML
// coercion to defend against.

// IsCorpse reports whether the entity carries the corpse tag.
func IsCorpse(e *entities.ItemInstance) bool {
	if e == nil {
		return false
	}
	for _, t := range e.Tags() {
		if t == TagCorpse {
			return true
		}
	}
	return false
}

// Coins returns the corpse's coin-pile amount (0 if absent or wrong type).
func Coins(e *entities.ItemInstance) int {
	if e == nil {
		return 0
	}
	if v, ok := e.Property(PropCoins); ok {
		if n, ok := v.(int); ok {
			return n
		}
	}
	return 0
}

// SetCoins writes the coin-pile amount (used after crediting a looter).
func SetCoins(e *entities.ItemInstance, n int) {
	if e != nil {
		e.SetProperty(PropCoins, n)
	}
}

// ClaimCoins atomically takes the corpse's coin pile, returning the
// amount and leaving the corpse with zero. The single-winner primitive
// for concurrent looters (loot-and-corpses §5.1): when two players loot
// the same open corpse at once, exactly one observes a non-zero amount,
// so coins are credited once — not duplicated.
func ClaimCoins(e *entities.ItemInstance) int {
	if e == nil {
		return 0
	}
	return e.ClaimIntProperty(PropCoins)
}

// Owners returns the looting-rights owner set (nil if absent).
func Owners(e *entities.ItemInstance) []string {
	if e == nil {
		return nil
	}
	if v, ok := e.Property(PropOwners); ok {
		if o, ok := v.([]string); ok {
			return o
		}
	}
	return nil
}

// CreatedTick returns the tick the corpse was created on (0 if absent).
func CreatedTick(e *entities.ItemInstance) uint64 {
	if e == nil {
		return 0
	}
	if v, ok := e.Property(PropCreatedTick); ok {
		if n, ok := v.(uint64); ok {
			return n
		}
	}
	return 0
}

// MayLoot reports whether actorID may take from the corpse, given the
// current tick and the ownership window (in ticks) — loot-and-corpses
// §4. The corpse is open (anyone may loot) when:
//   - its owner set is empty (no killer attribution → §4 "open
//     immediately"), or
//   - the window is zero (no reservation configured), or
//   - the window has elapsed since creation.
//
// Otherwise only an owner-set member may loot. The check never reveals
// who the owner is — the caller's refusal message must stay generic
// (§4 "does not name the owner").
func MayLoot(e *entities.ItemInstance, actorID string, nowTick, window uint64) bool {
	owners := Owners(e)
	if len(owners) == 0 {
		return true
	}
	if window == 0 {
		return true
	}
	created := CreatedTick(e)
	// Subtract-first (never created+window) so the elapsed check can't
	// overflow uint64; nowTick < created (clock skew / hand-crafted
	// corpse) means "not elapsed", falling through to the owner check.
	if nowTick >= created && nowTick-created >= window {
		return true
	}
	for _, o := range owners {
		if o == actorID {
			return true
		}
	}
	return false
}
