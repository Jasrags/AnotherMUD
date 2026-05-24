// Package keyword implements the shared item/entity keyword resolver
// per inventory-equipment-items §6. The resolver is pure and stateless:
// commands and operations call Resolve / ResolveAll against any
// iterable of Named values (room contents, holder inventory, container
// contents, equipped items).
//
// The Named interface is the minimum surface the rules need — Name()
// for substring matching (§6.1 step 4) and Keywords() for exact /
// prefix matching (§6.1 steps 2-3). ItemInstance satisfies it today;
// MobInstance and any other entity that wants to be addressable by
// keyword will satisfy it later.
package keyword

import (
	"strconv"
	"strings"
)

// Named is the contract the resolver requires of every candidate.
// Implementations may have richer APIs; the resolver only looks at
// these two methods.
type Named interface {
	Name() string
	Keywords() []string
}

// Resolve returns the first entity in candidates that matches input
// per the §6.1 single-entity selection rules, or nil if no match.
//
// Rule order (case-insensitive throughout):
//
//  1. Ordinal selector — "N.kw" with N a positive integer selects the
//     Nth (1-based) match for kw. "0.kw" and "-N.kw" do NOT take this
//     path; they fall through to rule 2 and treat the whole input as a
//     literal keyword.
//  2. Exact keyword match.
//  3. Keyword prefix match — candidate keyword starts with input AND
//     is longer (so exact matches don't satisfy "prefix").
//  4. Name substring match.
//
// Empty or whitespace input returns nil (§6.3).
func Resolve(candidates []Named, input string) Named {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	// §6.1 step 1: ordinal selector. A candidate counts as a match
	// here if it would satisfy any of the single-entity rules (exact
	// keyword, prefix keyword, or name substring) — the spec phrases
	// step 1 as "selects the Nth match for kw" and the natural
	// reading of "match" is the same chain used by steps 2-4.
	if base, n, ok := splitOrdinal(input); ok {
		base = strings.ToLower(base)
		idx := 0
		for _, c := range candidates {
			if matchesAny(c, base) {
				idx++
				if idx == n {
					return c
				}
			}
		}
		return nil
	}

	lower := strings.ToLower(input)
	// §6.1 step 2: exact keyword.
	for _, c := range candidates {
		if hasExactKeyword(c, lower) {
			return c
		}
	}
	// §6.1 step 3: prefix.
	for _, c := range candidates {
		if hasPrefixKeyword(c, lower) {
			return c
		}
	}
	// §6.1 step 4: name substring.
	for _, c := range candidates {
		if matchesName(c, lower) {
			return c
		}
	}
	return nil
}

// ResolveAll returns every candidate matching input per §6.2.
//
//   - "all" (case-insensitive) returns every candidate.
//   - "all.kw" returns every candidate matching kw by the same
//     exact-or-prefix-or-substring rules as Resolve.
//   - Any other input returns every candidate satisfying
//     exact-or-prefix-or-substring against the input.
//
// Iteration order of the input is preserved. Empty/whitespace input
// returns nil (§6.3).
func ResolveAll(candidates []Named, input string) []Named {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	lower := strings.ToLower(input)
	if lower == "all" {
		if len(candidates) == 0 {
			return nil
		}
		out := make([]Named, len(candidates))
		copy(out, candidates)
		return out
	}

	filter := lower
	if strings.HasPrefix(lower, "all.") {
		filter = strings.TrimPrefix(lower, "all.")
		if filter == "" {
			return nil
		}
	}

	var out []Named
	for _, c := range candidates {
		if matchesAny(c, filter) {
			out = append(out, c)
		}
	}
	return out
}

// splitOrdinal parses "N.keyword" into (keyword, N, true) when N is a
// positive integer. Inputs without a dot, with a non-positive N, or
// with a malformed prefix return false so the caller falls through to
// literal-keyword matching.
func splitOrdinal(input string) (string, int, bool) {
	dot := strings.IndexByte(input, '.')
	if dot <= 0 || dot == len(input)-1 {
		return "", 0, false
	}
	n, err := strconv.Atoi(input[:dot])
	if err != nil || n <= 0 {
		return "", 0, false
	}
	return input[dot+1:], n, true
}

// matchesAny reports whether c matches lowerInput by any of the
// single-entity rules: exact keyword, strict-prefix keyword, or name
// substring. Used by both ordinal selection (§6.1 step 1) and
// ResolveAll (§6.2). The lowerInput argument MUST already be
// lowercased — callers should normalize at the boundary, not inside
// every match helper.
func matchesAny(c Named, lowerInput string) bool {
	return hasExactKeyword(c, lowerInput) ||
		hasPrefixKeyword(c, lowerInput) ||
		matchesName(c, lowerInput)
}

func hasExactKeyword(c Named, lowerInput string) bool {
	for _, kw := range c.Keywords() {
		if strings.EqualFold(kw, lowerInput) {
			return true
		}
	}
	return false
}

// hasPrefixKeyword reports a strict-prefix match: the candidate's
// keyword starts with input AND is strictly longer (§6.1 step 3 — exact
// matches are not "prefix" matches in this rule). lowerInput MUST
// already be lowercased.
func hasPrefixKeyword(c Named, lowerInput string) bool {
	for _, kw := range c.Keywords() {
		lk := strings.ToLower(kw)
		if len(lk) > len(lowerInput) && strings.HasPrefix(lk, lowerInput) {
			return true
		}
	}
	return false
}

// matchesName reports whether the candidate's name contains
// lowerInput as a case-insensitive substring. lowerInput MUST already
// be lowercased.
func matchesName(c Named, lowerInput string) bool {
	return strings.Contains(strings.ToLower(c.Name()), lowerInput)
}
