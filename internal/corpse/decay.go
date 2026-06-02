package corpse

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
)

// DecaySweep removes every corpse whose lifetime (in ticks) has elapsed
// since its creation tick, destroying its remaining contents — unlooted
// items and coins are destroyed with the corpse, not spilled to the room
// (loot-and-corpses §7). Returns the number of corpses decayed.
//
// Wired as the `corpse-decay` tick handler (time-and-clock §3). Corpses
// are transient (never persisted), so a restart removes them too — this
// sweep just bounds growth on a live server.
//
// Concurrency: the sweep runs on the tick goroutine; a player `loot`
// runs on a session goroutine and can race it on the same corpse.
// Placement.Remove is the single-winner claim — if loot already removed
// the corpse, the sweep skips it (loot emitted corpse.looted instead).
// Contents.Take is likewise single-winner, so an item a looter grabbed
// first is left in their hands rather than double-untracked.
func (s *Service) DecaySweep(ctx context.Context, nowTick, lifetime uint64) int {
	if s.store == nil || s.placement == nil || s.contents == nil {
		return 0
	}
	decayed := 0
	for _, e := range s.store.GetByTag(TagCorpse) {
		it, ok := e.(*entities.ItemInstance)
		if !ok {
			continue
		}
		created := CreatedTick(it)
		// Expired when the lifetime has elapsed since creation.
		// Subtract-first (never created+lifetime) avoids uint64 overflow;
		// nowTick < created (clock skew) means "not expired".
		if nowTick < created || nowTick-created < lifetime {
			continue
		}

		// Claim the corpse before touching its contents — the loser of a
		// concurrent loot/decay race bails here.
		roomID, _ := s.placement.RoomOf(it.ID())
		if !s.placement.Remove(it.ID()) {
			continue
		}

		destroyed := 0
		for _, cid := range s.contents.In(it.ID()) {
			if s.contents.Take(cid) {
				_ = s.store.Untrack(cid)
				destroyed++
			}
		}
		coins := Coins(it)
		_ = s.store.Untrack(it.ID())
		decayed++

		if s.bus != nil {
			s.bus.Publish(ctx, eventbus.CorpseDecayed{
				CorpseID:  it.ID(),
				RoomID:    roomID,
				ItemCount: destroyed,
				Coins:     coins,
			})
		}
	}
	return decayed
}
