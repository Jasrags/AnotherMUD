package telnet

import (
	"bytes"
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
