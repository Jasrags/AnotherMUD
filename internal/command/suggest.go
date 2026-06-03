package command

import (
	"context"
	"fmt"
	"strings"
)

// SuggestHandler implements `suggest <partial command…>` — the line-mode
// completion stopgap (tab-completion proposal §7). Raw telnet has no TAB
// key, so this player-facing verb runs the Phase 0 completion query on a
// partial line and *lists* the candidates for the token being typed. It
// is the same query the (admin) `complete` debug verb runs, rendered for
// a player instead of for debugging.
//
// Limitation (shared with `complete`): dispatch trims the line, so a
// trailing space is lost — to see a fresh argument slot, type a partial
// letter (`suggest get s`, not `suggest get `).
func SuggestHandler(ctx context.Context, c *Context) error {
	partial := strings.Join(c.Args, " ")
	if strings.TrimSpace(partial) == "" {
		return c.Actor.Write(ctx,
			"Suggest what? Type part of a command — e.g. `suggest get s` or `suggest kill ba`.")
	}
	if c.registry == nil {
		return c.Actor.Write(ctx, "Suggestions aren't available right now.")
	}

	isAdmin := false
	if h, ok := c.Actor.(RoleHolder); ok {
		isAdmin = h.HasRole(defaultAdminRole)
	}
	res := c.registry.Complete(partial, c.BuildResolveContext(), CompletionOptions{IsAdmin: isAdmin})
	return c.Actor.Write(ctx, renderSuggest(partial, res))
}

// renderSuggest formats a completion result for a player (tab-completion
// §12: longest-common-prefix + list). Pure — unit-testable without an
// actor.
func renderSuggest(partial string, res CompletionResult) string {
	if len(res.Candidates) == 0 {
		return fmt.Sprintf("No suggestions for %q.", strings.TrimSpace(partial))
	}

	tokens := make([]string, len(res.Candidates))
	for i, cand := range res.Candidates {
		tokens[i] = cand.Completion
	}

	// Verb slot: just the command keywords.
	if res.Target == CompleteVerb {
		line := "Commands: " + strings.Join(tokens, ", ")
		if res.Truncated {
			line += ", …"
		}
		return line
	}

	// Argument slot. A single match shows the completed line; several show
	// the list, with the longest-common-prefix as a "narrow it" hint when
	// it extends what was typed.
	if len(res.Candidates) == 1 {
		cand := res.Candidates[0]
		out := fmt.Sprintf("→ %s %s", res.Verb, cand.Completion)
		if !strings.EqualFold(cand.Display, cand.Completion) {
			out += fmt.Sprintf("   (%s)", cand.Display)
		}
		return out
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s — %d matches", res.Verb, len(res.Candidates))
	last := lastToken(partial)
	if lcp := LongestCommonPrefix(tokens); len(lcp) > len(last) {
		fmt.Fprintf(&b, " (try %s %s…)", res.Verb, lcp)
	}
	b.WriteByte(':')
	for _, cand := range res.Candidates {
		fmt.Fprintf(&b, "\n  %s", cand.Completion)
		if !strings.EqualFold(cand.Display, cand.Completion) {
			fmt.Fprintf(&b, "   %s", cand.Display)
		}
	}
	if res.Truncated {
		b.WriteString("\n  … and more")
	}
	return b.String()
}

// lastToken returns the final whitespace-delimited token of partial (the
// token under completion), lowercased for prefix comparison.
func lastToken(partial string) string {
	f := strings.Fields(partial)
	if len(f) == 0 {
		return ""
	}
	return strings.ToLower(f[len(f)-1])
}

// LongestCommonPrefix returns the longest prefix shared by all tokens
// (case-insensitive), or "" when they diverge immediately.
func LongestCommonPrefix(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	prefix := strings.ToLower(tokens[0])
	for _, t := range tokens[1:] {
		t = strings.ToLower(t)
		n := 0
		for n < len(prefix) && n < len(t) && prefix[n] == t[n] {
			n++
		}
		prefix = prefix[:n]
		if prefix == "" {
			break
		}
	}
	return prefix
}
