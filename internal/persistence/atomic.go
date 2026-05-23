// Package persistence holds storage primitives shared by the account and
// player stores: atomic write-through-tmp-then-rename, .bak rotation, and
// path-safety helpers.
//
// Spec: docs/specs/persistence.md §3.3 (atomic writes) and §3.4 (path
// safety). The on-disk format itself is owned by the calling store.
package persistence

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrUnsafePath is returned by SafeJoin when a caller-supplied identifier
// would resolve outside the base directory.
var ErrUnsafePath = errors.New("unsafe path")

// AtomicWrite writes data to path using the rotation pattern from
// persistence spec §3.3:
//
//  1. Write the content to a sibling ".tmp" file.
//  2. If the canonical file exists, move it to a sibling ".bak".
//  3. Rename the ".tmp" into the canonical filename.
//  4. Remove the ".bak".
//
// A crash between steps leaves either the prior file intact (before step
// 2) or the ".bak" alongside the new file (before step 4); a reader sees
// one valid file in either case. Parent directories are created if
// missing.
func AtomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("atomic write %q: mkdir parents: %w", path, err)
	}

	tmp := path + ".tmp"
	bak := path + ".bak"

	// Step 1: write the new content to .tmp.
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("atomic write %q: write tmp: %w", path, err)
	}

	// Step 2: rotate any prior canonical file to .bak.
	hadPrior := false
	if _, err := os.Stat(path); err == nil {
		if err := os.Rename(path, bak); err != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("atomic write %q: rotate to bak: %w", path, err)
		}
		hadPrior = true
	} else if !errors.Is(err, os.ErrNotExist) {
		_ = os.Remove(tmp)
		return fmt.Errorf("atomic write %q: stat: %w", path, err)
	}

	// Step 3: promote .tmp to canonical.
	if err := os.Rename(tmp, path); err != nil {
		// Try to restore the prior file so callers don't lose data.
		if hadPrior {
			_ = os.Rename(bak, path)
		}
		_ = os.Remove(tmp)
		return fmt.Errorf("atomic write %q: rename tmp: %w", path, err)
	}

	// Step 4: drop the .bak. Best-effort — a stray .bak is recoverable.
	if hadPrior {
		_ = os.Remove(bak)
	}
	return nil
}

// SafeJoin resolves name against base and returns the absolute path,
// rejecting names that would escape base (parent traversal, absolute
// paths, empty input). The result is guaranteed to be a descendant of
// base.
func SafeJoin(base, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("safejoin: empty name: %w", ErrUnsafePath)
	}
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("safejoin: absolute name %q: %w", name, ErrUnsafePath)
	}
	// Clean and require it to stay relative.
	cleaned := filepath.Clean(name)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("safejoin: %q escapes base: %w", name, ErrUnsafePath)
	}

	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("safejoin: resolve base %q: %w", base, err)
	}
	joined := filepath.Join(absBase, cleaned)
	// Defensive: walk the result back through filepath.Rel to confirm
	// it is genuinely under absBase even after symlink-free normalization.
	rel, err := filepath.Rel(absBase, joined)
	if err != nil {
		return "", fmt.Errorf("safejoin: rel: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("safejoin: %q escapes base: %w", name, ErrUnsafePath)
	}
	return joined, nil
}
