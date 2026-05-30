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

// RegisterBuiltins binds the engine verbs into r. Each verb carries the
// listing metadata (category, brief, syntax, aliases) that help generation
// turns into a discoverable topic (spec commands-and-dispatch §8): typing
// `help commands` lists them and `help <verb>` shows usage. Movement
// directions and the admin `xp` probe register bare (no metadata) so they
// stay out of the player-facing command list — movement has its own
// authored topic, and `xp` is gated until the role system lands.
//
// Aliases route to the same handler via an exact match, so prefix-collision
// concerns (e.g. `eq`→`equip`, `con`→`color`) are moot: an exact alias
// short-circuits the prefix scan.
func RegisterBuiltins(r *Registry) error {
	commands := []Command{
		{Keyword: "look", Handler: LookHandler, Brief: "Examine your surroundings or a target.", Syntax: []string{"look", "look <target>"}},
		{Keyword: "quit", Handler: QuitHandler, Brief: "Leave the game; your progress is saved.", Syntax: []string{"quit"}},
		{Keyword: "color", Handler: ColorHandler, Brief: "Toggle ANSI color, or show the current setting.", Syntax: []string{"color", "color on", "color off"}},

		// Items (M5.5-M5.9).
		{Keyword: "get", Handler: GetHandler, Brief: "Pick up an item from the room or a container.", Syntax: []string{"get <item>", "get <item> from <container>"}},
		{Keyword: "drop", Handler: DropHandler, Brief: "Drop an item from your inventory.", Syntax: []string{"drop <item>"}},
		{Keyword: "give", Handler: GiveHandler, Brief: "Give an item to another character.", Syntax: []string{"give <item> <target>"}},
		{Keyword: "put", Handler: PutHandler, Brief: "Put an item into a container.", Syntax: []string{"put <item> in <container>"}},
		{Keyword: "fill", Handler: FillHandler, Brief: "Fill a container from a source.", Syntax: []string{"fill <container>"}},
		{Keyword: "equip", Handler: EquipHandler, Brief: "Wear or wield an item from your inventory.", Syntax: []string{"equip <item>"}},
		{Keyword: "unequip", Handler: UnequipHandler, Brief: "Remove an equipped item.", Syntax: []string{"unequip <item>"}},
		{Keyword: "inventory", Aliases: []string{"i"}, Handler: InventoryHandler, Brief: "List the items you are carrying.", Syntax: []string{"inventory"}},
		{Keyword: "equipment", Aliases: []string{"eq"}, Handler: EquipmentHandler, Brief: "Show what you have equipped.", Syntax: []string{"equipment"}},

		// Combat (M7).
		{Keyword: "consider", Aliases: []string{"con"}, Handler: ConsiderHandler, Brief: "Size up a target before fighting.", Syntax: []string{"consider <target>"}},
		{Keyword: "kill", Handler: KillHandler, Brief: "Attack a target.", Syntax: []string{"kill <target>"}},
		{Keyword: "flee", Handler: FleeHandler, Brief: "Try to escape from combat.", Syntax: []string{"flee"}},
		{Keyword: "wimpy", Handler: WimpyHandler, Brief: "Auto-flee when your health drops below a percent.", Syntax: []string{"wimpy <percent>"}},

		// Progression (M8.6).
		{Keyword: "train", Handler: TrainHandler, Brief: "Spend a train credit to raise a stat.", Syntax: []string{"train <stat>"}},
		{Keyword: "practice", Handler: PracticeHandler, Brief: "Raise an ability's cap at a trainer.", Syntax: []string{"practice <ability>"}},

		// Abilities (M9.6).
		{Keyword: "abilities", Aliases: []string{"abi"}, Handler: AbilitiesHandler, Brief: "List the abilities you have learned.", Syntax: []string{"abilities"}},
		{Keyword: "cast", Handler: CastHandler, Brief: "Use an ability by name.", Syntax: []string{"cast <ability>", "cast <ability> <target>"}},

		// Help (M10.5).
		{Keyword: "help", Handler: HelpHandler, Brief: "Find help on commands and topics.", Syntax: []string{"help", "help <topic>"}, Category: "general"},

		// Quests (M10.10).
		{Keyword: "accept", Handler: AcceptHandler, Brief: "Accept an offered quest.", Syntax: []string{"accept <quest>"}},
		{Keyword: "abandon", Handler: AbandonHandler, Brief: "Abandon an active quest.", Syntax: []string{"abandon <quest>"}},
		{Keyword: "quests", Aliases: []string{"journal"}, Handler: QuestsHandler, Brief: "Show your active quests.", Syntax: []string{"quests"}},

		// Economy (M11).
		{Keyword: "gold", Handler: GoldHandler, Brief: "Show how much gold you carry.", Syntax: []string{"gold"}},
		{Keyword: "buy", Handler: BuyHandler, Brief: "Buy an item from a shop.", Syntax: []string{"buy <item>"}},
		{Keyword: "sell", Handler: SellHandler, Brief: "Sell an item to a shop.", Syntax: []string{"sell <item>"}},
		{Keyword: "value", Handler: ValueHandler, Brief: "Ask a shop what it pays for an item.", Syntax: []string{"value <item>"}},
		{Keyword: "list", Handler: ListHandler, Brief: "List a shop's wares.", Syntax: []string{"list"}},
		{Keyword: "rest", Handler: RestHandler, Brief: "Rest to recover faster.", Syntax: []string{"rest"}},
		{Keyword: "sleep", Handler: SleepHandler, Brief: "Sleep to recover fastest.", Syntax: []string{"sleep"}},
		{Keyword: "wake", Aliases: []string{"stand"}, Handler: WakeHandler, Brief: "Stop resting or sleeping.", Syntax: []string{"wake"}},
		{Keyword: "eat", Handler: EatHandler, Brief: "Eat food to restore sustenance.", Syntax: []string{"eat <food>"}},
		{Keyword: "drink", Handler: DrinkHandler, Brief: "Drink to restore sustenance.", Syntax: []string{"drink <item>"}},
		{Keyword: "use", Handler: UseHandler, Brief: "Use a consumable item.", Syntax: []string{"use <item>"}},

		// Tells (M13.5).
		{Keyword: "tell", Handler: TellHandler, Brief: "Send a private message to another player.", Syntax: []string{"tell <name> <message>"}},
		{Keyword: "reply", Handler: ReplyHandler, Brief: "Reply to the player you last spoke with privately.", Syntax: []string{"reply <message>"}},
		{Keyword: "tells", Handler: TellsHandler, Brief: "Review the tells you've received this session.", Syntax: []string{"tells"}},

		// Channels (M13.6). Per-channel publish verbs (ooc, admin,
		// pack channels) are registered dynamically at composition
		// time from chat.Registry; these are the static management
		// verbs.
		{Keyword: "channels", Aliases: []string{"chanlist"}, Handler: ChatListHandler, Brief: "List the chat channels available to you.", Syntax: []string{"channels"}},
		{Keyword: "chathistory", Aliases: []string{"chhist"}, Handler: ChatHistoryHandler, Brief: "Show recent messages on a channel.", Syntax: []string{"chathistory <channel>", "chathistory <channel> <n>"}},

		// Emotes (M13.7). Table-driven emote verbs (smile, nod,
		// wave, …) are registered dynamically at composition time
		// from emote.Registry; this is the freeform pose verb.
		{Keyword: "emote", Aliases: []string{"pose"}, Handler: EmoteFreeformHandler, Brief: "Emote freeform text to the room.", Syntax: []string{"emote <text>"}},

		// Doors (M15.1). Operate the door on an exit; target is
		// either a direction or a door keyword (with optional
		// ordinal for disambiguation).
		{Keyword: "open", Handler: OpenHandler, Brief: "Open a door.", Syntax: []string{"open <direction>", "open <door>"}},
		{Keyword: "close", Aliases: []string{"shut"}, Handler: CloseHandler, Brief: "Close a door.", Syntax: []string{"close <direction>", "close <door>"}},
		{Keyword: "lock", Handler: LockHandler, Brief: "Lock a door (requires the key).", Syntax: []string{"lock <direction>", "lock <door>"}},
		{Keyword: "unlock", Handler: UnlockHandler, Brief: "Unlock a door (requires the key).", Syntax: []string{"unlock <direction>", "unlock <door>"}},
	}
	for _, c := range commands {
		if err := r.RegisterCommand(c); err != nil {
			return err
		}
	}

	// Admin XP probe (M8.2): self-grants XP, role-gated form lands with
	// the role system (M10+). Bare registration keeps it routable but out
	// of the player-facing command list.
	if err := r.Register("xp", XPHandler); err != nil {
		return err
	}

	// Movement: one keyword per direction (long + short). Registered bare
	// — the authored `movement` help topic covers them, so per-direction
	// generated topics would just be noise.
	for _, d := range []world.Direction{
		world.DirNorth, world.DirSouth, world.DirEast, world.DirWest,
		world.DirUp, world.DirDown,
	} {
		mh := movementHandler(d)
		if err := r.Register(d.Long(), mh); err != nil {
			return err
		}
		if err := r.Register(d.Short(), mh); err != nil {
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
	return c.Actor.Write(ctx, RenderRoom(room, c.Placement, c.Items, c.questMarker()))
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
			if errors.Is(err, world.ErrDoorClosed) {
				// M15.1c: publish door.blocked so subscribers
				// (renderer, AI, future scripting) can react. The
				// door is the one on the source exit; look it up
				// before rendering so KeyID + name come from the
				// authoritative state.
				if door, ok := c.World.GetDoor(room.ID, dir); ok {
					c.Publish(ctx, eventbus.DoorBlocked{DoorEvent: eventbus.DoorEvent{
						RoomID:    room.ID,
						Direction: dir.Short(),
						ActorID:   entities.EntityID(c.Actor.PlayerID()),
						DoorName:  door.Name,
					}})
					return c.Actor.Write(ctx, fmt.Sprintf("%s is closed.", capitalize(door.Name)))
				}
				return c.Actor.Write(ctx, "The way is closed.")
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
		if err := c.Actor.Write(ctx, RenderRoom(dst, c.Placement, c.Items, c.questMarker())); err != nil {
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
// RenderRoom renders a room's name, description, entities, and exits.
// marker, when non-nil, reports whether an entity's template id carries a
// quest marker for the viewer (M10.10b); such entities get a marker glyph
// before their name. Pass nil to skip marker decoration.
func RenderRoom(r *world.Room, placement *entities.Placement, items *entities.Store, marker func(templateID string) bool) string {
	var b strings.Builder
	b.WriteString(r.Name)
	b.WriteString("\n")
	b.WriteString(r.Description)
	b.WriteString("\n")
	if line := renderRoomEntities(r, placement, items, marker); line != "" {
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
func renderRoomEntities(r *world.Room, placement *entities.Placement, items *entities.Store, marker func(templateID string) bool) string {
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
		name := n.Name()
		if name == "" {
			continue
		}
		if marker != nil {
			if tid := templateIDOf(e); tid != "" && marker(tid) {
				name = "<good>(!)</good> " + name
			}
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return ""
	}
	return "You see here: " + strings.Join(names, ", ") + "."
}

// templateIDOf returns the content template id of an entity (item or
// mob), or "" when it has none.
func templateIDOf(e entities.Entity) string {
	switch inst := e.(type) {
	case *entities.ItemInstance:
		return string(inst.TemplateID())
	case *entities.MobInstance:
		return string(inst.TemplateID())
	default:
		return ""
	}
}

func renderExits(r *world.Room) string {
	if len(r.Exits) == 0 {
		return "Exits: none"
	}
	// Build a slice of (long-name, decorated-name) pairs so we can
	// sort by long-name (stable, alphabetical) while emitting the
	// decorated form (M15.1c: doors render their state).
	type labelled struct{ key, label string }
	out := make([]labelled, 0, len(r.Exits))
	for d, e := range r.Exits {
		out = append(out, labelled{key: d.Long(), label: decorateExit(d, e)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key < out[j].key })
	labels := make([]string, len(out))
	for i, lb := range out {
		labels[i] = lb.label
	}
	return fmt.Sprintf("Exits: %s", strings.Join(labels, ", "))
}

// decorateExit returns the exit's long-name with door state appended
// when the exit carries a door. Format: "north (closed)",
// "north (locked)", "north (open)". An unlocked open door renders
// as a plain direction since "open" is the implicit default; an
// open BUT locked door cannot exist (locked implies closed).
func decorateExit(d world.Direction, e world.Exit) string {
	long := d.Long()
	if e.Door == nil {
		return long
	}
	switch {
	case e.Door.Locked:
		return long + " (locked)"
	case e.Door.Closed:
		return long + " (closed)"
	default:
		return long
	}
}
