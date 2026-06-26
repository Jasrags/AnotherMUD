package pack

import (
	"errors"
	"slices"
	"testing"
)

// decodeRecruiter validates a mob's `recruiter:` block (hireable-mobs.md §3.1):
// absent → nil; a non-empty offers list (blanks trimmed away) → a spec; an empty
// or all-blank offers list → ErrInvalidContent.
func TestDecodeRecruiter(t *testing.T) {
	if spec, err := decodeRecruiter(nil, "p"); err != nil || spec != nil {
		t.Fatalf("absent block = (%v, %v), want (nil, nil)", spec, err)
	}

	spec, err := decodeRecruiter(&RecruiterFile{Offers: []string{"sellsword", "  ", "wot:mercenary"}}, "p")
	if err != nil {
		t.Fatalf("valid offers errored: %v", err)
	}
	if !slices.Equal(spec.Offers, []string{"sellsword", "wot:mercenary"}) {
		t.Errorf("offers = %v, want blanks trimmed away", spec.Offers)
	}

	for _, f := range []*RecruiterFile{{Offers: nil}, {Offers: []string{"", "   "}}} {
		if _, err := decodeRecruiter(f, "p"); !errors.Is(err, ErrInvalidContent) {
			t.Errorf("offers %v: err = %v, want ErrInvalidContent", f.Offers, err)
		}
	}
}
