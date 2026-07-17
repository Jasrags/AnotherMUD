package render

import (
	"strconv"
	"strings"
)

// DefaultPromptTemplate returns the fallback template used when a player
// has no prompt_template set (ui-rendering-help §7.1). It is *adaptive*:
// HP and MV always show, but the mana segment appears only when the
// character actually has a mana pool (MaxMana > 0). A mana-less archetype
// — a Shadowrun street samurai — therefore sees no dead `[MA 0/0]`, while
// a mage or a WoT channeler keeps it. The varying thing is which resource
// pools the character has, not its class — so future optional pools (a
// decker's Matrix monitor, a rigger's vehicle) extend this the same way,
// with a segment gated on their own non-zero max, rather than via a
// per-class template table. Only the mana segment branches today.
//
// It uses semantic color tags (<hp>/<mana>/<mv>) which the color renderer
// resolves downstream; the {token} placeholders are substituted by
// RenderPrompt first. Player-set templates are unaffected — they render
// exactly as typed, mana segment or not.
func DefaultPromptTemplate(v PromptVitals) string {
	var b strings.Builder
	b.Grow(80)
	b.WriteString("<hp>[HP {hp}/{maxhp}]</hp> ")
	// Stun sits right after HP: the two are a pair of condition monitors
	// (Physical=HP, Stun) in Shadowrun, where every character has both. It
	// shows only when the character has a stun pool (MaxStun > 0), so a
	// pack without one never renders it.
	if v.MaxStun > 0 {
		b.WriteString("<stun>[ST {stun}/{maxstun}]</stun> ")
	}
	if v.MaxMana > 0 {
		b.WriteString("<mana>[MA {mana}/{maxmana}]</mana> ")
	}
	b.WriteString("<mv>[MV {mv}/{maxmv}]</mv>> ")
	return b.String()
}

// PromptVitals carries the values the prompt token table substitutes
// (§7.2). All are plain integers; formatting is the template's job.
type PromptVitals struct {
	HP      int
	MaxHP   int
	Stun    int
	MaxStun int
	Mana    int
	MaxMana int
	MV      int
	MaxMV   int
	Gold    int
}

// RenderPrompt substitutes {token} placeholders in template with values
// from v (ui-rendering-help §7.2). Matching is case-insensitive. An
// empty template uses DefaultPromptTemplate.
//
// A recognized token resolves to its integer value. An UNrecognized
// token (a `{word}` made of ASCII letters that is not in the table)
// resolves to the empty string — the spec's typo-tolerant behavior, so
// a broken template leaves a gap rather than deleting the whole prompt.
// Anything that is not a `{letters}` shape (a lone `{`, `{1}`, `{{`, or
// a brace with no close) is left verbatim, so it survives to the color
// renderer untouched. Prompt templates are expected to color with
// `<...>` semantic tags, not `{...}` brace shorthand (§7.1).
func RenderPrompt(template string, v PromptVitals) string {
	if template == "" {
		template = DefaultPromptTemplate(v)
	}
	if !strings.ContainsRune(template, '{') {
		return template
	}

	var b strings.Builder
	b.Grow(len(template) + 8)
	i := 0
	for i < len(template) {
		c := template[i]
		if c != '{' {
			b.WriteByte(c)
			i++
			continue
		}
		end := strings.IndexByte(template[i:], '}')
		if end < 0 {
			b.WriteByte('{') // unterminated: leave verbatim
			i++
			continue
		}
		end += i
		token := template[i+1 : end]
		if !isLetters(token) {
			b.WriteByte('{') // not a token shape: leave the '{' literal
			i++
			continue
		}
		b.WriteString(promptTokenValue(strings.ToLower(token), v))
		i = end + 1
	}
	return b.String()
}

// promptTokenValue returns the substitution for a lower-cased token, or
// "" if the token is not in the table.
func promptTokenValue(token string, v PromptVitals) string {
	switch token {
	case "hp":
		return strconv.Itoa(v.HP)
	case "maxhp":
		return strconv.Itoa(v.MaxHP)
	case "stun":
		return strconv.Itoa(v.Stun)
	case "maxstun":
		return strconv.Itoa(v.MaxStun)
	case "mana":
		return strconv.Itoa(v.Mana)
	case "maxmana":
		return strconv.Itoa(v.MaxMana)
	case "mv":
		return strconv.Itoa(v.MV)
	case "maxmv":
		return strconv.Itoa(v.MaxMV)
	case "gold":
		return strconv.Itoa(v.Gold)
	default:
		return ""
	}
}

// isLetters reports whether s is non-empty and all ASCII letters. Used
// to decide whether a `{...}` is a token placeholder (substituted) vs.
// some other brace construct (left verbatim for the color renderer).
func isLetters(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			return false
		}
	}
	return true
}
