package gameclock_test

import (
	"context"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/gameclock"
)

func TestPeriodFor_DefaultBoundaries(t *testing.T) {
	// Default boundaries [5, 8, 18, 20]:
	//   0..4   → Night (pre-dawn)
	//   5..7   → Dawn
	//   8..17  → Day
	//   18..19 → Dusk
	//   20..23 → Night
	cases := []struct {
		hour int
		want string
	}{
		{0, gameclock.PeriodNight},
		{4, gameclock.PeriodNight},
		{5, gameclock.PeriodDawn},
		{7, gameclock.PeriodDawn},
		{8, gameclock.PeriodDay},
		{17, gameclock.PeriodDay},
		{18, gameclock.PeriodDusk},
		{19, gameclock.PeriodDusk},
		{20, gameclock.PeriodNight},
		{23, gameclock.PeriodNight},
	}
	for _, tc := range cases {
		// Drive a fresh clock to `hour` by direct tick advancement
		// — 1 tick/hour to keep the test cheap.
		c := gameclock.New(gameclock.Config{TicksPerGameHour: 1})
		for i := 0; i < tc.hour; i++ {
			c.Tick(context.Background())
		}
		if got := c.CurrentPeriod(); got != tc.want {
			t.Errorf("hour %d period = %q, want %q", tc.hour, got, tc.want)
		}
	}
}

func TestClock_InitialStateIsNight(t *testing.T) {
	c := gameclock.New(gameclock.Config{})
	if got := c.CurrentHour(); got != 0 {
		t.Errorf("CurrentHour = %d, want 0", got)
	}
	if got := c.DayCount(); got != 0 {
		t.Errorf("DayCount = %d, want 0", got)
	}
	if got := c.CurrentPeriod(); got != gameclock.PeriodNight {
		t.Errorf("CurrentPeriod = %q, want %q", got, gameclock.PeriodNight)
	}
}

func TestClock_AdvancesOneHourPerTicksPerGameHour(t *testing.T) {
	c := gameclock.New(gameclock.Config{TicksPerGameHour: 10})
	for i := range 9 {
		if c.Tick(context.Background()) {
			t.Fatalf("tick %d: unexpected hour advance", i)
		}
		if c.CurrentHour() != 0 {
			t.Fatalf("tick %d: hour advanced early to %d", i, c.CurrentHour())
		}
	}
	if !c.Tick(context.Background()) {
		t.Fatal("10th tick should advance the hour")
	}
	if c.CurrentHour() != 1 {
		t.Errorf("after 10 ticks: hour = %d, want 1", c.CurrentHour())
	}
}

func TestClock_DayWrap(t *testing.T) {
	c := gameclock.New(gameclock.Config{TicksPerGameHour: 1})
	for range 23 {
		c.Tick(context.Background())
	}
	if c.CurrentHour() != 23 || c.DayCount() != 0 {
		t.Fatalf("pre-wrap: hour=%d day=%d, want 23/0", c.CurrentHour(), c.DayCount())
	}
	c.Tick(context.Background())
	if c.CurrentHour() != 0 {
		t.Errorf("post-wrap hour = %d, want 0", c.CurrentHour())
	}
	if c.DayCount() != 1 {
		t.Errorf("post-wrap day = %d, want 1", c.DayCount())
	}
}

func TestClock_HourChangeFiresEveryAdvance(t *testing.T) {
	bus := eventbus.New()
	hours := captureHourChanges(bus)
	c := gameclock.New(gameclock.Config{TicksPerGameHour: 1, Bus: bus})

	for range 5 {
		c.Tick(context.Background())
	}
	if len(*hours) != 5 {
		t.Fatalf("hour events = %d, want 5", len(*hours))
	}
	for i, ev := range *hours {
		wantHour := i + 1
		if ev.Hour != wantHour {
			t.Errorf("event[%d] Hour = %d, want %d", i, ev.Hour, wantHour)
		}
	}
}

func TestClock_PeriodChangeFiresOnlyOnTransition(t *testing.T) {
	bus := eventbus.New()
	periods := capturePeriodChanges(bus)
	c := gameclock.New(gameclock.Config{TicksPerGameHour: 1, Bus: bus})
	// Default boundaries: night at hour 0; dawn at hour 5; day at 8;
	// dusk at 18; night at 20. Tick through one full day and count
	// transitions.
	for range 24 {
		c.Tick(context.Background())
	}
	// Transitions over hours 0→24 with default boundaries:
	//   1..4 night→night (no event), 5 night→dawn,
	//   6,7 dawn→dawn, 8 dawn→day, 9..17 day→day,
	//   18 day→dusk, 19 dusk→dusk, 20 dusk→night,
	//   21..23 night→night, 24=0 night→night (wrap, same period).
	// Expected transitions: night→dawn, dawn→day, day→dusk, dusk→night.
	wantTransitions := []struct {
		from, to string
	}{
		{gameclock.PeriodNight, gameclock.PeriodDawn},
		{gameclock.PeriodDawn, gameclock.PeriodDay},
		{gameclock.PeriodDay, gameclock.PeriodDusk},
		{gameclock.PeriodDusk, gameclock.PeriodNight},
	}
	if len(*periods) != len(wantTransitions) {
		t.Fatalf("period events = %d, want %d (%+v)", len(*periods), len(wantTransitions), *periods)
	}
	for i, tw := range wantTransitions {
		ev := (*periods)[i]
		if ev.PreviousPeriod != tw.from || ev.Period != tw.to {
			t.Errorf("event[%d] = %s→%s, want %s→%s",
				i, ev.PreviousPeriod, ev.Period, tw.from, tw.to)
		}
	}
}

func TestClock_PeriodChangeFiresBeforeHourChange(t *testing.T) {
	// Spec §3.1: when both events fire on the same advance, the
	// period-change must arrive first so subscribers see the
	// more-specific transition before the cadence-driven hour
	// event triggers e.g. weather rolls.
	bus := eventbus.New()
	var (
		mu    sync.Mutex
		order []string
	)
	bus.Subscribe(eventbus.EventTimeHourChange, func(_ context.Context, _ eventbus.Event) {
		mu.Lock()
		order = append(order, "hour")
		mu.Unlock()
	})
	bus.Subscribe(eventbus.EventTimePeriodChange, func(_ context.Context, _ eventbus.Event) {
		mu.Lock()
		order = append(order, "period")
		mu.Unlock()
	})
	c := gameclock.New(gameclock.Config{TicksPerGameHour: 1, Bus: bus})
	for range 5 {
		c.Tick(context.Background()) // tick the clock to hour 5 (night → dawn)
	}
	mu.Lock()
	defer mu.Unlock()
	// First 4 ticks fire only hour. Tick 5 fires period then hour.
	want := []string{"hour", "hour", "hour", "hour", "period", "hour"}
	if len(order) != len(want) {
		t.Fatalf("order length = %d, want %d (%v)", len(order), len(want), order)
	}
	for i, w := range want {
		if order[i] != w {
			t.Errorf("event[%d] = %q, want %q (full: %v)", i, order[i], w, order)
		}
	}
}

func TestClock_HourChangePayloadCarriesDayCount(t *testing.T) {
	bus := eventbus.New()
	hours := captureHourChanges(bus)
	c := gameclock.New(gameclock.Config{TicksPerGameHour: 1, Bus: bus})
	// Advance through a day wrap: 24 ticks puts us at hour 0 day 1.
	for range 24 {
		c.Tick(context.Background())
	}
	last := (*hours)[len(*hours)-1]
	if last.Hour != 0 || last.DayCount != 1 {
		t.Errorf("wrap event = hour=%d day=%d, want 0/1", last.Hour, last.DayCount)
	}
}

func TestClock_NilBusIsSafe(t *testing.T) {
	c := gameclock.New(gameclock.Config{TicksPerGameHour: 1})
	for range 5 {
		c.Tick(context.Background())
	}
	if c.CurrentHour() != 5 {
		t.Errorf("nil-bus advance reached hour %d, want 5", c.CurrentHour())
	}
}

func TestClock_PanicsOnNegativeTicksPerGameHour(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on negative TicksPerGameHour")
		}
	}()
	gameclock.New(gameclock.Config{TicksPerGameHour: -1})
}

func TestClock_ZeroTicksPerGameHourUsesDefault(t *testing.T) {
	c := gameclock.New(gameclock.Config{TicksPerGameHour: 0})
	// At the default 600 ticks per hour, 599 ticks should NOT
	// advance and the 600th SHOULD.
	for i := range 599 {
		if c.Tick(context.Background()) {
			t.Fatalf("tick %d: advanced before reaching 600", i)
		}
	}
	if !c.Tick(context.Background()) {
		t.Error("600th tick did not advance")
	}
}

func TestClock_CustomPeriodBoundaries(t *testing.T) {
	// All-day-night world: dawn at 6, day at 6, dusk at 6, night at 6.
	// Ascending requirement is "should" not "must" per spec; the
	// clock doesn't validate. With degenerate boundaries everything
	// past hour 6 reads as Night. We use a non-degenerate but
	// non-default set to exercise the wiring.
	c := gameclock.New(gameclock.Config{
		TicksPerGameHour: 1,
		PeriodBoundaries: [4]int{6, 10, 16, 22},
	})
	if c.CurrentPeriod() != gameclock.PeriodNight {
		t.Errorf("hour 0 should be night, got %q", c.CurrentPeriod())
	}
	// Tick to hour 10 → Day under these boundaries.
	for range 10 {
		c.Tick(context.Background())
	}
	if c.CurrentPeriod() != gameclock.PeriodDay {
		t.Errorf("hour 10 with custom boundaries = %q, want day", c.CurrentPeriod())
	}
}

func captureHourChanges(bus *eventbus.Bus) *[]eventbus.TimeHourChange {
	var (
		mu  sync.Mutex
		out []eventbus.TimeHourChange
	)
	bus.Subscribe(eventbus.EventTimeHourChange, func(_ context.Context, e eventbus.Event) {
		if ev, ok := e.(eventbus.TimeHourChange); ok {
			mu.Lock()
			out = append(out, ev)
			mu.Unlock()
		}
	})
	return &out
}

func capturePeriodChanges(bus *eventbus.Bus) *[]eventbus.TimePeriodChange {
	var (
		mu  sync.Mutex
		out []eventbus.TimePeriodChange
	)
	bus.Subscribe(eventbus.EventTimePeriodChange, func(_ context.Context, e eventbus.Event) {
		if ev, ok := e.(eventbus.TimePeriodChange); ok {
			mu.Lock()
			out = append(out, ev)
			mu.Unlock()
		}
	})
	return &out
}
