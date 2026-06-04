package session

import (
	"fmt"
	"strings"
)

// Description generates the player's appearance prose for `look <player>`
// (the appearance lens — ui-rendering-help). Unlike content mobs/items,
// players author no prose: the line is composed at render time from the
// resolved race + class so it stays live as the character changes. It is
// built from an ordered set of fragments so future descriptors (visible
// equipment, title, posture, condition) drop in without a rewrite.
//
// connActor satisfies the optional Describer surface the command-layer
// look handler type-asserts (describePlayer). An actor with neither race
// nor class returns "" — the look handler then renders its generic
// "nothing special" fallback rather than an empty line.
//
// race/class are read lock-free under the same write-before-publish
// discipline as Race()/RaceID(): both are set during construction before
// cfg.Manager.Add makes the actor reachable, and never reassigned. The
// caller reaches this actor through Manager.PlayersInRoom, whose
// Manager.mu acquisition supplies the happens-before edge.
func (a *connActor) Description() string {
	noun := a.appearanceNoun()
	if noun == "" {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "You see %s, %s %s.", a.Name(), indefiniteArticle(noun), noun)
	if a.race != nil && a.race.Tagline != "" {
		fmt.Fprintf(&b, " %s", a.race.Tagline)
	}
	return b.String()
}

// appearanceNoun composes the "<Race> <Class>" noun phrase from whichever
// of race/class are known, preferring display names. Empty when neither
// is set.
func (a *connActor) appearanceNoun() string {
	var race, class string
	if a.race != nil {
		race = strings.TrimSpace(a.race.DisplayName)
	}
	if a.class != nil {
		class = strings.TrimSpace(a.class.DisplayName)
	}
	switch {
	case race != "" && class != "":
		return race + " " + class
	case race != "":
		return race
	default:
		return class // "" when classless too — caller handles the empty case.
	}
}

// indefiniteArticle returns "an" before a vowel-initial word, else "a".
// Coarse on purpose (no "hour"/"honest" h-rules) — good enough for the
// race/class nouns it fronts.
func indefiniteArticle(s string) string {
	if s == "" {
		return "a"
	}
	switch s[0] {
	case 'a', 'e', 'i', 'o', 'u', 'A', 'E', 'I', 'O', 'U':
		return "an"
	}
	return "a"
}
