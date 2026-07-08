package progression

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// CapTier is the ordered ladder of ability-cap tiers raised by
// in-room trainers (spec progression.md §7.2). The ladder is fixed
// by the engine — values are not content-configurable today (spec
// §9 open question records the externalization gap).
type CapTier int

const (
	// CapNone is the implicit "no trainer has touched this ability
	// yet" tier, with cap value 0. Used as the starting point for
	// NextTier; never appears as a trainer's tier.
	CapNone CapTier = 0
	// CapNovice is the first tier (cap value 25).
	CapNovice CapTier = 25
	// CapApprentice is the second tier (cap value 50).
	CapApprentice CapTier = 50
	// CapJourneyman is the third tier (cap value 75).
	CapJourneyman CapTier = 75
	// CapMaster is the top tier (cap value 100).
	CapMaster CapTier = 100
)

// NextTier returns the cap tier immediately above current. Returns
// CapNone when current is already at or above the top of the ladder
// (Master) — the practice path uses this as the AlreadyAtTop signal.
//
// Inputs are clamped to the ladder: any value < CapNovice returns
// CapNovice (a never-trained ability practices first to Novice).
func NextTier(current int) CapTier {
	switch {
	case current < int(CapNovice):
		return CapNovice
	case current < int(CapApprentice):
		return CapApprentice
	case current < int(CapJourneyman):
		return CapJourneyman
	case current < int(CapMaster):
		return CapMaster
	default:
		return CapNone
	}
}

// TrainerConfig is the per-mob trainer payload (spec §7.3). A mob
// carrying the `skill_trainer` tag MUST also carry a non-nil
// TrainerConfig; the pack loader rejects the tag without the config.
//
// Tier is the cap value the trainer can raise an ability TO (e.g.
// CapNovice → 25). Teach is the case-insensitive list of ability
// ids the trainer is willing to teach; empty means the trainer
// teaches nothing (functionally inert but legal).
type TrainerConfig struct {
	Tier  CapTier
	Teach []string
}

// CanTeach reports whether the trainer teaches abilityID. Lookups
// are case-insensitive against the lowercased ingest in the
// loader.
func (tc *TrainerConfig) CanTeach(abilityID string) bool {
	if tc == nil {
		return false
	}
	id := strings.ToLower(strings.TrimSpace(abilityID))
	return slices.Contains(tc.Teach, id)
}

// TagSkillTrainer is the entity tag a mob carries to advertise
// itself as a trainer (spec §7.3 "find trainer in room"). Paired
// with TrainerConfig — the tag without the config is rejected at
// load time so misconfigured content surfaces at boot, not when a
// player tries to practice.
const TagSkillTrainer = "skill_trainer"

// TrainingConfig is the host-side configuration for the training
// feature (spec §7.6). Zero-value config disables both the safe-
// room gate (matching the spec default "off") AND any stat
// training (empty Trainable list = nothing trainable). Hosts should
// build via DefaultTrainingConfig and tweak.
//
// **Lock contract.** RequireSafeRoomForStats, CatchUpBoost, and
// DefaultRaceCap are construction-time fields: set them before
// handing the config to a TrainingManager, then treat as
// immutable. Only the Trainable map has a runtime mutator
// (SetTrainable), and both Trainable and SetTrainable take
// TrainingConfig.mu — a future admin verb that toggles the
// other fields at runtime MUST grow accessors that take the
// same lock.
type TrainingConfig struct {
	mu sync.RWMutex

	// RequireSafeRoomForStats gates §7.4 step 2. When true, the
	// entity's current room MUST carry the `safe` tag for
	// TryTrain to succeed. Construction-time only (see lock
	// contract above).
	RequireSafeRoomForStats bool

	// Trainable lists the lowercased stat names allowed for
	// stat training (§7.4 step 3). The default is the six
	// classic attributes. Content may add/remove via the
	// SetTrainable runtime setter (which takes mu).
	//
	// SR-M1: this list is now only the FALLBACK for an entity whose
	// AttributeSet() is nil (a degenerate/headless boot). A connected
	// character resolves trainability from its content-declared attribute
	// set instead (entityTrainable), so this map does not govern live play.
	Trainable map[string]bool

	// CatchUpBoost is the proficiency bump (§7.5) applied when
	// TryPractice raises a cap and the entity's current
	// proficiency lags the prior cap. Defaults to 5.
	// Construction-time only.
	CatchUpBoost int

	// DefaultRaceCap is the per-stat cap consulted when the
	// entity's race declares no entry for the stat OR when the
	// entity is raceless. Spec §7.4 step 5 defaults to 25.
	// Construction-time only.
	DefaultRaceCap int
}

// DefaultTrainingConfig returns the engine-default training
// configuration: safe-room gate OFF, six classic attributes
// trainable, catch-up boost 5, default race cap 25.
func DefaultTrainingConfig() *TrainingConfig {
	return &TrainingConfig{
		Trainable: map[string]bool{
			"str":  true,
			"int":  true,
			"wis":  true,
			"dex":  true,
			"con":  true,
			"luck": true,
		},
		CatchUpBoost:   5,
		DefaultRaceCap: 25,
	}
}

// IsTrainable reports whether stat is in the trainable list (spec
// §7.4 step 3). Case-insensitive.
func (cfg *TrainingConfig) IsTrainable(stat string) bool {
	if cfg == nil {
		return false
	}
	s := strings.ToLower(strings.TrimSpace(stat))
	cfg.mu.RLock()
	defer cfg.mu.RUnlock()
	return cfg.Trainable[s]
}

// entityTrainable reports whether stat may be trained for entity (SR-M1 step
// 4 gate). The character's attribute set is authoritative when present: the
// stat must be declared in the set AND flagged trainable — so a Shadowrun
// character trains body/agility while a classic one trains str/dex, without a
// per-world global config. When the entity has no set (a degenerate boot),
// fall back to the global TrainingConfig.Trainable list.
func entityTrainable(entity TrainingEntity, cfg *TrainingConfig, stat string) bool {
	if set := entity.AttributeSet(); set != nil {
		a, ok := set.Get(StatType(stat)) // stat is already normalized; Get re-normalizes defensively
		return ok && a.Trainable
	}
	return cfg != nil && cfg.IsTrainable(stat)
}

// SetTrainable enables or disables stat in the trainable list.
// Runtime setter used by admin tooling (spec §7.6).
//
// SR-M1 narrowed the effect: the Trainable list governs only entities whose
// AttributeSet() is nil (a degenerate/headless boot). Connected characters
// resolve trainability from their content-declared attribute set, so this has
// no effect on live play — a future admin "toggle trainable" verb must edit
// the world's attribute-set content, not this map.
func (cfg *TrainingConfig) SetTrainable(stat string, enabled bool) {
	if cfg == nil {
		return
	}
	s := strings.ToLower(strings.TrimSpace(stat))
	if s == "" {
		return
	}
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	if cfg.Trainable == nil {
		cfg.Trainable = make(map[string]bool)
	}
	cfg.Trainable[s] = enabled
}

// PracticeOutcome enumerates the structured results of TryPractice
// (spec §7.3).
type PracticeOutcome int

const (
	// PracticeNotLearned — the player has no proficiency entry
	// for the requested ability.
	PracticeNotLearned PracticeOutcome = iota
	// PracticeNoTrainer — no `skill_trainer` mob in the room.
	PracticeNoTrainer
	// PracticeCannotTeach — the trainer's Teach list does not
	// include this ability.
	PracticeCannotTeach
	// PracticeAlreadyAtOrAboveTier — current cap >= trainer
	// tier.
	PracticeAlreadyAtOrAboveTier
	// PracticeTierSkip — trainer tier is not the next step
	// above current cap.
	PracticeTierSkip
	// PracticeSuccess — cap raised, optional catch-up boost
	// applied.
	PracticeSuccess
)

// PracticeResult is the structured return from TryPractice. The
// caller renders Message verbatim or substitutes its own copy. On
// success, NewCap is the new cap value and Boosted reports whether
// the catch-up boost was applied.
type PracticeResult struct {
	Outcome     PracticeOutcome
	AbilityID   string
	AbilityName string
	NewCap      int
	Boosted     bool
	Message     string
}

// TrainOutcome enumerates the structured results of TryTrain (spec
// §7.4).
type TrainOutcome int

const (
	// TrainNotTrainable — stat is not in the trainable list, or
	// the entity is unknown.
	TrainNotTrainable TrainOutcome = iota
	// TrainUnsafeRoom — RequireSafeRoomForStats is set and the
	// entity's room lacks the `safe` tag.
	TrainUnsafeRoom
	// TrainNoTrains — entity has no trains_available.
	TrainNoTrains
	// TrainAtRaceCap — effective stat already >= race cap (or
	// default).
	TrainAtRaceCap
	// TrainSuccess — stat increased by one, train spent, cache
	// invalidated.
	TrainSuccess
)

// TrainResult is the structured return from TryTrain. NewBase /
// NewEffective are populated on success; the verb renders these
// so the player sees the bump take effect.
type TrainResult struct {
	Outcome      TrainOutcome
	Stat         string
	NewBase      int
	NewEffective int
	Message      string
}

// TrainingEntity is the per-entity surface TrainingManager needs.
// Implemented by session.connActor on the player side. The
// interface is small (1-3 methods per spec) so test fakes are
// cheap.
//
// All methods MUST be safe for concurrent access; the
// TrainingManager holds no per-entity locks of its own.
type TrainingEntity interface {
	// StatBlock returns the entity's stat block. Used to read
	// effective values for the race-cap check and to mutate
	// base via AdjustBase on success.
	StatBlock() *StatBlock
	// TrainsAvailable returns the current pool.
	TrainsAvailable() int
	// SpendTrain decrements the pool by one. Returns false if
	// the pool is already zero (the manager guards before
	// calling, so a false here surfaces a TOCTOU race).
	SpendTrain() bool
	// RaceID returns the entity's race id, or empty when
	// raceless. Manager consults Races registry to read the
	// per-stat cap.
	RaceID() string
	// HasRoomTag reports whether the entity's current room
	// carries tag. Used for the §7.4 step 2 safe-room check.
	HasRoomTag(tag string) bool
	// AttributeSet returns the entity's resolved base attribute set (SR-M1),
	// authoritative for which stats are trainable — a stat must be declared in
	// the set AND flagged trainable. nil falls back to the global
	// TrainingConfig.Trainable list.
	AttributeSet() *AttributeSet
}

// TrainerSource resolves an in-room trainer for an entity and a target
// ability. The session-side adapter walks Placement.InRoom + Store.GetByID
// over the MobInstances carrying TagSkillTrainer (spec §7.3 "find trainer
// in room"). When a room holds MORE THAN ONE trainer, it PREFERS the one
// whose teach list includes abilityID — otherwise a second trainer that
// can't teach the requested skill would shadow the one that can.
//
// Returns (cfg, name, true) when any trainer is present — the cfg of the
// one that can teach abilityID if such a trainer exists, else the first
// trainer found (so the caller's CanTeach check still renders "X cannot
// teach you that"). Returns (nil, "", false) only when no trainer at all
// is present.
type TrainerSource interface {
	TrainerInRoom(entityID, abilityID string) (*TrainerConfig, string, bool)
}

// AbilityProficiency is the seam to the M9 proficiency feature
// (spec abilities-and-effects §3). M8.6 ships a nop implementation
// so TryPractice's plumbing is testable today without M9 landing
// first — the unknown-ability log is the expected failure mode
// before M9 wires real proficiencies.
//
// GetCap returns the entity's current cap on abilityID and whether
// the entity has learned it. SetCap writes a new cap. AddProficiency
// adds delta to current proficiency (clamped at the cap by the
// implementation). All operations are case-insensitive on ids.
type AbilityProficiency interface {
	GetCap(entityID, abilityID string) (capValue int, prof int, learned bool)
	SetCap(entityID, abilityID string, capValue int)
	AddProficiency(entityID, abilityID string, delta int)
	AbilityName(abilityID string) (string, bool)
}

// TrainingManager is the §7 training service. Holds references to
// the host-side registries (races for the cap check), config (the
// trainable list + flags), trainer source, and the proficiency
// seam. All fields are required; nils surface as Not-Trainable /
// No-Trainer outcomes rather than panics — a host that wired
// training without one of these gets a clean diagnostic.
type TrainingManager struct {
	Config      *TrainingConfig
	Races       *RaceRegistry
	Trainers    TrainerSource
	Proficiency AbilityProficiency
}

// NewTrainingManager returns a manager with the supplied
// dependencies. Callers may pass nil for Proficiency when M9 is
// not wired — TryPractice will return PracticeNotLearned for every
// ability id, which is the correct behavior before any ability is
// teachable.
func NewTrainingManager(cfg *TrainingConfig, races *RaceRegistry, trainers TrainerSource, prof AbilityProficiency) *TrainingManager {
	return &TrainingManager{
		Config:      cfg,
		Races:       races,
		Trainers:    trainers,
		Proficiency: prof,
	}
}

// TryPractice implements spec §7.3. Returns a structured result;
// callers MUST render Message (or substitute their own copy).
func (m *TrainingManager) TryPractice(ctx context.Context, entity TrainingEntity, entityID, abilityID string) PracticeResult {
	abilityID = strings.ToLower(strings.TrimSpace(abilityID))
	if abilityID == "" {
		return PracticeResult{
			Outcome: PracticeNotLearned,
			Message: "Practice what?",
		}
	}

	// Proficiency seam — if it's not wired, every ability reads as
	// not-learned, matching the spec §7.3 NotLearned case for
	// content that hasn't taught a single ability yet.
	currentCap, prof, learned := 0, 0, false
	var displayName string
	if m.Proficiency != nil {
		currentCap, prof, learned = m.Proficiency.GetCap(entityID, abilityID)
		if name, ok := m.Proficiency.AbilityName(abilityID); ok {
			displayName = name
		}
	}
	if displayName == "" {
		displayName = abilityID
	}
	if !learned {
		return PracticeResult{
			Outcome:     PracticeNotLearned,
			AbilityID:   abilityID,
			AbilityName: displayName,
			Message:     "You haven't learned " + displayName + " yet.",
		}
	}

	// Trainer-in-room check.
	if m.Trainers == nil {
		return PracticeResult{
			Outcome:     PracticeNoTrainer,
			AbilityID:   abilityID,
			AbilityName: displayName,
			Message:     "There is no one here who can teach you.",
		}
	}
	tc, trainerName, ok := m.Trainers.TrainerInRoom(entityID, abilityID)
	if !ok || tc == nil {
		return PracticeResult{
			Outcome:     PracticeNoTrainer,
			AbilityID:   abilityID,
			AbilityName: displayName,
			Message:     "There is no one here who can teach you.",
		}
	}
	if !tc.CanTeach(abilityID) {
		return PracticeResult{
			Outcome:     PracticeCannotTeach,
			AbilityID:   abilityID,
			AbilityName: displayName,
			Message:     trainerName + " cannot teach you " + displayName + ".",
		}
	}

	trainerCap := int(tc.Tier)
	if currentCap >= trainerCap {
		return PracticeResult{
			Outcome:     PracticeAlreadyAtOrAboveTier,
			AbilityID:   abilityID,
			AbilityName: displayName,
			Message:     "You have surpassed what " + trainerName + " can teach.",
		}
	}
	if int(NextTier(currentCap)) != trainerCap {
		return PracticeResult{
			Outcome:     PracticeTierSkip,
			AbilityID:   abilityID,
			AbilityName: displayName,
			Message:     "You must master the basics before " + trainerName + " can teach you.",
		}
	}

	// Apply the cap raise + optional catch-up boost (§7.5).
	priorCap := currentCap
	m.Proficiency.SetCap(entityID, abilityID, trainerCap)
	boosted := false
	if m.Config != nil && m.Config.CatchUpBoost > 0 && prof < priorCap {
		boost := m.Config.CatchUpBoost
		if prof+boost > priorCap {
			boost = priorCap - prof
		}
		if boost > 0 {
			m.Proficiency.AddProficiency(entityID, abilityID, boost)
			boosted = true
		}
	}

	return PracticeResult{
		Outcome:     PracticeSuccess,
		AbilityID:   abilityID,
		AbilityName: displayName,
		NewCap:      trainerCap,
		Boosted:     boosted,
		Message:     trainerName + " teaches you " + displayName + ". (cap " + tierLabel(trainerCap) + ")",
	}
}

// tierLabel returns the human label for a cap value. Falls back to
// the integer when the value is not on the ladder (defensive — the
// only path that produces a non-ladder value would be a content
// authoring error).
func tierLabel(capValue int) string {
	switch capValue {
	case int(CapNovice):
		return "Novice"
	case int(CapApprentice):
		return "Apprentice"
	case int(CapJourneyman):
		return "Journeyman"
	case int(CapMaster):
		return "Master"
	default:
		return strconv.Itoa(capValue)
	}
}

// TryTrain implements spec §7.4. Returns a structured result;
// callers MUST render Message.
func (m *TrainingManager) TryTrain(ctx context.Context, entity TrainingEntity, stat string) TrainResult {
	stat = strings.ToLower(strings.TrimSpace(stat))
	if stat == "" {
		return TrainResult{
			Outcome: TrainNotTrainable,
			Message: "Train which stat?",
		}
	}
	if entity == nil {
		return TrainResult{
			Outcome: TrainNotTrainable,
			Stat:    stat,
			Message: "You cannot train.",
		}
	}

	// §7.4 step 2: safe-room gate (optional).
	if m.Config != nil && m.Config.RequireSafeRoomForStats {
		if !entity.HasRoomTag("safe") {
			return TrainResult{
				Outcome: TrainUnsafeRoom,
				Stat:    stat,
				Message: "It is not safe to train here.",
			}
		}
	}

	// §7.4 step 3: trainable-stat gate.
	if !entityTrainable(entity, m.Config, stat) {
		return TrainResult{
			Outcome: TrainNotTrainable,
			Stat:    stat,
			Message: "You cannot train " + stat + ".",
		}
	}

	// §7.4 step 4: trains-available gate.
	if entity.TrainsAvailable() <= 0 {
		return TrainResult{
			Outcome: TrainNoTrains,
			Stat:    stat,
			Message: "You have no trains available.",
		}
	}

	// §7.4 step 5: race-cap gate. Resolve the cap by consulting
	// the race registry, with the configured default when the
	// race is missing or declares no entry for this stat.
	sb := entity.StatBlock()
	if sb == nil {
		return TrainResult{
			Outcome: TrainNotTrainable,
			Stat:    stat,
			Message: "You cannot train.",
		}
	}
	raceCap := defaultRaceCapValue(m.Config)
	if m.Races != nil {
		if race, ok := m.Races.Get(entity.RaceID()); ok {
			if v, has := race.StatCaps[StatType(stat)]; has {
				raceCap = v
			}
		}
	}
	current := sb.Effective(StatType(stat))
	if current >= raceCap {
		return TrainResult{
			Outcome: TrainAtRaceCap,
			Stat:    stat,
			Message: "You have reached the limit of your potential in " + stat + ".",
		}
	}

	// §7.4 steps 6-9: spend the train, bump the base attribute,
	// invalidate (AdjustBase already does this), return the new
	// effective value.
	if !entity.SpendTrain() {
		// TOCTOU: another goroutine drained the pool between
		// step 4 and here. Bail without mutating the stat.
		return TrainResult{
			Outcome: TrainNoTrains,
			Stat:    stat,
			Message: "You have no trains available.",
		}
	}
	newBase := sb.AdjustBase(StatType(stat), 1)
	newEff := sb.Effective(StatType(stat))
	return TrainResult{
		Outcome:      TrainSuccess,
		Stat:         stat,
		NewBase:      newBase,
		NewEffective: newEff,
		Message:      "You feel stronger in " + stat + ".",
	}
}

// defaultRaceCapValue returns the configured default race cap, or
// the spec-mandated 25 when the config is missing or unset.
func defaultRaceCapValue(cfg *TrainingConfig) int {
	if cfg == nil || cfg.DefaultRaceCap <= 0 {
		return 25
	}
	return cfg.DefaultRaceCap
}
