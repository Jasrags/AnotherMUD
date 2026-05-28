package render

import "testing"

func TestStripTags(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"semantic", "<highlight>hi</highlight>", "hi"},
		{"literal color", `<color fg="x">danger</color>`, "danger"},
		{"no tags", "plain text", "plain text"},
		{"unterminated drops rest", "visible<broken", "visible"},
		{"adjacent tags", "<a>x</a><b>y</b>", "xy"},
		{"brace untouched", "{yellow}hi{/}", "{yellow}hi{/}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripTags(tt.in); got != tt.want {
				t.Errorf("StripTags(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestVisibleLength(t *testing.T) {
	inputs := []string{
		"<highlight>hi</highlight>",
		`<color fg="x">danger</color>`,
		"plain",
		"visible<broken",
		"<a>x</a><b>y</b>",
	}
	for _, in := range inputs {
		if got, want := VisibleLength(in), len(StripTags(in)); got != want {
			t.Errorf("VisibleLength(%q) = %d, want %d", in, got, want)
		}
	}
}
