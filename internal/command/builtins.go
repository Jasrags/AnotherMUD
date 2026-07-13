package command

import (
	"context"
	"errors"
	"fmt"
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
		{Keyword: "minimap", Handler: MinimapHandler, Brief: "Toggle the active minimap, or set its size.", Syntax: []string{"minimap", "minimap on|off", "minimap auto|small|medium|large"}},
		{Keyword: "quit", Handler: QuitHandler, Brief: "Leave the game; your progress is saved.", Syntax: []string{"quit"}},
		{Keyword: "color", Handler: ColorHandler, Brief: "Toggle ANSI color, or show the current setting.", Syntax: []string{"color", "color on", "color off"}},

		// Reveal-on-action policy (visibility §4.5): the "loud" verbs below
		// carry BreaksConcealment — combat (kill), casting (cast/channel/
		// overchannel), and physical item/door manipulation (get/take, drop,
		// give, put, loot, open, close). DELIBERATELY NOT flagged in v1, as a
		// stealth-friendly call: quiet lock work (pick/lock/unlock — a thief
		// stays hidden) and slow careful activity (forage/harvest/craft/build).
		// The vocal/emote tier (emote/pose + pack emotes + chat channels) is
		// also unflagged — channels are cross-room, and the dynamic emote
		// registration has no flag-carrying path yet; revisit "speaking aloud"
		// reveal as its own slice if it earns its keep.
		//
		// Items (M5.5-M5.9).
		// get/take is hand-parsed (the item scope flips on the `from`
		// preposition — room items for the bare form, container contents
		// for the `from` form — which the single-scope auto-resolver can't
		// express). It declares Args + HandParsed so completion can still
		// enumerate the bare-form room item and the `from` container,
		// while GetHandler keeps reading raw Args. `take` is an alias.
		{Keyword: "get", Aliases: []string{"take"}, Handler: GetHandler, Brief: "Pick up an item from the room or a container.", Syntax: []string{"get <item>", "get <item> from <container>", "get coins from <corpse>"},
			HandParsed: true, BreaksConcealment: true, Args: []ArgDefinition{
				{Name: "item", Type: ArgRoomItem},
				{Name: "container", Type: ArgContainer, Prepositions: []string{"from"}},
			}},
		{Keyword: "drop", Handler: DropHandler, Brief: "Drop an item from your inventory.", Syntax: []string{"drop <item>"}, BreaksConcealment: true, Args: []ArgDefinition{{Name: "item", Type: ArgInventory}}},
		{Keyword: "give", Handler: GiveHandler, Brief: "Give an item to another character or an NPC.", Syntax: []string{"give <item> to <target>"}, BreaksConcealment: true, Args: []ArgDefinition{{Name: "item", Type: ArgInventory}, {Name: "target", Type: ArgGiveTarget, Prepositions: []string{"to"}}}},
		{Keyword: "put", Handler: PutHandler, Brief: "Put an item into a container.", Syntax: []string{"put <item> in <container>"}, BreaksConcealment: true, Args: []ArgDefinition{{Name: "item", Type: ArgInventory}, {Name: "container", Type: ArgContainer, Prepositions: []string{"in", "into"}}}},
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
		{Keyword: "loot", Handler: LootHandler, Brief: "Take everything from a corpse.", Syntax: []string{"loot", "loot <corpse>"}, BreaksConcealment: true},
		{Keyword: "autoloot", Handler: AutolootHandler, Brief: "Toggle auto-looting your own kills.", Syntax: []string{"autoloot", "autoloot on|off"}},
		{Keyword: "autoreload", Handler: AutoreloadHandler, Brief: "Toggle auto-reloading a dry firearm from a spare clip.", Syntax: []string{"autoreload", "autoreload on|off"}},
		{Keyword: "autoassist", Handler: AutoAssistHandler, Brief: "Toggle automatically joining your party's fights.", Syntax: []string{"autoassist", "autoassist on|off"}},
		{Keyword: "equip", Aliases: []string{"wear", "wield", "hold"}, Handler: EquipHandler, Brief: "Wear or wield an item from your inventory.", Syntax: []string{"equip <item> [slot]"}, Args: []ArgDefinition{{Name: "item", Type: ArgInventory}, {Name: "slot", Type: ArgKeyword, Optional: true}}, IsAction: true},
		// unequip declares its equipped-item arg + HandParsed so completion
		// enumerates the worn items while the handler keyword-resolves the
		// raw term itself.
		{Keyword: "unequip", Handler: UnequipHandler, Brief: "Remove an equipped item.", Syntax: []string{"unequip <item>"},
			HandParsed: true, Args: []ArgDefinition{{Name: "item", Type: ArgEquipped}}, IsAction: true},
		{Keyword: "hastydon", Aliases: []string{"quickdon"}, Handler: HastyDonHandler, Brief: "Throw on armor fast — quicker, but it protects less until re-donned.", Syntax: []string{"hastydon <item>"}, Args: []ArgDefinition{{Name: "item", Type: ArgInventory}}, IsAction: true},
		{Keyword: "stop", Aliases: []string{"cancel"}, Handler: StopHandler, Brief: "Stop what you're currently doing.", Syntax: []string{"stop"}},
		{Keyword: "follow", Handler: FollowHandler, Brief: "Follow another character or creature so you travel with them.", Syntax: []string{"follow", "follow <target>"}, Args: []ArgDefinition{{Name: "target", Type: ArgEntity, Optional: true}}},
		{Keyword: "unfollow", Handler: UnfollowHandler, Brief: "Stop following whoever you're following.", Syntax: []string{"unfollow"}},
		{Keyword: "lose", Handler: LoseHandler, Brief: "Shake off everyone following you.", Syntax: []string{"lose"}},
		{Keyword: "group", Handler: GroupHandler, Brief: "Invite a player to your party, or list your party.", Syntax: []string{"group", "group <player>"}, Args: []ArgDefinition{{Name: "target", Type: ArgPlayer, Optional: true}}},
		{Keyword: "join", Handler: JoinHandler, Brief: "Accept a party invitation.", Syntax: []string{"join <leader>"}, HandParsed: true, Args: []ArgDefinition{{Name: "target", Type: ArgPlayer}}},
		{Keyword: "leave", Aliases: []string{"ungroup"}, Handler: LeaveHandler, Brief: "Leave your party.", Syntax: []string{"leave"}},
		{Keyword: "disband", Handler: DisbandHandler, Brief: "Disband the party you lead.", Syntax: []string{"disband"}},
		{Keyword: "promote", Handler: PromoteHandler, Brief: "Hand party leadership to a member.", Syntax: []string{"promote <member>"}, HandParsed: true, Args: []ArgDefinition{{Name: "target", Type: ArgPlayer}}},
		{Keyword: "gtell", Handler: GtellHandler, Brief: "Send a message to your party.", Syntax: []string{"gtell <message>"}, HandParsed: true, Args: []ArgDefinition{{Name: "message", Type: ArgText}}},
		{Keyword: "lootmode", Handler: LootModeHandler, Brief: "Set or show your party's loot rules.", Syntax: []string{"lootmode", "lootmode ffa", "lootmode master [<member>]"}, HandParsed: true},
		{Keyword: "load", Handler: LoadHandler, Brief: "Load a reloadable ranged weapon (a crossbow).", Syntax: []string{"load"}, IsAction: true},
		{Keyword: "reload", Handler: ReloadHandler, Brief: "Reload a firearm (or fill a clip: reload <clip>).", Syntax: []string{"reload", "reload <clip>"}, IsAction: true, HandParsed: true},
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
			HandParsed: true, BreaksConcealment: true, Args: []ArgDefinition{{Name: "target", Type: ArgEntity}}},
		{Keyword: "assist", Handler: AssistHandler, Brief: "Join the fight an ally is in (attack their foe).", Syntax: []string{"assist <ally>"},
			HandParsed: true, BreaksConcealment: true, Args: []ArgDefinition{{Name: "target", Type: ArgEntity}}},
		{Keyword: "throw", Handler: ThrowHandler, Brief: "Hurl your wielded thrown weapon at a target.", Syntax: []string{"throw <target>"},
			HandParsed: true, BreaksConcealment: true, Args: []ArgDefinition{{Name: "target", Type: ArgEntity}}},
		{Keyword: "shoot", Aliases: []string{"fire"}, Handler: ShootHandler, Brief: "Loose a projectile at a target in an adjacent room.", Syntax: []string{"shoot <target> <direction>"},
			HandParsed: true, BreaksConcealment: true},
		{Keyword: "advance", Handler: AdvanceHandler, Brief: "Close one range band toward melee.", Syntax: []string{"advance"}, BreaksConcealment: true},
		{Keyword: "withdraw", Handler: WithdrawHandler, Brief: "Open one range band (kite), staying in the room.", Syntax: []string{"withdraw"}, BreaksConcealment: true},
		{Keyword: "flee", Handler: FleeHandler, Brief: "Try to escape from combat.", Syntax: []string{"flee"}},
		{Keyword: "wimpy", Handler: WimpyHandler, Brief: "Auto-flee when your health drops below a percent.", Syntax: []string{"wimpy <percent>"}},
		{Keyword: "powerattack", Handler: PowerAttackHandler, Brief: "Toggle the Power Attack stance: trade accuracy for melee damage.", Syntax: []string{"powerattack <on|off>"}},

		// Progression (M8.6).
		{Keyword: "train", Handler: TrainHandler, Brief: "Spend a train credit to raise a stat.", Syntax: []string{"train <stat>"}},
		{Keyword: "practice", Handler: PracticeHandler, Brief: "Raise an ability's cap at a trainer.", Syntax: []string{"practice <ability>"}},
		{Keyword: "feats", Handler: FeatsHandler, Brief: "List your feats and available feat slots.", Syntax: []string{"feats"}},
		{Keyword: "languages", Aliases: []string{"langs"}, Handler: LanguagesHandler, Brief: "List the languages you speak.", Syntax: []string{"languages"}},
		{Keyword: "feat", Handler: FeatHandler, Brief: "Spend a feat slot to take a feat.", Syntax: []string{"feat <name> [target]"}},
		{Keyword: "learn", Handler: LearnHandler, Brief: "Learn a crafting discipline from a trainer.", Syntax: []string{"learn <discipline>"}},
		{Keyword: "craft", Handler: CraftHandler, Brief: "Craft an item from a known recipe.", Syntax: []string{"craft", "craft <recipe>"}},
		{Keyword: "build", Handler: BuildHandler, Brief: "Build an improvised crafting station (e.g. a campfire).", Syntax: []string{"build <thing>"}},

		// Abilities (M9.6).
		{Keyword: "abilities", Aliases: []string{"abi"}, Handler: AbilitiesHandler, Brief: "List the abilities you have learned.", Syntax: []string{"abilities"}},
		{Keyword: "standing", Aliases: []string{"factions", "reputation"}, Handler: StandingHandler, Brief: "Show your standing with each faction.", Syntax: []string{"standing"}},
		{Keyword: "skills", Handler: SkillsHandler, Brief: "List your skills and their proficiency.", Syntax: []string{"skills"}},
		// BreaksConcealment: channeling the One Power / casting is dramatic and
		// gives a hidden caster away (visibility §4.5). v1 reveals on ANY cast;
		// restricting it to offensive weaves only is a later refinement needing
		// per-ability offensive metadata.
		{Keyword: "cast", Aliases: []string{"channel"}, Handler: CastHandler, Brief: "Use an ability by name (channel a weave).", Syntax: []string{"cast <ability>", "cast <ability> <target>", "channel <weave>", "channel <weave> <target>"}, BreaksConcealment: true},
		// overchannel is a (riskier) cast, so it reveals a hidden channeler
		// exactly like `cast`/`channel` (visibility §4.5).
		{Keyword: "overchannel", Handler: OverchannelHandler, Brief: "Draw a weave past your safe reserve, at real risk.", Syntax: []string{"overchannel <weave>", "overchannel <weave> <target>"}, BreaksConcealment: true},

		// Help (M10.5).
		{Keyword: "help", Handler: HelpHandler, Brief: "Find help on commands and topics.", Syntax: []string{"help", "help <topic>"}, Category: "general"},

		// Quests (M10.10).
		// talk declares an npc arg + HandParsed so completion enumerates
		// room NPCs (quest givers); the handler resolves the raw term.
		{Keyword: "talk", Handler: TalkHandler, Brief: "Talk to a quest giver to hear offers or turn in a quest.", Syntax: []string{"talk <npc>"},
			HandParsed: true, Args: []ArgDefinition{{Name: "npc", Type: ArgNPC}}},
		// ask is the free-form dialogue verb: `ask <npc> about <topic>` speaks
		// the NPC's content-authored dialogue line (lore/rumours/hints). With
		// no "about <topic>" it falls through to the quest-giver behavior, so
		// `ask <npc>` remains a synonym for `talk <npc>`. Same npc arg +
		// HandParsed shape as talk so completion enumerates room NPCs.
		{Keyword: "ask", Handler: AskHandler, Brief: "Ask an NPC about a topic (or talk to a quest giver).", Syntax: []string{"ask <npc> about <topic>", "ask <npc>"},
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

		// Mounts (mounts.md §3/§9). buymount/stable/unstable require a
		// stablemaster in the room; mounts lists what you own anywhere.
		{Keyword: "mounts", Handler: MountsHandler, Brief: "List the mounts you own.", Syntax: []string{"mounts"}},
		{Keyword: "mount", Handler: MountHandler, Brief: "Mount a creature you own.", Syntax: []string{"mount <mount>"}, BreaksConcealment: true},
		{Keyword: "dismount", Handler: DismountHandler, Brief: "Dismount the creature you're riding.", Syntax: []string{"dismount"}},
		{Keyword: "buymount", Handler: BuyMountHandler, Brief: "Buy a mount from a stablemaster.", Syntax: []string{"buymount <mount>"}},
		{Keyword: "stable", Handler: StableHandler, Brief: "Stable a mount at a stablemaster.", Syntax: []string{"stable [<mount>]"}},
		{Keyword: "unstable", Aliases: []string{"retrieve"}, Handler: UnstableHandler, Brief: "Retrieve a stabled mount.", Syntax: []string{"unstable <mount>"}},
		{Keyword: "hire", Handler: HireHandler, Brief: "Hire a companion to follow and fight for you.", Syntax: []string{"hire <name>"}, HandParsed: true},
		{Keyword: "dismiss", Handler: DismissHandler, Brief: "Dismiss a hireling you've hired.", Syntax: []string{"dismiss <name|number>"}, HandParsed: true},
		{Keyword: "hirelings", Handler: HirelingsHandler, Brief: "List the hirelings under your contract.", Syntax: []string{"hirelings"}},
		{Keyword: "order", Handler: OrderHandler, Brief: "Order a hireling: follow, stay, guard, or attack <target>.", Syntax: []string{"order <name|number|all> follow|stay|guard", "order <name|number|all> attack <target>"}, HandParsed: true},

		// Direct trade (direct-trade.md). A same-room two-player swap:
		// `trade <player>` initiates and (symmetrically) accepts; offer/
		// rescind build each side's offer; confirm fires the atomic swap
		// only when both confirm an unchanged pair; decline aborts.
		{Keyword: "trade", Handler: TradeHandler, Brief: "Begin or accept a trade with another player.", Syntax: []string{"trade <player>"}, BreaksConcealment: true, Args: []ArgDefinition{{Name: "target", Type: ArgPlayer}}},
		{Keyword: "offer", Handler: OfferItemHandler, Brief: "Add an item to your trade offer.", Syntax: []string{"offer <item>"}, Args: []ArgDefinition{{Name: "item", Type: ArgInventory}}},
		{Keyword: "offergold", Aliases: []string{"offercoin"}, Handler: OfferGoldHandler, Brief: "Add gold to your trade offer.", Syntax: []string{"offergold <amount>"}, Args: []ArgDefinition{{Name: "amount", Type: ArgNumber}}},
		{Keyword: "rescind", Aliases: []string{"unoffer"}, Handler: RescindItemHandler, Brief: "Remove an item from your trade offer.", Syntax: []string{"rescind <item>"}, Args: []ArgDefinition{{Name: "item", Type: ArgText}}},
		{Keyword: "rescindgold", Aliases: []string{"rescindcoin"}, Handler: RescindGoldHandler, Brief: "Remove gold from your trade offer.", Syntax: []string{"rescindgold <amount>"}, Args: []ArgDefinition{{Name: "amount", Type: ArgNumber}}},
		{Keyword: "confirm", Handler: ConfirmHandler, Brief: "Confirm the current trade offers.", Syntax: []string{"confirm"}},
		{Keyword: "decline", Aliases: []string{"untrade"}, Handler: DeclineHandler, Brief: "Cancel the current trade.", Syntax: []string{"decline"}},

		// Auction house (auction-house.md). `auction <item> <price>` lists an
		// item at an auctioneer for buyout; `auctions` shows your own active
		// listings; `unlist <#>` withdraws one. browse/buyout/collect land in
		// later slices.
		{Keyword: "auction", Handler: AuctionHandler, Brief: "List an item for auction at an auctioneer.", Syntax: []string{"auction <item> <price>"}, Args: []ArgDefinition{{Name: "item", Type: ArgInventory}, {Name: "price", Type: ArgNumber}}},
		{Keyword: "auctions", Handler: AuctionsHandler, Brief: "Show your own active auction listings.", Syntax: []string{"auctions"}},
		{Keyword: "unlist", Handler: UnlistHandler, Brief: "Withdraw one of your auction listings.", Syntax: []string{"unlist <#>"}, Args: []ArgDefinition{{Name: "ref", Type: ArgText}}},
		{Keyword: "browse", Handler: BrowseHandler, Brief: "Browse the auction listings (filter by name).", Syntax: []string{"browse [name] [price|time] [page]"}},
		{Keyword: "buyout", Handler: BuyoutHandler, Brief: "Buy an auction listing outright.", Syntax: []string{"buyout <#>"}, Args: []ArgDefinition{{Name: "ref", Type: ArgText}}},
		{Keyword: "collect", Handler: CollectHandler, Brief: "Collect auction proceeds and won/returned items.", Syntax: []string{"collect"}},
		{Keyword: "auctionremove", Handler: AuctionRemoveHandler, Admin: true, Brief: "Remove an auction listing (admin).", Syntax: []string{"auctionremove <#>"}, Args: []ArgDefinition{{Name: "ref", Type: ArgText}}},
		{Keyword: "auctionrefund", Handler: AuctionRefundHandler, Admin: true, Brief: "Reverse an auction sale (admin).", Syntax: []string{"auctionrefund <#>"}, Args: []ArgDefinition{{Name: "ref", Type: ArgText}}},

		{Keyword: "affects", Aliases: []string{"effects"}, Handler: AffectsHandler, Brief: "List your active effects and conditions.", Syntax: []string{"affects"}},
		{Keyword: "rest", Handler: RestHandler, Brief: "Rest to recover faster.", Syntax: []string{"rest"}},
		{Keyword: "sleep", Handler: SleepHandler, Brief: "Sleep to recover fastest.", Syntax: []string{"sleep"}},
		{Keyword: "wake", Aliases: []string{"stand"}, Handler: WakeHandler, Brief: "Stop resting or sleeping.", Syntax: []string{"wake"}},
		{Keyword: "eat", Handler: EatHandler, Brief: "Eat food to restore sustenance.", Syntax: []string{"eat <food>"}, Args: []ArgDefinition{{Name: "item", Type: ArgInventory}}},
		{Keyword: "drink", Handler: DrinkHandler, Brief: "Drink to restore sustenance.", Syntax: []string{"drink <item>"}, Args: []ArgDefinition{{Name: "item", Type: ArgInventory}}},
		{Keyword: "use", Handler: UseHandler, Brief: "Use a consumable item.", Syntax: []string{"use <item>"}, Args: []ArgDefinition{{Name: "item", Type: ArgInventory}}},
		{Keyword: "read", Handler: ReadHandler, Brief: "Read a recipe scroll to learn it.", Syntax: []string{"read <item>"}, Args: []ArgDefinition{{Name: "item", Type: ArgInventory}}},
		{Keyword: "forage", Aliases: []string{"gather"}, Handler: ForageHandler, Brief: "Forage the area for ingredients.", Syntax: []string{"forage"}},
		{Keyword: "harvest", Handler: HarvestHandler, Brief: "Harvest a resource node.", Syntax: []string{"harvest <node>"}},

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
		{Keyword: "open", Handler: OpenHandler, Brief: "Open a door.", Syntax: []string{"open <direction>", "open <door>"}, BreaksConcealment: true, Args: []ArgDefinition{{Name: "door", Type: ArgDoor}}},
		{Keyword: "close", Aliases: []string{"shut"}, Handler: CloseHandler, Brief: "Close a door.", Syntax: []string{"close <direction>", "close <door>"}, BreaksConcealment: true, Args: []ArgDefinition{{Name: "door", Type: ArgDoor}}},
		{Keyword: "lock", Handler: LockHandler, Brief: "Lock a door (requires the key).", Syntax: []string{"lock <direction>", "lock <door>"}, Args: []ArgDefinition{{Name: "door", Type: ArgDoor}}},
		{Keyword: "unlock", Handler: UnlockHandler, Brief: "Unlock a door (requires the key).", Syntax: []string{"unlock <direction>", "unlock <door>"}, Args: []ArgDefinition{{Name: "door", Type: ArgDoor}}},
		{Keyword: "pick", Aliases: []string{"picklock"}, Handler: PickHandler, Brief: "Pick a lock with the Open Lock skill (no key needed).", Syntax: []string{"pick <direction>", "pick <door>"}, Args: []ArgDefinition{{Name: "door", Type: ArgDoor}}},

		// Recall (M15.3). `recall set` binds the current room as the
		// character's return point; `recall` teleports back to it. (The
		// binding verb moved from the former `set recall` to `recall set` in
		// M19.4c when admin `set` reclaimed the top-level `set` keyword.)
		// Spec: docs/specs/recall.md.
		{Keyword: "recall", Handler: RecallHandler, Brief: "Return to your bound recall point.", Syntax: []string{"recall", "recall set"}},

		// Hide / reveal (visibility.md §3.1): conceal the actor in its
		// current room; emerge voluntarily. Discovery by others is the
		// per-observer perception contest (visibility §4).
		{Keyword: "hide", Handler: HideHandler, Brief: "Conceal yourself in the current room.", Syntax: []string{"hide"}},
		{Keyword: "unhide", Aliases: []string{"reveal"}, Handler: RevealHandler, Brief: "Step out of hiding.", Syntax: []string{"unhide"}},
		// Sneak (visibility §3.2): toggle moving concealment — your enter/leave
		// lines reach only occupants who pierce a perception contest. Survives
		// room changes (unlike hide); dropped by a revealing action (§4.5).
		{Keyword: "sneak", Handler: SneakHandler, Brief: "Toggle moving quietly between rooms.", Syntax: []string{"sneak"}},
		// Search (visibility §4.4 / hidden-exits §3.1): a deliberate scan of
		// the room that runs a perception contest against hidden exits and can
		// reveal a secret passage. BreaksConcealment: rummaging around is not a
		// quiet action — it gives away a hidden searcher.
		{Keyword: "search", Handler: SearchHandler, Brief: "Search the room for hidden exits.", Syntax: []string{"search"}, BreaksConcealment: true},

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
		{Keyword: "reloadscripts", Handler: ReloadScriptsHandler, Admin: true, Brief: "Reload pack scripts (admin).", Syntax: []string{"reloadscripts"}},
		// wizinvis (visibility §3.4): toggle admin invisibility — staff walk
		// unseen by lower ranks (render, target resolution, who). Flag-gated,
		// does not break on action.
		{Keyword: "wizinvis", Aliases: []string{"invis"}, Handler: WizinvisHandler, Admin: true, Brief: "Toggle admin invisibility.", Syntax: []string{"wizinvis"}},

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
		{Keyword: "showspawns", Handler: ShowSpawnsHandler, Admin: true, Brief: "Toggle seeing other players' quest spawns (admin/builder).", Syntax: []string{"showspawns", "showspawns on", "showspawns off"}},

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

		// afflict / cure (conditions §5 — EPIC S5): the admin inflict path for
		// status conditions. afflict applies a condition effect by force (no
		// entry save); cure clears one or all conditions.
		{Keyword: "afflict", Handler: AfflictHandler, Admin: true, Brief: "Inflict a status condition on a target.", Syntax: []string{"afflict <target> <condition> [duration]"}},
		{Keyword: "cure", Handler: CureHandler, Admin: true, Brief: "Clear a status condition from a target.", Syntax: []string{"cure <target> [condition]"}},

		// teleport (M19.4d — admin-verbs §5, alias `goto`): move the actor to
		// a room by id or to the room of an online player (§3 world-scoped
		// resolution). Reuses SetRoom's room-change events.
		{Keyword: "teleport", Aliases: []string{"goto"}, Handler: TeleportHandler, Admin: true, Brief: "Teleport to a room or player.", Syntax: []string{"teleport <room-id>", "teleport <player>"}},

		// purge (M19.4e — admin-verbs §5): remove a non-player entity (mob
		// or room item) from the world, untracking it. Never targets a
		// player. Removal mirrors the death-cleanup path; audited.
		{Keyword: "purge", Handler: PurgeHandler, Admin: true, Brief: "Remove a mob or item from the world.", Syntax: []string{"purge <target>"}},
		{Keyword: "spawn", Handler: SpawnHandler, Admin: true, Brief: "Spawn an item, mob, or gold into the world (admin/builder).", Syntax: []string{"spawn item <id> [here|me]", "spawn mob <id>", "spawn gold <amount>"}},
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
		return c.writeRoomView(ctx, room, c.effectiveLight(room))
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
		// Knowledge-gated traversal (hidden-exits §4.1): an undiscovered hidden
		// exit is non-existent for this player — typing its direction fails
		// exactly like there is no exit (same message, so it is indistinguishable
		// from a wall). The move primitive stays unconditional (§4.1); the gate
		// lives here, in the player-volition command, so mob AI / scripted /
		// admin moves through the primitive are not blocked by player discovery.
		if e, ok := room.Exits[dir]; ok && e.Hidden && !c.canSeeExit(dir, e) {
			return c.Actor.Write(ctx, "You cannot go that way.")
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
		// Movement-cost gate (world-rooms-movement §3.3): walking spends
		// movement points. The move primitive stays unconditional on
		// resource availability — the spend lives here, in the
		// player-volition command, so mob AI / flee / scripted / admin
		// moves through the primitive never pay. Checked after the other
		// "can I take this step" gates (hidden exit, door, darkness) and
		// before any side effect, so an insufficient pool aborts cleanly.
		// Cost is the destination biome's weight (rough terrain costs more),
		// falling back to the configured flat default.
		// Mounted travel (mounts.md §5): while RIDDEN the MOUNT is the metered
		// mover — the step spends the mount's travel pool, not the rider's
		// movement, and some terrain a mount cannot enter at all (§5.3). The
		// mount bears the load, so the rider's own encumbrance/armor surcharge
		// does NOT apply. On foot it's the rider's movement pool plus that
		// surcharge. Either way the move primitive stays unconditional; the
		// spend lives here in the volition verb.
		var allowed, charged bool
		// NOTE: mountedSteed clears a stale ride pointer as a side effect (lazy
		// never-strand) — call it once and reuse the result.
		if steed := mountedSteed(c); steed != nil {
			if mountBlockedBy(c, dst, steed) {
				return c.Actor.Write(ctx, mountImpassableText)
			}
			allowed, charged = spendMountTravel(steed, terrainStepCost(c, dst))
			if !allowed {
				return c.Actor.Write(ctx, mountBlownText)
			}
		} else {
			// The mover's surcharges (encumbrance + armor speed) depend only on
			// the mover, not the room, so they add equally to every step.
			moverSurcharge := c.encumbranceSurcharge() + c.armorSpeedSurcharge()
			allowed, charged = spendMovement(c, terrainStepCost(c, dst)+moverSurcharge)
			if !allowed {
				return c.Actor.Write(ctx, tooWindedText)
			}
		}
		// Terrain-difficulty hint: surfaced only when the step actually cost
		// the mover extra — they were charged (so an unmetered/free mover stays
		// silent) AND the destination is rougher than the room just left. Fired
		// once on the transition; walking within one terrain stays quiet. The
		// mover surcharge cancels by construction (it is identical for source and
		// destination), so the hint compares TERRAIN only — purely room-driven.
		harderGoing := charged && terrainStepCost(c, dst) > terrainStepCost(c, room)
		srcID := room.ID
		name := c.Actor.Name()
		pid := c.Actor.PlayerID()
		// Announce departure to the source room before the actor
		// leaves so other occupants there see it. Broadcaster is
		// optional (tests pass nil); skip the announcement when name
		// or PlayerID is empty (test actors that don't participate in
		// presence).
		//
		// Sneak filter (visibility §3.2): when the mover is sneaking, the
		// enter/leave lines reach only occupants who pierce the sneak in a
		// fresh perception contest — those who fail are added to the
		// exclusion list alongside the mover itself, so they see nothing.
		// sneakUnseenBy returns nil for a non-sneaking mover, preserving the
		// legacy "everyone sees the line" path.
		if c.Broadcaster != nil && name != "" {
			depExcl := append([]string{pid}, sneakUnseenBy(c, srcID, c.Actor)...)
			c.Broadcaster.SendToRoom(ctx, srcID,
				fmt.Sprintf("%s heads %s.", name, dir.Long()), depExcl...)
		}
		c.Actor.SetRoom(dst)
		if c.Broadcaster != nil && name != "" {
			from := dir.Opposite().Long()
			if from == "" {
				from = "elsewhere"
			}
			arrExcl := append([]string{pid}, sneakUnseenBy(c, dst.ID, c.Actor)...)
			c.Broadcaster.SendToRoom(ctx, dst.ID,
				fmt.Sprintf("%s arrives from the %s.", name, from), arrExcl...)
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
		if err := c.writeRoomView(ctx, dst, dstLvl); err != nil {
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
		// Below the room text, flag the rougher terrain (see harderGoing).
		if harderGoing {
			_ = c.Actor.Write(ctx, goingHardText)
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

// fallbackMoveCost is the last-resort movement-point cost of a step when
// neither the destination biome nor the Context configures one (e.g. test
// paths with no biome registry and no DefaultMoveCost). Live boots set
// Context.DefaultMoveCost from ANOTHERMUD_MOVE_COST (default 1), so this
// only governs bare fixtures.
const fallbackMoveCost = 1

// tooWindedText is the refusal when the mover lacks the movement points
// for a step.
const tooWindedText = "You are too winded to go that way. Catch your breath."

// goingHardText is the subtle line shown when a step crosses onto terrain
// that costs more movement than the ground just left (rough country).
const goingHardText = "<subtle>The going is hard here.</subtle>"

// movementCostSubject is the optional view the movement-cost gate needs.
// The live connActor satisfies it; bare test actors do not, so the gate
// is a no-op for them — keeping movement-only command tests untouched.
type movementCostSubject interface {
	Movement() int
	MovementMax() int
	DeductMovement(int)
}

// terrainStepCost is the destination terrain's contribution to a step. The
// destination biome's MoveCost wins when it sets one (rough terrain costs
// more, world-rooms-movement §3.3); otherwise the Context's configured
// flat default applies, and fallbackMoveCost backstops a Context that
// configures none (bare fixtures).
func terrainStepCost(c *Context, dst *world.Room) int {
	if c.Biomes != nil && dst != nil {
		if b, ok := c.Biomes.Resolve(dst.Terrain); ok && b.MoveCost > 0 {
			return b.MoveCost
		}
	}
	if c.DefaultMoveCost > 0 {
		return c.DefaultMoveCost
	}
	return fallbackMoveCost
}

// spendMovement charges the actor cost points for one step. It reports
// whether the step may proceed (allowed) and whether the cost was actually
// deducted (charged). charged is false for an unmetered mover — no movement
// pool (mobs / test actors) or a cost above the pool's capacity — both of
// which move for free and must never be stranded. A step blocked by an
// insufficient pool returns (false, false).
func spendMovement(c *Context, cost int) (allowed, charged bool) {
	mc, ok := c.Actor.(movementCostSubject)
	if !ok {
		return true, false
	}
	if cost <= 0 || mc.MovementMax() < cost {
		return true, false
	}
	if mc.Movement() < cost {
		return false, false
	}
	mc.DeductMovement(cost)
	return true, true
}
