package conn

import "context"

// CompletionItem is one candidate the char-mode line editor can offer on
// Tab. Value is the token to insert (it round-trips through ordinary
// resolution); Display is the human label.
type CompletionItem struct {
	Value   string
	Display string
}

// Completion is a CompletionProvider's answer for a partial line: the
// longest common prefix to complete the token to, plus the candidates.
type Completion struct {
	Common     string
	Candidates []CompletionItem
}

// CompletionProvider answers "what completes this partial line?" — the
// callback the char-mode line editor invokes on Tab. The session installs
// it (wrapping command.Registry.CompleteLine); the telnet layer can't
// import command, so the result is transport-neutral. Runs synchronously
// on the read goroutine, so it must be fast and must not block.
type CompletionProvider func(ctx context.Context, line string) Completion

// CharModeConn is the optional capability a Connection implements when it
// can do server-side character-at-a-time line editing (telnet). The
// session installs a completion provider and toggles char-mode on/off.
type CharModeConn interface {
	// SetCompletionProvider installs the Tab-completion callback.
	SetCompletionProvider(p CompletionProvider)
	// SetCharMode turns server-side char-at-a-time editing on/off,
	// negotiating ECHO+SGA. A no-op transport-wise when already in the
	// requested state.
	SetCharMode(ctx context.Context, on bool)
	// CharModeActive reports the current mode.
	CharModeActive() bool
}
