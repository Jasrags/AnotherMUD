package pack

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDeriveNamespace(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bare", "legends-forgotten", "legends-forgotten"},
		{"scoped", "@anthropic/tapestry-core", "anthropic-tapestry-core"},
		{"engine", "tapestry-core", "tapestry-core"},
		{"scoped no at", "scope/name", "scope-name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DeriveNamespace(tt.in); got != tt.want {
				t.Errorf("DeriveNamespace(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestManifestIsActive(t *testing.T) {
	yes := true
	no := false
	cases := []struct {
		name string
		m    Manifest
		want bool
	}{
		{"default true", Manifest{}, true},
		{"explicit true", Manifest{Active: &yes}, true},
		{"explicit false", Manifest{Active: &no}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.m.IsActive(); got != c.want {
				t.Errorf("IsActive = %v, want %v", got, c.want)
			}
		})
	}
}

func TestLoadManifest(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name    string
		body    string
		wantErr error
		check   func(*testing.T, *Manifest)
	}{
		{
			name: "minimal",
			body: "name: legends-forgotten\n",
			check: func(t *testing.T, m *Manifest) {
				if m.Name != "legends-forgotten" {
					t.Errorf("Name = %q", m.Name)
				}
				if !m.IsActive() {
					t.Errorf("expected default active=true")
				}
				if m.Namespace() != "legends-forgotten" {
					t.Errorf("Namespace = %q", m.Namespace())
				}
			},
		},
		{
			name: "scoped with deps",
			body: "name: \"@scope/foo\"\nactive: true\ndependencies:\n  tapestry-core: \"*\"\n",
			check: func(t *testing.T, m *Manifest) {
				if m.Namespace() != "scope-foo" {
					t.Errorf("Namespace = %q", m.Namespace())
				}
				if _, ok := m.Dependencies["tapestry-core"]; !ok {
					t.Errorf("missing tapestry-core dep")
				}
			},
		},
		{
			name: "inactive",
			body: "name: skipme\nactive: false\n",
			check: func(t *testing.T, m *Manifest) {
				if m.IsActive() {
					t.Errorf("expected inactive")
				}
			},
		},
		{
			name:    "missing name",
			body:    "version: 1.0\n",
			wantErr: ErrManifestInvalid,
		},
		{
			name:    "bad yaml",
			body:    "name: [unterminated\n",
			wantErr: ErrManifestInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := filepath.Join(dir, tt.name+".yaml")
			if err := os.WriteFile(p, []byte(tt.body), 0o600); err != nil {
				t.Fatal(err)
			}
			m, err := LoadManifest(p)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, m)
			}
		})
	}
}

func TestLoadManifestMissing(t *testing.T) {
	_, err := LoadManifest(filepath.Join(t.TempDir(), "nope.yaml"))
	if !errors.Is(err, ErrManifestMissing) {
		t.Fatalf("err = %v, want ErrManifestMissing", err)
	}
}
