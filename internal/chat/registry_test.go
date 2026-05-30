package chat

import (
	"strings"
	"testing"
)

func TestRegistry_RegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(Channel{ID: "tapestry-core:ooc", DisplayName: "ooc", Kind: KindPublic}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if c, ok := r.Get("tapestry-core:ooc"); !ok || c.DisplayName != "ooc" {
		t.Errorf("Get(ooc) miss: %v / %v", c, ok)
	}
	if c, ok := r.ByDisplayName("OOC"); !ok || c.ID != "tapestry-core:ooc" {
		t.Errorf("ByDisplayName(OOC) miss: %v / %v", c, ok)
	}
}

func TestRegistry_RegisterDuplicateID(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(Channel{ID: "x:a", DisplayName: "a"})
	err := r.Register(Channel{ID: "x:a", DisplayName: "b"})
	if err == nil || !strings.Contains(err.Error(), "duplicate id") {
		t.Errorf("dup id: err = %v, want duplicate", err)
	}
}

func TestRegistry_RegisterDisplayNameCollision(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(Channel{ID: "p1:trade", DisplayName: "trade"})
	err := r.Register(Channel{ID: "p2:trade", DisplayName: "Trade"})
	if err == nil || !strings.Contains(err.Error(), "display-name collision") {
		t.Errorf("dup disp: err = %v, want collision", err)
	}
}

func TestRegistry_RegisterEmptyArgs(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(Channel{}); err == nil {
		t.Errorf("empty: want error")
	}
	if err := r.Register(Channel{ID: "x"}); err == nil {
		t.Errorf("no DisplayName: want error")
	}
}

func TestRegistry_AllInRegistrationOrder(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(Channel{ID: "a", DisplayName: "a"})
	_ = r.Register(Channel{ID: "b", DisplayName: "b"})
	_ = r.Register(Channel{ID: "c", DisplayName: "c"})
	got := r.All()
	if len(got) != 3 || got[0].ID != "a" || got[1].ID != "b" || got[2].ID != "c" {
		t.Errorf("All order = %v", idList(got))
	}
	if r.Len() != 3 {
		t.Errorf("Len = %d, want 3", r.Len())
	}
}

func idList(cs []*Channel) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}
