package render

import "strings"

// DimWrap prepares a room description for the reduced-light (Dim) render
// so that inline keyword-highlight tags still pop against the muted prose
// (ui-rendering-help §2.6). It wraps the plain, untagged text runs in
// `{dim}…{/}` while leaving highlight tag spans (`<feature>…</feature>`,
// `<exit>…`, `<threat>…`, `<cmd>…`) bare, so the color renderer emits the
// prose faint and each highlight at full brightness.
//
// Why not just wrap the whole description in one `{dim}…{/}`? The color
// renderer is flat — a tag close emits an unconditional reset with no
// attribute stack (see renderer.go). Inside a single `{dim}` wrapper a
// highlight would (a) render dimmed, because the `2m` faint attribute is
// still active under its color, and (b) drop the dim for the remainder of
// the line when its close reset fires. Dimming only the gaps between
// highlights sidesteps both: each highlight sits in its own bare span at
// full color, and the following plain run re-opens `{dim}`.
//
// Tag depth is tracked by matching `<name>` opens against `</name>`
// closes; only depth-0 literal text is dimmed. An unmatched `<` (no
// closing `>`) is treated as literal prose. Brace codes are not special-
// cased — a description carries only the highlight tags, and any stray
// brace rides along inside the plain run. On a no-color client the whole
// thing strips back to clean prose (RenderPlain drops `{dim}` and the
// tags alike), so this is invisible where it can't render.
func DimWrap(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 16)

	depth := 0
	plainOpen := false
	openDim := func() {
		if !plainOpen {
			b.WriteString("{dim}")
			plainOpen = true
		}
	}
	closeDim := func() {
		if plainOpen {
			b.WriteString("{/}")
			plainOpen = false
		}
	}

	i := 0
	for i < len(s) {
		if s[i] == '<' {
			if end := tagEnd(s, i); end >= 0 {
				// A tag boundary: close any open dim run, emit the marker
				// bare, and adjust depth so the tag's content is not dimmed.
				closeDim()
				b.WriteString(s[i : end+1])
				if strings.HasPrefix(s[i+1:end], "/") {
					if depth > 0 {
						depth--
					}
				} else {
					depth++
				}
				i = end + 1
				continue
			}
			// Unmatched '<' — fall through and treat as literal prose.
		}
		if depth == 0 {
			openDim()
		}
		b.WriteByte(s[i])
		i++
	}
	closeDim()
	return b.String()
}
