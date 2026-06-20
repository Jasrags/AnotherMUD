package command

import (
	"context"
	"strings"
)

// LanguageHolder is the capability a connActor exposes for the `languages` verb
// (languages.md §4): the display names of the tongues the character knows. The
// session layer owns the resolution (it holds the save + the language
// registry); this handler is thin, mirroring `feats`.
type LanguageHolder interface {
	// KnownLanguages returns the display names of every known language,
	// id-sorted; nil/empty when the character knows none.
	KnownLanguages() []string
}

// LanguagesHandler implements `languages` — list the tongues the character
// speaks, reads, and writes. An actor with no known languages (or no language
// capability at all) gets a friendly nudge rather than an empty list.
func LanguagesHandler(ctx context.Context, c *Context) error {
	holder, ok := c.Actor.(LanguageHolder)
	if !ok {
		return c.Actor.Write(ctx, "You speak no languages worth noting.")
	}
	langs := holder.KnownLanguages()
	if len(langs) == 0 {
		return c.Actor.Write(ctx, "You speak no languages worth noting.")
	}
	var b strings.Builder
	b.WriteString("You speak, read, and write:")
	for _, name := range langs {
		b.WriteString("\n  ")
		b.WriteString(name)
	}
	return c.Actor.Write(ctx, b.String())
}
