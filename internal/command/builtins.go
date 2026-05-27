package command

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// RegisterBuiltins binds the M1 verbs into r: look, quit, and one
// keyword per movement direction (long + short form). Movement uses
// world.World.Move; look renders the actor's current room.
func RegisterBuiltins(r *Registry) error {
	bindings := []struct {
		key string
		h   Handler
	}{
		{"look", LookHandler},
		{"quit", QuitHandler},
		{"color", ColorHandler},
		{"get", GetHandler},
		{"drop", DropHandler},
		{"give", GiveHandler},
		{"put", PutHandler},
		{"fill", FillHandler},
		{"equip", EquipHandler},
		{"unequip", UnequipHandler},
		// Display verbs (M5.7). Aliases are registered explicitly
		// rather than relying on prefix match: `eq` would otherwise
		// resolve to `equip` (registered earlier, lower order in the
		// prefix-tiebreaker) instead of `equipment`. `i` is
		// unambiguous today but reserving it explicitly avoids
		// surprise the first time an `inspect` / `ignore` / `info`
		// verb shows up.
		{"inventory", InventoryHandler},
		{"i", InventoryHandler},
		{"equipment", EquipmentHandler},
		{"eq", EquipmentHandler},
		// Combat status (M7.1). `con` would also match `color` by
		// prefix (alphabetical), but `color` is registered earlier so
		// its lower order wins the tiebreaker — `con` resolves to
		// consider here. Spelled out for clarity.
		{"consider", ConsiderHandler},
		{"con", ConsiderHandler},
		// Combat engage (M7.2). `k` is too aggressive a prefix to
		// commit (would collide with future verbs like `kick` /
		// `keep`); deliberately not aliased.
		{"kill", KillHandler},
		// Flee / wimpy (M7.6). `flee` has no short alias — running
		// from combat shouldn't be a one-key reflex. `wimpy` is
		// likewise spelled out.
		{"flee", FleeHandler},
		{"wimpy", WimpyHandler},
		// Admin XP probe (M8.2). End-to-end test verb for the
		// progression layer — self-grants XP. Role-gated grants
		// + target-by-name form land with the role system (M10+).
		{"xp", XPHandler},
		// Training verbs (M8.6 — progression.md §7). `train` bumps
		// a base stat by spending a train credit; `practice` raises
		// the cap on an ability via an in-room trainer. `pra` and
		// `tra` aliases are NOT registered today — short prefixes
		// would collide with `put` / `tra…` futures. Spell out the
		// verbs.
		{"train", TrainHandler},
		{"practice", PracticeHandler},
	}
	for _, d := range []world.Direction{
		world.DirNorth, world.DirSouth, world.DirEast, world.DirWest,
		world.DirUp, world.DirDown,
	} {
		dir := d
		mh := movementHandler(dir)
		bindings = append(bindings,
			struct {
				key string
				h   Handler
			}{dir.Long(), mh},
			struct {
				key string
				h   Handler
			}{dir.Short(), mh},
		)
	}
	for _, b := range bindings {
		if err := r.Register(b.key, b.h); err != nil {
			return err
		}
	}
	return nil
}

// LookHandler renders the actor's current room.
func LookHandler(ctx context.Context, c *Context) error {
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You float in formless void.")
	}
	return c.Actor.Write(ctx, RenderRoom(room, c.Placement, c.Items))
}

// ColorHandler implements the `color` verb (spec ui-rendering-help —
// color subset). With no argument it reports the current state; with
// "on"/"off" it toggles the per-actor flag.
func ColorHandler(ctx context.Context, c *Context) error {
	if len(c.Args) == 0 {
		state := "off"
		if c.Actor.ColorEnabled() {
			state = "on"
		}
		return c.Actor.Write(ctx, "Color is currently "+state+". Use 'color on' or 'color off'.")
	}
	switch strings.ToLower(c.Args[0]) {
	case "on":
		c.Actor.SetColorEnabled(true)
		// Confirm in color so the user sees it took effect; the auto-reset
		// in ansi.Render closes the sequence cleanly.
		return c.Actor.Write(ctx, "{G}Color enabled.{x}")
	case "off":
		c.Actor.SetColorEnabled(false)
		return c.Actor.Write(ctx, "Color disabled.")
	default:
		return c.Actor.Write(ctx, "Usage: color [on|off]")
	}
}

// QuitHandler signals the session loop to disconnect cleanly.
//
// The farewell Write error is intentionally discarded: ErrQuit drives
// the session loop to close the connection regardless of whether the
// peer received the goodbye, and surfacing a write failure here would
// only escalate a benign condition (peer already gone) into a warning
// in the connection's tear-down path.
func QuitHandler(ctx context.Context, c *Context) error {
	_ = c.Actor.Write(ctx, "Goodbye.")
	return ErrQuit
}

func movementHandler(dir world.Direction) Handler {
	return func(ctx context.Context, c *Context) error {
		room := c.Actor.Room()
		if room == nil {
			return c.Actor.Write(ctx, "You cannot move from nowhere.")
		}
		dst, err := c.World.Move(room.ID, dir)
		if err != nil {
			if errors.Is(err, world.ErrNoExit) {
				return c.Actor.Write(ctx, "You cannot go that way.")
			}
			return c.Actor.Write(ctx, "Something blocks your way.")
		}
		srcID := room.ID
		name := c.Actor.Name()
		pid := c.Actor.PlayerID()
		// Announce departure to the source room before the actor
		// leaves so other occupants there see it. Broadcaster is
		// optional (tests pass nil); skip the announcement when name
		// or PlayerID is empty (test actors that don't participate in
		// presence).
		if c.Broadcaster != nil && name != "" {
			c.Broadcaster.SendToRoom(ctx, srcID,
				fmt.Sprintf("%s heads %s.", name, dir.Long()), pid)
		}
		c.Actor.SetRoom(dst)
		if c.Broadcaster != nil && name != "" {
			from := dir.Opposite().Long()
			if from == "" {
				from = "elsewhere"
			}
			c.Broadcaster.SendToRoom(ctx, dst.ID,
				fmt.Sprintf("%s arrives from the %s.", name, from), pid)
		}
		// Publish player.moved so the disposition evaluator can clear
		// per-room reaction state for srcID (spec mobs-ai-spawning
		// §5.2). Published unconditionally — even tests-without-bus
		// flow through Publish's nil guard.
		c.Publish(ctx, eventbus.PlayerMoved{
			PlayerID: pid,
			From:     srcID,
			To:       dst.ID,
		})
		// Immediate (aggro-only) hook BEFORE the description so
		// hostile reactions can dispatch to combat before the player
		// sees the room. Players have no tags today; nil is safe.
		if c.Disposition != nil && pid != "" {
			c.Disposition.OnPlayerEnteredImmediate(ctx, pid, name, nil, dst.ID)
		}
		if err := c.Actor.Write(ctx, RenderRoom(dst, c.Placement, c.Items)); err != nil {
			return err
		}
		// Deferred (full) hook AFTER the description so non-hostile
		// reactions arrive below the room text.
		if c.Disposition != nil && pid != "" {
			c.Disposition.OnPlayerEnteredDeferred(ctx, pid, name, nil, dst.ID)
		}
		return nil
	}
}

// RenderRoom is the M1 room renderer, extended in M6.3 to include
// Placement-tracked entities (items + mobs). Replaced by the
// ui-rendering-help pipeline in a later milestone; lives here for now
// so the session layer has something to call.
//
// placement and items may be nil — older callers and tests that only
// care about geography pass nil for both. When supplied, the renderer
// appends a "You see here:" line listing each placed entity by name
// in insertion order. Entities nested inside containers are not
// shown: those live in Contents, not Placement (the put pipeline
// removes from Placement when nesting).
func RenderRoom(r *world.Room, placement *entities.Placement, items *entities.Store) string {
	var b strings.Builder
	b.WriteString(r.Name)
	b.WriteString("\n")
	b.WriteString(r.Description)
	b.WriteString("\n")
	if line := renderRoomEntities(r, placement, items); line != "" {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(renderExits(r))
	return b.String()
}

// renderRoomEntities builds the "You see here: …" line. Returns the
// empty string when there's nothing to show — Placement is nil, the
// Store is nil, the room has no placed entities, or every placed id
// fails resolution. Each branch is a silent skip rather than a panic
// because the renderer is on the player-visible path; missing data
// should look like nothing-here, not a runtime error.
func renderRoomEntities(r *world.Room, placement *entities.Placement, items *entities.Store) string {
	if placement == nil || items == nil {
		return ""
	}
	ids := placement.InRoom(r.ID)
	if len(ids) == 0 {
		return ""
	}
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		e, ok := items.GetByID(id)
		if !ok {
			continue
		}
		n, ok := e.(interface{ Name() string })
		if !ok {
			continue
		}
		if name := n.Name(); name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	return "You see here: " + strings.Join(names, ", ") + "."
}

func renderExits(r *world.Room) string {
	if len(r.Exits) == 0 {
		return "Exits: none"
	}
	dirs := make([]string, 0, len(r.Exits))
	for d := range r.Exits {
		dirs = append(dirs, d.Long())
	}
	sort.Strings(dirs)
	return fmt.Sprintf("Exits: %s", strings.Join(dirs, ", "))
}
