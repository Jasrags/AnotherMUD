package pack

import (
	"errors"
	"fmt"
	"sort"
)

// ErrCycle is returned by Order when the dependency graph contains a cycle.
var ErrCycle = errors.New("pack dependency cycle")

// ErrUnknownDep is returned when a manifest depends on a namespace that
// was not discovered.
var ErrUnknownDep = errors.New("pack dependency not discovered")

// Order returns the discovered packs in load order: dependencies first,
// then dependents, with alphabetical-by-namespace tie-breaks among
// independent packs (spec §3.2 / §2.4).
//
// Manifest `dependencies` keys are interpreted as pack namespaces
// (post §2.3 derivation). Bare-name keys match bare-namespace packs;
// scoped-name keys like `@scope/foo` resolve to `scope-foo`.
//
// The input slice is not mutated.
func Order(packs []Discovered) ([]Discovered, error) {
	if len(packs) == 0 {
		return nil, nil
	}

	byNS := make(map[string]Discovered, len(packs))
	for _, p := range packs {
		ns := p.Namespace()
		if _, dup := byNS[ns]; dup {
			return nil, fmt.Errorf("duplicate pack namespace %q (from %s)", ns, p.ManifestPath)
		}
		byNS[ns] = p
	}

	// Build edges: dep -> dependent. Track indegree per namespace.
	indegree := make(map[string]int, len(packs))
	dependents := make(map[string][]string, len(packs))
	for ns := range byNS {
		indegree[ns] = 0
	}
	for ns, p := range byNS {
		// Two manifest keys may derive to the same namespace (e.g. "alpha"
		// and "@scope/alpha" → "scope-alpha"); count each edge once or
		// indegree never reaches zero.
		seen := make(map[string]struct{}, len(p.Manifest.Dependencies))
		for depName := range p.Manifest.Dependencies {
			depNS := DeriveNamespace(depName)
			if _, dup := seen[depNS]; dup {
				continue
			}
			seen[depNS] = struct{}{}
			if _, ok := byNS[depNS]; !ok {
				return nil, fmt.Errorf("%w: pack %q requires %q", ErrUnknownDep, p.Manifest.Name, depName)
			}
			if depNS == ns {
				return nil, fmt.Errorf("%w: pack %q depends on itself", ErrCycle, p.Manifest.Name)
			}
			dependents[depNS] = append(dependents[depNS], ns)
			indegree[ns]++
		}
	}

	// Kahn's algorithm with a sorted-namespace queue to keep tie-breaks
	// deterministic and alphabetical.
	var ready []string
	for ns, d := range indegree {
		if d == 0 {
			ready = append(ready, ns)
		}
	}
	sort.Strings(ready)

	out := make([]Discovered, 0, len(packs))
	for len(ready) > 0 {
		next := ready[0]
		ready = ready[1:]
		out = append(out, byNS[next])

		// Collect newly-ready namespaces so we can sort them as a batch.
		var newlyReady []string
		for _, dep := range dependents[next] {
			indegree[dep]--
			if indegree[dep] == 0 {
				newlyReady = append(newlyReady, dep)
			}
		}
		if len(newlyReady) > 0 {
			ready = append(ready, newlyReady...)
			sort.Strings(ready)
		}
	}

	if len(out) != len(packs) {
		return nil, fmt.Errorf("%w: unresolved namespaces remain", ErrCycle)
	}
	return out, nil
}
