package telnettest

import (
	"bytes"
	"io"
	"testing"
)

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
