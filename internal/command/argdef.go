package command

// ArgType is the canonical name of an argument type — engine-known
// (keyword, text, number, …) or pack-registered. Stored as a
// distinct string type so a typo in a registration call is caught
// at the call site rather than at resolve time.
//
// Spec: docs/specs/commands-and-dispatch.md §5.2.
type ArgType string

// Engine-baseline arg type names. Packs MAY add new names via the
// resolver registry (§5.3) but MUST NOT override these — every
// ArgResolverRegistry rejects collisions with these constants.
//
// M17.2a ships ArgKeyword / ArgText / ArgNumber. The entity /
// inventory / room family lands in M17.2b once the resolver
// reaches into actor + room state.
const (
	ArgKeyword   ArgType = "keyword"
	ArgText      ArgType = "text"
	ArgNumber    ArgType = "number"
	ArgInventory ArgType = "inventory"
	ArgRoomItem  ArgType = "room_item"
	ArgEntity    ArgType = "entity"
	ArgPlayer    ArgType = "player"
	ArgNPC       ArgType = "npc"
	ArgContainer ArgType = "container"
	ArgVisible   ArgType = "visible"
	ArgFindable  ArgType = "findable"
	ArgDoor      ArgType = "door"
	// ArgQuest enumerates the quests offered to the actor by NPCs in the
	// current room (the OffersFrom set `talk` shows) for completion. Used
	// by `accept`, which stays HandParsed — the type exists for the
	// completion enumerator, not a §5 resolver; the handler resolves the
	// term via quest.Service.ResolveID. The completion token is the bare
	// quest id, which round-trips through ResolveID.
	ArgQuest ArgType = "quest"
	// ArgActiveQuest is the abandon-side counterpart to ArgQuest: it
	// enumerates the actor's ACTIVE, abandonable quests (what `abandon`
	// will accept) rather than the room's offers, and isn't giver-bound.
	// Same HandParsed + bare-id round-trip contract; the distinct type
	// just selects the other enumerator.
	ArgActiveQuest ArgType = "active_quest"
	// ArgEquipped enumerates the actor's currently-equipped (worn/wielded)
	// items for completion — the scope `unequip` resolves against. Item-
	// flavored (reuses the keyword disambiguation the inventory/room types
	// use); the completion token is a distinguishing keyword that
	// round-trips through unequip's keyword resolver.
	ArgEquipped ArgType = "equipped"
)

// engineArgTypes is the immutable set of engine-baseline type
// names. Registry registration rejects an entry whose name matches
// any of these regardless of which resolver implementation a pack
// supplies. Spec §5.3 makes engine types immutable to prevent a
// pack from silently changing built-in command behavior.
var engineArgTypes = map[ArgType]struct{}{
	ArgKeyword:     {},
	ArgText:        {},
	ArgNumber:      {},
	ArgInventory:   {},
	ArgRoomItem:    {},
	ArgEntity:      {},
	ArgPlayer:      {},
	ArgNPC:         {},
	ArgContainer:   {},
	ArgVisible:     {},
	ArgFindable:    {},
	ArgDoor:        {},
	ArgQuest:       {},
	ArgActiveQuest: {},
	ArgEquipped:    {},
}

// IsEngineArgType reports whether name is one of the engine-
// baseline arg types — useful for pack-side validation that
// surfaces "this type collides with an engine type" diagnostics
// at registration time rather than at first use.
func IsEngineArgType(name ArgType) bool {
	_, ok := engineArgTypes[name]
	return ok
}

// ArgDefinition declares one positional argument's expected type,
// required-ness, and surrounding prepositional sugar.
//
// Spec: §5.1. The spec defaults Required to true; idiomatic Go
// can't express "default true" via a bool zero-value, so the
// struct exposes Optional (zero-value = false = required) instead.
type ArgDefinition struct {
	// Name keys the resolved value in the final map the handler
	// receives. By convention lowercase snake_case; the resolver
	// does not enforce a shape — `Name` is just a map key.
	Name string

	// Type is the engine or pack-registered type. Unknown types
	// fall back to passthrough (treated as ArgKeyword) per §5.3
	// with a warning at resolve time.
	Type ArgType

	// Optional flips the §5.1 default. Zero-value (false) means
	// the arg is REQUIRED — a missing required arg short-
	// circuits the whole resolve with "What <name>?". Set
	// Optional=true to allow a nil entry in the result map when
	// no token matches.
	Optional bool

	// Bulk enables the `all` / `all.<keyword>` syntax for the
	// inventory- and room-item-style types. Meaningless for
	// keyword / text / number (the resolver ignores it).
	Bulk bool

	// Prepositions lists tokens the resolver silently consumes
	// when seen IMMEDIATELY BEFORE this argument's token. Lower-
	// case at registration so the resolver's case-insensitive
	// match is one allocation cheaper per resolve.
	//
	// Example: `put <gem> in <chest>` → the `in` arg's
	// prepositions = ["in"] so the player can naturally type
	// "put gem in chest".
	Prepositions []string

	// BypassVisibility suppresses the resolver's visibility
	// filter for this argument. Used for verbs that intentionally
	// target hidden entities ("look at <fixture>" matches a
	// fixture the player can't normally see).
	BypassVisibility bool
}
