package pack

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/weather"
)

// weatherPack writes a minimal pack carrying one weather zone, one
// area referencing the zone, and one room with terrain + exposure
// flags. Tests stitch areas/rooms/zones together for the loader
// end-to-end paths.
func weatherPack(t *testing.T, zoneBody, areaBody, roomBody string) string {
	t.Helper()
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  weather_zones: [weather_zones/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "weather_zones/temperate.yaml"), zoneBody)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), areaBody)
	writeFile(t, filepath.Join(pack, "rooms/square.yaml"), roomBody)
	return root
}

func TestLoad_DecodesWeatherZone(t *testing.T) {
	root := weatherPack(t, `
id: temperate
initial_state: clear
roll_interval_hours: 3
transitions:
  clear:
    - next: cloudy
      weight: 4
    - next: clear
      weight: 6
weather_messages:
  rain:
    outdoors:
      start: rain starts
      ongoing: rain falls
      end: rain stops
time_messages:
  dawn:
    outdoors: dawn breaks
`, `
id: town
name: Town
weather_zone: temperate
`, `
id: square
area: town
name: The Square
description: stones
`)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	z, err := regs.Weather.Get("tapestry-core:temperate")
	if err != nil {
		t.Fatalf("Weather.Get: %v", err)
	}
	if z.InitialState != "clear" {
		t.Errorf("InitialState = %q", z.InitialState)
	}
	if z.RollIntervalHours != 3 {
		t.Errorf("RollIntervalHours = %d, want 3", z.RollIntervalHours)
	}
	row, ok := z.Transitions["clear"]
	if !ok || len(row) != 2 {
		t.Fatalf("clear row = %+v", row)
	}
	rain := z.WeatherMessages["rain"]["outdoors"]
	if rain.Start != "rain starts" || rain.Ongoing != "rain falls" || rain.End != "rain stops" {
		t.Errorf("rain triple = %+v", rain)
	}
	if z.TimeMessages["dawn"]["outdoors"] != "dawn breaks" {
		t.Errorf("dawn message = %q", z.TimeMessages["dawn"]["outdoors"])
	}
}

func TestLoad_AreaQualifiesWeatherZoneReference(t *testing.T) {
	root := weatherPack(t,
		"id: temperate\n",
		"id: town\nname: Town\nweather_zone: temperate\n",
		"id: square\narea: town\nname: The Square\ndescription: x\n",
	)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	area, err := regs.World.Area("tapestry-core:town")
	if err != nil {
		t.Fatalf("Area: %v", err)
	}
	if area.WeatherZone != "tapestry-core:temperate" {
		t.Errorf("Area.WeatherZone = %q, want %q",
			area.WeatherZone, "tapestry-core:temperate")
	}
}

func TestLoad_AreaWithoutWeatherZoneIsEmpty(t *testing.T) {
	root := weatherPack(t,
		"id: temperate\n",
		"id: town\nname: Town\n", // no weather_zone
		"id: square\narea: town\nname: The Square\ndescription: x\n",
	)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	area, _ := regs.World.Area("tapestry-core:town")
	if area.WeatherZone != "" {
		t.Errorf("absent weather_zone produced %q, want empty", area.WeatherZone)
	}
}

func TestLoad_RoomTerrainAndExposureFlags(t *testing.T) {
	root := weatherPack(t,
		"id: temperate\n",
		"id: town\nname: Town\nweather_zone: temperate\n",
		`id: square
area: town
name: The Square
description: x
terrain: indoors
weather_exposed: true
time_exposed: false
`,
	)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	r, err := regs.World.Room("tapestry-core:square")
	if err != nil {
		t.Fatalf("Room: %v", err)
	}
	if r.Terrain != weather.TerrainIndoors {
		t.Errorf("Terrain = %q, want %q", r.Terrain, weather.TerrainIndoors)
	}
	if !r.WeatherExposed {
		t.Error("WeatherExposed = false, want true")
	}
	if r.TimeExposed {
		t.Error("TimeExposed = true, want false")
	}
}

func TestLoad_RejectsZeroOrNegativeTransitionWeight(t *testing.T) {
	root := weatherPack(t, `
id: temperate
transitions:
  clear:
    - next: rain
      weight: 0
`,
		"id: town\nname: Town\n",
		"id: square\narea: town\nname: The Square\ndescription: x\n",
	)
	regs := NewRegistries()
	err := Load(context.Background(), root, nil, regs, nil, nil)
	if err == nil {
		t.Fatal("expected error on zero-weight transition")
	}
	if !errors.Is(err, ErrInvalidContent) {
		t.Errorf("err = %v, want ErrInvalidContent wrap", err)
	}
}

func TestLoad_RejectsMissingNextInTransition(t *testing.T) {
	root := weatherPack(t, `
id: temperate
transitions:
  clear:
    - next: ""
      weight: 1
`,
		"id: town\nname: Town\n",
		"id: square\narea: town\nname: The Square\ndescription: x\n",
	)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil); err == nil {
		t.Fatal("expected error on missing transition.next")
	}
}

func TestLoad_RejectsNegativeRollInterval(t *testing.T) {
	root := weatherPack(t, `
id: temperate
roll_interval_hours: -1
`,
		"id: town\nname: Town\n",
		"id: square\narea: town\nname: The Square\ndescription: x\n",
	)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil); err == nil {
		t.Fatal("expected error on negative roll_interval_hours")
	}
}

func TestLoad_RejectsDuplicateWeatherZoneID(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  weather_zones: [weather_zones/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "weather_zones/a.yaml"), "id: temperate\n")
	writeFile(t, filepath.Join(pack, "weather_zones/b.yaml"), "id: temperate\n")
	regs := NewRegistries()
	err := Load(context.Background(), root, nil, regs, nil, nil)
	if err == nil {
		t.Fatal("expected error on duplicate zone id")
	}
	if !errors.Is(err, weather.ErrDuplicateZone) {
		t.Errorf("err = %v, want weather.ErrDuplicateZone wrap", err)
	}
}
