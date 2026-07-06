package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

//go:embed template.html
var templateHTML string

// mapEmitter renders the interactive, self-contained HTML world map — the
// original worldmap output, now one emitter among several. Pan/zoom/click,
// region tinting, per-room feature badges, feature filter, feature-aware
// search, distinct hidden/locked exit rendering, and z-level toggles.
var mapEmitter = emitter{
	name:      "map",
	worldOnly: true,
	render: func(m *worldModel, packDir string) ([]string, error) {
		world := assemble(m)
		data, err := json.Marshal(world)
		if err != nil {
			return nil, fmt.Errorf("marshaling world: %w", err)
		}
		html := strings.Replace(templateHTML, "__WORLD_DATA__", string(data), 1)
		out := filepath.Join(packDir, "map.html")
		if err := os.MkdirAll(packDir, 0o755); err != nil {
			return nil, fmt.Errorf("creating output dir: %w", err)
		}
		if err := os.WriteFile(out, []byte(html), 0o644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", out, err)
		}
		return []string{out}, nil
	},
}

// --- JSON shapes (what gets embedded in the HTML) ---

type exitJSON struct {
	Dir    string `json:"dir"`
	To     string `json:"to"`
	Cross  bool   `json:"cross"`  // leaves this room's area
	Hidden bool   `json:"hidden"` // an authored hidden exit (search to reveal)
	Locked bool   `json:"locked"` // a locked door bars this exit
	Door   string `json:"door"`   // door display name, if any
}

type roomJSON struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Area     string     `json:"area"`
	Region   string     `json:"region"`
	Terrain  string     `json:"terrain"`
	X        int        `json:"x"`
	Y        int        `json:"y"`
	Z        int        `json:"z"`
	Exits    []exitJSON `json:"exits"`
	Mobs     []mobJSON  `json:"mobs"`
	Spawn    bool       `json:"spawn"`
	Items    bool       `json:"items"`
	Station  bool       `json:"station"`
	Light    string     `json:"light"`    // room light override (lit/dim/black), if any
	Weather  string     `json:"weather"`  // the room's area weather zone, if any
	Features []string   `json:"features"` // canonical feature keys — drives badges/filter/search
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

// assemble folds the parsed model into the JSON the map template renders.
func assemble(m *worldModel) worldJSON {
	areas, mobs, rooms, coords, start := m.Areas, m.Mobs, m.Rooms, m.Coords, m.Start

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
		anyLocked, anyHidden := false, false
		for _, dir := range dirOrder {
			to, ok := r.Exits[dir]
			if !ok {
				continue
			}
			cross := false
			if tr, ok := rooms[to]; ok {
				cross = tr.Area != r.Area
			}
			d := r.Doors[dir]
			_, hidden := r.HiddenExits[dir]
			if d.Locked {
				anyLocked = true
			}
			if hidden {
				anyHidden = true
			}
			exits = append(exits, exitJSON{
				Dir: dir, To: to, Cross: cross,
				Hidden: hidden, Locked: d.Locked, Door: clean(d.Name),
			})
		}

		mobList := make([]mobJSON, 0, len(r.Mobs))
		for _, mid := range r.Mobs {
			if mob, ok := mobs[mid]; ok {
				mobList = append(mobList, mob)
			} else {
				mobList = append(mobList, mobJSON{Name: mid})
			}
		}

		_, station := r.Properties["craft_stations"]
		light, _ := r.Properties["light"].(string)
		weather := areas[r.Area].WeatherZone

		feats := computeFeatures(id == start, len(r.Items) > 0, station, light, anyLocked, anyHidden, mobList)
		roomsJSON = append(roomsJSON, roomJSON{
			ID: id, Name: clean(r.Name), Area: r.Area, Region: region,
			Terrain: r.Terrain, X: c.x, Y: c.y, Z: c.z,
			Exits: exits, Mobs: mobList,
			Spawn: id == start, Items: len(r.Items) > 0, Station: station,
			Light: light, Weather: weather, Features: feats,
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
		Pack:      m.Pack,
		Generated: time.Now().Format("2006-01-02 15:04"),
		Regions:   regions,
		Areas:     areaMetas,
		Rooms:     roomsJSON,
	}
}

// computeFeatures collapses a room's badge/filter/search-relevant traits into a
// canonical, ordered key list. The template drives the badge glyphs, the feature
// filter chips, and the "search matches feature keys" behavior off exactly this
// list, so a new feature is added in one place (here + the glyph map in the
// template).
func computeFeatures(spawn, items, station bool, light string, locked, hidden bool, mobs []mobJSON) []string {
	var f []string
	add := func(k string) { f = append(f, k) }
	if spawn {
		add("spawn")
	}
	if anyMob(mobs, func(m mobJSON) bool { return m.Shop }) {
		add("shop")
	}
	if anyMob(mobs, func(m mobJSON) bool { return m.Trainer }) {
		add("trainer")
	}
	if station {
		add("craft")
	}
	if anyMob(mobs, func(m mobJSON) bool { return m.Stable }) {
		add("stable")
	}
	if anyMob(mobs, func(m mobJSON) bool { return m.Hireling || m.Recruiter }) {
		add("hire")
	}
	if anyMob(mobs, func(m mobJSON) bool { return m.Quest }) {
		add("quest")
	}
	if anyMob(mobs, func(m mobJSON) bool { return m.Faction != "" }) {
		add("faction")
	}
	if anyMob(mobs, func(m mobJSON) bool { return m.Hostile }) {
		add("hostile")
	}
	if locked {
		add("locked")
	}
	if hidden {
		add("hidden")
	}
	if light == "black" || light == "dark" {
		add("dark")
	}
	if items {
		add("items")
	}
	return f
}

func anyMob(mobs []mobJSON, pred func(mobJSON) bool) bool {
	for _, m := range mobs {
		if pred(m) {
			return true
		}
	}
	return false
}
