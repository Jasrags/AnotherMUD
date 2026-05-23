package persistence_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/persistence"
)

func TestAtomicWrite_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "thing.yaml")

	if err := persistence.AtomicWrite(path, []byte("hello")); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("content = %q, want %q", got, "hello")
	}
}

func TestAtomicWrite_OverwritesAndLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "thing.yaml")

	if err := persistence.AtomicWrite(path, []byte("v1")); err != nil {
		t.Fatalf("AtomicWrite v1: %v", err)
	}
	if err := persistence.AtomicWrite(path, []byte("v2")); err != nil {
		t.Fatalf("AtomicWrite v2: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "v2" {
		t.Fatalf("content = %q, want %q", got, "v2")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "thing.yaml" {
			t.Errorf("unexpected leftover file: %q", e.Name())
		}
	}
}

func TestAtomicWrite_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deeper", "thing.yaml")

	if err := persistence.AtomicWrite(path, []byte("hi")); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "hi" {
		t.Fatalf("content = %q", got)
	}
}

func TestSafeJoin(t *testing.T) {
	base := t.TempDir()

	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{"clean name", "alice", nil},
		{"nested segments", filepath.Join("a", "b"), nil},
		{"parent escape", "..", persistence.ErrUnsafePath},
		{"parent escape nested", filepath.Join("a", "..", ".."), persistence.ErrUnsafePath},
		{"absolute", string(os.PathSeparator) + "etc", persistence.ErrUnsafePath},
		{"empty", "", persistence.ErrUnsafePath},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := persistence.SafeJoin(base, tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			rel, relErr := filepath.Rel(base, got)
			if relErr != nil || rel == ".." || filepath.IsAbs(rel) {
				t.Fatalf("result %q not under base %q (rel=%q)", got, base, rel)
			}
		})
	}
}
