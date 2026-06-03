package telnet

import (
	"bytes"
	"context"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/conn"
)

// Character-at-a-time line editing (tab-completion Phase 2). When char-mode
// is on, the server echoes keystrokes and owns the input line so Tab can
// trigger completion. Only active post-login (Read's full-line contract is
// unchanged — the buffer is internal and a line is still returned on
// Enter). MVP scope: printable input, backspace, Enter, Tab. No cursor
// movement, history, or kill-line yet.

// charModeByte processes one data byte in char-mode, editing buf in place
// and echoing. Returns the completed line + true on Enter.
func (c *Conn) charModeByte(ctx context.Context, buf *[]byte, b byte) (string, bool) {
	switch {
	case b == '\r':
		// Enter. Echo the newline and hand back the line.
		c.echo(ctx, []byte("\r\n"))
		line := string(*buf)
		*buf = (*buf)[:0]
		return line, true
	case b == '\n' || b == 0:
		// LF / NUL trailing a CR (CRLF / CRNUL Enter) — already handled.
	case b == 0x7f || b == 0x08:
		// Backspace / DEL: erase the last char visually.
		if len(*buf) > 0 {
			*buf = (*buf)[:len(*buf)-1]
			c.echo(ctx, []byte("\b \b"))
		}
	case b == '\t':
		c.charModeComplete(ctx, buf)
	case b >= 0x20 && b < 0x7f:
		*buf = append(*buf, b)
		c.echo(ctx, []byte{b})
	default:
		// Ignore other control bytes (arrow-key ESC sequences, etc.).
	}
	return "", false
}

// charModeComplete runs the completion provider on the current buffer and
// applies the result: a single match completes inline; several complete to
// the common prefix (when it extends what's typed) and list the options,
// then redraw the line (tab-completion §12 — LCP + list).
func (c *Conn) charModeComplete(ctx context.Context, buf *[]byte) {
	c.cmu.Lock()
	p := c.completionProvider
	c.cmu.Unlock()
	if p == nil {
		return
	}
	res := p(ctx, string(*buf))
	if len(res.Candidates) == 0 {
		return
	}
	last := lastTokenOf(*buf)

	if len(res.Candidates) == 1 {
		nb, out := applyCompletion(*buf, last, res.Candidates[0].Value)
		*buf = nb
		c.echo(ctx, out)
		return
	}

	// Several matches: extend to the common prefix if it grows the token.
	if strings.HasPrefix(res.Common, last) && len(res.Common) > len(last) {
		nb, out := applyCompletion(*buf, last, res.Common)
		*buf = nb
		c.echo(ctx, out)
	}
	// List the candidates, then redraw the (now possibly extended) line so
	// the player can keep typing. (No prompt prefix in v1; the next Enter
	// re-renders the prompt.)
	c.echo(ctx, []byte("\r\n"+candidateLine(res.Candidates)+"\r\n"+string(*buf)))
}

// applyCompletion returns the new buffer + the bytes to echo when the last
// token (last) of buf is completed to value. When value extends last, it
// just appends the suffix; otherwise it backspaces over last and rewrites
// (handles ordinals / name-substring matches where value isn't a prefix).
func applyCompletion(buf []byte, last, value string) (newBuf, echo []byte) {
	if strings.HasPrefix(value, last) {
		suffix := value[len(last):]
		nb := append(append([]byte(nil), buf...), suffix...)
		return nb, []byte(suffix)
	}
	base := buf[:len(buf)-len(last)]
	nb := append(append([]byte(nil), base...), value...)
	out := make([]byte, 0, len(last)*3+len(value))
	for i := 0; i < len(last); i++ {
		out = append(out, '\b', ' ', '\b')
	}
	out = append(out, value...)
	return nb, out
}

// lastTokenOf returns the whitespace-delimited token under the cursor (the
// partial being completed) — "" when buf is empty or ends with a space.
func lastTokenOf(buf []byte) string {
	if i := bytes.LastIndexByte(buf, ' '); i >= 0 {
		return string(buf[i+1:])
	}
	return string(buf)
}

// candidateLine formats the candidate list for the Tab listing: `value`,
// or `value (display)` when the display differs.
func candidateLine(items []conn.CompletionItem) string {
	parts := make([]string, len(items))
	for i, it := range items {
		if it.Display != "" && it.Display != it.Value {
			parts[i] = it.Value + " (" + it.Display + ")"
		} else {
			parts[i] = it.Value
		}
	}
	return strings.Join(parts, "  ")
}

// echo writes raw bytes to the peer (server-side echo / redraw). Uses the
// public Write, which is mutex-guarded against the async writers.
func (c *Conn) echo(ctx context.Context, b []byte) {
	_, _ = c.Write(ctx, b)
}
