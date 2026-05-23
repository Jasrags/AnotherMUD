package telnet

import "testing"

func TestStripIAC(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "no IAC",
			in:   "Jasrags",
			want: "Jasrags",
		},
		{
			name: "WILL ECHO reply prefixed",
			in:   "\xff\xfd\x01Jasrags",
			want: "Jasrags",
		},
		{
			name: "DONT ECHO reply prefixed",
			in:   "\xff\xfe\x01hello",
			want: "hello",
		},
		{
			name: "IAC in the middle",
			in:   "Jas\xff\xfd\x01rags",
			want: "Jasrags",
		},
		{
			name: "escaped literal 0xFF preserved",
			in:   "a\xff\xffb",
			want: "a\xffb",
		},
		{
			name: "subnegotiation block dropped",
			in:   "x\xff\xfa\x18\x00ANSI\xff\xf0y",
			want: "xy",
		},
		{
			name: "two-byte command dropped",
			in:   "a\xff\xf1b", // IAC NOP
			want: "ab",
		},
		{
			name: "trailing lone IAC dropped",
			in:   "alice\xff",
			want: "alice",
		},
		{
			name: "truncated negotiation at end",
			in:   "alice\xff\xfd",
			want: "alice",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripIAC(tc.in); got != tc.want {
				t.Errorf("stripIAC(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
