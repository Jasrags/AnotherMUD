package telnet

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
)

func TestEscapeIAC_DoublesLiteralIACByte(t *testing.T) {
	// Direct unit on the escape helper — no fake conn needed.
	cases := []struct {
		name string
		in   []byte
		want []byte
	}{
		{"all-ascii passthrough", []byte("hello"), []byte("hello")},
		{"single IAC doubled", []byte{0x68, 0xFF, 0x69}, []byte{0x68, 0xFF, 0xFF, 0x69}},
		{"adjacent IACs doubled", []byte{0xFF, 0xFF}, []byte{0xFF, 0xFF, 0xFF, 0xFF}},
		{"leading IAC", []byte{0xFF, 'x'}, []byte{0xFF, 0xFF, 'x'}},
		{"trailing IAC", []byte{'x', 0xFF}, []byte{'x', 0xFF, 0xFF}},
		{"empty", []byte{}, []byte{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			idx := bytesIndexByte(c.in, tnIAC)
			var got []byte
			if idx < 0 {
				got = c.in
			} else {
				got = escapeIAC(c.in, idx)
			}
			if !bytes.Equal(got, c.want) {
				t.Errorf("got %x, want %x", got, c.want)
			}
		})
	}
}

func TestEscapeIAC_RoundTripsThroughStripIAC(t *testing.T) {
	// Doubled-IAC on the wire decodes back to a single literal
	// 0xFF in the inbound strip path — verifies symmetry.
	original := []byte{'h', 0xFF, 'i', 0xFF, 0xFF, 'x'}
	idx := bytesIndexByte(original, tnIAC)
	escaped := escapeIAC(original, idx)
	stripped := stripIAC(string(escaped))
	if stripped != string(original) {
		t.Errorf("round-trip: got %x, want %x", stripped, original)
	}
}

func TestMapEscapedWriteCount(t *testing.T) {
	// 1-byte chars cost 1, 0xFF costs 2.
	p := []byte{'a', 0xFF, 'b'} // escaped = a FF FF b (4 bytes)
	cases := []struct {
		nWritten int
		want     int
	}{
		{0, 0}, // nothing written
		{1, 1}, // 'a' covered
		{2, 1}, // partial IAC pair — only 'a' is fully covered
		{3, 2}, // full IAC pair + 'a'
		{4, 3}, // all covered
	}
	for _, c := range cases {
		if got := mapEscapedWriteCount(p, c.nWritten); got != c.want {
			t.Errorf("mapEscapedWriteCount(%d) = %d, want %d", c.nWritten, got, c.want)
		}
	}
}

// TestWriteCommandDoesNotEscapeIAC pins the password-masking fix: a
// telnet command sequence (IAC WILL ECHO) must reach the wire verbatim,
// NOT IAC-doubled the way Write escapes content. net.Pipe is unbuffered,
// so the reader runs concurrently.
func TestWriteCommandDoesNotEscapeIAC(t *testing.T) {
	srv, cli := net.Pipe()
	defer srv.Close()
	defer cli.Close()
	c := New("t1", srv)

	cmd := []byte{0xFF, 0xFB, 0x01} // IAC WILL ECHO
	got := make([]byte, len(cmd))
	readErr := make(chan error, 1)
	go func() {
		_, err := io.ReadFull(cli, got)
		readErr <- err
	}()
	if _, err := c.WriteCommand(context.Background(), cmd); err != nil {
		t.Fatalf("WriteCommand: %v", err)
	}
	if err := <-readErr; err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, cmd) {
		t.Errorf("wire = % x, want verbatim % x (no IAC doubling)", got, cmd)
	}
}

// TestWriteStillEscapesIAC is the companion: ordinary content Write keeps
// doubling 0xFF so untrusted text can't inject telnet commands.
func TestWriteStillEscapesIAC(t *testing.T) {
	srv, cli := net.Pipe()
	defer srv.Close()
	defer cli.Close()
	c := New("t2", srv)

	in := []byte{'h', 0xFF, 'i'}
	want := []byte{'h', 0xFF, 0xFF, 'i'}
	got := make([]byte, len(want))
	readErr := make(chan error, 1)
	go func() {
		_, err := io.ReadFull(cli, got)
		readErr <- err
	}()
	if _, err := c.Write(context.Background(), in); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := <-readErr; err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("wire = % x, want escaped % x", got, want)
	}
}

func TestWriteExpandsBareLFToCRLF(t *testing.T) {
	srv, cli := net.Pipe()
	defer srv.Close()
	defer cli.Close()
	c := New("crlf", srv)

	// Multi-line render: interior bare '\n' must reach the wire as "\r\n"
	// so a raw/char-mode client doesn't staircase.
	in := []byte("line1\nline2\n")
	want := []byte("line1\r\nline2\r\n")
	got := make([]byte, len(want))
	readErr := make(chan error, 1)
	go func() {
		_, err := io.ReadFull(cli, got)
		readErr <- err
	}()
	if _, err := c.Write(context.Background(), in); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := <-readErr; err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("wire = %q, want %q", got, want)
	}
}

func TestWriteLeavesExistingCRLFIntact(t *testing.T) {
	srv, cli := net.Pipe()
	defer srv.Close()
	defer cli.Close()
	c := New("crlf2", srv)

	// Already-CRLF input is idempotent — no doubled CR.
	in := []byte("a\r\nb")
	got := make([]byte, len(in))
	readErr := make(chan error, 1)
	go func() {
		_, err := io.ReadFull(cli, got)
		readErr <- err
	}()
	if _, err := c.Write(context.Background(), in); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := <-readErr; err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, in) {
		t.Errorf("wire = %q, want unchanged %q", got, in)
	}
}

func TestCRLFNormalize(t *testing.T) {
	cases := map[string]string{
		"a\nb":       "a\r\nb",   // bare LF expands
		"a\r\nb":     "a\r\nb",   // existing CRLF intact
		"\n":         "\r\n",     // leading bare LF
		"a\n\nb":     "a\r\n\r\nb", // consecutive bare LFs
		"plain text": "plain text", // no newline, zero-copy
		"a\rb":       "a\rb",     // bare CR (no LF) left alone
	}
	for in, want := range cases {
		if got := string(crlfNormalize([]byte(in))); got != want {
			t.Errorf("crlfNormalize(%q) = %q, want %q", in, got, want)
		}
	}
}
