package telnet

import (
	"bytes"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/conn"
)

// --- pure helpers ---

func TestLastTokenOf(t *testing.T) {
	cases := map[string]string{"get sw": "sw", "get ": "", "get": "get", "": ""}
	for in, want := range cases {
		if got := lastTokenOf([]byte(in)); got != want {
			t.Errorf("lastTokenOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestApplyCompletion(t *testing.T) {
	// value extends the typed token → append suffix.
	nb, echo := applyCompletion([]byte("get sw"), "sw", "sword")
	if string(nb) != "get sword" || string(echo) != "ord" {
		t.Errorf("extend: buf=%q echo=%q", nb, echo)
	}
	// value does NOT start with the token (ordinal) → backspace + rewrite.
	nb, echo = applyCompletion([]byte("drop ring"), "ring", "2.ring")
	if string(nb) != "drop 2.ring" {
		t.Errorf("replace: buf=%q", nb)
	}
	if !strings.HasPrefix(string(echo), "\b \b") || !strings.HasSuffix(string(echo), "2.ring") {
		t.Errorf("replace echo=%q, want backspaces then '2.ring'", echo)
	}
}

func TestCandidateLine(t *testing.T) {
	got := candidateLine([]conn.CompletionItem{
		{Value: "sword", Display: "a short sword"},
		{Value: "north", Display: "north"}, // display==value → no parens
	})
	if got != "sword (a short sword)  north" {
		t.Errorf("candidateLine = %q", got)
	}
}

// --- editor integration (echo + buffer) ---

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) write(p []byte)  { s.mu.Lock(); s.b.Write(p); s.mu.Unlock() }
func (s *syncBuf) String() string  { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func drain(c net.Conn) *syncBuf {
	sb := &syncBuf{}
	go func() {
		b := make([]byte, 256)
		for {
			n, err := c.Read(b)
			if n > 0 {
				sb.write(b[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	return sb
}

func feedBytes(server *Conn, buf *[]byte, s string) (line string, done bool) {
	for i := 0; i < len(s); i++ {
		line, done = server.charModeByte(context.Background(), buf, s[i])
	}
	return
}

func TestCharModeByte_TypeBackspaceEnter(t *testing.T) {
	server, client := pairConn(t)
	echoed := drain(client)
	server.charMode = true // skip the negotiation write; test the editor only
	var buf []byte

	feedBytes(server, &buf, "hiX")
	feedBytes(server, &buf, "\x7f") // backspace removes the X
	line, done := feedBytes(server, &buf, "\r")
	if !done || line != "hi" {
		t.Fatalf("enter: line=%q done=%v", line, done)
	}

	time.Sleep(50 * time.Millisecond)
	out := echoed.String()
	if !strings.Contains(out, "hiX") {
		t.Errorf("typed chars not echoed: %q", out)
	}
	if !strings.Contains(out, "\b \b") {
		t.Errorf("backspace not echoed: %q", out)
	}
	if !strings.Contains(out, "\r\n") {
		t.Errorf("enter newline not echoed: %q", out)
	}
}

func TestCharModeByte_TabSingleCompletes(t *testing.T) {
	server, client := pairConn(t)
	_ = drain(client)
	server.charMode = true
	server.SetCompletionProvider(func(_ context.Context, line string) conn.Completion {
		return conn.Completion{Candidates: []conn.CompletionItem{{Value: "sword", Display: "a short sword"}}}
	})
	var buf []byte
	feedBytes(server, &buf, "get sw")
	server.charModeByte(context.Background(), &buf, '\t')
	if string(buf) != "get sword" {
		t.Fatalf("buffer after Tab = %q, want 'get sword'", buf)
	}
	line, done := server.charModeByte(context.Background(), &buf, '\r')
	if !done || line != "get sword" {
		t.Errorf("line=%q done=%v", line, done)
	}
}

func TestCharModeByte_TabMultipleExtendsAndLists(t *testing.T) {
	server, client := pairConn(t)
	echoed := drain(client)
	server.charMode = true
	server.SetCompletionProvider(func(_ context.Context, line string) conn.Completion {
		return conn.Completion{
			Common: "sw",
			Candidates: []conn.CompletionItem{
				{Value: "sword", Display: "a sword"},
				{Value: "swap", Display: "swap"},
			},
		}
	})
	var buf []byte
	feedBytes(server, &buf, "get s")
	server.charModeByte(context.Background(), &buf, '\t')
	// common "sw" extends typed "s" → buffer becomes "get sw"
	if string(buf) != "get sw" {
		t.Fatalf("buffer after Tab = %q, want 'get sw' (extended to common)", buf)
	}
	time.Sleep(50 * time.Millisecond)
	out := echoed.String()
	if !strings.Contains(out, "sword") || !strings.Contains(out, "swap") {
		t.Errorf("candidate list not echoed: %q", out)
	}
}
