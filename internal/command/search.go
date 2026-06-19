package command

import (
	"context"
	"fmt"
	"sort"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Search/hidden-exit tuning (visibility §8 / hidden-exits §8). Hardcoded v1;
// externalizing to the configuration surface is deferred.
const (
	// activeSearchBonus is the positive modifier a deliberate `search` adds
	// over passive perception (visibility §4.4).
	activeSearchBonus = 5
	// defaultSearchDifficulty is the concealment score used for a hidden exit
	// that declares no search_difficulty (hidden-exits §8).
	defaultSearchDifficulty = 15
	// DetectHiddenFlag is the trait/effect counter that auto-discovers
	// roll-gated concealment — hidden entities and hidden exits — without a
	// contest (visibility §4.3). Honored as a tag or an active effect flag.
	DetectHiddenFlag = "detect_hidden"
)

// exitDiscoverer is the optional per-observer hidden-exit memory (hidden-exits
// §3.4). connActor implements it; an actor that doesn't (test fakes) simply
// can never discover or recall a hidden exit.
type exitDiscoverer interface {
	IsExitDiscovered(dir world.Direction) bool
	DiscoverExit(dir world.Direction) bool
}

// actorDetectsHidden reports whether the actor carries the detect-hidden
// counter (visibility §4.3) — as a racial/ability tag OR an active effect
// flag — which auto-pierces hide/sneak and auto-discovers hidden exits.
func (c *Context) actorDetectsHidden() bool {
	if t, ok := c.Actor.(taggable); ok && t.HasTag(DetectHiddenFlag) {
		return true
	}
	return c.Effects != nil && c.Effects.HasFlag(c.Actor.PlayerID(), DetectHiddenFlag)
}

// canSeeExit reports whether the actor may see/use the exit in dir
// (hidden-exits §4): a non-hidden exit always; a hidden one only when the
// actor has discovered it, carries detect_hidden, or is an admin (admins are
// never blocked by a secret door, §3.3). The single gate shared by the
// movement command and the exits-line render so "what you can walk" and "what
// you see listed" agree.
func (c *Context) canSeeExit(dir world.Direction, e world.Exit) bool {
	if !e.Hidden {
		return true
	}
	if actorIsAdmin(c.Actor, c.AdminRole) || c.actorDetectsHidden() {
		return true
	}
	d, ok := c.Actor.(exitDiscoverer)
	return ok && d.IsExitDiscovered(dir)
}

// SearchHandler implements `search` (visibility §4.4 / hidden-exits §3.1): a
// deliberate, higher-effort detection attempt in the current room. v1 tests
// every hidden EXIT (active perception contest with the search bonus, or an
// auto-discover via detect_hidden / admin); each newly-found exit is added to
// the actor's ephemeral discovery memory, announced (actor-only), and emits
// exit.discovered. A search that finds nothing emits the empty-result line.
// (Active search of concealed entities is a documented later extension.)
func SearchHandler(ctx context.Context, c *Context) error {
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You search, but there is nothing here.")
	}
	discoverer, _ := c.Actor.(exitDiscoverer)
	autoFind := actorIsAdmin(c.Actor, c.AdminRole) || c.actorDetectsHidden()

	// Stable direction order so multiple discoveries report deterministically.
	dirs := make([]world.Direction, 0, len(room.Exits))
	for d, e := range room.Exits {
		if e.Hidden {
			dirs = append(dirs, d)
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Long() < dirs[j].Long() })

	var found int
	for _, dir := range dirs {
		exit := room.Exits[dir]
		if discoverer == nil {
			continue // can't record discovery (test fake) — nothing to find
		}
		if discoverer.IsExitDiscovered(dir) {
			continue // already found this room-visit
		}
		if !autoFind && !c.searchContestSucceeds(exit) {
			continue
		}
		if !discoverer.DiscoverExit(dir) {
			continue // raced to discovered — count it once
		}
		found++
		_ = c.Actor.Write(ctx, fmt.Sprintf("You discover a hidden passage leading %s.", dir.Long()))
		if c.Bus != nil {
			c.Bus.Publish(ctx, eventbus.ExitDiscovered{
				ActorID:    c.Actor.PlayerID(),
				Room:       room.ID,
				Direction:  dir.Short(),
				TargetRoom: exit.Target,
			})
		}
	}
	// An active search trains the Perception skill (use-gain; skills §2 — the
	// collapsed Spot/Listen/Search surface). Finding something is the successful
	// use; a fruitless search still gains at the ability's reduced rate.
	rollSkillGain(c, skillPerception, found > 0)
	if found == 0 {
		return c.Actor.Write(ctx, "You search carefully but find nothing hidden.")
	}
	return nil
}

// searchContestSucceeds runs the §4.2 perception contest for a deliberate
// search against a hidden exit: d20 + perception + the active-search bonus vs
// the exit's search difficulty (defaulted when unset). Without a perceiver or
// roller the actor cannot find a roll-gated exit (degraded but safe).
func (c *Context) searchContestSucceeds(e world.Exit) bool {
	per, ok := c.Actor.(perceiver)
	if !ok || c.SkillRoller == nil {
		return false
	}
	dc := e.SearchDifficulty
	if dc <= 0 {
		dc = defaultSearchDifficulty
	}
	return progression.ResolveSkillCheck(c.SkillRoller, per.PerceptionBonus()+activeSearchBonus, dc).Success
}
