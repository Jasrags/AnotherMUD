package crafting

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/recipe"
)

// CraftBusy is the transient per-actor "occupied by a craft" state the
// timed-craft path drives (crafting-and-cooking §3, "time — how long the
// craft occupies the player"). The command layer's player actor satisfies
// it; a test actor that doesn't model occupation simply omits these
// methods and the command layer falls back to instant crafting. It mirrors
// economy.RestEntity — a small optional capability layered onto Crafter.
//
// The state is transient (never persisted): a craft in flight at logout or
// crash is simply lost, and because the lazy-completion model removes no
// inputs until completion (see CompleteReady), nothing is lost with it.
type CraftBusy interface {
	Crafter
	// PendingCraft returns the in-flight craft and whether one is active.
	PendingCraft() (PendingCraft, bool)
	// SetPendingCraft records a started timed craft. Returns false when one
	// is already in flight (the caller should refuse the new craft).
	SetPendingCraft(p PendingCraft) bool
	// ClearPendingCraft drops any in-flight craft, returning what was
	// cleared (for an interrupt notice) and whether one was active.
	ClearPendingCraft() (PendingCraft, bool)
}

// PendingCraft is the small transient record of an in-flight timed craft.
// StationTier is captured when the craft begins (the player can't leave the
// room without cancelling, so it stays valid through completion) so the
// completion need not recompute the world-coupled station tier off the tick
// goroutine.
type PendingCraft struct {
	RecipeID    recipe.RecipeID
	ReadyAt     uint64 // engine tick the craft completes on
	StationTier int    // station tier present when the craft began
	DisplayName string // recipe display name, for messages
}

// BeginCraftResult is the outcome of a BeginCraft probe: either a refusal
// (Outcome != CraftOK, with Message to show) or an OK token carrying what
// the command layer needs to start the delay (TimePulses, PresentTier) and
// to complete it later (RecipeID).
type BeginCraftResult struct {
	Outcome     CraftOutcome
	Message     string
	RecipeID    recipe.RecipeID
	Discipline  string
	DisplayName string
	TimePulses  int
	PresentTier int
}

// BeginCraft runs the read-only gates for a timed craft without mutating
// anything: it resolves query to a known recipe and checks skill floor,
// station, ingredient presence, and the output template — the same gates,
// and the same refusal messages, as an instant Craft. On success it returns
// an OK token the command layer uses to start the occupation timer; the
// actual consume/produce happens in CompleteReady when the timer elapses.
//
// stationTier is evaluated once here and the result captured in the token
// (PresentTier) so completion need not recompute it on the tick goroutine.
func (s *Service) BeginCraft(_ context.Context, c Crafter, query string, stationTier StationTierFunc) BeginCraftResult {
	if s == nil || s.recipes == nil || s.known == nil || s.store == nil || s.tpls == nil {
		return BeginCraftResult{Outcome: CraftNotEnabled, Message: "Crafting is not enabled in this build."}
	}
	eid := entityID(c)
	rec, ok := s.resolveKnownRecipe(eid, query)
	if !ok {
		return BeginCraftResult{Outcome: CraftUnknownRecipe, Message: "You don't know how to craft that."}
	}
	present := evalStationTier(stationTier, rec.Discipline)
	if _, _, _, _, _, fail, ok := s.gate(c, rec, present); !ok {
		return BeginCraftResult{Outcome: fail.Outcome, Message: fail.Message, RecipeID: rec.ID}
	}
	return BeginCraftResult{
		Outcome:     CraftOK,
		RecipeID:    rec.ID,
		Discipline:  rec.Discipline,
		DisplayName: rec.DisplayName,
		TimePulses:  rec.TimePulses,
		PresentTier: present,
	}
}

// CompleteReady finishes b's in-flight craft if it has come due (now >=
// ReadyAt), running the same atomic consume/produce as an instant craft
// against the crafter's CURRENT inventory and the station tier captured
// when the craft began. It clears the pending state before crafting, so a
// failed completion never loops. Returns the result and true when a craft
// completed (or failed cleanly) this call, false when none was due.
//
// Lazy-completion model: no input was reserved at begin, so if the crafter
// dropped, sold, or used an ingredient meanwhile, the gate simply refuses
// here (CraftMissingIngredients) — nothing is lost, mirroring the instant
// path's behavior when ingredients are absent.
func (s *Service) CompleteReady(ctx context.Context, b CraftBusy, now uint64) (CraftResult, bool) {
	if s == nil || b == nil {
		return CraftResult{}, false
	}
	p, ok := b.PendingCraft()
	if !ok || now < p.ReadyAt {
		return CraftResult{}, false
	}
	// Claim the craft by clearing it: ClearPendingCraft is the single-winner
	// CAS (it returns had=false to all but the first caller), so even if a
	// second completion path is ever added alongside the tick handler, only
	// one caller proceeds — no double-completion. Today the tick is the sole
	// caller, so this is defence-in-depth.
	cleared, had := b.ClearPendingCraft()
	if !had {
		return CraftResult{}, false
	}
	p = cleared

	rec, err := s.recipes.Get(p.RecipeID)
	if err != nil || rec == nil {
		// The recipe left content while the craft was in flight (§9).
		return CraftResult{
			Outcome: CraftUnknownRecipe, RecipeID: p.RecipeID,
			Message: "Your work comes to nothing; the technique escapes you.",
		}, true
	}
	return s.craftResolved(ctx, b, rec, p.StationTier), true
}
