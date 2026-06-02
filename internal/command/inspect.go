package command

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// inspect read-surface interfaces. Each is the minimal slice of a target
// the inspect dump reads; the handler type-asserts the live entity for
// each and renders only the sections it satisfies, so a mob (no roles, no
// progression) and a player (no template properties) each produce the
// subset that applies. Defined at the use site per the small-interface
// convention; connActor / MobInstance already satisfy the relevant ones.
type (
	roleReader  interface{ Roles() []string }
	tagReader   interface{ Tags() []string }
	propReader  interface{ Properties() map[string]any }
	equipReader interface {
		Equipment() map[string]entities.EntityID
	}
)

// InspectHandler implements `inspect [<target>]` (admin-verbs §5): the
// read-only diagnostic dump of a target's identity, vitals, stats, and —
// where the kind carries them — roles, levels, equipment, tags, and
// properties. Admin-marked (M19.3 gate authorizes); audited via the shared
// auditAdmin choke point (§6).
//
// Resolution (§3): no argument or a self-reference inspects the actor;
// otherwise the target resolves in the actor's room (player or mob) through
// the shared §5 entity path. §3's visibility bypass is a no-op today — the
// hide/sneak rules it bypasses are still greenfield (BACKLOG), so admin
// resolution is the same room scope an ordinary look uses. When those rules
// land, the bypass attaches here.
func InspectHandler(ctx context.Context, c *Context) error {
	target := strings.TrimSpace(strings.Join(c.Args, " "))

	// Self: no target, or an explicit self-reference.
	if target == "" || isSelfReference(c.Actor.Name(), target) {
		auditAdmin(ctx, c, "inspect", c.Actor.PlayerID(), "self")
		return renderInspect(ctx, c, c.Actor, "yourself")
	}

	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You see nothing here.")
	}
	cb, name, ok := findCombatantInRoom(c, room.ID, target)
	if !ok {
		return c.Actor.Write(ctx, "You don't see them here.")
	}

	auditAdmin(ctx, c, "inspect", inspectID(cb), target)
	return renderInspect(ctx, c, cb, name)
}

// renderInspect writes the multi-line diagnostic dump for target under
// displayName. It accumulates the lines that apply (by capability) and
// writes them in order, stopping on the first write error. target is any
// live entity — the actor (self) or a resolved room combatant.
func renderInspect(ctx context.Context, c *Context, target any, displayName string) error {
	id, kind := inspectIdentity(target)

	lines := []string{fmt.Sprintf("--- %s (%s) [%s] ---", displayName, kind, id)}

	if cb, ok := target.(combat.Combatant); ok {
		cur, max := cb.Vitals().Snapshot()
		st := cb.Stats()
		lines = append(lines,
			fmt.Sprintf("Vitals:  %d/%d HP", cur, max),
			fmt.Sprintf("Stats:   HitMod %+d  AC %d  STR %d", st.HitMod, st.AC, st.STR))
	}

	if rr, ok := target.(roleReader); ok {
		if roles := rr.Roles(); len(roles) > 0 {
			lines = append(lines, "Roles:   "+strings.Join(roles, ", "))
		}
	}

	if ph, ok := target.(ProgressionHolder); ok && c.Progression != nil {
		lines = append(lines, inspectLevels(c, ph)...)
	}

	if er, ok := target.(equipReader); ok {
		lines = append(lines, inspectEquipment(c, er)...)
	}

	if tr, ok := target.(tagReader); ok {
		if tags := tr.Tags(); len(tags) > 0 {
			lines = append(lines, "Tags:    "+strings.Join(tags, ", "))
		}
	}

	if pr, ok := target.(propReader); ok {
		lines = append(lines, inspectProperties(pr)...)
	}

	for _, line := range lines {
		if err := c.Actor.Write(ctx, line); err != nil {
			return err
		}
	}
	return nil
}

// inspectLevels renders one line per progression track the target has
// touched (or that lazily initializes), mirroring the xp verb's read path.
func inspectLevels(c *Context, ph ProgressionHolder) []string {
	var out []string
	for _, td := range c.Progression.Tracks().All() {
		info, ok := ph.TrackInfo(c.Progression, td.Name)
		if !ok {
			continue
		}
		label := td.DisplayName
		if label == "" {
			label = td.Name
		}
		out = append(out, fmt.Sprintf("Level:   %-16s level %d (xp %d)", label, info.Level, info.XP))
	}
	return out
}

// inspectEquipment renders the target's equipped items, slot → item name
// (best-effort via the item store; falls back to the entity id when the
// store is unwired or the lookup misses). Slots are sorted for stable,
// readable output.
func inspectEquipment(c *Context, er equipReader) []string {
	eq := er.Equipment()
	if len(eq) == 0 {
		return nil
	}
	slots := make([]string, 0, len(eq))
	for slot := range eq {
		slots = append(slots, slot)
	}
	sort.Strings(slots)

	out := make([]string, 0, len(slots))
	for _, slot := range slots {
		id := eq[slot]
		label := string(id)
		if c.Items != nil {
			if e, ok := c.Items.GetByID(id); ok {
				if named, ok := e.(interface{ Name() string }); ok {
					label = named.Name()
				}
			}
		}
		out = append(out, fmt.Sprintf("Equip:   %-12s %s", slot, label))
	}
	return out
}

// inspectProperties renders the target's property bag, key=value, sorted by
// key for deterministic output.
func inspectProperties(pr propReader) []string {
	props := pr.Properties()
	if len(props) == 0 {
		return nil
	}
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("Prop:    %s = %v", k, props[k]))
	}
	return out
}

// inspectIdentity reports the target's (id, kind) for the dump header.
// A *MobInstance is an "npc"; everything else is a "player" (the actor or
// a resolved player combatant).
func inspectIdentity(target any) (id, kind string) {
	if _, ok := target.(*entities.MobInstance); ok {
		return inspectID(target), "npc"
	}
	return inspectID(target), "player"
}

// inspectID extracts a stable id from a live entity: the mob's entity id,
// or the player's player id. Empty when neither applies (defensive — a
// test stub without identity).
func inspectID(target any) string {
	if mob, ok := target.(*entities.MobInstance); ok {
		return mob.EntityID()
	}
	if p, ok := target.(interface{ PlayerID() string }); ok {
		return p.PlayerID()
	}
	return ""
}
