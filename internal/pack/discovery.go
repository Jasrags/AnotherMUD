package pack

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Discovered is a single pack found by the walker, paired with its
// origin (so later phases can resolve content paths) and parsed
// manifest.
type Discovered struct {
	// Dir is the absolute or root-relative path to the pack directory
	// (the directory that contained the manifest file).
	Dir string

	// ManifestPath is the resolved manifest filename inside Dir.
	ManifestPath string

	// Manifest is the parsed manifest content.
	Manifest *Manifest
}

// Namespace returns the derived namespace for the discovered pack.
func (d Discovered) Namespace() string { return d.Manifest.Namespace() }

// Discover walks the pack root and returns every active pack it finds,
// in alphabetical order (spec §2.4). Inactive packs are skipped before
// any further processing (spec §3.1 step 1).
//
// Both bare subdirectories (`packs/legends-forgotten/`) and scoped
// directories (`packs/@scope/legends-forgotten/`) are walked.
//
// If filter is non-empty, only packs whose name, namespace, or folder
// matches an entry are returned; non-matching packs are silently
// skipped (spec §2.4).
//
// Discovery does NOT sort by dependencies — call Order afterwards.
func Discover(root string, filter []string) ([]Discovered, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("reading pack root %s: %w", root, err)
	}

	// Sort for deterministic discovery order regardless of FS iteration order.
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	allow := newFilter(filter)
	var found []Discovered

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		dir := filepath.Join(root, name)

		if strings.HasPrefix(name, "@") {
			// Scoped: walk one level deeper.
			scoped, err := discoverScope(dir, name, allow)
			if err != nil {
				return nil, err
			}
			found = append(found, scoped...)
			continue
		}

		d, err := loadPackDir(dir)
		if err != nil {
			if errors.Is(err, ErrManifestMissing) {
				// Bare subdir with no manifest is not a pack — skip silently.
				continue
			}
			return nil, err
		}
		if !d.Manifest.IsActive() {
			continue
		}
		if !allow.permits(d, name) {
			continue
		}
		found = append(found, d)
	}

	return found, nil
}

func discoverScope(scopeDir, scopeName string, allow filterSet) ([]Discovered, error) {
	entries, err := os.ReadDir(scopeDir)
	if err != nil {
		return nil, fmt.Errorf("reading scope dir %s: %w", scopeDir, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	var out []Discovered
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(scopeDir, e.Name())
		d, err := loadPackDir(dir)
		if err != nil {
			if errors.Is(err, ErrManifestMissing) {
				continue
			}
			return nil, err
		}
		if !d.Manifest.IsActive() {
			continue
		}
		folderPath := scopeName + "/" + e.Name()
		if !allow.permits(d, folderPath) {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

// loadPackDir looks for a manifest inside dir and returns the
// Discovered record. Returns ErrManifestMissing if no recognized
// manifest filename exists.
func loadPackDir(dir string) (Discovered, error) {
	for _, fname := range ManifestFilenames {
		p := filepath.Join(dir, fname)
		_, err := os.Stat(p)
		switch {
		case err == nil:
			m, lerr := LoadManifest(p)
			if lerr != nil {
				return Discovered{}, lerr
			}
			return Discovered{Dir: dir, ManifestPath: p, Manifest: m}, nil
		case errors.Is(err, os.ErrNotExist):
			// Try the next candidate filename.
			continue
		default:
			// Permission denied / I/O error must not masquerade as "no manifest".
			return Discovered{}, fmt.Errorf("checking manifest %s: %w", p, err)
		}
	}
	return Discovered{}, fmt.Errorf("%w: %s", ErrManifestMissing, dir)
}

// filterSet matches against pack names, namespaces, or folder paths.
// An empty filter permits everything (spec §2.4: "non-empty" config list).
type filterSet struct {
	entries map[string]struct{}
}

func newFilter(in []string) filterSet {
	if len(in) == 0 {
		return filterSet{}
	}
	m := make(map[string]struct{}, len(in))
	for _, s := range in {
		m[s] = struct{}{}
	}
	return filterSet{entries: m}
}

func (f filterSet) permits(d Discovered, folder string) bool {
	if f.entries == nil {
		return true
	}
	if _, ok := f.entries[d.Manifest.Name]; ok {
		return true
	}
	if _, ok := f.entries[d.Manifest.Namespace()]; ok {
		return true
	}
	if _, ok := f.entries[folder]; ok {
		return true
	}
	return false
}

// filterClosure reduces an already-discovered pack set to the `requested`
// packs PLUS their transitive dependency closure (over manifest
// Dependencies). This is the boot-time pack-selection allowlist: naming a
// setting's world pack (e.g. "wot") automatically keeps the baseline it
// depends on ("tapestry-core"), so the operator need not enumerate deps.
//
// A requested token matches a pack by manifest name, derived namespace, or
// directory base name (mirroring filterSet.permits, minus scoped-folder
// paths). An empty `requested` returns `all` unchanged (load everything).
// Tokens matching no pack are ignored (lenient, like the literal Discover
// filter). A dependency absent from `all` (e.g. a deactivated pack) is not
// added here — Order surfaces it as ErrUnknownDep with a clear message.
// Order within `all` is preserved.
func filterClosure(all []Discovered, requested []string) []Discovered {
	want := make(map[string]struct{}, len(requested))
	for _, r := range requested {
		if r = strings.TrimSpace(r); r != "" {
			want[r] = struct{}{}
		}
	}
	if len(want) == 0 {
		return all
	}

	byNS := make(map[string]Discovered, len(all))
	for _, d := range all {
		byNS[d.Namespace()] = d
	}

	matches := func(d Discovered) bool {
		for _, key := range []string{d.Manifest.Name, d.Namespace(), filepath.Base(d.Dir)} {
			if _, ok := want[key]; ok {
				return true
			}
		}
		return false
	}

	keep := make(map[string]struct{})
	var queue []string
	enqueue := func(ns string) {
		if _, done := keep[ns]; !done {
			keep[ns] = struct{}{}
			queue = append(queue, ns)
		}
	}
	for _, d := range all {
		if matches(d) {
			enqueue(d.Namespace())
		}
	}
	for len(queue) > 0 {
		ns := queue[0]
		queue = queue[1:]
		d, ok := byNS[ns]
		if !ok {
			continue
		}
		for depName := range d.Manifest.Dependencies {
			depNS := DeriveNamespace(depName)
			if _, present := byNS[depNS]; present {
				enqueue(depNS)
			}
		}
	}

	out := make([]Discovered, 0, len(keep))
	for _, d := range all {
		if _, ok := keep[d.Namespace()]; ok {
			out = append(out, d)
		}
	}
	return out
}
