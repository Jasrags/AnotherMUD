// Package logging adapts log/slog so loggers travel on context.Context.
//
// Foundations F2: structured logging via log/slog, with the logger
// attached to ctx so handlers automatically log with the right scope
// (session_id, entity_id, tick, …) without threading a logger through
// every signature.
//
// Field naming follows the table in docs/ROADMAP.md#foundations.
package logging

import (
	"context"
	"log/slog"
	"strings"
	"unicode"
)

type ctxKey struct{}

// Sanitize returns s with invalid UTF-8 replaced and every control rune
// (except tab) swapped for the Unicode replacement character. Use it on any
// untrusted text logged inline — a newline or terminal escape in raw player
// input could otherwise forge or split a line under the text handler.
func Sanitize(s string) string {
	s = strings.ToValidUTF8(s, string(unicode.ReplacementChar))
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return unicode.ReplacementChar
		}
		return r
	}, s)
}

// Default is the logger returned by From when no logger has been
// attached to ctx. Replace at process start if a non-default handler
// is wanted (e.g. JSON in production).
var Default = slog.Default()

// With returns a copy of ctx carrying logger derived from the current
// context logger plus the supplied attrs.
func With(ctx context.Context, attrs ...slog.Attr) context.Context {
	base := From(ctx)
	if len(attrs) == 0 {
		return ctx
	}
	args := make([]any, 0, len(attrs))
	for _, a := range attrs {
		args = append(args, a)
	}
	return context.WithValue(ctx, ctxKey{}, base.With(args...))
}

// WithLogger replaces the logger on ctx outright. Prefer With for
// attribute-only changes.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	if l == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, l)
}

// From retrieves the logger attached to ctx, or Default if none.
func From(ctx context.Context) *slog.Logger {
	if ctx == nil {
		return Default
	}
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return Default
}
