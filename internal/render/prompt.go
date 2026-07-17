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
// from v (ui-rendering-help §7.2) and resolves {?name}…{/name} conditional
// segments (§7.5). Matching is case-insensitive. An empty template uses
// DefaultPromptTemplate.
//
// A recognized token resolves to its integer value. An UNrecognized
// token (a `{word}` made of ASCII letters that is not in the table)
// resolves to the empty string — the spec's typo-tolerant behavior, so
// a broken template leaves a gap rather than deleting the whole prompt.
// Anything that is not a `{letters}` shape (a lone `{`, `{1}`, `{{`, or
// a brace with no close) is left verbatim, so it survives to the color
// renderer untouched. Prompt templates are expected to color with
// `<...>` semantic tags, not `{...}` brace shorthand (§7.1).
//
// A `{?name}…{/name}` pair is a conditional segment: its body renders
// only when the character has the named pool (its max > 0), letting a
// hand-written custom template adapt the way the default does — e.g.
// `{?stun}<stun>[ST {stun}/{maxstun}]</stun> {/stun}` shows the Stun
// monitor only for a character that has one. The body is rendered
// recursively, so tokens, color tags, and nested conditionals of a
// *different* name compose inside it (same-name nesting is not
// supported — the first `{/name}` closes). See renderPromptInto.
func RenderPrompt(template string, v PromptVitals) string {
	if template == "" {
		template = DefaultPromptTemplate(v)
	}
	if !strings.ContainsRune(template, '{') {
		return template
	}

	var b strings.Builder
	b.Grow(len(template) + 8)
	renderPromptInto(&b, template, v)
	return b.String()
}

// renderPromptInto writes the rendered form of template into b. It is the
// recursive worker behind RenderPrompt: a conditional segment's body is
// rendered by calling back into this function, so nested constructs of a
// different name resolve naturally.
//
// Recursion depth is bounded by the nesting depth of conditional segments,
// which is in turn bounded by the template length — every level costs at
// least a `{?x}` open. The only caller feeds a template capped at
// command.MaxPromptTemplateLen, so depth is small and there is no internal
// depth guard. A new caller that is NOT length-capped must add one.
func renderPromptInto(b *strings.Builder, template string, v PromptVitals) {
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
		inner := template[i+1 : end]

		switch {
		case strings.HasPrefix(inner, "?"):
			// Conditional open {?name}. Find its matching {/name} close.
			name := inner[1:]
			if !isLetters(name) {
				b.WriteByte('{') // malformed (e.g. {?}, {?1}): leave literal
				i++
				continue
			}
			lowerName := strings.ToLower(name)
			// Pair the close case-insensitively, like tokens — {?Stun} pairs
			// with {/stun}. The matched close is the same length as the open's
			// name plus the "{/" and "}" (3 bytes), regardless of case.
			rel := findConditionClose(template[end+1:], lowerName)
			if rel < 0 {
				// No matching close: drop the open marker and render the rest,
				// so a missing close leaves no literal `{?name}` garbage.
				i = end + 1
				continue
			}
			bodyStart, bodyEnd := end+1, end+1+rel
			if promptConditionTrue(lowerName, v) {
				renderPromptInto(b, template[bodyStart:bodyEnd], v)
			}
			i = bodyEnd + len(name) + 3
		case strings.HasPrefix(inner, "/"):
			// A close tag. A well-formed template's closes are consumed by
			// their opens above; a stray letters-named close is dropped
			// (lenient). A non-letter name ({/1}, {/}) is left literal so it
			// mirrors the malformed-open guard rather than vanishing.
			if !isLetters(inner[1:]) {
				b.WriteByte('{')
				i++
				continue
			}
			i = end + 1
		case !isLetters(inner):
			b.WriteByte('{') // not a token shape: leave the '{' literal
			i++
		default:
			b.WriteString(promptTokenValue(strings.ToLower(inner), v))
			i = end + 1
		}
	}
}

// findConditionClose returns the index (relative to s) of the first
// {/name} close tag whose name matches lowerName case-insensitively, or
// -1 if none. Scanning for closes this way — rather than a fixed-case
// string search — lets {?Stun} pair with {/stun}.
func findConditionClose(s, lowerName string) int {
	from := 0
	for {
		rel := strings.Index(s[from:], "{/")
		if rel < 0 {
			return -1
		}
		open := from + rel
		closeBrace := strings.IndexByte(s[open:], '}')
		if closeBrace < 0 {
			return -1
		}
		closeBrace += open
		if strings.EqualFold(s[open+2:closeBrace], lowerName) {
			return open
		}
		from = open + 2
	}
}

// promptConditionTrue reports whether a {?name} conditional segment's body
// should render: true when the character has the named pool (its max > 0).
// An unknown name is not a pool the character has, so it is false — the
// body is hidden. This differs from the unknown-*token* rule (which leaves
// an empty gap) because a conditional's whole purpose is to suppress a
// segment for an absent pool; hiding on an unknown name is that contract,
// not a silent deletion of an otherwise-valid prompt.
func promptConditionTrue(name string, v PromptVitals) bool {
	switch name {
	case "hp":
		return v.MaxHP > 0
	case "stun":
		return v.MaxStun > 0
	case "mana":
		return v.MaxMana > 0
	case "mv":
		return v.MaxMV > 0
	case "gold":
		return v.Gold > 0
	default:
		return false
	}
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
