package render

import (
	"strconv"
	"strings"
)

// DefaultPromptTemplate is used when a player has no prompt_template set
// (ui-rendering-help §7.1). It uses semantic color tags (<hp>/<mana>/
// <mv>) which the color renderer resolves downstream; the {token}
// placeholders are substituted by RenderPrompt first.
const DefaultPromptTemplate = "<hp>[HP {hp}/{maxhp}]</hp> <mana>[MA {mana}/{maxmana}]</mana> <mv>[MV {mv}/{maxmv}]</mv>> "

// PromptVitals carries the values the prompt token table substitutes
// (§7.2). All are plain integers; formatting is the template's job.
type PromptVitals struct {
	HP      int
	MaxHP   int
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
		template = DefaultPromptTemplate
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
