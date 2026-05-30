package chat

import (
	"sync"
	"time"
)

// Message is one entry in a channel's scrollback. Snapshot of the
// publisher's identity and the rendered text at publish time;
// immutable after construction.
//
// Spec: docs/specs/chat-channels-and-tells.md §4.2.
type Message struct {
	PublishedAt time.Time
	SenderID    string
	SenderName  string // display-cased name at publish time
	Text        string // pre-rendered prefix included ("[ooc] Alice: hi")
}

// Scrollback is a per-channel global ring buffer of recent
// messages. Append-only externally; oldest-evicts when cap is
// reached. Safe for concurrent use.
//
// Spec: docs/specs/chat-channels-and-tells.md §4.
type Scrollback struct {
	mu  sync.RWMutex
	cap int
	buf []Message
}

// NewScrollback returns a buffer with the given capacity. cap <= 0
// is treated as DefaultBufferCap.
func NewScrollback(cap int) *Scrollback {
	if cap <= 0 {
		cap = DefaultBufferCap
	}
	return &Scrollback{cap: cap}
}

// Append pushes m onto the buffer. When the buffer is at cap the
// oldest entry evicts.
func (s *Scrollback) Append(m Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.buf) >= s.cap {
		s.buf = append(s.buf[:0], s.buf[1:]...)
	}
	s.buf = append(s.buf, m)
}

// Tail returns the most recent n messages in publish order
// (oldest first). When n exceeds the buffer length, every
// message is returned. The returned slice is a fresh copy.
func (s *Scrollback) Tail(n int) []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if n <= 0 || len(s.buf) == 0 {
		return nil
	}
	if n > len(s.buf) {
		n = len(s.buf)
	}
	start := len(s.buf) - n
	out := make([]Message, n)
	copy(out, s.buf[start:])
	return out
}

// Len returns the current number of buffered messages.
func (s *Scrollback) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.buf)
}

// Cap returns the configured capacity.
func (s *Scrollback) Cap() int {
	return s.cap
}
