// Command worldmap renders the static world content of a pack into a single
// self-contained interactive HTML map (docs/maps/world.html by default).
//
// It parses the pack's areas/rooms/mobs YAML directly — no server boot, no
// engine dependency — and lays every room out with a breadth-first walk of the
// exit graph, mirroring the engine's coordinate derivation (north = +y,
// east = +x, up = +z; one exit = one unit step — see internal/world/coords.go).
// The result is dependency-free HTML: pan/zoom/click, region tinting,
// shop/trainer/spawn/item badges, search, and z-level toggles.
//
// Usage:
//
//	go run ./cmd/worldmap [-content ./content] [-pack wot] [-start the-green] [-out docs/maps/world.html]
package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed template.html
var templateHTML string

// markupRE strips the engine's {c}…{x} color markup from display names.
var markupRE = regexp.MustCompile(`\{[^}]*\}`)

// dirOrder is the deterministic visit order for the layout BFS, and dirDelta
// the per-direction unit step (room-coordinates §2.1: north = +y, east = +x,
// up = +z). Only these six directions appear in content.
var dirOrder = []string{"north", "south", "east", "west", "up", "down"}
var dirDelta = map[string][3]int{
	"north": {0, 1, 0}, "south": {0, -1, 0},
	"east": {1, 0, 0}, "west": {-1, 0, 0},
	"up": {0, 0, 1}, "down": {0, 0, -1},
}

// --- YAML shapes (partial — only the fields the map needs) ---

type areaYAML struct {
	ID     string `yaml:"id"`
	Name   string `yaml:"name"`
	Region string `yaml:"region"`
}

type roomYAML struct {
	ID         string            `yaml:"id"`
	Area       string            `yaml:"area"`
	Name       string            `yaml:"name"`
	Terrain    string            `yaml:"terrain"`
	Exits      map[string]string `yaml:"exits"`
	Mobs       []string          `yaml:"mobs"`
	Items      []any             `yaml:"items"`
	Properties map[string]any    `yaml:"properties"`
}

type mobYAML struct {
	ID         string         `yaml:"id"`
	Name       string         `yaml:"name"`
	Tags       []string       `yaml:"tags"`
	Trainer    any            `yaml:"trainer"`
	Properties map[string]any `yaml:"properties"`
}

// --- JSON shapes (what gets embedded in the HTML) ---

type exitJSON struct {
	Dir   string `json:"dir"`
	To    string `json:"to"`
	Cross bool   `json:"cross"` // leaves this room's area
}

type mobJSON struct {
	Name    string `json:"name"`
	Shop    bool   `json:"shop"`
	Trainer bool   `json:"trainer"`
}

type roomJSON struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Area    string     `json:"area"`
	Region  string     `json:"region"`
	Terrain string     `json:"terrain"`
	X       int        `json:"x"`
	Y       int        `json:"y"`
	Z       int        `json:"z"`
	Exits   []exitJSON `json:"exits"`
	Mobs    []mobJSON  `json:"mobs"`
	Spawn   bool       `json:"spawn"`
	Items   bool       `json:"items"`
	Station bool       `json:"station"`
}

type worldJSON struct {
	Pack      string     `json:"pack"`
	Generated string     `json:"generated"`
	Regions   []string   `json:"regions"`
	Areas     []areaMeta `json:"areas"`
	Rooms     []roomJSON `json:"rooms"`
}

type areaMeta struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Region string `json:"region"`
}

func main() {
	content := flag.String("content", "./content", "content directory")
	pack := flag.String("pack", "wot", "pack to render")
	start := flag.String("start", "the-green", "starting room id (spawn / BFS seed)")
	out := flag.String("out", "docs/maps/world.html", "output HTML path")
	flag.Parse()

	if err := run(*content, *pack, *start, *out); err != nil {
		fmt.Fprintln(os.Stderr, "worldmap:", err)
		os.Exit(1)
	}
}

func run(content, pack, start, out string) error {
	base := filepath.Join(content, pack)

	areas, err := loadAreas(filepath.Join(base, "areas"))
	if err != nil {
		return fmt.Errorf("loading areas: %w", err)
	}
	mobs, err := loadMobs(filepath.Join(base, "mobs"))
	if err != nil {
		return fmt.Errorf("loading mobs: %w", err)
	}
	rooms, err := loadRooms(filepath.Join(base, "rooms"))
	if err != nil {
		return fmt.Errorf("loading rooms: %w", err)
	}
	if len(rooms) == 0 {
		return fmt.Errorf("no rooms found under %s", base)
	}

	coords := layout(rooms, start)

	world := assemble(pack, start, areas, mobs, rooms, coords)
	data, err := json.Marshal(world)
	if err != nil {
		return fmt.Errorf("marshaling world: %w", err)
	}

	html := strings.Replace(templateHTML, "__WORLD_DATA__", string(data), 1)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}
	if err := os.WriteFile(out, []byte(html), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", out, err)
	}
	fmt.Printf("worldmap: wrote %s — %d rooms, %d areas (pack %q)\n", out, len(rooms), len(areas), pack)
	return nil
}

// --- loaders ---

func loadAreas(dir string) (map[string]areaYAML, error) {
	files, _ := filepath.Glob(filepath.Join(dir, "*.yaml"))
	out := make(map[string]areaYAML, len(files))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var a areaYAML
		if err := yaml.Unmarshal(b, &a); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if a.ID != "" {
			out[a.ID] = a
		}
	}
	return out, nil
}

func loadRooms(dir string) (map[string]roomYAML, error) {
	files, _ := filepath.Glob(filepath.Join(dir, "*.yaml"))
	out := make(map[string]roomYAML, len(files))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var r roomYAML
		if err := yaml.Unmarshal(b, &r); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if r.ID != "" {
			out[r.ID] = r
		}
	}
	return out, nil
}

func loadMobs(dir string) (map[string]mobJSON, error) {
	files, _ := filepath.Glob(filepath.Join(dir, "*.yaml"))
	out := make(map[string]mobJSON, len(files))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var m mobYAML
		if err := yaml.Unmarshal(b, &m); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if m.ID == "" {
			continue
		}
		_, hasShopProp := m.Properties["shop"]
		out[m.ID] = mobJSON{
			Name:    clean(m.Name),
			Shop:    hasShopProp || hasTag(m.Tags, "shop"),
			Trainer: m.Trainer != nil || hasTag(m.Tags, "skill_trainer"),
		}
	}
	return out, nil
}

// --- layout: global BFS over the exit graph with collision spread ---

type pt struct{ x, y, z int }

func layout(rooms map[string]roomYAML, start string) map[string]pt {
	coords := make(map[string]pt, len(rooms))
	occupied := make(map[pt]string, len(rooms))

	place := func(id string, want pt) {
		if _, taken := occupied[want]; !taken {
			occupied[want] = id
			coords[id] = want
			return
		}
		// Spiral outward on the same z-plane for the nearest free cell.
		for r := 1; r < 2000; r++ {
			for dx := -r; dx <= r; dx++ {
				for dy := -r; dy <= r; dy++ {
					if abs(dx) != r && abs(dy) != r {
						continue // ring only
					}
					q := pt{want.x + dx, want.y + dy, want.z}
					if _, taken := occupied[q]; !taken {
						occupied[q] = id
						coords[id] = q
						return
					}
				}
			}
		}
		coords[id] = want // give up (pathological); overlap is harmless visually
	}

	visited := make(map[string]bool, len(rooms))
	bfs := func(seed string, origin pt) {
		place(seed, origin)
		visited[seed] = true
		queue := []string{seed}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			cp := coords[cur]
			for _, dir := range dirOrder {
				to := rooms[cur].Exits[dir]
				if to == "" || visited[to] {
					continue
				}
				if _, ok := rooms[to]; !ok {
					continue // dangling / cross-pack target — skip
				}
				d := dirDelta[dir]
				place(to, pt{cp.x + d[0], cp.y + d[1], cp.z + d[2]})
				visited[to] = true
				queue = append(queue, to)
			}
		}
	}

	// Main component from the spawn seed, then any disconnected components
	// stacked below it (deterministic: seeds sorted by id).
	if _, ok := rooms[start]; ok {
		bfs(start, pt{0, 0, 0})
	}
	ids := make([]string, 0, len(rooms))
	for id := range rooms {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if visited[id] {
			continue
		}
		lowest := 0
		for _, c := range coords {
			if c.y < lowest {
				lowest = c.y
			}
		}
		bfs(id, pt{0, lowest - 4, 0})
	}
	return coords
}

// --- assembly ---

func assemble(pack, start string, areas map[string]areaYAML, mobs map[string]mobJSON, rooms map[string]roomYAML, coords map[string]pt) worldJSON {
	regionSet := map[string]bool{}
	areaMetas := make([]areaMeta, 0, len(areas))
	for _, a := range areas {
		regionSet[a.Region] = true
		areaMetas = append(areaMetas, areaMeta{ID: a.ID, Name: a.Name, Region: a.Region})
	}
	sort.Slice(areaMetas, func(i, j int) bool { return areaMetas[i].ID < areaMetas[j].ID })

	roomsJSON := make([]roomJSON, 0, len(rooms))
	for id, r := range rooms {
		c := coords[id]
		region := areas[r.Area].Region

		exits := make([]exitJSON, 0, len(r.Exits))
		for _, dir := range dirOrder {
			to, ok := r.Exits[dir]
			if !ok {
				continue
			}
			cross := false
			if tr, ok := rooms[to]; ok {
				cross = tr.Area != r.Area
			}
			exits = append(exits, exitJSON{Dir: dir, To: to, Cross: cross})
		}

		mobList := make([]mobJSON, 0, len(r.Mobs))
		for _, mid := range r.Mobs {
			if m, ok := mobs[mid]; ok {
				mobList = append(mobList, m)
			} else {
				mobList = append(mobList, mobJSON{Name: mid})
			}
		}

		_, station := r.Properties["craft_stations"]
		roomsJSON = append(roomsJSON, roomJSON{
			ID: id, Name: clean(r.Name), Area: r.Area, Region: region,
			Terrain: r.Terrain, X: c.x, Y: c.y, Z: c.z,
			Exits: exits, Mobs: mobList,
			Spawn: id == start, Items: len(r.Items) > 0, Station: station,
		})
	}
	sort.Slice(roomsJSON, func(i, j int) bool { return roomsJSON[i].ID < roomsJSON[j].ID })

	regions := make([]string, 0, len(regionSet))
	for r := range regionSet {
		if r != "" {
			regions = append(regions, r)
		}
	}
	sort.Strings(regions)

	return worldJSON{
		Pack:      pack,
		Generated: time.Now().Format("2006-01-02 15:04"),
		Regions:   regions,
		Areas:     areaMetas,
		Rooms:     roomsJSON,
	}
}

// --- helpers ---

func clean(s string) string { return strings.TrimSpace(markupRE.ReplaceAllString(s, "")) }

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
