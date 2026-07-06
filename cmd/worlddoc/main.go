// Command worlddoc renders the static world content of a pack into generated
// documentation under docs/world/<pack>/. The pack YAML (areas/rooms/mobs/…) is
// the single source of truth, parsed once (no server boot, no engine dependency)
// and rendered by one or more emitters. The interactive HTML map is one emitter;
// later phases add a gazetteer, content catalogs, a world-health report, and a
// player guide (see docs/plans/world-docs-plan.md).
//
// Layout mirrors the engine's coordinate derivation (north = +y, east = +x,
// up = +z; one exit = one unit step — see internal/world/coords.go).
//
// Usage:
//
//	go run ./cmd/worlddoc [-content ./content] [-pack wot|all] [-start the-green] [-emit all|map] [-outdir docs/world]
//
// With -pack all it renders every kind:world pack under -content and writes a
// cross-pack index (docs/world/index.md). Output is derived — regenerate rather
// than hand-edit.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// emitter renders one artifact for a pack into packDir (docs/world/<pack>/),
// returning the path written. New artifacts register here as later phases land.
type emitter struct {
	name   string
	render func(m *worldModel, packDir string) (string, error)
}

// emitters is the ordered registry. `-emit all` runs each in turn; `-emit <name>`
// runs just one. Catalogs/health/guide append here as later phases land.
var emitters = []emitter{mapEmitter, gazetteerEmitter}

// defaultStarts seeds the layout BFS (and spawn marker) per known world pack for
// `-pack all`, where the single -start flag can't apply. Unknown packs fall back
// to an empty seed (deterministic id-ordered layout, no spawn marker).
var defaultStarts = map[string]string{
	"wot":           "the-green",
	"starter-world": "town-square",
}

func main() {
	content := flag.String("content", "./content", "content directory")
	pack := flag.String("pack", "wot", "pack to render, or 'all' for every kind:world pack")
	start := flag.String("start", "the-green", "starting room id (spawn / BFS seed); ignored for -pack all")
	emit := flag.String("emit", "all", "artifact to emit: all, or one of ["+emitterNames()+"]")
	outdir := flag.String("outdir", filepath.Join("docs", "world"), "output root directory")
	flag.Parse()

	if err := run(*content, *pack, *start, *emit, *outdir); err != nil {
		fmt.Fprintln(os.Stderr, "worlddoc:", err)
		os.Exit(1)
	}
}

func run(content, pack, start, emit, outdir string) error {
	sel, err := resolveEmitters(emit)
	if err != nil {
		return err
	}
	packs, starts, err := resolvePacks(content, pack, start)
	if err != nil {
		return err
	}

	single := len(packs) == 1
	var results []packResult
	var failed int
	for _, p := range packs {
		m, err := loadPack(content, p, starts[p])
		if err != nil {
			if single {
				return err
			}
			fmt.Fprintf(os.Stderr, "worlddoc: skipping %s: %v\n", p, err)
			failed++
			continue
		}
		pr := packResult{Pack: p, Rooms: len(m.Rooms), Areas: len(m.Areas)}
		packDir := filepath.Join(outdir, p)
		renderFailed := false
		for _, e := range sel {
			path, err := e.render(m, packDir)
			if err != nil {
				// Same per-pack isolation as loadPack: a single named pack
				// hard-errors; in -pack all mode we log, count, and move on so
				// one bad pack can't abort the whole batch mid-run.
				if single {
					return fmt.Errorf("emitting %s for %s: %w", e.name, p, err)
				}
				fmt.Fprintf(os.Stderr, "worlddoc: skipping %s for %s: %v\n", e.name, p, err)
				failed++
				renderFailed = true
				break
			}
			pr.Artifacts = append(pr.Artifacts, artifact{Emitter: e.name, Path: path})
			fmt.Printf("worlddoc: wrote %s — %d rooms, %d areas (pack %q)\n", path, pr.Rooms, pr.Areas, p)
		}
		if renderFailed {
			continue // exclude a partially-rendered pack from the index roll-up
		}
		results = append(results, pr)
	}

	// The index is a full-world roll-up: only (re)write it on a `-pack all` run
	// so a single-pack render never clobbers the cross-pack table of contents.
	if pack == "all" {
		idx, err := writeIndex(outdir, results)
		if err != nil {
			return err
		}
		fmt.Printf("worlddoc: wrote %s\n", idx)
	}

	if failed > 0 {
		return fmt.Errorf("%d pack(s) failed to render", failed)
	}
	return nil
}

// resolveEmitters maps the -emit flag to the emitters to run.
func resolveEmitters(emit string) ([]emitter, error) {
	if emit == "all" {
		return emitters, nil
	}
	for _, e := range emitters {
		if e.name == emit {
			return []emitter{e}, nil
		}
	}
	return nil, fmt.Errorf("unknown -emit %q; available: all, %s", emit, emitterNames())
}

// resolvePacks expands the -pack flag into the packs to render and their BFS
// seeds. A named pack uses -start; `all` discovers kind:world packs and seeds
// each from defaultStarts.
func resolvePacks(content, pack, start string) ([]string, map[string]string, error) {
	if pack != "all" {
		return []string{pack}, map[string]string{pack: start}, nil
	}
	ps, err := discoverWorldPacks(content)
	if err != nil {
		return nil, nil, err
	}
	if len(ps) == 0 {
		return nil, nil, fmt.Errorf("no kind:world packs found under %s", content)
	}
	starts := make(map[string]string, len(ps))
	for _, p := range ps {
		starts[p] = defaultStarts[p] // "" when unknown — layout falls back
	}
	return ps, starts, nil
}

// discoverWorldPacks returns the directory names of every kind:world pack under
// content, sorted. Library packs (e.g. tapestry-core) have no world to render
// and are skipped.
func discoverWorldPacks(content string) ([]string, error) {
	entries, err := os.ReadDir(content)
	if err != nil {
		return nil, fmt.Errorf("reading content dir %s: %w", content, err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(content, e.Name(), "pack.yaml"))
		if err != nil {
			continue // not a pack directory
		}
		var mf struct {
			Kind string `yaml:"kind"`
		}
		if err := yaml.Unmarshal(b, &mf); err != nil {
			return nil, fmt.Errorf("%s/pack.yaml: %w", e.Name(), err)
		}
		if mf.Kind == "world" {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

func emitterNames() string {
	names := make([]string, len(emitters))
	for i, e := range emitters {
		names[i] = e.name
	}
	return strings.Join(names, ", ")
}
