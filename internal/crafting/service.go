package crafting

import (
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/decoration"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
)

// Service owns crafting resolution: the quality roll (§5) and the atomic
// craft execution (§3). It mirrors economy.ShopService — it holds the
// content registries + entity store + roller, and the command layer hands
// it a Crafter adapter to mutate inventory through.
type Service struct {
	tpls    *item.Templates
	store   *entities.Store
	recipes *recipe.Registry
	known   *recipe.KnownManager
	prof    *progression.ProficiencyManager
	rarity  *decoration.RarityRegistry
	roller  Roller
	cfg     Config

	// rollMu guards roller use. Crafts arrive on per-session goroutines,
	// so the (not necessarily concurrent-safe) roller needs serializing —
	// unlike combat's roller, which runs only on the single tick goroutine.
	rollMu sync.Mutex
}

// NewService wires a crafting service. All registry args should be non-nil
// in production; the roller and rarity registry are required for the
// quality roll (a nil rarity registry yields no rarity stamp).
func NewService(
	tpls *item.Templates,
	store *entities.Store,
	recipes *recipe.Registry,
	known *recipe.KnownManager,
	prof *progression.ProficiencyManager,
	rarity *decoration.RarityRegistry,
	roller Roller,
	cfg Config,
) *Service {
	return &Service{
		tpls:    tpls,
		store:   store,
		recipes: recipes,
		known:   known,
		prof:    prof,
		rarity:  rarity,
		roller:  roller,
		cfg:     cfg,
	}
}
