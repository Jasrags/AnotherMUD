package pack

import (
	"github.com/Jasrags/AnotherMUD/internal/biome"
	"github.com/Jasrags/AnotherMUD/internal/channel"
	"github.com/Jasrags/AnotherMUD/internal/chat"
	"github.com/Jasrags/AnotherMUD/internal/decoration"
	"github.com/Jasrags/AnotherMUD/internal/effect"
	"github.com/Jasrags/AnotherMUD/internal/emote"
	"github.com/Jasrags/AnotherMUD/internal/faction"
	"github.com/Jasrags/AnotherMUD/internal/feat"
	"github.com/Jasrags/AnotherMUD/internal/gathering"
	"github.com/Jasrags/AnotherMUD/internal/grade"
	"github.com/Jasrags/AnotherMUD/internal/help"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/loot"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/property"
	"github.com/Jasrags/AnotherMUD/internal/quest"
	"github.com/Jasrags/AnotherMUD/internal/rangedflavor"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
	"github.com/Jasrags/AnotherMUD/internal/render"
	"github.com/Jasrags/AnotherMUD/internal/script"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/weather"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Registries bundles the boot-time content registries the pack loader
// writes into. Adding a new registry (mobs, abilities, …) means
// adding a field here, not widening Load's signature.
//
// All fields MUST be non-nil. NewRegistries is the supported way to
// construct one.
type Registries struct {
	World   *world.World
	Items   *item.Templates
	Slots   *slot.Registry
	Mobs    *mob.Templates
	Tracks  *progression.TrackRegistry
	Races   *progression.RaceRegistry
	Classes *progression.ClassRegistry
	// Backgrounds is the character-creation origin registry (backgrounds §2).
	Backgrounds *progression.BackgroundRegistry
	// AttributeSets is the content-defined base attribute-set registry (SR-M1 —
	// shadowrun-mvp.md Appendix A). Packs register sets from their
	// `attribute_sets:` glob; the core pack ships `classic` (the engine six). A
	// world seeds its characters + its `score` sheet + its trainable gate from
	// the set its manifest selects (wiring lands in later SR-M1 steps). Set ids
	// are global; higher priority wins.
	AttributeSets *progression.AttributeSetRegistry
	// Languages is the tongue registry (languages.md §2). Packs register
	// languages from their `languages:` glob; a background grants its
	// home_language through it, and `score`/`languages` resolve known ids to
	// display names. Ids are namespace-qualified at load.
	Languages *progression.LanguageRegistry
	// Feats is the player-chosen feat registry (EPIC S4 Phase 0 —
	// docs/proposals/wot-feats.md §2.1).
	Feats     *feat.Registry
	Abilities *progression.AbilityRegistry
	// Theme is the M10 UI theme registry. Packs register semantic
	// tag → {fg,bg,html} entries; the composition root compiles it
	// once after Load and binds a render.ColorRenderer to it.
	Theme *render.ThemeRegistry
	// Help is the M10.5 help-topic service. Packs register topics from
	// their help/*.yaml files; the help command queries it.
	Help *help.Service
	// Quests is the M10.6 quest-definition registry. Packs register
	// quests from their quests/*.yaml files; the quest service (M10.7)
	// reads it.
	Quests *quest.Registry
	// Effects is the M14.2 effect-template registry. Packs register
	// effect templates from their effects/*.yaml files; the
	// item.consumed subscriber resolves an event's effect_id through
	// this registry before calling EffectManager.Apply.
	Effects *effect.Registry
	// Properties is the M14.4 property-key registry. Engine code
	// registers known property keys at boot (RegisterEngine); pack
	// init code may register pack-scoped keys (RegisterPack). The
	// room loader validates each room's Properties bag against this
	// registry — snake_case + known-name + type-match.
	Properties *property.Registry
	// Weather is the M15.4 weather-zone registry. Packs register
	// zones from their weather_zones/*.yaml files; the weather
	// Service resolves area weather_zone ids through this registry
	// at HourChanged time.
	Weather *weather.Registry
	// Scripts is the M17.1b script-source registry. Packs register
	// Lua source bodies from their `scripts:` manifest glob; the
	// composition root (M17.1c) reads from here to install handlers
	// at boot. Compile-checked at load time so syntax errors
	// surface with pack + path attribution before the engine
	// starts ticking.
	Scripts *script.Registry
	// Rarity / Essence are the M20 item-decoration registries. Packs
	// register tier ladders + essence glyphs from their `rarity:` /
	// `essence:` manifest globs; the composition root seeds their colors
	// into Theme (RegisterTheme, if-absent) before Compile. Item display
	// (M20.5) resolves an item's rarity/essence key through these.
	Rarity  *decoration.RarityRegistry
	Essence *decoration.EssenceRegistry
	// Grades is the masterwork quality-grade vocabulary (masterwork §2).
	// Packs register grade ladders from their `grades:` manifest glob; the
	// equip path reads an item's grade key through it to apply the
	// grade-scaled bonus (weapon to-hit this slice). Mechanically
	// independent of Rarity/Essence (masterwork §5).
	Grades *grade.Registry
	// Loot is the M22.1 loot-table registry. Packs register tables from
	// their `loot_tables:` manifest glob; the spawn pipeline rolls a
	// mob's referenced table into its contents at spawn time
	// (mobs-ai-spawning §6.3).
	Loot *loot.Registry
	// Recipes is the crafting-recipe registry (crafting-and-cooking §3).
	// Packs register recipes from their `recipes:` manifest glob; the
	// crafting resolution path (Phase 2) reads it, and the per-character
	// known-recipe set keys on recipe ids.
	Recipes *recipe.Registry
	// Biomes is the biome-definition registry (biomes.md §2). The engine
	// baseline (outdoors/indoors/underground) is registered before Load
	// (like Slots); packs add biomes from their `biomes:` glob. A room's
	// `terrain` value resolves through it for shielding (§3), ambience
	// (§4), and the gathering resource tables (§2, §5). Unregistered
	// terrain keeps today's bare-string behavior (§2.3).
	Biomes *biome.Registry
	// ForageTables is the ambient-forage resource-pool registry
	// (gathering.md §2). Packs register tables from their `forage_tables:`
	// glob; a biome's ForageTable id references one, and the `forage` verb
	// rolls it. Ids + item refs are namespace-qualified at load.
	ForageTables *gathering.ForageRegistry
	// Nodes is the resource-node registry (gathering.md §3): node templates
	// + per-biome node spawn tables. The boot pipeline turns a biome's node
	// spawn table into area spawn rules; the `harvest` verb reads node
	// templates. Ids + refs are namespace-qualified at load.
	Nodes *gathering.NodeRegistry
	// Channels is the chat-channel registry (chat-channels-and-tells §3).
	// Packs register channels from their `channels:` glob; the engine
	// baseline (ooc) ships in the core pack. main derives per-channel verbs
	// + scrollbacks from it. Ids are namespace-qualified at load.
	Channels *chat.Registry
	// Emotes is the social-emote registry (emotes.md §2). Packs register
	// emotes from their `emotes:` glob; the engine baseline (smile/nod/…)
	// ships in the core pack. main derives per-emote verbs (+ aliases) from
	// it. Ids are namespace-qualified at load.
	Emotes *emote.Registry
	// RangedFlavor holds ranged-weapon flavor styles (rangedflavor), keyed by
	// `ranged_style` id. A weapon resolves its dry-fire / load / shoot text
	// through it; the core baseline ships bow/crossbow/thrown + a `default`.
	// Last-writer-wins across packs (a global vocabulary, not namespaced).
	RangedFlavor *rangedflavor.Registry
	// ChannelMap is the combat-channel derivation registry (the channel
	// layer — docs/themes/channel-vocabulary.md §7). Packs register
	// channel→formula entries from their `channel_map:` glob, later-wins;
	// the composition root Build()s it into a channel.Mapping that
	// combat.Stats derives HitMod/AC through. Distinct from Channels (chat).
	ChannelMap *channel.Registry
	// Factions is the faction/standing registry (faction.md §2). Packs register
	// faction definitions from their `factions:` glob; a character's per-faction
	// standing (player save `faction_standing`) is read/written through the
	// faction.Manager the composition root builds over this registry. Ids are
	// namespace-qualified at load. May be empty (no factions defined).
	Factions *faction.Registry

	// Worlds is the active world set (character-identity §2): the
	// namespaces of the loaded packs flagged `kind: world`, in load order.
	// Library/baseline packs are loaded but excluded. Populated by Load
	// (not a content registry — loaded-pack metadata the composition root
	// reads to gate character login/creation by world). Empty means the
	// active packs declare no world — a configuration the boot rejects.
	Worlds []string

	// WorldAttributeSets maps a world pack's namespace → the attribute-set id
	// it selects (its manifest `attribute_set:`), for world packs that declare
	// one (SR-M1 — shadowrun-mvp.md Appendix A). A world absent from this map
	// (the common case today) falls back to the engine `classic` set at the
	// character seed. Populated by Load; the composition root threads it into
	// session.Config so the actor constructor seeds a character from its
	// world's set. Not a content registry — loaded-pack metadata like Worlds.
	WorldAttributeSets map[string]string

	// Splashes maps a world pack's namespace → its connect splash text
	// (the raw file contents with engine color markup, not yet rendered).
	// Populated by Load for every kind:world pack (required, validated);
	// the composition root renders the primary world's entry through the
	// theme and hands it to the login front door. Not a content registry —
	// loaded-pack metadata, like Worlds.
	Splashes map[string]string
}

// NewRegistries returns a Registries with every field initialized.
// Callers that want the engine-baseline slot set should call
// slot.RegisterEngineBaseline on the returned Slots registry before
// invoking Load so packs can supplement (and collide cleanly if they
// try to redefine baseline slots).
func NewRegistries() *Registries {
	return &Registries{
		World:         world.New(),
		Items:         item.NewTemplates(),
		Slots:         slot.NewRegistry(),
		Mobs:          mob.NewTemplates(),
		Tracks:        progression.NewTrackRegistry(),
		Races:         progression.NewRaceRegistry(),
		Classes:       progression.NewClassRegistry(),
		Backgrounds:   progression.NewBackgroundRegistry(),
		AttributeSets: progression.NewAttributeSetRegistry(),
		Languages:     progression.NewLanguageRegistry(),
		Feats:         feat.NewRegistry(),
		Abilities:     progression.NewAbilityRegistry(),
		Theme:         render.NewThemeRegistry(),
		Help:          help.NewService(),
		Quests:        quest.NewRegistry(),
		Effects:       effect.NewRegistry(),
		Properties:    property.NewRegistry(),
		Weather:       weather.NewRegistry(),
		Scripts:       script.New(),
		Rarity:        decoration.NewRarityRegistry(),
		Essence:       decoration.NewEssenceRegistry(),
		Grades:        grade.NewRegistry(),
		Loot:          loot.NewRegistry(),
		Recipes:       recipe.NewRegistry(),
		Biomes:        biome.NewRegistry(),
		ForageTables:  gathering.NewForageRegistry(),
		Nodes:         gathering.NewNodeRegistry(),
		Channels:      chat.NewRegistry(),
		Emotes:        emote.NewRegistry(),
		RangedFlavor:  rangedflavor.NewRegistry(),
		ChannelMap:    channel.NewRegistry(),
		Factions:      faction.NewRegistry(faction.DefaultConfig()),
		// Non-nil per the all-fields-initialized invariant; Load resets and
		// repopulates it (the active world set is empty until Load runs).
		Worlds:             []string{},
		WorldAttributeSets: map[string]string{},
		Splashes:           map[string]string{},
	}
}
