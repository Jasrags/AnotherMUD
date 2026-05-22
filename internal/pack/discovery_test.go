package pack

import (
	"os"
	"path/filepath"
	"testing"
)

// writePack writes a manifest into root/<rel>/pack.yaml. rel may contain "/".
func writePack(t *testing.T, root, rel, body string) {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pack.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverBareAndScoped(t *testing.T) {
	root := t.TempDir()
	writePack(t, root, "bravo", "name: bravo\n")
	writePack(t, root, "alpha", "name: alpha\n")
	writePack(t, root, "@scope/charlie", "name: \"@scope/charlie\"\n")
	writePack(t, root, "@scope/delta", "name: \"@scope/delta\"\n")
	// Empty dir without a manifest is silently skipped.
	if err := os.MkdirAll(filepath.Join(root, "no-manifest-here"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Discover(root, nil)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	// "@scope/..." sorts before bare names (ASCII '@' < 'a'), and within
	// each level entries are alphabetical.
	want := []string{"scope-charlie", "scope-delta", "alpha", "bravo"}
	if len(got) != len(want) {
		t.Fatalf("got %d packs, want %d (%v)", len(got), len(want), discoveredNS(got))
	}
	for i, ns := range want {
		if got[i].Namespace() != ns {
			t.Errorf("[%d] namespace = %q, want %q", i, got[i].Namespace(), ns)
		}
	}
}

func TestDiscoverInactiveSkipped(t *testing.T) {
	root := t.TempDir()
	writePack(t, root, "live", "name: live\n")
	writePack(t, root, "dead", "name: dead\nactive: false\n")

	got, err := Discover(root, nil)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 1 || got[0].Manifest.Name != "live" {
		t.Errorf("got %v, want only live", discoveredNS(got))
	}
}

func TestDiscoverFilter(t *testing.T) {
	root := t.TempDir()
	writePack(t, root, "alpha", "name: alpha\n")
	writePack(t, root, "bravo", "name: bravo\n")
	writePack(t, root, "@scope/charlie", "name: \"@scope/charlie\"\n")

	tests := []struct {
		name   string
		filter []string
		want   []string
	}{
		{"by name", []string{"alpha"}, []string{"alpha"}},
		{"by namespace", []string{"scope-charlie"}, []string{"scope-charlie"}},
		{"by folder", []string{"@scope/charlie"}, []string{"scope-charlie"}},
		{"multi", []string{"alpha", "bravo"}, []string{"alpha", "bravo"}},
		{"miss", []string{"nope"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Discover(root, tt.filter)
			if err != nil {
				t.Fatalf("Discover: %v", err)
			}
			gotNS := discoveredNS(got)
			if !equalStrings(gotNS, tt.want) {
				t.Errorf("got %v, want %v", gotNS, tt.want)
			}
		})
	}
}

func TestDiscoverInvalidManifestPropagates(t *testing.T) {
	root := t.TempDir()
	writePack(t, root, "bad", "name: [oops\n")
	if _, err := Discover(root, nil); err == nil {
		t.Fatal("expected error from invalid manifest, got nil")
	}
}

func TestDiscoverTapestryYamlAccepted(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "alt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tapestry.yaml"), []byte("name: alt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Discover(root, nil)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 1 || got[0].Manifest.Name != "alt" {
		t.Errorf("got %v", discoveredNS(got))
	}
}

func discoveredNS(ds []Discovered) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.Namespace())
	}
	return out
}

func equalStrings(a, b []string) bool {
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
