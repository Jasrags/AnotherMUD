package weather_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/weather"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// fixedRoller mirrors the in-package test helper for the external
// test package.
type fixedRoller struct {
	values []int
	idx    int
}

func (f *fixedRoller) IntN(_ int) int {
	v := f.values[f.idx]
	f.idx++
	return v
}

// recordingBroadcaster captures every SendToRoom call so tests
// can assert which rooms received what.
type recordingBroadcaster struct {
	mu    sync.Mutex
	calls []sendCall
}

type sendCall struct {
	roomID world.RoomID
	text   string
}

func (b *recordingBroadcaster) SendToRoom(_ context.Context, roomID world.RoomID, text string, _ ...string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, sendCall{roomID, text})
}

func (b *recordingBroadcaster) snapshot() []sendCall {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]sendCall, len(b.calls))
	copy(out, b.calls)
	return out
}

func clearToRainZone() *weather.Zone {
	return &weather.Zone{
		ID:           "temperate",
		InitialState: "clear",
		Transitions: map[string][]weather.TransitionWeight{
			"clear": {{NextState: "rain", Weight: 1}},
			"rain":  {{NextState: "clear", Weight: 1}},
		},
		WeatherMessages: map[string]map[string]weather.MessageTriple{
			"rain":  {weather.TerrainOutdoors: {Start: "Rain begins.", End: "Rain ends."}},
			"clear": {weather.TerrainOutdoors: {Start: "The sky clears.", End: "Clouds gather."}},
		},
	}
}

func newTwoRoomWorld(t *testing.T) *world.World {
	t.Helper()
	w := world.New()
	w.AddArea(&world.Area{ID: "town", Name: "Town", WeatherZone: "temperate"})
	w.AddRoom(&world.Room{ID: "square", AreaID: "town", Name: "Square"})
	w.AddRoom(&world.Room{ID: "tavern", AreaID: "town", Name: "Tavern", Terrain: weather.TerrainIndoors})
	return w
}

func newService(t *testing.T, picks ...int) (*weather.Service, *world.World, *recordingBroadcaster, *eventbus.Bus) {
	t.Helper()
	reg := weather.NewRegistry()
	if err := reg.Add(clearToRainZone()); err != nil {
		t.Fatalf("Add zone: %v", err)
	}
	w := newTwoRoomWorld(t)
	bus := eventbus.New()
	bc := &recordingBroadcaster{}
	return weather.New(weather.Config{
		Registry:    reg,
		World:       w,
		Bus:         bus,
		Broadcaster: bc,
		Roller:      &fixedRoller{values: picks},
	}), w, bc, bus
}

func TestService_CurrentWeather_DefaultsToZoneInitial(t *testing.T) {
	s, _, _, _ := newService(t)
	if got := s.CurrentWeather("town"); got != "clear" {
		t.Errorf("CurrentWeather = %q, want %q", got, "clear")
	}
}

func TestService_CurrentWeather_NoZoneFallsBackToDefault(t *testing.T) {
	reg := weather.NewRegistry()
	w := world.New()
	w.AddArea(&world.Area{ID: "void"}) // no zone
	s := weather.New(weather.Config{Registry: reg, World: w})
	if got := s.CurrentWeather("void"); got != weather.DefaultWeatherState {
		t.Errorf("zoneless area current = %q, want %q", got, weather.DefaultWeatherState)
	}
}

func TestService_HourChanged_RollsAndPublishes(t *testing.T) {
	s, _, bc, bus := newService(t, 0) // single deterministic pick
	got := captureWeatherChanges(bus)

	s.HourChanged(context.Background(), 1)

	if state := s.CurrentWeather("town"); state != "rain" {
		t.Errorf("post-roll state = %q, want rain", state)
	}
	if len(*got) != 1 {
		t.Fatalf("weather.changed events = %d, want 1", len(*got))
	}
	ev := (*got)[0]
	if ev.PreviousState != "clear" || ev.NewState != "rain" {
		t.Errorf("event payload = %+v", ev)
	}
	calls := bc.snapshot()
	// Outdoor square gets BOTH the end of clear and the start of rain;
	// indoor tavern gets nothing (shielded, no exposure flag).
	if len(calls) != 2 {
		t.Fatalf("broadcast count = %d, want 2", len(calls))
	}
	for _, c := range calls {
		if c.roomID != "square" {
			t.Errorf("unexpected room: %+v", c)
		}
	}
	if !strings.Contains(calls[0].text, "Clouds gather") {
		t.Errorf("first call should be end-of-clear, got %q", calls[0].text)
	}
	if !strings.Contains(calls[1].text, "Rain begins") {
		t.Errorf("second call should be start-of-rain, got %q", calls[1].text)
	}
}

func TestService_HourChanged_IdenticalStateRollIsNoOp(t *testing.T) {
	// Build a zone whose only transition for "clear" is back to "clear".
	zone := &weather.Zone{
		ID:           "stagnant",
		InitialState: "clear",
		Transitions: map[string][]weather.TransitionWeight{
			"clear": {{NextState: "clear", Weight: 1}},
		},
		WeatherMessages: map[string]map[string]weather.MessageTriple{
			"clear": {weather.TerrainOutdoors: {Start: "should-not-send"}},
		},
	}
	reg := weather.NewRegistry()
	_ = reg.Add(zone)
	w := world.New()
	w.AddArea(&world.Area{ID: "town", WeatherZone: "stagnant"})
	w.AddRoom(&world.Room{ID: "square", AreaID: "town"})

	bus := eventbus.New()
	bc := &recordingBroadcaster{}
	s := weather.New(weather.Config{
		Registry: reg, World: w, Bus: bus, Broadcaster: bc,
		Roller: &fixedRoller{values: []int{0}},
	})
	got := captureWeatherChanges(bus)

	s.HourChanged(context.Background(), 1)

	if len(*got) != 0 {
		t.Errorf("identical-state roll published %d events, want 0", len(*got))
	}
	if calls := bc.snapshot(); len(calls) != 0 {
		t.Errorf("identical-state roll broadcast %d times, want 0: %+v", len(calls), calls)
	}
}

func TestService_HourChanged_SkipsOffInterval(t *testing.T) {
	reg := weather.NewRegistry()
	zone := clearToRainZone()
	zone.RollIntervalHours = 3
	_ = reg.Add(zone)
	w := newTwoRoomWorld(t)
	s := weather.New(weather.Config{
		Registry: reg, World: w,
		Roller: &fixedRoller{values: []int{0, 0, 0, 0}},
	})
	// Hours 1 and 2 are off-interval (3 % 3 = 0, so hour 3 rolls).
	s.HourChanged(context.Background(), 1)
	s.HourChanged(context.Background(), 2)
	if s.CurrentWeather("town") != "clear" {
		t.Errorf("off-interval hours rolled: %s", s.CurrentWeather("town"))
	}
	s.HourChanged(context.Background(), 3)
	if s.CurrentWeather("town") != "rain" {
		t.Errorf("on-interval hour did not roll: %s", s.CurrentWeather("town"))
	}
}

func TestService_HourChanged_SkipsAreasWithoutZone(t *testing.T) {
	reg := weather.NewRegistry()
	_ = reg.Add(clearToRainZone())
	w := world.New()
	w.AddArea(&world.Area{ID: "void"}) // intentionally zoneless
	s := weather.New(weather.Config{
		Registry: reg, World: w,
		Roller: &fixedRoller{values: []int{0}},
	})
	s.HourChanged(context.Background(), 1)
	if got := s.CurrentWeather("void"); got != weather.DefaultWeatherState {
		t.Errorf("zoneless area was rolled: %q", got)
	}
}

func TestService_SetWeather_PublishesAndBroadcasts(t *testing.T) {
	s, _, bc, bus := newService(t)
	got := captureWeatherChanges(bus)

	s.SetWeather(context.Background(), "town", "rain")

	if s.CurrentWeather("town") != "rain" {
		t.Errorf("SetWeather did not persist: %q", s.CurrentWeather("town"))
	}
	if len(*got) != 1 {
		t.Fatalf("SetWeather event count = %d, want 1", len(*got))
	}
	if len(bc.snapshot()) != 2 {
		t.Errorf("SetWeather broadcasts = %d, want 2", len(bc.snapshot()))
	}
}

func TestService_SetWeather_IdenticalIsNoOp(t *testing.T) {
	s, _, bc, bus := newService(t)
	got := captureWeatherChanges(bus)

	// Initial state is "clear"; setting to "clear" is a no-op.
	s.SetWeather(context.Background(), "town", "clear")
	if len(*got) != 0 {
		t.Errorf("identical SetWeather published %d events", len(*got))
	}
	if len(bc.snapshot()) != 0 {
		t.Errorf("identical SetWeather broadcast")
	}
}

func TestService_PeriodChanged_DeliversToEligibleRooms(t *testing.T) {
	reg := weather.NewRegistry()
	zone := clearToRainZone()
	zone.TimeMessages = map[string]map[string]string{
		"dawn": {weather.TerrainOutdoors: "Dawn breaks."},
	}
	_ = reg.Add(zone)
	w := newTwoRoomWorld(t)
	bc := &recordingBroadcaster{}
	s := weather.New(weather.Config{
		Registry: reg, World: w, Broadcaster: bc,
		Roller: &fixedRoller{values: []int{0}},
	})

	s.PeriodChanged(context.Background(), "dawn")

	calls := bc.snapshot()
	if len(calls) != 1 {
		t.Fatalf("period broadcast count = %d, want 1 (indoor tavern shielded)", len(calls))
	}
	if calls[0].roomID != "square" || calls[0].text != "Dawn breaks." {
		t.Errorf("call = %+v", calls[0])
	}
}

func TestService_PeriodChanged_TimeExposedOverridesShielding(t *testing.T) {
	reg := weather.NewRegistry()
	zone := clearToRainZone()
	zone.TimeMessages = map[string]map[string]string{
		"dusk": {weather.TerrainOutdoors: "Dusk falls."},
	}
	_ = reg.Add(zone)
	w := world.New()
	w.AddArea(&world.Area{ID: "town", WeatherZone: "temperate"})
	w.AddRoom(&world.Room{ID: "shrine", AreaID: "town",
		Terrain: weather.TerrainIndoors, TimeExposed: true})
	bc := &recordingBroadcaster{}
	s := weather.New(weather.Config{Registry: reg, World: w, Broadcaster: bc})

	s.PeriodChanged(context.Background(), "dusk")

	calls := bc.snapshot()
	if len(calls) != 1 {
		t.Fatalf("time-exposed indoor should receive: %d calls", len(calls))
	}
}

func TestService_HourChanged_NilWorldOrRollerIsNoOp(t *testing.T) {
	// Both no-op paths should not panic; smoke-test only.
	reg := weather.NewRegistry()
	weather.New(weather.Config{Registry: reg}).HourChanged(context.Background(), 0)
	w := world.New()
	w.AddArea(&world.Area{ID: "town", WeatherZone: "z"})
	weather.New(weather.Config{Registry: reg, World: w}).HourChanged(context.Background(), 0)
}

// captureWeatherChanges subscribes to weather.changed and returns
// a slice pointer that the test inspects after dispatch. The
// pointer lets the test assert against the post-dispatch state
// without copying.
func captureWeatherChanges(bus *eventbus.Bus) *[]eventbus.WeatherChanged {
	var (
		mu  sync.Mutex
		out []eventbus.WeatherChanged
	)
	bus.Subscribe(eventbus.EventWeatherChanged, func(_ context.Context, e eventbus.Event) {
		if ev, ok := e.(eventbus.WeatherChanged); ok {
			mu.Lock()
			out = append(out, ev)
			mu.Unlock()
		}
	})
	return &out
}
