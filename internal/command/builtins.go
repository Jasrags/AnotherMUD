package command

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/light"
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
		// look is hand-parsed (bare look / look <target> / look in|at
		// <target>, branching on the preposition in LookHandler). It
		// declares a `visible` target + HandParsed so completion enumerates
		// look-at-able things (self/inventory/room items/entities); the
		// at/in prepositions let `look at X` and `look in X` complete too.
		{Keyword: "look", Handler: LookHandler, Brief: "Examine your surroundings or a target.", Syntax: []string{"look", "look <target>"},
			HandParsed: true, Args: []ArgDefinition{{Name: "target", Type: ArgVisible, Prepositions: []string{"at", "in"}}}},
		{Keyword: "map", Handler: MapHandler, Brief: "Show the map of rooms you've explored in this area.", Syntax: []string{"map"}},
		{Keyword: "minimap", Handler: MinimapHandler, Brief: "Toggle the active minimap on the room view.", Syntax: []string{"minimap", "minimap on", "minimap off"}},
		{Keyword: "quit", Handler: QuitHandler, Brief: "Leave the game; your progress is saved.", Syntax: []string{"quit"}},
		{Keyword: "color", Handler: ColorHandler, Brief: "Toggle ANSI color, or show the current setting.", Syntax: []string{"color", "color on", "color off"}},

		// Items (M5.5-M5.9).
		// get/take is hand-parsed (the item scope flips on the `from`
		// preposition — room items for the bare form, container contents
		// for the `from` form — which the single-scope auto-resolver can't
		// express). It declares Args + HandParsed so completion can still
		// enumerate the bare-form room item and the `from` container,
		// while GetHandler keeps reading raw Args. `take` is an alias.
		{Keyword: "get", Aliases: []string{"take"}, Handler: GetHandler, Brief: "Pick up an item from the room or a container.", Syntax: []string{"get <item>", "get <item> from <container>", "get coins from <corpse>"},
			HandParsed: true, Args: []ArgDefinition{
				{Name: "item", Type: ArgRoomItem},
				{Name: "container", Type: ArgContainer, Prepositions: []string{"from"}},
			}},
		{Keyword: "drop", Handler: DropHandler, Brief: "Drop an item from your inventory.", Syntax: []string{"drop <item>"}, Args: []ArgDefinition{{Name: "item", Type: ArgInventory}}},
		{Keyword: "give", Handler: GiveHandler, Brief: "Give an item to another character.", Syntax: []string{"give <item> to <target>"}, Args: []ArgDefinition{{Name: "item", Type: ArgInventory}, {Name: "target", Type: ArgPlayer, Prepositions: []string{"to"}}}},
		{Keyword: "put", Handler: PutHandler, Brief: "Put an item into a container.", Syntax: []string{"put <item> in <container>"}, Args: []ArgDefinition{{Name: "item", Type: ArgInventory}, {Name: "container", Type: ArgContainer, Prepositions: []string{"in", "into"}}}},
		// fill <target> [from] <source>: a carried vessel filled from a
		// room source (the well). HandParsed (the handler runs
		// parseFillArgs); the two args let completion enumerate the
		// inventory (target) and room items (source) — `from` is the
		// optional preposition, mirroring `put <item> in <container>`.
		{Keyword: "fill", Handler: FillHandler, Brief: "Fill a container from a source.", Syntax: []string{"fill <target> from <source>"},
			HandParsed: true, Args: []ArgDefinition{
				{Name: "target", Type: ArgInventory},
				{Name: "source", Type: ArgRoomItem, Prepositions: []string{"from"}},
			}},
		{Keyword: "loot", Handler: LootHandler, Brief: "Take everything from a corpse.", Syntax: []string{"loot", "loot <corpse>"}},
		{Keyword: "autoloot", Handler: AutolootHandler, Brief: "Toggle auto-looting your own kills.", Syntax: []string{"autoloot", "autoloot on|off"}},
		{Keyword: "equip", Handler: EquipHandler, Brief: "Wear or wield an item from your inventory.", Syntax: []string{"equip <item> <slot>"}, Args: []ArgDefinition{{Name: "item", Type: ArgInventory}, {Name: "slot", Type: ArgKeyword}}},
		// unequip declares its equipped-item arg + HandParsed so completion
		// enumerates the worn items while the handler keyword-resolves the
		// raw term itself.
		{Keyword: "unequip", Handler: UnequipHandler, Brief: "Remove an equipped item.", Syntax: []string{"unequip <item>"},
			HandParsed: true, Args: []ArgDefinition{{Name: "item", Type: ArgEquipped}}},
		{Keyword: "inventory", Aliases: []string{"i"}, Handler: InventoryHandler, Brief: "List the items you are carrying.", Syntax: []string{"inventory"}},
		{Keyword: "equipment", Aliases: []string{"eq"}, Handler: EquipmentHandler, Brief: "Show your equipment slots (empty ones included).", Syntax: []string{"equipment"}},

		// Light sources (light-and-darkness §3.1). Hand-parsed: the item
		// resolves over carried + equipped, a wider scope than ArgInventory.
		{Keyword: "light", Handler: LightHandler, Brief: "Light a torch, lantern, or other source.", Syntax: []string{"light <item>"}, HandParsed: true},
		{Keyword: "extinguish", Aliases: []string{"douse"}, Handler: ExtinguishHandler, Brief: "Put out a lit light source.", Syntax: []string{"extinguish <item>"}, HandParsed: true},
		{Keyword: "daylight", Aliases: []string{"time"}, Handler: DaylightHandler, Brief: "Report the time of day and how well you can see.", Syntax: []string{"daylight"}},

		// Combat (M7).
		// consider is hand-parsed like kill (resolves via findCombatantInRoom).
		// It declares its entity target + HandParsed so completion enumerates
		// room mobs/players. Target-only now — self stats live in `score`.
		{Keyword: "consider", Aliases: []string{"con"}, Handler: ConsiderHandler, Brief: "Size up a target before fighting.", Syntax: []string{"consider <target>"},
			HandParsed: true, Args: []ArgDefinition{{Name: "target", Type: ArgEntity}}},

		// score (`sc`): the player character sheet — identity, level,
		// vitals, attributes, alignment, gold, sustenance. Self-only;
		// consider sizes up others. Reads through the actor's interfaces.
		{Keyword: "score", Aliases: []string{"sc"}, Handler: ScoreHandler, Brief: "Show your character sheet.", Syntax: []string{"score"}},
		{Keyword: "who", Handler: WhoHandler, Brief: "List the characters currently online.", Syntax: []string{"who"}},

		// suggest: the line-mode completion stopgap (tab-completion §7/§13).
		// Lists completion candidates for a partial command — real
		// completion without a TAB key, usable on raw telnet. Same query as
		// the admin `complete` debug verb, rendered for players.
		{Keyword: "suggest", Handler: SuggestHandler, Brief: "List completions for a partial command.", Syntax: []string{"suggest <partial command>"}},

		// tabcomplete (tab-completion Phase 2): toggle server-side TAB
		// completion (char-mode line editing). Default-on for raw telnet;
		// unavailable on GMCP/WebSocket clients (they complete client-side).
		{Keyword: "tabcomplete", Handler: TabCompleteHandler, Brief: "Toggle server-side TAB completion (raw telnet).", Syntax: []string{"tabcomplete [on|off]"}},
		// kill is hand-parsed (the self-check must run BEFORE resolving,
		// and the entity arg excludes self). It declares its entity target
		// + HandParsed so completion enumerates room mobs/players, while
		// KillHandler keeps its self-check-first resolution via
		// findCombatantInRoom (which resolves the same `entity` arg).
		{Keyword: "kill", Handler: KillHandler, Brief: "Attack a target.", Syntax: []string{"kill <target>"},
			HandParsed: true, Args: []ArgDefinition{{Name: "target", Type: ArgEntity}}},
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
		// talk declares an npc arg + HandParsed so completion enumerates
		// room NPCs (quest givers); the handler resolves the raw term.
		{Keyword: "talk", Aliases: []string{"ask"}, Handler: TalkHandler, Brief: "Talk to a quest giver to hear offers or turn in a quest.", Syntax: []string{"talk <npc>"},
			HandParsed: true, Args: []ArgDefinition{{Name: "npc", Type: ArgNPC}}},
		// accept declares a `quest` arg + HandParsed so completion
		// enumerates the room givers' offers (the bare quest id round-trips
		// through ResolveID), while the handler keeps resolving the raw
		// term itself.
		{Keyword: "accept", Handler: AcceptHandler, Brief: "Accept an offered quest.", Syntax: []string{"accept <quest>"},
			HandParsed: true, Args: []ArgDefinition{{Name: "quest", Type: ArgQuest}}},
		// abandon mirrors accept: a `quest` arg (active variant) +
		// HandParsed so completion enumerates the actor's active quests
		// while the handler resolves the raw term itself.
		{Keyword: "abandon", Handler: AbandonHandler, Brief: "Abandon an active quest.", Syntax: []string{"abandon <quest>"},
			HandParsed: true, Args: []ArgDefinition{{Name: "quest", Type: ArgActiveQuest}}},
		{Keyword: "quests", Aliases: []string{"journal"}, Handler: QuestsHandler, Brief: "Show your active quests.", Syntax: []string{"quests"}},

		// Economy (M11).
		{Keyword: "gold", Handler: GoldHandler, Brief: "Show how much gold you carry.", Syntax: []string{"gold"}},
		// buy declares a shop-item arg + HandParsed so completion
		// enumerates the room shop's stock; the handler resolves the raw
		// term through ShopService.
		{Keyword: "buy", Handler: BuyHandler, Brief: "Buy an item from a shop.", Syntax: []string{"buy <item>"},
			HandParsed: true, Args: []ArgDefinition{{Name: "item", Type: ArgShopItem}}},
		// sell/value resolve a held item against the shop; declare the
		// inventory arg + HandParsed so completion enumerates what you
		// carry (the handler resolves it through ShopService).
		{Keyword: "sell", Handler: SellHandler, Brief: "Sell an item to a shop.", Syntax: []string{"sell <item>"},
			HandParsed: true, Args: []ArgDefinition{{Name: "item", Type: ArgInventory}}},
		{Keyword: "value", Handler: ValueHandler, Brief: "Ask a shop what it pays for an item.", Syntax: []string{"value <item>"},
			HandParsed: true, Args: []ArgDefinition{{Name: "item", Type: ArgInventory}}},
		{Keyword: "list", Handler: ListHandler, Brief: "List a shop's wares.", Syntax: []string{"list"}},
		{Keyword: "rest", Handler: RestHandler, Brief: "Rest to recover faster.", Syntax: []string{"rest"}},
		{Keyword: "sleep", Handler: SleepHandler, Brief: "Sleep to recover fastest.", Syntax: []string{"sleep"}},
		{Keyword: "wake", Aliases: []string{"stand"}, Handler: WakeHandler, Brief: "Stop resting or sleeping.", Syntax: []string{"wake"}},
		{Keyword: "eat", Handler: EatHandler, Brief: "Eat food to restore sustenance.", Syntax: []string{"eat <food>"}, Args: []ArgDefinition{{Name: "item", Type: ArgInventory}}},
		{Keyword: "drink", Handler: DrinkHandler, Brief: "Drink to restore sustenance.", Syntax: []string{"drink <item>"}, Args: []ArgDefinition{{Name: "item", Type: ArgInventory}}},
		{Keyword: "use", Handler: UseHandler, Brief: "Use a consumable item.", Syntax: []string{"use <item>"}, Args: []ArgDefinition{{Name: "item", Type: ArgInventory}}},

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
		{Keyword: "open", Handler: OpenHandler, Brief: "Open a door.", Syntax: []string{"open <direction>", "open <door>"}, Args: []ArgDefinition{{Name: "door", Type: ArgDoor}}},
		{Keyword: "close", Aliases: []string{"shut"}, Handler: CloseHandler, Brief: "Close a door.", Syntax: []string{"close <direction>", "close <door>"}, Args: []ArgDefinition{{Name: "door", Type: ArgDoor}}},
		{Keyword: "lock", Handler: LockHandler, Brief: "Lock a door (requires the key).", Syntax: []string{"lock <direction>", "lock <door>"}, Args: []ArgDefinition{{Name: "door", Type: ArgDoor}}},
		{Keyword: "unlock", Handler: UnlockHandler, Brief: "Unlock a door (requires the key).", Syntax: []string{"unlock <direction>", "unlock <door>"}, Args: []ArgDefinition{{Name: "door", Type: ArgDoor}}},

		// Recall (M15.3). `recall set` binds the current room as the
		// character's return point; `recall` teleports back to it. (The
		// binding verb moved from the former `set recall` to `recall set` in
		// M19.4c when admin `set` reclaimed the top-level `set` keyword.)
		// Spec: docs/specs/recall.md.
		{Keyword: "recall", Handler: RecallHandler, Brief: "Return to your bound recall point.", Syntax: []string{"recall", "recall set"}},

		// Prompt (ui-rendering-help §7.4). Show / set / reset the
		// status prompt template. The template uses {tokens} (§7.2)
		// and color tags (§2).
		{Keyword: "prompt", Handler: PromptHandler, Brief: "Show or change your status prompt.", Syntax: []string{"prompt", "prompt <template>", "prompt default"}},

		// Roles (M19.2 — roles-and-permissions §4). grant/revoke a role
		// to/from another online character. Admin-marked (M19.3): the
		// dispatcher gates them on the admin role and hides them from
		// non-admins in help. The handler ALSO self-gates on the granting
		// role (§4) — the granting role may differ from the admin role.
		{Keyword: "grant", Handler: GrantHandler, Admin: true, Brief: "Grant a role to another player.", Syntax: []string{"grant <role> to <player>"}},
		{Keyword: "revoke", Handler: RevokeHandler, Admin: true, Brief: "Revoke a role from another player.", Syntax: []string{"revoke <role> from <player>"}},

		// Admin verbs (M19.3 — admin-verbs §2). Admin-marked → dispatcher
		// gates them on the admin role, hidden from non-admins in help.
		// xp self-grants XP (a probe); reload hot-swaps pack Lua. These
		// were ungated/bare until the role system landed (the standing
		// "ungated until roles" verbs the spec §2 calls out).
		{Keyword: "xp", Handler: XPHandler, Admin: true, Brief: "Grant yourself XP (admin probe).", Syntax: []string{"xp", "xp <amount> [track]"}},
		{Keyword: "reload", Handler: ReloadHandler, Admin: true, Brief: "Reload pack scripts.", Syntax: []string{"reload"}},

		// announce (M19.4a — admin-verbs §5): broadcast a server-wide
		// message to every connected session, attributed as an
		// administrative announcement. Emits the admin.action audit fact
		// (§6) via the shared auditAdmin choke point.
		{Keyword: "announce", Handler: AnnounceHandler, Admin: true, Brief: "Broadcast a server-wide announcement.", Syntax: []string{"announce <message>"}},

		// inspect (M19.4b — admin-verbs §5): read-only diagnostic dump of a
		// target's identity/vitals/stats (+ roles/levels/equipment/tags/
		// properties where the kind carries them). No argument inspects
		// self; otherwise resolves a player or mob in the room (§3). Audited
		// via auditAdmin.
		{Keyword: "inspect", Handler: InspectHandler, Admin: true, Brief: "Inspect a target's full diagnostic record.", Syntax: []string{"inspect [<target>]"}},
		{Keyword: "roomdata", Handler: RoomDataHandler, Admin: true, Brief: "Toggle the room metadata block on look (admin/builder).", Syntax: []string{"roomdata", "roomdata on", "roomdata off"}},

		// set (M19.4c — admin-verbs §4): the general-purpose admin field
		// write. `set <kind> <type> <target> <value>` mutates one field on a
		// resolved target; a bare/incomplete set renders the catalogue.
		// Admin-marked → dispatch-gated + hidden from non-admins, so it
		// reclaims the top-level `set` keyword cleanly (the former player
		// `set recall` moved to `recall set`). Audited via auditAdmin.
		{Keyword: "set", Handler: SetHandler, Admin: true, Brief: "Set a field on a target (admin).", Syntax: []string{"set <kind> <type> <target> <value>"}},

		// restore (M19.4d — admin-verbs §5): the mercy verb. Set a target's
		// vitals to full (Vitals.SetCurrent(max)). No arg restores self.
		{Keyword: "restore", Handler: RestoreHandler, Admin: true, Brief: "Restore a target's vitals to full.", Syntax: []string{"restore [<target>]"}},

		// teleport (M19.4d — admin-verbs §5, alias `goto`): move the actor to
		// a room by id or to the room of an online player (§3 world-scoped
		// resolution). Reuses SetRoom's room-change events.
		{Keyword: "teleport", Aliases: []string{"goto"}, Handler: TeleportHandler, Admin: true, Brief: "Teleport to a room or player.", Syntax: []string{"teleport <room-id>", "teleport <player>"}},

		// purge (M19.4e — admin-verbs §5): remove a non-player entity (mob
		// or room item) from the world, untracking it. Never targets a
		// player. Removal mirrors the death-cleanup path; audited.
		{Keyword: "purge", Handler: PurgeHandler, Admin: true, Brief: "Remove a mob or item from the world.", Syntax: []string{"purge <target>"}},
		{Keyword: "badinput", Handler: BadInputHandler, Admin: true, Brief: "Show the unknown-verb tracker (admin).", Syntax: []string{"badinput", "badinput clear"}},

		// complete (tab-completion §9 — Phase 0 exercise surface): run the
		// completion query on a partial line and print the candidate set.
		// Admin-gated + read-only — an introspection tool that smokes the
		// enumeration substrate live, NOT the player completion surface
		// (GMCP/char-mode are Phase 1/2).
		{Keyword: "complete", Handler: CompleteHandler, Admin: true, Brief: "Show completion candidates for a partial line (debug).", Syntax: []string{"complete <partial line>"}},
	}
	for _, c := range commands {
		if err := r.RegisterCommand(c); err != nil {
			return err
		}
	}

	// xp and reload are now admin-marked commands in the slice above
	// (M19.3 — admin-verbs §2), gated on the admin role at dispatch.

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
	// `look` / `look in <target>` / `look at <target>`. The bare form
	// renders the room; a target form looks at an item or into a
	// container (loot-and-corpses §2.2 corpse look-in).
	args := c.Args
	if len(args) > 0 && (strings.EqualFold(args[0], "in") || strings.EqualFold(args[0], "at")) {
		args = args[1:]
	}
	// Bare look, or a targeted look with no item store wired (test /
	// headless paths), renders the room — never a misleading
	// "you don't see that" for a missing subsystem.
	if len(args) == 0 || c.Items == nil {
		return c.Actor.Write(ctx, c.renderRoomWithData(room, c.effectiveLight(room)))
	}
	return c.lookAtTarget(ctx, args)
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
		// §5.4 darkness-hazard gate: a destination room may opt into
		// being impassable when the mover cannot see it at all. Light (a
		// carried torch) lets you brave it; total darkness (effective
		// black) refuses the step. Off by default — only rooms that set
		// dark_blocked are gated, and only at black, so the escape
		// invariant holds (outdoors is never black, and a retrace leads
		// to the navigable room you came from). Computed before the move
		// commits.
		dstLvl := c.effectiveLight(dst)
		if blocked, _ := dst.PropertyBool(PropRoomDarkBlocked); c.Light != nil && dstLvl <= light.Black && blocked {
			return c.Actor.Write(ctx, darkBlockedText)
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
		if err := c.Actor.Write(ctx, c.renderRoomWithData(dst, dstLvl)); err != nil {
			return err
		}
		// Escape-invariant affordance (§5.4): when the mover arrives
		// somewhere they cannot fully see, name the way back so the entry
		// direction is always known — they can retrace even when the
		// obscured/suppressed render hides the exits. Only emitted when
		// the destination has a real exit back the way they came.
		if dstLvl <= light.Gloom {
			if back := dir.Opposite(); back != world.DirInvalid {
				if _, ok := dst.Exits[back]; ok {
					_ = c.Actor.Write(ctx, fmt.Sprintf("<subtle>(You can feel your way back %s.)</subtle>", back.Long()))
				}
			}
		}
		// Deferred (full) hook AFTER the description so non-hostile
		// reactions arrive below the room text.
		if c.Disposition != nil && pid != "" {
			c.Disposition.OnPlayerEnteredDeferred(ctx, pid, name, nil, dst.ID)
		}
		return nil
	}
}

// PropRoomDarkBlocked is the room property opting a room into the §5.4
// darkness-movement hazard: a mover who cannot see it at all (effective
// black) is refused entry. darkBlockedText is the refusal.
const (
	PropRoomDarkBlocked = "dark_blocked"
	darkBlockedText     = "It is too dark to risk going that way."
)

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
// otherPlayerNames returns the display names of players in roomID other
// than the acting player, for the room render's "You see here:" line.
// Empty when no Locator is wired (tests / headless paths).
func (c *Context) otherPlayerNames(roomID world.RoomID) []string {
	if c.Locator == nil {
		return nil
	}
	self := c.Actor.PlayerID()
	var out []string
	for _, p := range c.Locator.PlayersInRoom(roomID) {
		if p == nil || (self != "" && p.PlayerID() == self) {
			continue
		}
		if n := p.Name(); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// hostileMarker returns a predicate reporting whether a placed mob is
// hostile to the viewing actor, for RenderRoom's red coloring. Returns
// nil when disposition is unwired or the actor has no player id (tests,
// pre-login) so the renderer falls back to the neutral <present.mob>
// color. Players carry no tags here — the same nil-tags simplification
// the room-entry hook already uses (move handler above) — so the v1
// reddens statically-hostile mobs and tag-free hostile rules.
func (c *Context) hostileMarker() func(*entities.MobInstance) bool {
	if c.Disposition == nil || c.Actor == nil {
		return nil
	}
	pid := c.Actor.PlayerID()
	if pid == "" {
		return nil
	}
	name := c.Actor.Name()
	return func(m *entities.MobInstance) bool {
		return c.Disposition.Hostile(m, pid, name, nil)
	}
}

// RenderRoom renders a room's name, description, entities, and exits.
//
// marker, when non-nil, reports whether an entity's template id
// carries a quest marker for the viewer (M10.10b); such entities get
// a marker glyph before their name. Pass nil to skip marker
// decoration.
//
// ambience, when non-nil and non-empty for r, is appended after the
// room description on its own line. The current consumer is the
// M15.4b₂b weather hook (weather.Service.Ambience). Pass nil for
// renderers (tests, link-dead recovery before weather is wired)
// that don't have an ambience source; an empty return from a
// non-nil ambience is also treated as "nothing to render".
// players are the display names of OTHER players present (the viewer
// excludes themselves before calling). Variadic so existing callers
// without a player list stay source-compatible; players are listed in
// the "You see here:" line alongside mobs and items.
// hostile, when non-nil, reports whether a placed mob is hostile to the
// viewer; such mobs render in <present.hostile> (red) instead of the
// neutral <present.mob>. Pass nil (tests, renderers without a
// disposition source) to color every mob neutrally.
//
// lvl is the viewer's effective light level (light-and-darkness §5.1).
// It branches the render: `lit` is the full render; `dim` is the full
// render with the description muted; `gloom` obscures (terse prose,
// coarse occupant presence with identities hidden, bare-direction
// exits); `black` suppresses everything to a single dark line. Callers
// that do not gate on light (tests, unwired paths) pass light.Lit for
// the unchanged full render.
func RenderRoom(r *world.Room, placement *entities.Placement, items *entities.Store, marker func(templateID string) bool, ambience func(*world.Room) string, hostile func(*entities.MobInstance) bool, lvl light.Level, players ...string) string {
	switch {
	case lvl <= light.Black:
		// Suppressed: name, description, occupants all withheld (§5.1).
		return "<subtle>" + blackRoomText + "</subtle>"
	case lvl == light.Gloom:
		return renderGloomRoom(r, placement, items, players)
	default:
		// Lit or Dim: full render; dim mutes the description prose.
		return renderFullRoom(r, placement, items, marker, ambience, hostile, lvl == light.Dim, players)
	}
}

// Reduced-light render strings (§5.1). Hardcoded for v1; externalizing
// them to the configuration surface (§11) is deferred.
const (
	blackRoomText = "It is pitch black. You can see nothing."
	gloomRoomText = "It is too dark to make out any detail; you can sense only shapes and directions."
)

// renderFullRoom is the lit/dim render: the room name, description,
// ambience, occupants, and exits. When dim is true the description is
// wrapped in the {dim} attribute so the prose reads muted while the
// rest of the body keeps its semantic colors (a single SGR attribute
// over plain prose, so no nested-tag reset problem). Both forms degrade
// to clean text on no-color clients.
func renderFullRoom(r *world.Room, placement *entities.Placement, items *entities.Store, marker func(templateID string) bool, ambience func(*world.Room) string, hostile func(*entities.MobInstance) bool, dim bool, players []string) string {
	var b strings.Builder
	b.WriteString("<title>")
	b.WriteString(r.Name)
	b.WriteString("</title>")
	b.WriteString("\n")
	if dim && r.Description != "" {
		b.WriteString("{dim}")
		b.WriteString(r.Description)
		b.WriteString("{/}")
	} else {
		b.WriteString(r.Description)
	}
	b.WriteString("\n")
	if ambience != nil {
		if line := ambience(r); line != "" {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	if line := renderRoomEntities(r, placement, items, marker, hostile, players); line != "" {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(renderExits(r))
	return b.String()
}

// renderGloomRoom is the obscured render (§5.1 gloom): the room name
// still anchors (you know where you stand), but the prose is replaced
// by a terse dark line, occupants are coarsened to presence-without-
// identity (names hidden), and exits render as bare directions with no
// door/weather detail.
func renderGloomRoom(r *world.Room, placement *entities.Placement, items *entities.Store, players []string) string {
	var b strings.Builder
	b.WriteString("<title>")
	b.WriteString(r.Name)
	b.WriteString("</title>")
	b.WriteString("\n")
	b.WriteString("{dim}")
	b.WriteString(gloomRoomText)
	b.WriteString("{/}")
	b.WriteString("\n")
	if line := renderCoarseOccupants(r, placement, items, players); line != "" {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(renderBareExits(r))
	return b.String()
}

// renderCoarseOccupants lists occupant PRESENCE at gloom without
// identity: each other player and each placed mob becomes an
// anonymous shape; items are not made out at all (objects need detail).
// Names are hidden — the §5.1 occupant-coarsening rule. The
// granularity here (one anonymous token per occupant) is the v1
// default; configurable presence/count/kind granularity (§11) is
// deferred.
func renderCoarseOccupants(r *world.Room, placement *entities.Placement, items *entities.Store, players []string) string {
	shapes := make([]string, 0, len(players))
	for range players {
		shapes = append(shapes, "someone")
	}
	if placement != nil && items != nil {
		for _, id := range placement.InRoom(r.ID) {
			e, ok := items.GetByID(id)
			if !ok {
				continue
			}
			if _, ok := e.(*entities.MobInstance); ok {
				shapes = append(shapes, "a shape")
			}
		}
	}
	if len(shapes) == 0 {
		return ""
	}
	return "<subtle>You can make out:</subtle> " + strings.Join(shapes, ", ") + "."
}

// renderBareExits lists exit directions only — no door state, no
// decoration — for the gloom render (§5.1: "exits shown as bare
// directions").
func renderBareExits(r *world.Room) string {
	if len(r.Exits) == 0 {
		return "<subtle>Exits:</subtle> none"
	}
	longs := make([]string, 0, len(r.Exits))
	for d := range r.Exits {
		longs = append(longs, "<exit>"+d.Long()+"</exit>")
	}
	sort.Strings(longs)
	return "<subtle>Exits:</subtle> " + strings.Join(longs, ", ")
}

// renderRoomEntities builds the "You see here: …" line. Other players
// (passed in, viewer already excluded) list first, then placed
// mobs/items. Returns the empty string when there's nothing to show —
// no other players AND no resolvable placed entities (Placement/Store
// nil, no placed ids, or every placed id fails resolution). Each
// entity branch is a silent skip rather than a panic because the
// renderer is on the player-visible path; missing data should look
// like nothing-here, not a runtime error.
func renderRoomEntities(r *world.Room, placement *entities.Placement, items *entities.Store, marker func(templateID string) bool, hostile func(*entities.MobInstance) bool, players []string) string {
	// Other players first (highlighted), then placed mobs/items colored
	// by kind. The "You see here:" label dims to <subtle> so the names
	// it introduces carry the visual weight.
	names := make([]string, 0, len(players))
	for _, p := range players {
		names = append(names, "<present.player>"+p+"</present.player>")
	}
	ids := []entities.EntityID(nil)
	if placement != nil && items != nil {
		ids = placement.InRoom(r.ID)
	}
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
		// Color the bare name by entity kind first, then prepend the
		// quest marker OUTSIDE the color tag so the two never nest
		// (spec §2.4: a nested tag's close resets the outer color).
		name = colorizeEntityName(e, name, hostile)
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
	return "<subtle>You see here:</subtle> " + strings.Join(names, ", ") + "."
}

// colorizeEntityName wraps a placed entity's display name in the
// semantic tag for its kind: items take an item.* rarity tag (from the
// reserved "rarity" instance property — the same source the
// item-decorations system reads, decorate.go) and mobs take
// <present.mob>. Other players are tagged at the call site (they arrive
// as bare names, not entities). An unrecognized entity kind renders
// plain. hostile, when non-nil, reddens a mob the viewer is hostile
// toward (<present.hostile>) instead of the neutral mob color.
func colorizeEntityName(e entities.Entity, name string, hostile func(*entities.MobInstance) bool) string {
	switch inst := e.(type) {
	case *entities.ItemInstance:
		tag := itemRarityTag(inst)
		return "<" + tag + ">" + name + "</" + tag + ">"
	case *entities.MobInstance:
		if hostile != nil && hostile(inst) {
			return "<present.hostile>" + name + "</present.hostile>"
		}
		return "<present.mob>" + name + "</present.mob>"
	default:
		return name
	}
}

// itemRarityTag returns the item.<key> theme tag for an item's rarity,
// read from the canonical "rarity" instance property (propRarity in
// decorate.go). Only the tiers the default theme ships colors for are
// honored; an absent, empty, or unrecognized key falls back to
// item.common so the room line never emits an unregistered tag (which
// the renderer would pass through as literal "<item.foo>" text). A pack
// that adds a custom tier registers its color via decoration but won't
// be name-colored in the room line until this whitelist or a registry
// hand-off grows — a deliberate safe default, not a silent drop.
func itemRarityTag(it *entities.ItemInstance) string {
	key, ok := stringProp(it, propRarity)
	if !ok {
		return "item.common"
	}
	switch key {
	case "uncommon", "rare", "legendary":
		return "item." + key
	default:
		return "item.common"
	}
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
		return "<subtle>Exits:</subtle> none"
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
	return fmt.Sprintf("<subtle>Exits:</subtle> %s", strings.Join(labels, ", "))
}

// decorateExit returns the exit's long-name with door state appended
// when the exit carries a door. Format: "north (closed)",
// "north (locked)", "north (open)". An unlocked open door renders
// as a plain direction since "open" is the implicit default; an
// open BUT locked door cannot exist (locked implies closed).
//
// The direction word renders as an <exit> (cyan) so exits read as
// actionable; a closed door's "(closed)" suffix reuses <warning>
// (yellow) and a locked door's "(locked)" reuses <danger> (red), so
// the obstacle stands out without a legend. The severity tag sits
// OUTSIDE the <exit> tag (sequential, not nested) per spec §2.4.
func decorateExit(d world.Direction, e world.Exit) string {
	long := "<exit>" + d.Long() + "</exit>"
	if e.Door == nil {
		return long
	}
	switch {
	case e.Door.Locked:
		return long + " <danger>(locked)</danger>"
	case e.Door.Closed:
		return long + " <warning>(closed)</warning>"
	default:
		return long
	}
}
