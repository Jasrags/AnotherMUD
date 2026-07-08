package session

import (
	"slices"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// TrainerSource is the production progression.TrainerSource: given a
// player id, scans the player's current room for a mob carrying the
// `skill_trainer` tag and a non-zero trainer tier, and returns a
// freshly-built *progression.TrainerConfig along with the trainer's
// display name (spec progression.md §7.3 "find trainer in room").
//
// Lookup walks Placement.InRoom in placement order — the spec is
// silent on disambiguating multiple trainers in one room, so we
// take the first match and let content avoid stacking trainers
// when the choice matters. A future precedence rule (highest tier
// wins, or exact-tier preferred) would land in this resolver
// without touching the progression manager.
type TrainerSource struct {
	Mgr       *Manager
	Placement *entities.Placement
	Items     *entities.Store
}

// NewTrainerSource returns a configured source. All three
// dependencies are required; nil callers should pass an empty
// struct and the resolver will report "no trainer" for every
// lookup (graceful degradation).
func NewTrainerSource(mgr *Manager, placement *entities.Placement, items *entities.Store) *TrainerSource {
	return &TrainerSource{Mgr: mgr, Placement: placement, Items: items}
}

// TrainerInRoom implements progression.TrainerSource. It PREFERS a trainer
// whose teach list includes abilityID; if none in the room teaches it, it
// falls back to the first trainer present (so the caller's CanTeach check
// renders "X cannot teach you that"). Returns (nil, "", false) only when no
// trainer at all is present. This lets two trainers share a room — e.g. a
// combat master and a craft trainer in the same forge — without the first
// one shadowing the other.
func (ts *TrainerSource) TrainerInRoom(playerID, abilityID string) (*progression.TrainerConfig, string, bool) {
	if ts == nil || ts.Mgr == nil || ts.Placement == nil || ts.Items == nil {
		return nil, "", false
	}
	roomID, ok := ts.Mgr.RoomOfPlayer(playerID)
	if !ok || roomID == world.RoomID("") {
		return nil, "", false
	}
	var cands []trainerCandidate
	for _, id := range ts.Placement.InRoom(roomID) {
		ent, ok := ts.Items.GetByID(id)
		if !ok {
			continue
		}
		mob, ok := ent.(*entities.MobInstance)
		if !ok {
			continue
		}
		if !hasTag(mob.Tags(), progression.TagSkillTrainer) {
			continue
		}
		tier := mob.TrainerTier()
		if tier == 0 {
			continue
		}
		cands = append(cands, trainerCandidate{
			cfg:  &progression.TrainerConfig{Tier: progression.CapTier(tier), Teach: mob.TrainerTeach()},
			name: mob.Name(),
		})
	}
	return selectTrainer(cands, abilityID)
}

// trainerCandidate is one eligible in-room trainer.
type trainerCandidate struct {
	cfg  *progression.TrainerConfig
	name string
}

// selectTrainer picks the trainer to consult for abilityID: the first whose
// teach list includes it, else the first present (so the caller's CanTeach
// check renders "X cannot teach you that"), else not-found. Pure so it can
// be tested without a Manager/Placement/Store.
func selectTrainer(cands []trainerCandidate, abilityID string) (*progression.TrainerConfig, string, bool) {
	for i := range cands {
		if cands[i].cfg.CanTeach(abilityID) {
			return cands[i].cfg, cands[i].name, true
		}
	}
	if len(cands) > 0 {
		return cands[0].cfg, cands[0].name, true
	}
	return nil, "", false
}

func hasTag(tags []string, want string) bool {
	return slices.Contains(tags, want)
}
