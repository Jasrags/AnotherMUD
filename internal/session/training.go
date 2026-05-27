package session

import (
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

// TrainerInRoom implements progression.TrainerSource. Returns
// (cfg, name, true) for the first eligible mob in the player's
// room; (nil, "", false) otherwise.
func (ts *TrainerSource) TrainerInRoom(playerID string) (*progression.TrainerConfig, string, bool) {
	if ts == nil || ts.Mgr == nil || ts.Placement == nil || ts.Items == nil {
		return nil, "", false
	}
	roomID, ok := ts.Mgr.RoomOfPlayer(playerID)
	if !ok || roomID == world.RoomID("") {
		return nil, "", false
	}
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
		return &progression.TrainerConfig{
			Tier:  progression.CapTier(tier),
			Teach: mob.TrainerTeach(),
		}, mob.Name(), true
	}
	return nil, "", false
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}
