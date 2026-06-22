package command

import (
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/keyword"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// findCombatantInRoom resolves target against the combatants currently
// in roomID. Two channels feed the result:
//
//  1. Mobs via Placement + the shared keyword resolver. *MobInstance
//     already satisfies keyword.Named (Name + Keywords), so the
//     resolver runs directly against the live mob list — no adapter.
//
//  2. Players via the session Locator (name-based, not keyword-based).
//     Asymmetric with mobs by design: the Locator is the existing
//     surface other targeted verbs (give, future tell/follow) use and
//     we keep that contract intact. The implication is that "kill al"
//     won't partial-match a player named Alice — the player's full
//     name is required.
//
// Mobs win ties. If a mob with keyword "guard" and a player named
// "Guard" share the room, "kill guard" hits the mob. This matches
// every other verb that scans mobs first.
//
// Returns (nil, "", false) when nothing matched, when the room has
// no candidates, or when Placement / Items is unwired (test paths).
// The display-name return is the resolved combatant's Name() —
// callers use it for "You attack a village guard." style messages
// without having to round-trip through the Combatant pointer.
//
// Promoted out of consider.go in M7.2 because kill needs the same
// resolution shape and a second copy would diverge. Lives in
// internal/command as an unexported helper because both call sites
// are command handlers; no other package needs it.
func findCombatantInRoom(c *Context, roomID world.RoomID, target string) (combat.Combatant, string, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, "", false
	}

	// M17.2d₄b: resolve through the §5 `entity` arg (mobs + players in
	// the room, self excluded — §5.2). This unifies combat targeting
	// with the rest of the arg pipeline: players are now keyword/
	// partial-matchable and ordinals (`kill 2.rat`) work for both kinds,
	// where the old mob-keyword / player-exact-name split did not.
	// Callers self-check before reaching here (consider/kill) or target
	// self via the empty-target path (cast), so self-exclusion is fine.
	reg := c.ArgResolver
	if reg == nil {
		// Direct-call test path: Dispatch normally injects the shared
		// registry. A fresh one carries the same engine resolvers.
		reg = NewArgResolverRegistry()
	}
	out, _, _, err := reg.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "target", Type: ArgEntity}},
		strings.Fields(target),
		c.BuildResolveContext(),
	)
	if err != nil {
		// Resolver miss (not found / ambiguous-by-rule) — the caller
		// owns the player-facing "you don't see them here" copy.
		return nil, "", false
	}
	ref, ok := out["target"].(EntityRef)
	if !ok {
		return nil, "", false
	}
	return resolveCombatantRef(c, roomID, ref)
}

// resolveCombatantRef re-fetches the live combat.Combatant named by a
// resolved EntityRef. Mobs come from the entity store; players from the
// Locator (by the resolved exact name). Returns false when the id no
// longer resolves to a combatant — the standard resolve→re-fetch TOCTOU
// shape used across the migrated verbs.
func resolveCombatantRef(c *Context, roomID world.RoomID, ref EntityRef) (combat.Combatant, string, bool) {
	switch ref.Type {
	case entityTypeMob:
		if c.Items != nil {
			if e, ok := c.Items.GetByID(entities.EntityID(ref.ID)); ok {
				if mob, ok := e.(*entities.MobInstance); ok {
					return mob, mob.Name(), true
				}
			}
		}
	case entityTypePlayer:
		if c.Locator != nil {
			if other := c.Locator.FindInRoom(roomID, ref.Name); other != nil {
				if cb, ok := other.(combat.Combatant); ok {
					return cb, other.Name(), true
				}
			}
		}
	}
	return nil, "", false
}

// resolveCombatantByID resolves a combatant from its CombatantID (not a
// keyword) — used by `assist` to name and engage the foe a party-mate is
// fighting. A player id routes through ActorByID; a mob id through the entity
// store. Returns false when it no longer resolves (the foe left / fell).
func resolveCombatantByID(c *Context, id combat.CombatantID) (combat.Combatant, string, bool) {
	eid := combat.EntityIDOf(id)
	if strings.HasPrefix(string(id), combat.PlayerPrefix) {
		if c.ActorByID != nil {
			if a, ok := c.ActorByID(eid); ok {
				if cb, ok := a.(combat.Combatant); ok {
					return cb, a.Name(), true
				}
			}
		}
		return nil, "", false
	}
	if c.Items != nil {
		if e, ok := c.Items.GetByID(entities.EntityID(eid)); ok {
			if mob, ok := e.(*entities.MobInstance); ok {
				return mob, mob.Name(), true
			}
		}
	}
	return nil, "", false
}

// findMobByKeyword scans Placement-tracked entities in roomID, filters
// to *MobInstance (item entities and any other future Entity type
// drop out), and runs the shared keyword resolver. Returns nil if any
// of Placement / Items is unwired (tests) or no mob matches.
//
// Kept as its own helper rather than inlined into findCombatantInRoom
// so the unit tests covering mob-only resolution can target it
// directly without setting up a Locator stub.
func findMobByKeyword(c *Context, roomID world.RoomID, target string) *entities.MobInstance {
	if c.Placement == nil || c.Items == nil {
		return nil
	}
	ids := c.Placement.InRoom(roomID)
	if len(ids) == 0 {
		return nil
	}
	candidates := make([]keyword.Named, 0, len(ids))
	for _, id := range ids {
		e, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		mob, ok := e.(*entities.MobInstance)
		if !ok {
			continue
		}
		candidates = append(candidates, mob)
	}
	if len(candidates) == 0 {
		return nil
	}
	hit := keyword.Resolve(candidates, target)
	if hit == nil {
		return nil
	}
	mob, _ := hit.(*entities.MobInstance)
	return mob
}
