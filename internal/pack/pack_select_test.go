package pack

import "testing"

// disc builds a Discovered with a manifest name and optional dependency
// names — enough to exercise filterClosure without touching the filesystem.
func disc(name string, deps ...string) Discovered {
	m := &Manifest{Name: name}
	if len(deps) > 0 {
		m.Dependencies = make(map[string]string, len(deps))
		for _, d := range deps {
			m.Dependencies[d] = "*"
		}
	}
	return Discovered{Dir: "/packs/" + name, Manifest: m}
}

func names(ds []Discovered) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.Manifest.Name)
	}
	return out
}

func eqStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFilterClosure(t *testing.T) {
	// all = baseline + two world packs depending on it (load order: alpha sort).
	all := []Discovered{
		disc("tapestry-core"),
		disc("starter-world", "tapestry-core"),
		disc("wot", "tapestry-core"),
	}

	tests := []struct {
		name      string
		requested []string
		want      []string // by manifest name, in `all` order
	}{
		{"empty selects all", nil, []string{"tapestry-core", "starter-world", "wot"}},
		{"world pack pulls in its dependency", []string{"wot"}, []string{"tapestry-core", "wot"}},
		{"the other world pack", []string{"starter-world"}, []string{"tapestry-core", "starter-world"}},
		{"baseline alone", []string{"tapestry-core"}, []string{"tapestry-core"}},
		{"explicit dep listing is idempotent", []string{"tapestry-core", "wot"}, []string{"tapestry-core", "wot"}},
		{"unknown token is ignored", []string{"nope"}, nil},
		{"match by directory base", []string{"wot"}, []string{"tapestry-core", "wot"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := names(filterClosure(all, tt.requested))
			if !eqStrs(got, tt.want) {
				t.Errorf("filterClosure(%v) = %v, want %v", tt.requested, got, tt.want)
			}
		})
	}
}

// TestFilterClosure_Transitive confirms multi-hop dependency chains are
// fully included.
func TestFilterClosure_Transitive(t *testing.T) {
	all := []Discovered{
		disc("a", "b"),
		disc("b", "c"),
		disc("c"),
	}
	got := names(filterClosure(all, []string{"a"}))
	if !eqStrs(got, []string{"a", "b", "c"}) {
		t.Errorf("transitive closure = %v, want [a b c]", got)
	}
}

// TestFilterClosure_MissingDepNotAdded confirms a requested pack whose
// dependency is absent from `all` (e.g. a deactivated baseline) returns only
// what is present — Order is left to surface the missing dependency.
func TestFilterClosure_MissingDepNotAdded(t *testing.T) {
	all := []Discovered{disc("wot", "tapestry-core")} // baseline not present
	got := names(filterClosure(all, []string{"wot"}))
	if !eqStrs(got, []string{"wot"}) {
		t.Errorf("missing-dep closure = %v, want [wot]", got)
	}
}
