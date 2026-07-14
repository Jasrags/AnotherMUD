package command

import "sort"

// Command categorization for the help index (ui-rendering-help §9.7). The
// Command struct carries a free-form Category string, but assigning it inline
// on all ~146 builtin literals would scatter the taxonomy across the file and
// invite drift. Instead the mapping lives here as one reviewable table, applied
// in RegisterBuiltins after the slice is built. A keyword absent from the map
// keeps whatever its registration set (or the "commands" default), and an admin
// verb with no explicit mapping falls back to the admin group — so a newly
// added, uncategorized verb lands in a visible "needs a home" bucket rather than
// silently vanishing (TestBuiltinsAreCategorized enforces this).

// Category keys. These are the stable identifiers used for `help <category>`
// drill-in and the ordered index; the human-facing titles live in categoryOrder.
const (
	catGeneral       = "general"
	catMovement      = "movement"
	catCommunication = "communication"
	catCombat        = "combat"
	catItems         = "items"
	catCharacter     = "character"
	catSurvival      = "survival"
	catCrafting      = "crafting"
	catEconomy       = "economy"
	catTrade         = "trade"
	catGroups        = "groups"
	catStealth       = "stealth"
	catQuests        = "quests"
	catAdmin         = "admin"
)

// categoryMeta pairs a category key with its display title.
type categoryMeta struct {
	Key   string
	Title string
}

// categoryOrder is the canonical display order and titles for the help index.
// The bare `help` screen walks this list, so the order here is the order a
// player sees. Categories not in this list (e.g. leftover "commands" from
// dynamically-registered ability verbs, or pack-defined categories) render
// after these, alphabetically, so nothing is ever hidden.
var categoryOrder = []categoryMeta{
	{catGeneral, "General"},
	{catMovement, "Movement & Travel"},
	{catCommunication, "Communication"},
	{catCombat, "Combat"},
	{catItems, "Items & Inventory"},
	{catCharacter, "Character & Progression"},
	{catSurvival, "Rest & Survival"},
	{catCrafting, "Crafting & Gathering"},
	{catEconomy, "Shops & Money"},
	{catTrade, "Trade & Auction"},
	{catGroups, "Groups & Companions"},
	{catStealth, "Stealth & Doors"},
	{catQuests, "Quests"},
	{catAdmin, "Admin"},
}

// categoryTitle returns the display title for a category key, falling back to a
// capitalized form of the key for categories outside categoryOrder.
func categoryTitle(key string) string {
	for _, m := range categoryOrder {
		if m.Key == key {
			return m.Title
		}
	}
	return capitalizeKey(key)
}

// capitalizeKey title-cases a bare category key for display when it isn't in
// categoryOrder (e.g. "commands" → "Commands").
func capitalizeKey(key string) string {
	if key == "" {
		return key
	}
	r := []rune(key)
	if r[0] >= 'a' && r[0] <= 'z' {
		r[0] -= 'a' - 'A'
	}
	return string(r)
}

// commandCategories maps a builtin keyword to its help category. Admin verbs are
// intentionally omitted — they default to catAdmin in RegisterBuiltins — so this
// table only enumerates the player-facing taxonomy.
var commandCategories = map[string]string{
	// General — orientation, self, and client settings.
	"help":        catGeneral,
	"look":        catGeneral,
	"score":       catGeneral,
	"who":         catGeneral,
	"quit":        catGeneral,
	"stop":        catGeneral,
	"prompt":      catGeneral,
	"color":       catGeneral,
	"suggest":     catGeneral,
	"tabcomplete": catGeneral,
	"daylight":    catGeneral,

	// Movement & Travel.
	"map":      catMovement,
	"minimap":  catMovement,
	"recall":   catMovement,
	"follow":   catMovement,
	"unfollow": catMovement,
	"lose":     catMovement,
	"mount":    catMovement,
	"dismount": catMovement,
	"mounts":   catMovement,
	"buymount": catMovement,
	"stable":   catMovement,
	"unstable": catMovement,

	// Communication.
	"tell":        catCommunication,
	"reply":       catCommunication,
	"tells":       catCommunication,
	"emote":       catCommunication,
	"channels":    catCommunication,
	"chathistory": catCommunication,
	"gtell":       catCommunication,

	// Combat.
	"kill":        catCombat,
	"consider":    catCombat,
	"assist":      catCombat,
	"autoassist":  catCombat,
	"throw":       catCombat,
	"shoot":       catCombat,
	"advance":     catCombat,
	"withdraw":    catCombat,
	"flee":        catCombat,
	"wimpy":       catCombat,
	"powerattack": catCombat,
	"cast":        catCombat,
	"overchannel": catCombat,

	// Items & Inventory.
	"get":        catItems,
	"drop":       catItems,
	"give":       catItems,
	"put":        catItems,
	"fill":       catItems,
	"loot":       catItems,
	"lootmode":   catItems,
	"autoloot":   catItems,
	"inventory":  catItems,
	"equipment":  catItems,
	"equip":      catItems,
	"unequip":    catItems,
	"hastydon":   catItems,
	"light":      catItems,
	"extinguish": catItems,
	"use":        catItems,
	"read":       catItems,
	"load":       catItems,
	"reload":     catItems,
	"autoreload": catItems,
	"modify":     catItems,
	"unmodify":   catItems,

	// Character & Progression.
	"abilities": catCharacter,
	"skills":    catCharacter,
	"feats":     catCharacter,
	"feat":      catCharacter,
	"affects":   catCharacter,
	"standing":  catCharacter,
	"languages": catCharacter,
	"train":     catCharacter,
	"practice":  catCharacter,
	"learn":     catCharacter,

	// Rest & Survival.
	"rest":  catSurvival,
	"sleep": catSurvival,
	"wake":  catSurvival,
	"eat":   catSurvival,
	"drink": catSurvival,

	// Crafting & Gathering.
	"craft":   catCrafting,
	"build":   catCrafting,
	"forage":  catCrafting,
	"harvest": catCrafting,

	// Shops & Money.
	"buy":   catEconomy,
	"sell":  catEconomy,
	"value": catEconomy,
	"list":  catEconomy,
	"gold":  catEconomy,

	// Trade & Auction.
	"trade":       catTrade,
	"offer":       catTrade,
	"offergold":   catTrade,
	"rescind":     catTrade,
	"rescindgold": catTrade,
	"confirm":     catTrade,
	"decline":     catTrade,
	"auction":     catTrade,
	"auctions":    catTrade,
	"unlist":      catTrade,
	"browse":      catTrade,
	"buyout":      catTrade,
	"collect":     catTrade,

	// Groups & Companions.
	"group":     catGroups,
	"join":      catGroups,
	"leave":     catGroups,
	"disband":   catGroups,
	"promote":   catGroups,
	"hire":      catGroups,
	"dismiss":   catGroups,
	"hirelings": catGroups,
	"order":     catGroups,
	"shoo":      catGroups,

	// Stealth & Doors.
	"hide":   catStealth,
	"unhide": catStealth,
	"sneak":  catStealth,
	"search": catStealth,
	"open":   catStealth,
	"close":  catStealth,
	"lock":   catStealth,
	"unlock": catStealth,
	"pick":   catStealth,

	// Quests.
	"quests":  catQuests,
	"talk":    catQuests,
	"ask":     catQuests,
	"accept":  catQuests,
	"abandon": catQuests,
}

// CatalogCommand is one discoverable command in the grouped command catalog —
// the menu-facing view a rich client (GMCP Char.Commands) renders as a clickable
// entry. Syntax is the primary usage line (synthesized from typed args, else the
// first hand-authored syntax).
type CatalogCommand struct {
	Keyword string
	Brief   string
	Syntax  string
}

// CatalogCategory groups CatalogCommands under a display category, in the same
// canonical order and titles the help index uses.
type CatalogCategory struct {
	Key      string
	Title    string
	Commands []CatalogCommand
}

// Catalog returns the discoverable commands grouped by category in the canonical
// help order (categoryOrder), for pushing to rich clients or any menu UI. It
// mirrors the bare-help index's grouping and role gate: admin commands (and the
// admin category) are included only when includeAdmin is true, empty categories
// are omitted, and categories outside categoryOrder render after the canonical
// ones, alphabetically, so nothing is dropped. Commands()' bare-Register /
// alias exclusions mean ability verbs and un-metadata'd verbs never appear.
func (r *Registry) Catalog(includeAdmin bool) []CatalogCategory {
	byCat := make(map[string][]CatalogCommand)
	for _, ci := range r.Commands() {
		if ci.Admin && !includeAdmin {
			continue
		}
		byCat[ci.Category] = append(byCat[ci.Category], catalogCommand(ci))
	}

	out := make([]CatalogCategory, 0, len(byCat))
	seen := make(map[string]bool, len(categoryOrder))
	appendCat := func(key, title string) {
		list := byCat[key]
		if len(list) == 0 {
			return
		}
		out = append(out, CatalogCategory{Key: key, Title: title, Commands: list})
	}
	for _, m := range categoryOrder {
		seen[m.Key] = true
		appendCat(m.Key, m.Title)
	}
	leftover := make([]string, 0)
	for key := range byCat {
		if !seen[key] {
			leftover = append(leftover, key)
		}
	}
	sort.Strings(leftover)
	for _, key := range leftover {
		appendCat(key, categoryTitle(key))
	}
	return out
}

// catalogCommand projects a CommandInfo into its catalog view, choosing the
// primary syntax line the same way help generation does (synthesized from typed
// args when present, else the first hand-authored syntax line).
func catalogCommand(ci CommandInfo) CatalogCommand {
	syntax := ""
	if len(ci.Args) > 0 {
		syntax = synthesizeSyntax(ci.Keyword, ci.Args)
	} else if len(ci.Syntax) > 0 {
		syntax = ci.Syntax[0]
	}
	return CatalogCommand{Keyword: ci.Keyword, Brief: ci.Brief, Syntax: syntax}
}

// categoryFor resolves the help category for a builtin command: the explicit
// mapping wins, then an admin verb that is ALREADY discoverable falls back to
// the admin group, and anything else keeps its registration's own Category.
//
// The admin fallback is deliberately gated on hasListingMetadata. Setting
// Category is itself metadata that makes RegisterCommand synthesize a help topic
// (the metadata gate in registry.go), so an unconditional admin fallback would
// pull a bare admin verb — keyword+handler only, meant to stay an undiscoverable
// debug probe — into the Admin grid just by acquiring a category. Gating on
// existing metadata keeps "bare registration ⇒ undiscoverable" true for admin
// verbs too; every admin builtin today carries a Brief, so behavior is unchanged.
func categoryFor(c Command) string {
	if cat, ok := commandCategories[c.Keyword]; ok {
		return cat
	}
	if c.Admin && hasListingMetadata(c) {
		return catAdmin
	}
	return c.Category
}

// hasListingMetadata reports whether c already carries the listing metadata that
// makes RegisterCommand register a discoverable help topic. It mirrors the gate
// in registry.go's RegisterCommand, minus Category — the field categoryFor is
// deciding — so the two must stay in sync if that gate changes.
func hasListingMetadata(c Command) bool {
	return c.Brief != "" || len(c.Syntax) > 0 || len(c.Keywords) > 0 || len(c.Aliases) > 0
}
