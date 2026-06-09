package session

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/crafting"
)

// CompleteReadyCrafts is the craft-complete tick handler's body
// (crafting-and-cooking §3, the timed-craft path). It sweeps every
// logged-in actor and finishes any craft whose occupation timer has reached
// now, delivering the result to the crafter and announcing a completed
// craft to the room. Runs on the tick goroutine. Every connActor inventory
// mutator takes a.mu individually, so there is no data race with a
// concurrent session-side mutation; and because the consume loop re-checks
// each input via RemoveFromInventory, an input the player dropped/sold/used
// between begin and completion just yields a clean
// CraftMissingIngredients/Interrupted — the materials are never corrupted
// or lost.
//
// svc nil disables the sweep (a build without crafting wired).
func (m *Manager) CompleteReadyCrafts(ctx context.Context, now uint64, svc *crafting.Service) {
	if m == nil || svc == nil {
		return
	}
	for _, a := range m.playingActors() {
		res, done := svc.CompleteReady(ctx, a, now)
		if !done {
			continue
		}
		_ = a.Write(ctx, res.Message)
		if res.Outcome == crafting.CraftOK {
			if room := a.Room(); room != nil {
				m.SendToRoom(ctx, room.ID, a.Name()+" finishes some careful work.", a.PlayerID())
			}
		}
	}
}
