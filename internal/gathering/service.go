package gathering

import (
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/decoration"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// GatheringAbility is the proficiency id the gathering skill rides
// (gathering.md PD-2 / §4). It is an ordinary progression proficiency —
// use-based gain, no new store — so a content pack defines a `gathering`
// ability and players gain it by foraging/harvesting. A single skill in
// v1; per-resource sub-skills (mining/herbalism) are deferred (§9).
const GatheringAbility = "gathering"

// Service owns gathering resolution: the quality roll (§4) and (in later
// slices) the forage/harvest operations. It mirrors crafting.Service —
// holds the rarity ladder, roller, proficiency manager, config, and a roll
// mutex — but is its own type so the two features stay decoupled. The forage
// table / node registries + entity store are wired in the B2/B3 slices.
type Service struct {
	rarity *decoration.RarityRegistry
	prof   *progression.ProficiencyManager
	roller Roller
	cfg    Config
	// stats scales the §4 use-gain by the gathering ability's gain_stat
	// (shared with the M26 passive-gain seam); nil = un-scaled gain.
	stats progression.StatReader
	// store + tpls spawn the yielded item (the §2 forage create step). Like
	// crafting.Service, the gathering service owns item creation; the
	// command layer supplies only the Gatherer adapter to file it into. Nil
	// in roll/gain-only tests.
	store *entities.Store
	tpls  *item.Templates

	// rollMu serializes roller use (the roller need not be concurrent-safe
	// and gathers arrive on per-session goroutines), mirroring crafting.
	rollMu sync.Mutex
}

// NewService wires the gathering service. rarity + roller are required for
// the quality roll (a nil rarity registry yields no rarity stamp); prof +
// stats may be nil in tests that don't exercise skill gain; store + tpls
// are required for the forage create step (nil in roll/gain-only tests).
func NewService(rarity *decoration.RarityRegistry, prof *progression.ProficiencyManager, roller Roller, cfg Config, stats progression.StatReader, store *entities.Store, tpls *item.Templates) *Service {
	return &Service{rarity: rarity, prof: prof, roller: roller, cfg: cfg, stats: stats, store: store, tpls: tpls}
}

// RollQuality computes the rarity-tier key for one gather from its §4
// inputs. Exposed for the forage/harvest paths (and tests).
func (s *Service) RollQuality(in QualityInputs) string {
	if s == nil {
		return ""
	}
	return s.rollQuality(in)
}

// GainFromUse rolls a use-based gathering-proficiency gain for entityID
// after a successful gather (§4 "a gather attempt grants use-based
// proficiency on success"). No-op (returns false) when the proficiency
// manager isn't wired. Mirrors crafting's gain roll, serializing the roller
// through the same rollMu.
func (s *Service) GainFromUse(entityID string) bool {
	if s == nil || s.prof == nil {
		return false
	}
	return s.prof.RollUseGain(entityID, GatheringAbility, true, lockedRoller{mu: &s.rollMu, r: s.roller}, s.stats)
}

// lockedRoller serializes roller use for the gain roll (gathers run on
// per-session goroutines). Mirrors the inline guard in rollQuality and the
// crafting package's identically-named helper.
type lockedRoller struct {
	mu interface {
		Lock()
		Unlock()
	}
	r Roller
}

func (l lockedRoller) IntN(n int) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.r.IntN(n)
}
