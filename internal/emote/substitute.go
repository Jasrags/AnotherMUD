package emote

import "strings"

// Subject describes one party (actor or target) in a substitution
// pass. Name is the display name as the player chose it; Pronouns
// drives the $s / $S / $m / $M token forms.
type Subject struct {
	Name     string
	Pronouns PronounSet
}

// Substitute walks template left-to-right and expands each
// substitution token using the actor and target subjects. Unknown
// tokens (other $X sequences) pass through unchanged so future
// extensions remain backward-compatible.
//
// Token grammar (spec §2.2, locked Diku-derived):
//
//	$n  actor display name
//	$s  actor possessive    ("their")
//	$m  actor reflexive     ("themselves")
//	$N  target display name
//	$S  target possessive
//	$M  target reflexive
//
// Target may be the zero Subject; templates that reference target
// tokens with no target are a load-time error (Emote.Validate) and
// should never reach this function.
func Substitute(template string, actor, target Subject) string {
	if template == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(template))
	i := 0
	for i < len(template) {
		c := template[i]
		if c != '$' || i+1 >= len(template) {
			b.WriteByte(c)
			i++
			continue
		}
		tok := template[i+1]
		switch tok {
		case 'n':
			b.WriteString(actor.Name)
		case 's':
			b.WriteString(actor.Pronouns.Possessive)
		case 'm':
			b.WriteString(actor.Pronouns.Reflexive)
		case 'N':
			b.WriteString(target.Name)
		case 'S':
			b.WriteString(target.Pronouns.Possessive)
		case 'M':
			b.WriteString(target.Pronouns.Reflexive)
		default:
			// Unknown token — emit it raw so the template author
			// sees their typo in output rather than a silent drop.
			b.WriteByte('$')
			b.WriteByte(tok)
		}
		i += 2
	}
	return b.String()
}
