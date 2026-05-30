package emote

// PronounSet is the four-form gender-neutral substitution table the
// emote substrate uses to fill template tokens. Spec §2.2 lists the
// tokens; this struct holds the actual forms.
//
// v1 ships a single DefaultPronouns ("they/them") used for every
// actor and target. Per-entity overrides land when character-
// creation pronouns ship (M13.7b or later).
type PronounSet struct {
	Subject    string // they
	Object     string // them
	Possessive string // their
	Reflexive  string // themselves
}

// DefaultPronouns is the v1 fall-back pronoun set: gender-neutral,
// reads correctly for any actor without requiring a character
// creation prompt.
var DefaultPronouns = PronounSet{
	Subject:    "they",
	Object:     "them",
	Possessive: "their",
	Reflexive:  "themselves",
}

// ItPronouns is the substitution set used for inanimate targets
// (items, future scenery). Reserved for the M13.7b item-target
// path; v1 does not yet target items.
var ItPronouns = PronounSet{
	Subject:    "it",
	Object:     "it",
	Possessive: "its",
	Reflexive:  "itself",
}
