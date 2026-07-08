package telnettest

import (
	"bytes"
	"io"
	"testing"
)

// drainIACCapturing drains src through an iacReader with a GMCP capture hook,
// returning the clean text and the captured GMCP payloads in arrival order.
func drainIACCapturing(t *testing.T, src io.Reader) (string, []string) {
	t.Helper()
	var frames []string
	r := newIACReader(src)
	r.onGMCP = func(p string) { frames = append(frames, p) }
	var out []byte
	buf := make([]byte, 4) // tiny, to force multi-read + straddled sequences
	for {
		n, err := r.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil {
			break
		}
	}
	return string(out), frames
}

func gmcpFrameBytes(payload string) []byte {
	b := []byte{iac, sb, optGMCP}
	b = append(b, []byte(payload)...)
	return append(b, iac, se)
}

// A GMCP subnegotiation frame is captured whole while surrounding data bytes
// still stream through cleanly, even when the frame straddles read boundaries.
func TestIACReader_CapturesGMCPFrame(t *testing.T) {
	payload := `Room.Info {"num":"starter-world:town-square","x":0,"y":0,"z":0}`
	var raw []byte
	raw = append(raw, []byte("hi")...)
	raw = append(raw, gmcpFrameBytes(payload)...)
	raw = append(raw, []byte("bye")...)

	text, frames := drainIACCapturing(t, bytes.NewReader(raw))
	if text != "hibye" {
		t.Errorf("clean text = %q, want %q", text, "hibye")
	}
	if len(frames) != 1 || frames[0] != payload {
		t.Errorf("captured frames = %#v, want exactly [%q]", frames, payload)
	}
}

// Two adjacent GMCP frames are each captured separately (no cross-frame bleed),
// and the subOpt/sub buffer resets between them.
func TestIACReader_CapturesMultipleGMCPFrames(t *testing.T) {
	p1 := `Char.Vitals {"hp":20,"maxhp":20}`
	p2 := `Room.Info {"num":"x"}`
	var raw []byte
	raw = append(raw, gmcpFrameBytes(p1)...)
	raw = append(raw, gmcpFrameBytes(p2)...)

	// Split every frame across the tiny read buffer via chunkReader too.
	_, frames := drainIACCapturing(t, bytes.NewReader(raw))
	if len(frames) != 2 || frames[0] != p1 || frames[1] != p2 {
		t.Errorf("captured frames = %#v, want [%q %q]", frames, p1, p2)
	}
}

// An escaped IAC (IAC IAC = a literal 0xFF byte) inside a GMCP payload is
// captured as a single 0xFF, not treated as the start of a command or an SE.
func TestIACReader_CapturesGMCPFrameWithEscapedIAC(t *testing.T) {
	// Payload "Room.Info {"x":<0xFF>}" — a raw 0xFF mid-JSON, escaped on the wire
	// as IAC IAC per RFC 854.
	var payload []byte
	payload = append(payload, []byte(`Room.Info {"x":`)...)
	payload = append(payload, iac) // the literal 0xFF the sender means
	payload = append(payload, '}')

	// On the wire the 0xFF is doubled (IAC IAC) inside the SB … SE frame.
	var raw []byte
	raw = append(raw, iac, sb, optGMCP)
	raw = append(raw, []byte(`Room.Info {"x":`)...)
	raw = append(raw, iac, iac) // escaped literal 0xFF
	raw = append(raw, '}')
	raw = append(raw, iac, se)

	_, frames := drainIACCapturing(t, bytes.NewReader(raw))
	if len(frames) != 1 || frames[0] != string(payload) {
		t.Errorf("captured frames = %#v, want exactly [%q] (escaped IAC → single 0xFF)", frames, string(payload))
	}
}

// A non-GMCP subnegotiation (e.g. NAWS, option 31) is NOT captured — the hook
// only fires for option 201.
func TestIACReader_IgnoresNonGMCPSubneg(t *testing.T) {
	var raw []byte
	raw = append(raw, []byte("a")...)
	raw = append(raw, iac, sb, 31, 0, 80, 0, 24, iac, se) // NAWS 80x24
	raw = append(raw, []byte("b")...)

	text, frames := drainIACCapturing(t, bytes.NewReader(raw))
	if text != "ab" {
		t.Errorf("clean text = %q, want %q", text, "ab")
	}
	if len(frames) != 0 {
		t.Errorf("captured %#v from a non-GMCP subneg; want none", frames)
	}
}

// chunkReader hands out the given chunks one per Read, so a test can force a
// telnet sequence to straddle a read boundary.
type chunkReader struct {
	chunks [][]byte
	i      int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if r.i >= len(r.chunks) {
		return 0, io.EOF
	}
	n := copy(p, r.chunks[r.i])
	r.i++
	return n, nil
}

func drainIAC(t *testing.T, src io.Reader) string {
	t.Helper()
	r := newIACReader(src)
	var out []byte
	buf := make([]byte, 4) // tiny, to exercise multiple Reads + out buffering
	for {
		n, err := r.Read(buf)
		out = append(out, buf[:n]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("iacReader.Read: %v", err)
		}
		if n == 0 {
			break
		}
	}
	return string(out)
}

func TestIACReader_StripsSequences(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want string
	}{
		{"plain text", []byte("hello"), "hello"},
		{"will-echo prefix", []byte{iac, will, 1, 'h', 'i'}, "hi"},
		{"do/dont around text", []byte{'a', iac, doo, 1, 'b', iac, dont, 3, 'c'}, "abc"},
		{"escaped 0xff is data", []byte{'a', iac, iac, 'b'}, "a\xffb"},
		{"two-byte command (GA) dropped", []byte{'x', iac, 249, 'y'}, "xy"},
		{"subnegotiation dropped", []byte{'p', iac, sb, 24, 'X', 'Y', iac, se, 'q'}, "pq"},
		{"escaped iac inside sb", []byte{iac, sb, 1, iac, iac, iac, se, 'z'}, "z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := drainIAC(t, bytes.NewReader(tc.in)); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIACReader_SequenceSplitAcrossReads(t *testing.T) {
	// IAC | WILL ECHO 'h' | 'i' — the command straddles three reads.
	src := &chunkReader{chunks: [][]byte{{iac}, {will, 1, 'h'}, {'i'}}}
	if got := drainIAC(t, src); got != "hi" {
		t.Errorf("split sequence: got %q, want %q", got, "hi")
	}
}
